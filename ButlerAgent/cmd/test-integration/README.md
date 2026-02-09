# Test Integration - Phases 1-6

Este programa prueba las fases 1, 2, 3, 4, 5 y 6 de CodeButler:
- **Phase 1**: WhatsApp Integration (conexión, QR code, mensajes, grupos)
- **Phase 2**: Configuration System (cargar/validar config.json)
- **Phase 3**: Access Control (solo grupo autorizado)
- **Phase 4**: Audio Transcription (mensajes de voz con Whisper API) - opcional
- **Phase 5**: Repository Management (escanear repositorios en Sources/)
- **Phase 6**: Claude Code Executor (ejecutar comandos de Claude Code)

## Preparación

### 1. Crear config.json

```bash
cp config.sample.json config.json
```

### 2. Editar config.json

Mínimo requerido para la prueba:

```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "personalNumber": "",
    "groupJID": "",
    "groupName": ""
  },
  "openai": {
    "apiKey": "sk-test-key"
  },
  "claudeCode": {
    "oauthToken": ""
  },
  "sources": {
    "rootPath": "./Sources"
  }
}
```

**Notas**:
- `personalNumber`, `groupJID` y `groupName` se pueden dejar vacíos por ahora
- `apiKey` puede ser un valor de prueba si **solo probás mensajes de texto**
- `apiKey` debe ser **real de OpenAI** si querés probar **mensajes de voz** (transcripción con Whisper)
- `oauthToken` se puede dejar vacío (se carga desde env variable si existe)

### 3. Crear el grupo en WhatsApp

Desde tu WhatsApp:
1. Crea un nuevo grupo
2. Nómbralo: **"CodeButler Developer"**
3. No agregues a nadie más (solo vos)

## Ejecutar la Prueba

```bash
go run ./cmd/test-integration/main.go
```

## Qué Esperar

### Primera Ejecución (Sin Session)

```
🤖 CodeButler - Test Integration (Phase 1 + Phase 2)

📝 Loading configuration...
   ✅ Config loaded
   📁 Session path: ./whatsapp-session
   📂 Sources path: ./Sources

📱 Connecting to WhatsApp...

📱 Scan this QR code with WhatsApp:
   (Go to WhatsApp > Settings > Linked Devices > Link a Device)

[QR CODE ASCII ART AQUÍ]

✅ Successfully paired!
✅ Connected to WhatsApp

👤 Connected as: 5491134567890@s.whatsapp.net
   Name: Leandro

📋 Fetching groups...
   Found 3 group(s):
   1. CodeButler Developer
      JID: 120363123456789012@g.us
      ⭐ This looks like your control group!
      💡 Add this to config.json:
         "groupJID": "120363123456789012@g.us",
         "groupName": "CodeButler Developer"
   2. Familia
      JID: 120363987654321098@g.us
   3. Trabajo
      JID: 120363111111111111@g.us

👂 Listening for messages... (Press Ctrl+C to stop)
```

### Actualizaciones Después de Primera Ejecución

1. **Copia el JID del grupo** que el programa te mostró
2. **Actualiza config.json**:
   ```json
   {
     "whatsapp": {
       "sessionPath": "./whatsapp-session",
       "personalNumber": "5491134567890@s.whatsapp.net",
       "groupJID": "120363123456789012@g.us",
       "groupName": "CodeButler Developer"
     },
     ...
   }
   ```

### Segunda Ejecución (Con Session)

```
🤖 CodeButler - Test Integration (Phase 1 + Phase 2)

📝 Loading configuration...
   ✅ Config loaded
   ...

📱 Connecting to WhatsApp...
✅ Connected to WhatsApp

👤 Connected as: 5491134567890@s.whatsapp.net
   ...

👂 Listening for messages... (Press Ctrl+C to stop)
```

Ya no muestra el QR porque usa la sesión guardada.

## Probar Mensajes

Con el programa corriendo:

1. **Abrí WhatsApp** en tu teléfono
2. **Abrí el grupo "CodeButler Developer"**
3. **Enviá un mensaje**: `ping`
4. **El programa debería responder**: `pong! 🏓`

En la consola verás:

```
📨 Message received:
   From: 5491134567890@s.whatsapp.net
   Chat: 120363123456789012@g.us
   Content: ping
   IsGroup: true
   IsFromMe: false
   ⭐ From CodeButler Developer group!

🤖 Sending 'pong' response...
   ✅ Response sent
```

## Probar Access Control (Phase 3)

Con el programa corriendo:

1. **Enviá "ping" desde otro grupo** (no "CodeButler Developer")
2. **El programa debería BLOQUEAR** el mensaje

En la consola verás:

```
📨 Message received:
   From: ...
   Chat: 120363XXXXXXXXXX@g.us
   Content: ping
   IsGroup: true
   IsFromMe: true
   ⛔ BLOCKED: Not from authorized group
```

3. **Enviá "ping" desde chat personal** (mensaje directo)
4. **También debería ser bloqueado**

Solo los mensajes del grupo "CodeButler Developer" son procesados.

## Probar Audio Transcription (Phase 4)

**Requisito**: Necesitás un API key **real** de OpenAI en config.json

Con el programa corriendo:

1. **Abrí WhatsApp en tu celular**
2. **Abrí el grupo "CodeButler Developer"**
3. **Grabá un mensaje de voz** diciendo: "ping"
4. **Envía el audio**
5. **El programa debería:**
   - Descargar el audio
   - Transcribirlo con Whisper API
   - Detectar "ping" en el texto
   - Responder "pong! 🏓 (from voice)"

En la consola verás:

```
📨 Message received:
   From: ...
   Chat: 120363405395407771@g.us
   Content: [Voice Message]
   IsGroup: true
   IsFromMe: false
   ⭐ From CodeButler Developer group!
   🎤 Voice message detected

🎤 Processing voice message...
   ✅ Audio downloaded: /tmp/codebutler-audio-1234567890.ogg
   🔄 Transcribing with Whisper API...
   ✅ Transcription: "ping"
   🤖 Sending 'pong' response...
   ✅ Response sent
```

**Costo aproximado**: $0.006 por minuto de audio (~$0.001 por mensaje de voz típico)

## Probar Repository Management (Phase 5)

El programa automáticamente escanea el directorio `Sources/` al arrancar.

### Setup

1. **Crear directorio Sources:**
   ```bash
   mkdir -p Sources
   ```

2. **Clonar algunos repositorios:**
   ```bash
   cd Sources
   git clone https://github.com/user/go-project
   git clone https://github.com/user/node-app
   git clone https://github.com/user/python-tool
   cd ..
   ```

3. **Ejecutar test-integration:**
   ```bash
   ./test-integration
   ```

### Output esperado:

```
📂 Scanning repositories...
   Found 3 repositor(y/ies):
   1. go-project ✅ CLAUDE.md
      Path: ./Sources/go-project
   2. node-app ❌ CLAUDE.md
      Path: ./Sources/node-app
   3. python-tool ✅ CLAUDE.md
      Path: ./Sources/python-tool
```

### Indicadores:

- **✅ CLAUDE.md**: Repositorio listo para Claude Code
- **❌ CLAUDE.md**: Repositorio sin CLAUDE.md (no se puede usar)

**Nota**: No se muestra el tipo de proyecto (go/node/python) porque Claude Code es **language-agnostic** - lee el CLAUDE.md para entender cualquier proyecto.

**IMPORTANTE**: CodeButler requiere que los repositorios tengan un archivo `CLAUDE.md` para poder trabajar con ellos. Este archivo contiene las instrucciones para Claude Code sobre cómo trabajar con el proyecto.

## Probar Claude Code Executor (Phase 6)

Con el programa corriendo, podés usar comandos `@codebutler` en el grupo:

### 1. Ver ayuda

```
@codebutler help
```

### 2. Listar repositorios

```
@codebutler repos
```

Responde con lista de repos y cuáles tienen CLAUDE.md (✅/❌)

### 3. Seleccionar repositorio

```
@codebutler use aurum
```

Solo funciona si el repo tiene CLAUDE.md ✅

### 4. Ver repo activo

```
@codebutler status
```

### 5. Ejecutar comando (REAL - ejecuta Claude Code)

```
@codebutler run add error handling to the API
```

**IMPORTANTE**: Este comando ahora **ejecuta realmente Claude Code**, no es un placeholder.

**Requisitos**:
- Claude CLI instalado: `brew install claude` (o desde https://docs.anthropic.com/en/docs/claude-code)
- OAuth token configurado en `config.json` o variable de entorno `CLAUDE_CODE_OAUTH_TOKEN`
- Repositorio con CLAUDE.md seleccionado

**Flujo**:
1. Bot responde inmediatamente: "🤖 Executing... ⏳"
2. Claude Code ejecuta en background (puede tardar minutos)
3. Cuando termina, bot envía resultado automáticamente

**Ejemplo de respuesta final**:
```
✅ Execution completed in *aurum*
⏱️  Duration: 127.3s

📤 Output:
```
Added error handling to:
- api/handlers.go
- api/middleware.go
Updated tests in api/handlers_test.go
```
```

### 6. Limpiar sesión

```
@codebutler clear
```

### Output esperado:

```
📨 Message received:
   From: ...
   Chat: 120363405395407771@g.us
   Content: @codebutler repos
   IsGroup: true
   IsFromMe: false
   ⭐ From CodeButler Developer group!
   🤖 CodeButler command detected
   📤 Sending response...
   ✅ Response sent
```

Y en WhatsApp recibirás:

```
📂 Found 1 repositor(y/ies):

1. *aurum* ✅

✅ Claude-ready: 1/1

💡 Use: @codebutler use <repo-name>
```

## Qué Prueba Este Programa

### Phase 1: WhatsApp Integration ✅
- ✅ Conexión a WhatsApp
- ✅ QR code en primera ejecución
- ✅ Persistencia de sesión (SQLite)
- ✅ Obtener info de la cuenta (JID, nombre)
- ✅ Listar grupos
- ✅ Recibir mensajes
- ✅ Enviar mensajes
- ✅ Detectar grupo específico

### Phase 2: Configuration System ✅
- ✅ Cargar config.json
- ✅ Validar campos requeridos
- ✅ Leer configuración de WhatsApp
- ✅ Leer configuración de OpenAI
- ✅ Leer configuración de Sources

### Phase 3: Access Control ✅
- ✅ Validar mensajes del grupo autorizado
- ✅ Bloquear mensajes de otros grupos
- ✅ Bloquear mensajes de chats personales
- ✅ Fail-safe cuando no hay grupo configurado

### Phase 4: Audio Transcription ✅
- ✅ Detectar mensajes de voz
- ✅ Descargar audio de WhatsApp
- ✅ Transcribir con OpenAI Whisper API
- ✅ Procesar texto transcrito
- ✅ Responder a comandos de voz

### Phase 5: Repository Management ✅
- ✅ Escanear directorio Sources/
- ✅ Detectar repositorios git
- ✅ Detectar CLAUDE.md en cada repo
- ✅ Listar repositorios disponibles
- ✅ Language-agnostic (no detecta tipo de proyecto)

### Phase 6: Claude Code Executor ✅ (COMPLETO)
- ✅ Parsear comandos @codebutler desde WhatsApp
- ✅ Comando help (mostrar ayuda)
- ✅ Comando repos (listar repositorios)
- ✅ Comando use (seleccionar repositorio)
- ✅ Comando status (ver repositorio activo)
- ✅ Comando run (**EJECUTA REALMENTE Claude Code**)
- ✅ Comando clear (limpiar sesión)
- ✅ Validación de comandos
- ✅ Session management (contexto por grupo)
- ✅ Verificar CLAUDE.md antes de usar repo
- ✅ Ejecución en background (no bloquea el bot)
- ✅ Captura de output y errores
- ✅ Envío automático de resultados cuando termina
- ✅ Timeout configurable (5 minutos default)
- ✅ Truncate de outputs largos (evita spam)

## Troubleshooting

### "config.json not found"
```bash
cp config.sample.json config.json
# Editá config.json con tus valores
```

### "failed to parse config"
- Verificá que config.json sea JSON válido
- Verificá que todos los campos requeridos estén presentes

### "No groups found"
- Creá el grupo "CodeButler Developer" en WhatsApp
- Esperá unos segundos y volvé a ejecutar

### QR code no aparece
- El programa lo muestra automáticamente en la primera ejecución
- Si ya escaneaste antes, usa la sesión existente
- Para resetear: `rm -rf ./whatsapp-session`

### "Failed to send message"
- Verificá que el groupJID en config.json sea correcto
- Verificá que estés en el grupo
- Verificá tu conexión a internet

### "Failed to download audio"
- El mensaje debe ser un audio/voz (no imagen, video, etc.)
- Verificá tu conexión a internet
- El audio puede estar corrupto

### "Failed to transcribe" / "API returned status 401"
- API key de OpenAI inválida o expirada
- Verificá tu key en https://platform.openai.com/api-keys
- Asegurate que esté bien copiada en config.json

### "API returned status 429"
- Rate limit excedido de OpenAI
- Esperá unos minutos y volvé a probar
- Verificá tu plan en OpenAI

### "Claude Code CLI not installed"
- Claude CLI no está instalado
- Instalá con: `brew install claude` (macOS)
- O desde: https://docs.anthropic.com/en/docs/claude-code
- Verificá con: `claude --version`

### "@codebutler run" se queda pensando mucho tiempo
- Claude Code puede tardar varios minutos (es normal)
- Timeout default: 5 minutos
- Si tarda más, vas a recibir error de timeout
- El bot te notifica cuando termina

### "context deadline exceeded" en run
- El comando tardó más de 5 minutos
- Probá con un prompt más simple
- O esperá que el bot implemente timeout configurable

### "No active repository" al hacer run
- Necesitás seleccionar un repo primero
- Hacé: `@codebutler use <repo-name>`
- Verificá con: `@codebutler status`

## Siguiente Paso

Una vez que todo funcione:
- **Phase 7**: First-time Setup (wizard interactivo)
- **Phase 8**: Advanced Features (workflows, multi-repo, etc.)
- **Phase 9**: Testing (test suite completo)
- **Phase 10**: Documentation (docs finales)
- **Phase 11**: Build & Deploy (deployment)