# CodeButler 2

Plan de evolución de CodeButler: migración WhatsApp → Slack + nuevas features.

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
  "openai": { "apiKey": "sk-..." },
  "kimi":   { "apiKey": "..." }
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
- Nuevo: `kimi.apiKey` en global

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
5. (Optional) Prompt: "Kimi API key:"
6. Save → ~/.codebutler/config.json (global)
7. Connect Socket Mode
8. List channels → select or create
9. Save → <repo>/.codebutler/config.json (per-repo)
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

---

## 16. Auto-Memory (Kimi)

El daemon extrae aprendizajes automáticamente al final de cada conversación
y los persiste en `memory.md`. Usa Kimi (barato y rápido) en vez de Claude.

### Archivo

```
<repo>/.codebutler/memory.md
```

Se inyecta como contexto al prompt de Claude en cada conversación nueva.

### Trigger

Cuando la conversación termina (60s de silencio), el daemon:

1. Lee `memory.md` actual
2. Arma un resumen de la conversación que acaba de terminar
3. Llama a Kimi con ambos
4. Aplica las operaciones que Kimi devuelve

### Prompt a Kimi

```
You manage a memory file. Given the current memory and a conversation
that just ended, respond with a JSON array of operations.

Each operation is one of:
- {"op": "none"}        — nothing new to remember
- {"op": "append", "line": "- ..."}  — add a new learning
- {"op": "replace", "old": "exact existing line", "new": "merged line"}
                        — merge new info into an existing entry

Rules:
- Use "replace" when new info can be combined with an existing line
  (e.g., "cats are carnivores" + learning "dogs are carnivores"
   → replace with "cats and dogs are carnivores")
- Use "append" only for genuinely new knowledge
- Keep each line concise (1 line max)
- Only record useful decisions, conventions, gotchas — not trivia
- Return [{"op": "none"}] if there is nothing worth remembering
- You can return as many operations as needed

Current memory:
---
{contents of memory.md}
---

Conversation:
---
{conversation messages}
---
```

### Respuesta esperada

```json
[
  {"op": "replace", "old": "- Los gatos son carnívoros", "new": "- Los gatos y los perros son carnívoros"},
  {"op": "append", "line": "- Deploy siempre con --force en staging"}
]
```

O si no hay nada nuevo:

```json
[{"op": "none"}]
```

### Implementación

- **Archivo**: `internal/memory/memory.go`
- **Funciones**:
  - `Load(path) string` — lee memory.md (o "" si no existe)
  - `Apply(content string, ops []Op) string` — aplica operaciones al contenido
  - `Save(path, content string)` — escribe memory.md
- **Kimi client**: `internal/kimi/client.go`
  - API compatible con OpenAI (chat completions)
  - Solo se usa para auto-memory
  - Necesita `kimi.apiKey` en config global
- **Integración en daemon**: al final de `endConversation()`, lanzar
  goroutine que llama a Kimi y actualiza memory.md (no bloquea el loop)

### Config

```json
// ~/.codebutler/config.json (global)
{
  "kimi": { "apiKey": "..." }
}
```

Si no hay Kimi API key configurada, auto-memory se desactiva silenciosamente.

---

## 17. Logging — Plain Structured Logs

Reemplazar el sistema dual (ring buffer + TUI con ANSI) por un único canal
de logs planos, estructurados, con buena información.

### Formato

```
2026-02-14 15:04:05 INF  slack connected
2026-02-14 15:04:08 MSG  leandro: "che arreglá el bug del login"
2026-02-14 15:04:08 MSG  leandro: "y fijate el CSS también"
2026-02-14 15:04:11 CLD  processing 2 messages (new session)
2026-02-14 15:04:45 CLD  done · 34s · 3 turns · $0.12
2026-02-14 15:04:45 RSP  "Arreglé el bug del login y ajusté el CSS..."
2026-02-14 15:05:45 INF  conversation ended (60s silence)
2026-02-14 15:05:46 MEM  kimi: append "Login usa bcrypt, no md5"
2026-02-14 15:06:00 WRN  slack reconnecting...
2026-02-14 15:06:01 ERR  kimi API timeout after 10s
```

### Niveles

| Tag | Significado |
|-----|-------------|
| `INF` | Info del sistema: conexión, config, estado |
| `WRN` | Warnings: reconexiones, timeouts recuperables |
| `ERR` | Errores: fallos de API, crashes recuperados |
| `DBG` | Debug: solo si se habilita verbose mode |
| `MSG` | Mensaje entrante del usuario |
| `CLD` | Actividad de Claude: start, done, resume |
| `RSP` | Respuesta enviada al canal |
| `MEM` | Operaciones de auto-memory |

### Qué se elimina

- `Clear()` — no más clear screen
- `Header()` — no más banners con separadores
- `UserMsg()` — reemplazado por `MSG`
- `BotStart()` / `BotResult()` / `BotText()` — reemplazado por `CLD` y `RSP`
- `Status()` — reemplazado por `INF`
- ANSI escape codes — todo plano
- Dependencia `go-isatty` — ya no se necesita

### Qué se mantiene

- **Ring buffer + SSE** para el web dashboard (misma mecánica, nuevo formato)
- **Subscribers** (`Subscribe()` / `Unsubscribe()`)

### Implementación

Un solo método interno `log(tag, format, args...)` que:
1. Formatea: `{datetime} {TAG}  {message}`
2. Escribe a stderr
3. Almacena en ring buffer
4. Notifica subscribers

Métodos públicos: `Inf()`, `Wrn()`, `Err()`, `Dbg()`, `Msg()`, `Cld()`, `Rsp()`, `Mem()`

---

## 18. Service Install — macOS + Linux

Correr CodeButler como servicio del sistema. En macOS usa **LaunchAgent**,
en Linux usa **systemd user service**. Ambos corren en la sesión del usuario,
lo que da acceso a:

- Xcode toolchain (`xcodebuild test`, `swift test`, `xcrun`)
- User keychain
- PATH con developer tools
- Homebrew binaries
- Environment variables del usuario

Un LaunchDaemon (macOS) o system-level systemd service correría como root
sin sesión y no tendría acceso a nada de esto.

### macOS: LaunchAgent Plist

```xml
<!-- ~/Library/LaunchAgents/com.codebutler.<repo>.plist -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.codebutler.myrepo</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/codebutler</string>
    </array>
    <key>WorkingDirectory</key>
    <string>/Users/leandro/projects/myrepo</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/Users/leandro/.codebutler/logs/myrepo.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/leandro/.codebutler/logs/myrepo.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/Applications/Xcode.app/Contents/Developer/usr/bin</string>
    </dict>
</dict>
</plist>
```

### Linux: systemd User Service

```ini
# ~/.config/systemd/user/codebutler-myrepo.service
[Unit]
Description=CodeButler - myrepo
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/codebutler
WorkingDirectory=/home/leandro/projects/myrepo
Restart=always
RestartSec=5
StandardOutput=append:/home/leandro/.codebutler/logs/myrepo.log
StandardError=append:/home/leandro/.codebutler/logs/myrepo.log

[Install]
WantedBy=default.target
```

```bash
# Para que user services arranquen sin login:
sudo loginctl enable-linger leandro
```

`enable-linger` permite que los servicios del usuario arranquen al boot
sin necesidad de login. Sin linger, arrancan al login (como LaunchAgent).

### CLI Commands

```bash
codebutler --install     # genera plist + launchctl load
codebutler --uninstall   # launchctl unload + borra plist
codebutler --status      # muestra si el servicio está corriendo
codebutler --logs        # tail -f del log file
```

### `--install` hace:

1. Detecta repo actual (`pwd`) y nombre (basename)
2. Detecta path del binario `codebutler`
3. Detecta OS (`runtime.GOOS`)
4. Crea `~/.codebutler/logs/` si no existe
5. **macOS**: genera plist → `~/Library/LaunchAgents/` → `launchctl load`
6. **Linux**: genera unit → `~/.config/systemd/user/` → `systemctl --user enable --now`

### Múltiples repos

Cada repo es un servicio independiente:

```
# macOS
~/Library/LaunchAgents/
  com.codebutler.myapp.plist
  com.codebutler.backend.plist
  com.codebutler.ios-app.plist

# Linux
~/.config/systemd/user/
  codebutler-myapp.service
  codebutler-backend.service
  codebutler-ios-app.service
```

Cada uno con su propio `WorkingDirectory`, log file, y canal de Slack.

### Comportamiento

- macOS: `RunAtLoad` + `KeepAlive` → arranca al login, reinicia si crashea
- Linux: `enable` + `Restart=always` → mismo comportamiento
- Linux con `enable-linger`: arranca al boot sin necesidad de login
- Logs van a `~/.codebutler/logs/<repo>.log` (formato plano, sección 17)
- El web dashboard sigue disponible en su puerto (auto-incrementa si busy)

---

## 19. Claude Sandboxing — System Prompt

El prompt de sistema que CodeButler le pasa a `claude -p` debe empezar con
restricciones claras para "jailear" al agente dentro del repo.

### Prefijo obligatorio del prompt

```
RESTRICTIONS — READ FIRST:
- You MUST NOT install software, packages, or dependencies (no brew, apt,
  npm install, pip install, go install, etc.)
- You MUST NOT leave the current working directory or access files outside
  this repository
- You MUST NOT modify system files, configs outside the repo, or
  environment variables
- You MUST NOT make network requests except through tools already available
  in the repo
- You MUST NOT run destructive commands (rm -rf, git push --force,
  DROP TABLE, etc.)
- If a task requires any of the above, explain what is needed and STOP

You are working in: {repo_path}
```

### Por qué

Dado que Claude corre con `permissionMode: bypassPermissions`, tiene acceso
completo al shell. Sin estas restricciones en el prompt, Claude podría:
- Instalar paquetes que rompan el sistema
- Navegar fuera del repo y leer/modificar otros archivos
- Hacer `git push --force` o borrar branches
- Ejecutar comandos destructivos

El prompt sandboxing es una capa de defensa por software (no es un sandbox
real del OS), pero en la práctica Claude respeta estas instrucciones
consistentemente.

### Implementación

En `internal/agent/agent.go`, el prompt se arma como:

```go
prompt := sandboxPrefix + "\n\n" + memoryContext + "\n\n" + userMessages
```

Donde `sandboxPrefix` es una constante con las restricciones.
