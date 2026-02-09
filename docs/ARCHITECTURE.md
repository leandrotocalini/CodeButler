# How CodeButler Works with Claude Code

## User Workflow

### 1. Clone and Open

```bash
git clone https://github.com/user/CodeButler.git
cd CodeButler
claude
```

### 2. Claude Code Detects Project

When Claude Code starts, it:
- Reads `CLAUDE.md`
- Sees setup instructions
- Checks if `config.json` exists

**If config.json doesn't exist (first time):**

```
👋 Welcome to CodeButler!

This project turns WhatsApp into an interface for Claude Code.
You can send coding tasks via WhatsApp and get results back.

Would you like to set it up now? (5 minutes)

[Yes, set up CodeButler] [No, just browse the code]
```

### 3. Setup (if user says yes)

Claude Code automatically runs:

```bash
# Install dependencies
go mod download

# Build setup wizard
go build -o codebutler cmd/codebutler/main.go

# Run wizard (interactive)
./codebutler
```

**Wizard asks:**
- Enable voice transcription? (OpenAI API key)
- WhatsApp group name? (default: "CodeButler Developer")
- Bot prefix? (default: "[BOT]")
- Source code directory? (default: "./Sources")

**Then:**
- Displays QR code
- User scans with WhatsApp
- Creates/finds the group
- Saves config.json

### 4. Start Agent

After setup completes:

```
✅ Setup complete!

Would you like to start the WhatsApp agent in background?

[Start Agent] [Start Later]
```

**If user clicks "Start Agent":**

```bash
# Build agent
go build -o codebutler-agent cmd/agent/main.go

# Run in background
./codebutler-agent &
```

**Output:**

```
📱 Connecting to WhatsApp...
✅ CodeButler agent running
📋 Monitoring group: CodeButler Developer

👂 Listening for WhatsApp messages...
   Press Ctrl+C to stop
```

## Message Flow

### When WhatsApp Message Arrives

User sends via WhatsApp:
```
"add authentication to the API"
```

**Agent output (Claude Code sees this):**

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📨 INCOMING WHATSAPP MESSAGE
From: 5491234567890@s.whatsapp.net
Chat: 120363123456789012@g.us
Content: add authentication to the API
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

EXECUTE_PROMPT: add authentication to the API
```

### How Claude Code Should React

When Claude Code sees `EXECUTE_PROMPT:`, it should:

1. **Extract the prompt** (everything after `EXECUTE_PROMPT:`)
2. **Execute it** like the user typed it directly
3. **Process normally** (read files, make changes, etc.)
4. **Send result back** (via the agent)

**Example processing:**

```
Claude Code: I'll add JWT authentication to your API.

[Reads files]
[Makes changes]
[Writes code]

Claude Code: ✅ Done! I've added:
- JWT token generation in auth.js
- Middleware in middleware/auth.js
- Protected routes in routes/api.js

Total: 3 files modified, 127 lines added
```

### Sending Result Back to WhatsApp

**Claude Code needs to signal the agent to send response.**

Currently, the agent sends acknowledgment immediately:
```
[BOT] Mensaje recibido! Procesando...
```

**For the final result**, Claude Code could:

**Option 1: Write to a file**
```bash
echo "✅ Authentication added successfully" > /tmp/codebutler-response.txt
```

Agent watches this file and sends contents to WhatsApp.

**Option 2: Call a script**
```bash
./send-whatsapp.sh "✅ Authentication added successfully"
```

Script communicates with running agent via IPC.

**Option 3: Agent polls Claude Code status**
Agent checks if Claude Code is still processing, then reads final output.

## Bidirectional Communication (Questions)

### When Claude Needs to Ask a Question

**Claude Code uses AskUserQuestion:**

```javascript
AskUserQuestion({
  questions: [{
    question: "Which database ORM?",
    header: "ORM",
    options: [
      { label: "Sequelize", description: "Popular SQL ORM" },
      { label: "Prisma", description: "Modern TypeScript ORM" },
      { label: "Mongoose", description: "MongoDB ODM" }
    ]
  }]
})
```

**Current challenge:** The agent needs to intercept this.

**Possible solution:**

Claude Code writes question to file:
```json
{
  "type": "question",
  "question": "Which database ORM?",
  "options": ["Sequelize", "Prisma", "Mongoose"]
}
```

Agent reads file, formats for WhatsApp:
```
[BOT] Which database ORM?

1. Sequelize - Popular SQL ORM
2. Prisma - Modern TypeScript ORM
3. Mongoose - MongoDB ODM
```

User responds: `"2"`

Agent writes response to file:
```json
{
  "type": "response",
  "answer": "2"
}
```

Claude Code reads response and continues with "Prisma".

## Voice Messages

If user sends voice message:

**Agent:**
1. Downloads audio file
2. Calls OpenAI Whisper API
3. Transcribes to text
4. Processes as normal text message

**Output:**

```
🎤 Voice transcribed: "add tests to the authentication module"

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📨 INCOMING WHATSAPP MESSAGE
From: 5491234567890@s.whatsapp.net
Chat: 120363123456789012@g.us
Content: add tests to the authentication module
Type: Voice (transcribed)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

EXECUTE_PROMPT: add tests to the authentication module
```

## Error Handling

### Agent Connection Lost

```
❌ Failed to connect: websocket connection closed
[Agent] Reconnecting in 5 seconds...
✅ CodeButler agent running
```

### Voice Transcription Failed

```
[BOT] ❌ No pude transcribir el mensaje de voz.

💳 Tu cuenta de OpenAI se quedó sin créditos.
💡 Agregá saldo en: https://platform.openai.com/account/billing
```

### Claude Code Timeout

If Claude Code takes too long (> 5 minutes), agent could send:

```
[BOT] ⏳ Esta tarea está tomando más tiempo de lo esperado.
Te aviso cuando termine.
```

## Repository Management

### Sources Directory Structure

```
CodeButler/
└── Sources/
    ├── my-api/
    │   ├── CLAUDE.md
    │   └── [code files]
    ├── my-frontend/
    │   ├── CLAUDE.md
    │   └── [code files]
    └── my-mobile/
        ├── CLAUDE.md
        └── [code files]
```

### Using Repository Commands

```
User: repos
Bot: 📂 Available repositories:
     1. my-api
     2. my-frontend
     3. my-mobile

User: use my-api
Bot: ✅ Selected: my-api

User: add authentication
[Claude Code processes in my-api context]
```

## Architecture Summary

```
┌─────────────────┐
│   WhatsApp      │ ← User sends message
│   (Mobile)      │
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│  codebutler-    │ ← Go agent (background)
│  agent          │   - Connects to WhatsApp
│                 │   - Prints to stdout
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│  Claude Code    │ ← You (reading stdout)
│  (Terminal)     │   - Sees EXECUTE_PROMPT:
│                 │   - Processes prompt
│                 │   - Makes code changes
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│  Response       │ ← Via file/script/IPC
│  Mechanism      │
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│  WhatsApp       │ ← User receives result
│  (Mobile)       │
└─────────────────┘
```

## Next Steps

To make this fully work, we need:

1. **Response mechanism**: How Claude Code sends final result to agent
2. **Question interception**: How to detect AskUserQuestion calls
3. **Status tracking**: Know when Claude Code is done processing

**Possible implementations:**

### Response Mechanism (File-based)

**Claude Code writes result:**
```bash
cat > /tmp/codebutler-response.txt << EOF
✅ Authentication added successfully!

Modified files:
- src/auth/jwt.js
- src/middleware/auth.js
- src/routes/api.js

Total: 127 lines added
EOF
```

**Agent watches file:**
```go
watcher, _ := fsnotify.NewWatcher()
watcher.Add("/tmp/codebutler-response.txt")

for {
    select {
    case event := <-watcher.Events:
        if event.Op&fsnotify.Write == fsnotify.Write {
            content, _ := os.ReadFile("/tmp/codebutler-response.txt")
            client.SendMessage(chatID, string(content))
            os.Remove("/tmp/codebutler-response.txt")
        }
    }
}
```

### Question Interception (File-based)

**Claude Code writes question:**
```bash
cat > /tmp/codebutler-question.json << EOF
{
  "question": "Which database ORM?",
  "options": ["Sequelize", "Prisma", "Mongoose"]
}
EOF
```

**Agent formats and sends to WhatsApp, waits for response.**

**Agent writes answer back:**
```bash
cat > /tmp/codebutler-answer.txt << EOF
2
EOF
```

**Claude Code reads answer and continues.**

---

**This architecture makes CodeButler a true bidirectional WhatsApp interface for Claude Code!** 🚀
