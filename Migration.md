# Migration: WhatsApp → Slack

Plan de migración de CodeButler de WhatsApp a Slack.

**Estado**: Planificación (no se ha empezado a implementar)

---

## 1. Motivación

Migrar el messaging backend de WhatsApp (whatsmeow) a Slack, manteniendo
la misma lógica core: daemon que monitorea un canal y spawna `claude -p`.

---

## 2. Mapping de Conceptos

| WhatsApp | Slack | Notas |
|----------|-------|-------|
| Group JID (`...@g.us`) | Channel ID (`C0123ABCDEF`) | Identificador del canal |
| User JID (`...@s.whatsapp.net`) | User ID (`U0123ABCDEF`) | Identificador del usuario |
| QR code pairing | OAuth App + Bot Token | Autenticación |
| whatsmeow events | Slack Socket Mode / Events API | Recepción de mensajes |
| `SendMessage(jid, text)` | `chat.postMessage(channel, text)` | Envío de texto |
| `SendImage(jid, png, caption)` | `files.upload` + message | Envío de imágenes |
| Read receipts (`MarkRead`) | No equivalente directo | Se puede omitir o usar reactions |
| Typing indicator (`SendPresence`) | No hay typing nativo en bots | Se puede omitir |
| Voice messages (Whisper) | Audio files en Slack → Whisper | Mismo flow, distinta descarga |
| Bot prefix `[BOT]` | Bot messages tienen `bot_id` | Slack filtra bots nativamente |
| Linked Devices (device name) | App name en workspace | Visible en Apps |
| `whatsapp-session/session.db` | Bot token (string) | No hay sesión persistente |
| Group creation | `conversations.create` | Channel privado/público |

---

## 3. Arquitectura Actual vs Propuesta

### Actual
```
WhatsApp <-> whatsmeow <-> Go daemon <-> spawns claude -p <-> repo context
                               |
                           SQLite DB
                      (messages + sessions)
```

### Propuesta
```
Slack <-> slack-go SDK <-> Go daemon <-> spawns claude -p <-> repo context
                               |
                           SQLite DB
                      (messages + sessions)
```

---

## 4. Dependencias

### Eliminar
- `go.mau.fi/whatsmeow` (y todas sus subdependencias: protobuf, signal protocol, etc.)
- `github.com/skip2/go-qrcode` (QR ya no se necesita)
- `github.com/mdp/qrterminal/v3` (QR terminal)

### Agregar
- `github.com/slack-go/slack` — SDK oficial de Slack para Go
  - Socket Mode (WebSocket, no necesita endpoint público)
  - Events API
  - Web API (chat.postMessage, files.upload, etc.)

---

## 5. Slack App Setup (pre-requisitos)

Antes de que el daemon funcione, el usuario necesita crear una Slack App:

1. Ir a https://api.slack.com/apps → Create New App
2. Configurar Bot Token Scopes (OAuth & Permissions):
   - `channels:history` — leer mensajes de canales públicos
   - `channels:read` — listar canales
   - `chat:write` — enviar mensajes
   - `files:read` — descargar archivos adjuntos (audio, imágenes)
   - `files:write` — subir archivos (imágenes generadas)
   - `groups:history` — leer mensajes de canales privados
   - `groups:read` — listar canales privados
   - `reactions:write` — (opcional) confirmar lectura con reaction
   - `users:read` — resolver nombres de usuario
3. Habilitar Socket Mode (Settings → Socket Mode → Enable)
   - Genera un App-Level Token (`xapp-...`)
4. Habilitar Events (Event Subscriptions → Enable):
   - Subscribe to bot events: `message.channels`, `message.groups`
5. Install to Workspace → copiar Bot Token (`xoxb-...`)

### Tokens necesarios
- **Bot Token** (`xoxb-...`): operaciones de API (enviar, leer, etc.)
- **App Token** (`xapp-...`): conexión Socket Mode (WebSocket)

---

## 6. Config Changes

### Actual (`config.json`)
```json
{
  "whatsapp": { "groupJID": "...@g.us", "groupName": "...", "botPrefix": "[BOT]" },
  "claude":   { "maxTurns": 10, "timeout": 30, "permissionMode": "bypassPermissions" },
  "openai":   { "apiKey": "sk-..." }
}
```

### Propuesta: Config Global + Per-Repo

Dos niveles de config. La global tiene las keys compartidas, la per-repo
solo lo específico del canal. Per-repo puede override valores globales.

**Global** (`~/.codebutler/config.json`) — se configura una sola vez:
```json
{
  "slack": {
    "botToken": "xoxb-...",
    "appToken": "xapp-..."
  },
  "openai": { "apiKey": "sk-..." }
}
```

**Per-repo** (`<repo>/.codebutler/config.json`) — uno por repo:
```json
{
  "slack": {
    "channelID": "C0123ABCDEF",
    "channelName": "codebutler-myrepo"
  },
  "claude": { "maxTurns": 10, "timeout": 30, "permissionMode": "bypassPermissions" }
}
```

**Merge strategy**: per-repo override global (campo por campo).
Si per-repo define `slack.botToken`, usa ese en vez del global.

**Cambios vs actual:**
- `whatsapp` → `slack`
- `groupJID` → `channelID`
- `groupName` → `channelName`
- `botPrefix` → **eliminado** (Slack identifica bots por `bot_id`)
- Nuevos: `botToken`, `appToken` (en global)
- `openai.apiKey` se mueve a global (compartido entre repos)

---

## 7. Storage Changes

### Directorios

```
~/.codebutler/
  config.json                    # Global: tokens de Slack, OpenAI key

<repo>/.codebutler/
  config.json                    # Per-repo: channelID, Claude settings
  store.db                       # Messages + Claude session IDs (SQLite) — SIN CAMBIOS
  images/                        # Generated images — SIN CAMBIOS
```

### SQLite `messages` table

```sql
-- Actual
CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    from_jid    TEXT NOT NULL,        -- → renombrar a from_id
    chat        TEXT NOT NULL,        -- → renombrar a channel_id
    content     TEXT NOT NULL,
    timestamp   TEXT NOT NULL,
    is_voice    INTEGER DEFAULT 0,
    acked       INTEGER DEFAULT 0,
    wa_msg_id   TEXT DEFAULT ''       -- → renombrar a platform_msg_id
);
```

**Cambios mínimos**: renombrar columnas para ser platform-agnostic, o dejarlas
como están internamente y solo cambiar el código que las llena.

### SQLite `sessions` table

```sql
-- Actual
CREATE TABLE sessions (
    chat_jid   TEXT PRIMARY KEY,      -- → channel_id (misma semántica)
    session_id TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

---

## 8. Archivos a Modificar/Crear/Eliminar

### Eliminar
| Archivo | Razón |
|---------|-------|
| `internal/whatsapp/client.go` | Reemplazado por cliente Slack |
| `internal/whatsapp/handler.go` | Reemplazado por event handler Slack |
| `internal/whatsapp/groups.go` | Reemplazado por channel operations |
| `internal/whatsapp/auth.go` | QR no aplica, Slack usa tokens |

### Crear
| Archivo | Propósito |
|---------|-----------|
| `internal/slack/client.go` | Conexión Socket Mode, estado, desconexión |
| `internal/slack/handler.go` | Parseo de eventos, envío de mensajes/imágenes |
| `internal/slack/channels.go` | Listar/crear canales, obtener info |

### Modificar
| Archivo | Cambios |
|---------|---------|
| `cmd/codebutler/main.go` | Setup wizard: pedir tokens en vez de QR, seleccionar canal en vez de grupo |
| `internal/config/types.go` | `WhatsAppConfig` → `SlackConfig`, separar `GlobalConfig` y `RepoConfig` |
| `internal/config/load.go` | Load global (`~/.codebutler/`) + per-repo, merge, save ambos |
| `internal/daemon/daemon.go` | Reemplazar `whatsapp.Client` por `slack.Client`, adaptar filtros |
| `internal/daemon/imagecmd.go` | `SendImage` → Slack `files.upload` |
| `internal/daemon/web.go` | Cambiar "WhatsApp state" por "Slack state" en status API |
| `internal/store/store.go` | Renombrar columnas: `from_id`, `channel_id`, `platform_msg_id` |
| `go.mod` / `go.sum` | Nuevas dependencias |

### Sin cambios
| Archivo | Razón |
|---------|-------|
| `internal/agent/agent.go` | Claude spawn es independiente del messaging |
| `internal/imagegen/generate.go` | OpenAI API es independiente |
| `internal/transcribe/whisper.go` | Whisper API es independiente |
| `internal/store/sessions.go` | Semántica idéntica (channel_id en vez de chat_jid) |
| `internal/daemon/logger.go` | Logger es independiente |

---

## 9. Nuevo `internal/slack/` — Diseño de Interfaces

### `client.go`

```go
package slack

type ConnectionState int

const (
    StateDisconnected ConnectionState = iota
    StateConnecting
    StateConnected
)

type Client struct {
    api       *slack.Client        // Web API (xoxb token)
    socket    *socketmode.Client   // Socket Mode (xapp token)
    state     ConnectionState
    botUserID string               // Bot's own user ID (para filtrar sus mensajes)
}

// Connect inicia Socket Mode y espera conexión
func Connect(botToken, appToken string) (*Client, error)

// Disconnect cierra la conexión
func (c *Client) Disconnect()

// GetState devuelve el estado actual
func (c *Client) GetState() ConnectionState

// IsConnected devuelve true si conectado
func (c *Client) IsConnected() bool

// GetBotUserID devuelve el user ID del bot
func (c *Client) GetBotUserID() string
```

### `handler.go`

```go
// Message es la abstracción de mensaje (equivalente a whatsapp.Message)
type Message struct {
    ID        string
    From      string    // User ID
    FromName  string    // Display name (resuelto via users.info)
    Channel   string    // Channel ID
    Content   string
    Timestamp string    // Slack ts (e.g., "1234567890.123456")
    IsFromMe  bool      // Es del bot
    IsVoice   bool      // Audio file adjunto
    IsImage   bool      // Image file adjunto
    FileURL   string    // URL del archivo (si hay)
    ThreadTS  string    // Thread timestamp (para responder en thread)
}

type MessageHandler func(Message)

// OnMessage registra callback para mensajes nuevos
func (c *Client) OnMessage(handler MessageHandler)

// SendMessage envía texto a un canal
func (c *Client) SendMessage(channelID, text string) error

// SendImage sube y envía imagen a un canal
func (c *Client) SendImage(channelID string, pngData []byte, caption string) error

// DownloadFile descarga un archivo de Slack
func (c *Client) DownloadFile(fileURL string) ([]byte, error)
```

### `channels.go`

```go
type Channel struct {
    ID   string
    Name string
}

// GetChannels lista canales donde el bot está presente
func (c *Client) GetChannels() ([]Channel, error)

// CreateChannel crea un canal nuevo
func (c *Client) CreateChannel(name string) (string, error)

// GetChannelInfo obtiene info de un canal
func (c *Client) GetChannelInfo(channelID string) (*Channel, error)
```

---

## 10. Setup Wizard — Nuevo Flow

### Actual (WhatsApp)
```
1. Show QR code
2. User scans with phone
3. List groups → select or create
4. Set bot prefix
5. (Optional) OpenAI API key
6. Save config
```

### Propuesto (Slack) — con config global

**Primera vez (no existe `~/.codebutler/config.json`):**
```
1. Prompt: "Bot Token (xoxb-...):"
2. Prompt: "App Token (xapp-...):"
3. Validate tokens (api.AuthTest)
4. (Optional) Prompt: "OpenAI API key:"
5. Save → ~/.codebutler/config.json (global)
6. Connect Socket Mode
7. List channels → select or create
8. Save → <repo>/.codebutler/config.json (per-repo)
```

**Repos siguientes (global ya existe):**
```
1. Load ~/.codebutler/config.json → tokens ya configurados
2. Connect Socket Mode
3. List channels → select or create
4. Save → <repo>/.codebutler/config.json (per-repo)
```

**Diferencia clave**: tokens y API keys se piden una sola vez y se guardan
en `~/.codebutler/`. Cada repo solo configura su canal.

---

## 11. Message Flow — Nuevo

### Recepción
```
Slack WebSocket (Socket Mode)
    ↓ socketmode.EventTypeEventsAPI
    ↓ EventTypeMessageChannels
Parse: user, channel, text, files
    ↓
Filter: channel match, not from bot
    ↓
Audio file? → Download → Whisper transcribe
    ↓
store.Insert(Message)
    ↓
Signal msgNotify channel
    ↓
(conversation state machine — SIN CAMBIOS)
```

### Envío
```
agent.Run() result
    ↓
slack.Client.SendMessage(channelID, text)
    ↓ api.PostMessage(channelID, slack.MsgOptionText(text, false))
Slack API
```

### Filtrado de mensajes propios
```
// WhatsApp actual: compara botPrefix en el contenido
if strings.HasPrefix(msg.Content, cfg.WhatsApp.BotPrefix) { skip }

// Slack nuevo: compara bot user ID
if msg.BotID != "" || msg.User == c.botUserID { skip }
```

**Ventaja**: Slack identifica bots nativamente, no necesitamos prefijo.

---

## 12. Features que Cambian

### Bot Prefix → Eliminado
- WhatsApp necesitaba `[BOT]` para filtrar mensajes propios
- Slack identifica bots por `bot_id` en el evento
- Los mensajes del bot se envían sin prefijo (más limpio)

### Read Receipts → Reactions
- WhatsApp: `MarkRead()` muestra ticks azules
- Slack: usar reactions como feedback visual
  - 👀 (`eyes`) cuando se empieza a procesar
  - ✅ (`white_check_mark`) cuando Claude termina de responder

### Typing Indicator → Eliminado
- WhatsApp: `SendPresence(composing=true)` muestra "typing..."
- Slack: bots no pueden mostrar typing indicator
- Se puede omitir sin impacto funcional

### Threads (nuevo en Slack)
- **Decidido**: siempre responder en thread del mensaje original
- Mantiene el canal limpio
- Agrupa conversación con Claude en un hilo visual

### Voice Messages
- WhatsApp: voz inline, descarga con `DownloadAudio()`
- Slack: audio como file attachment, descarga con `files.info` + HTTP GET con auth
- Mismo pipeline de Whisper después de la descarga

### Image Messages
- WhatsApp: imagen inline con `DownloadImage()`
- Slack: imagen como file attachment
- Envío: `files.upload` en vez de protobuf con media upload

---

## 13. Decisiones Tomadas

- [x] **Threads**: responder en thread del mensaje original
- [x] **Reactions**: sí, usar 👀 al empezar a procesar y ✅ al terminar
- [x] **Nombres de columnas en SQLite**: renombrar a `from_id`, `channel_id`, `platform_msg_id`
- [x] **Múltiples canales**: no, un canal por repo (como WhatsApp)
- [x] **Mention del bot**: responder a todos los mensajes del canal, sin requerir @mention
- [x] **Message length**: splitear en múltiples mensajes de ~4000 chars en el thread

### Pendientes
- [x] **Markdown**: Convertir output de Claude (Markdown standard) a mrkdwn de Slack antes de enviar

---

## 14. Orden de Implementación

1. **Config**: `SlackConfig` + load/save
2. **Slack client**: conexión Socket Mode, estado
3. **Slack handler**: recibir mensajes, enviar texto
4. **Daemon integration**: reemplazar whatsapp.Client por slack.Client
5. **Setup wizard**: flujo de tokens + selección de canal
6. **Image support**: `files.upload` para `/create-image`
7. **Voice support**: descarga de audio files → Whisper
8. **Cleanup**: eliminar `internal/whatsapp/`, actualizar `go.mod`
9. **Testing**: test manual end-to-end
10. **Docs**: actualizar CLAUDE.md

---

## 15. Riesgos

| Riesgo | Mitigación |
|--------|------------|
| Rate limiting de Slack (1 msg/s) | Implementar queue con backoff |
| Mensajes > 4000 chars | Splitear en múltiples mensajes |
| Socket Mode requiere app-level token | Documentar bien en setup |
| Files API cambió en 2024+ | Usar SDK actualizado |
| Bot no puede ver mensajes sin invitar al canal | Documentar en setup wizard |
