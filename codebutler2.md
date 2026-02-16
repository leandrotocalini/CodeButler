# CodeButler v2 — Per-Thread Parallel Processing

## Objetivo

Evolucionar CodeButler de un modelo single-conversation (un Claude a la vez)
a un modelo donde cada thread de Slack genera su propia goroutine, permitiendo
procesamiento paralelo entre threads y secuencial dentro de cada thread.

---

## Arquitectura General

```
Slack Events API
       │
       ▼
  Main Loop (goroutine principal)
       │
       ├─ Lee todos los eventos entrantes de Slack
       ├─ Identifica el thread_ts de cada mensaje
       ├─ Si existe goroutine para ese thread → envía mensaje por channel
       ├─ Si no existe → crea nueva goroutine + channel, envía mensaje
       │
       ▼
  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
  │  Thread A    │  │  Thread B    │  │  Thread C    │
  │  goroutine   │  │  goroutine   │  │  goroutine   │
  │              │  │              │  │              │
  │  ← channel   │  │  ← channel   │  │  ← channel   │
  │  → Slack API │  │  → Slack API │  │  → Slack API │
  └─────────────┘  └─────────────┘  └─────────────┘
        │                 │                 │
        ▼                 ▼                 ▼
   claude -p          claude -p          claude -p
   (sesión A)         (sesión B)         (sesión C)
```

---

## Componentes

### 1. Main Loop (goroutine principal)

Responsabilidades:
- Recibir **todos** los eventos de Slack (via Socket Mode o Events API)
- Extraer `thread_ts` de cada mensaje (o `ts` si es mensaje raíz que inicia thread)
- Mantener un `map[string]chan Message` — thread_ts → channel de la goroutine
- **Ruteo**:
  - Si el thread ya tiene goroutine viva → enviar mensaje al channel existente
  - Si no → crear channel, spawner goroutine, enviar mensaje
- **No procesa mensajes**. Solo lee y rutea.

### 2. Thread Worker (goroutine por thread)

Cada goroutine de thread:
- Recibe mensajes de su channel (enviados por main)
- Procesa mensajes **secuencialmente** (uno a la vez, en orden de llegada)
- Mantiene su propia sesión de Claude (`--resume <session_id>`)
- Envía respuestas directamente al thread de Slack via API
- **Muere** cuando no tiene nada que procesar:
  - El channel está vacío
  - No hay Claude corriendo
  - No se espera input del usuario (timeout de inactividad)

### 3. Lifecycle de una goroutine de thread

```
Main recibe mensaje para thread X
       │
       ▼
¿Existe goroutine para thread X?
       │
  NO ──┤── SI
  │         │
  ▼         ▼
Crear    Enviar msg
channel  al channel
+ spawn  existente
goroutine
  │
  ▼
Goroutine arranca
  │
  ▼
Loop:
  select {
  case msg := <-channel:
      → Acumular (ventana corta, ej: 3s)
      → Procesar batch con Claude
      → Enviar respuesta al thread
      → Volver al loop
  case <-timeout (ej: 60s sin mensajes):
      → Limpiar estado
      → Notificar a main (eliminar del map)
      → return (goroutine muere)
  }
```

### 4. Muerte y recreación de goroutines

- Las goroutines son **efímeras**: si no hay trabajo, mueren
- Es barato crearlas de nuevo — Go las maneja con ~2KB de stack inicial
- Cuando main recibe un mensaje para un thread sin goroutine activa,
  simplemente crea una nueva
- La sesión de Claude se recupera via `--resume` usando el session_id
  persistido en la DB (no se pierde estado de Claude al morir la goroutine)

---

## Modelo de Datos

### Mensaje interno (main → goroutine)

```
Message {
    ID          string    // ID único del mensaje Slack
    ThreadTS    string    // Thread timestamp (identifica el thread)
    ChannelID   string    // Canal de Slack
    Content     string    // Texto del mensaje
    From        string    // Usuario que envió
    Timestamp   time.Time // Cuándo llegó
    IsVoice     bool      // Audio (futuro)
    IsImage     bool      // Imagen (futuro)
}
```

### Thread Registry (en main)

```
ThreadRegistry {
    mu       sync.Mutex
    workers  map[string]chan Message   // thread_ts → channel
}

Métodos:
    Route(msg)       // Envía a goroutine existente o crea nueva
    Remove(threadTS) // Llamado por goroutine al morir
```

### Persistencia (SQLite)

Tabla `sessions` — ya existe, se usa por thread_ts en vez de chat JID:

```sql
CREATE TABLE sessions (
    thread_ts   TEXT PRIMARY KEY,
    channel_id  TEXT NOT NULL,
    session_id  TEXT NOT NULL,      -- Claude session ID para --resume
    updated_at  TEXT NOT NULL
);
```

Tabla `messages` — similar a v1, con thread_ts:

```sql
CREATE TABLE messages (
    id         TEXT PRIMARY KEY,
    thread_ts  TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    from_user  TEXT NOT NULL,
    content    TEXT NOT NULL,
    timestamp  TEXT NOT NULL,
    acked      INTEGER DEFAULT 0
);
```

---

## Flujo de Mensajes Detallado

### Caso 1: Mensaje nuevo en thread sin goroutine

```
1. Main recibe evento de Slack: mensaje en thread X
2. Main mira ThreadRegistry → no hay entrada para thread X
3. Main crea channel (buffered, ej: cap 100)
4. Main registra en ThreadRegistry: workers[X] = channel
5. Main spawna goroutine: go threadWorker(X, channel)
6. Main envía mensaje al channel
7. Goroutine arranca, lee del channel
8. Goroutine espera ventana de acumulación (3s) por si llegan más
9. Goroutine construye prompt, llama agent.Run()
10. Goroutine envía respuesta al thread de Slack
11. Goroutine guarda session_id en DB
12. Goroutine vuelve al select{} esperando más mensajes
```

### Caso 2: Mensaje en thread con goroutine activa (Claude idle)

```
1. Main recibe evento → thread X tiene goroutine
2. Main envía al channel existente
3. Goroutine lo lee inmediatamente del channel
4. Acumula, procesa, responde (como caso 1 pasos 8-12)
```

### Caso 3: Mensaje en thread con goroutine activa (Claude busy)

```
1. Main recibe evento → thread X tiene goroutine
2. Main envía al channel existente
3. El mensaje queda en el buffer del channel
4. Cuando Claude termina, goroutine vuelve al select{}
5. Lee el mensaje pendiente del channel
6. Lo procesa como follow-up (--resume)
```

### Caso 4: Goroutine muere por inactividad, luego llega mensaje

```
1. Goroutine de thread X: 60s sin mensajes → timeout
2. Goroutine llama registry.Remove(X)
3. Goroutine retorna (muere)
4. ... tiempo pasa ...
5. Main recibe mensaje para thread X
6. No hay goroutine → caso 1 (crea nueva)
7. Nueva goroutine busca session_id en DB → usa --resume
8. Claude retoma contexto anterior
```

---

## Paralelismo y Concurrencia

### Qué corre en paralelo
- Cada thread tiene su propia goroutine con su propio Claude
- Thread A puede estar procesando con Claude mientras Thread B acumula mensajes
- N threads = N instancias potenciales de `claude -p` simultáneas

### Qué corre secuencialmente
- Dentro de un mismo thread: un mensaje a la vez
- Si llegan 3 mensajes al mismo thread mientras Claude procesa,
  se encolan en el channel y se procesan uno por uno (o batcheados)

### Protecciones
- El channel tiene buffer para no bloquear a main si la goroutine está ocupada
- Main nunca bloquea: solo rutea y sigue
- Si una goroutine paniquea, no afecta a las demás ni a main
  (recover en el wrapper de la goroutine)

---

## Diferencias con CodeButler v1

| Aspecto | v1 (WhatsApp) | v2 (Slack threads) |
|---------|---------------|-------------------|
| Plataforma | WhatsApp (whatsmeow) | Slack (Events API / Socket Mode) |
| Paralelismo | Un solo Claude a la vez | Un Claude por thread (N en paralelo) |
| Unidad de conversación | Grupo completo | Thread individual |
| State machine | Cold/Active/Queued global | Por goroutine (local a cada thread) |
| Goroutines de trabajo | 1 (poll loop) | N (una por thread activo) |
| Vida de workers | Permanente (poll loop siempre vive) | Efímera (muere sin trabajo) |
| Sesiones Claude | Por chat JID | Por thread_ts |
| Mensajes entre componentes | `msgNotify` channel (señal) | Channel tipado por thread (datos) |

---

## Consideraciones

### Recursos
- Cada Claude corriendo consume CPU/memoria del proceso `claude -p`
- Considerar un semáforo global para limitar Claudes concurrentes (ej: max 5)
- Las goroutines en sí son baratas (~2-8KB), el costo real es el proceso Claude

### Graceful Shutdown
- Main recibe SIGINT/SIGTERM
- Cierra todos los channels (los goroutines detectan channel cerrado y terminan)
- Espera a que goroutines activas terminen (con timeout)
- Procesos Claude en vuelo reciben context cancellation

### Recuperación de estado
- Al reiniciar, no hay goroutines vivas
- Mensajes no procesados (acked=0) se pueden re-procesar al arrancar
- Sessions en DB permiten retomar conversaciones con --resume

### Rate Limits
- Slack API tiene rate limits por workspace
- Respuestas al mismo thread en ráfaga pueden throttlearse
- Considerar rate limiter compartido para envío de mensajes
