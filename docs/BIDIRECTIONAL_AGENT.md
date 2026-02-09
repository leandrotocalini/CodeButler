# CodeButler as Bidirectional Subagent

## Architecture Overview

CodeButler runs as a **bidirectional subagent** within Claude Code. This means:

1. **WhatsApp → Claude Code**: Messages from WhatsApp are executed as prompts in Claude Code
2. **Claude Code → WhatsApp**: Questions from Claude are sent to WhatsApp, answers come back

## Components

### 1. Go Agent (`cmd/agent/main.go`)
- Connects to WhatsApp
- Listens for incoming messages
- Sends messages back to WhatsApp
- Communicates with coordinator via JSON over stdin/stdout

### 2. JavaScript Coordinator (`coordinator.js`)
- Manages the Go agent process
- Handles first-time setup (compile, wizard)
- Forwards WhatsApp messages to Claude Code
- Sends Claude's questions to WhatsApp
- Waits for user responses

### 3. Claude Code (you!)
- Receives prompts from WhatsApp
- Processes them like normal Claude Code tasks
- Can use AskUserQuestion - answers come from WhatsApp!
- Sends final results back

## Message Flow

### Incoming Message from WhatsApp

```
User (WhatsApp): "add authentication to the API"
         ↓
Go Agent receives message
         ↓
Sends JSON to coordinator:
{
  "type": "incoming",
  "content": {
    "from": "5491234567890@s.whatsapp.net",
    "chat": "120363123456789012@g.us",
    "content": "add authentication to the API",
    "isVoice": false
  }
}
         ↓
Coordinator executes Claude Code:
[Claude Code processes the prompt]
         ↓
Claude Code returns result
         ↓
Coordinator sends to Go Agent:
{
  "type": "reply",
  "content": "✅ Added JWT authentication with..."
}
         ↓
Go Agent sends to WhatsApp
         ↓
User receives: "[BOT] ✅ Added JWT authentication with..."
```

### Bidirectional Question/Answer

```
User (WhatsApp): "refactor the user model"
         ↓
Claude Code receives prompt and needs to ask:
"What database ORM are you using?"
         ↓
Coordinator sends to Go Agent:
{
  "type": "question",
  "content": {
    "question": "What database ORM are you using?",
    "options": ["Sequelize", "TypeORM", "Prisma", "Mongoose"]
  }
}
         ↓
Go Agent sends to WhatsApp:
"[BOT] What database ORM are you using?
1. Sequelize
2. TypeORM
3. Prisma
4. Mongoose"
         ↓
User responds: "3"
         ↓
Go Agent detects numeric response, sends:
{
  "type": "response",
  "content": {
    "answer": "3"
  }
}
         ↓
Coordinator passes to Claude Code
         ↓
Claude Code continues with Prisma choice
         ↓
Final result sent back to WhatsApp
```

## Setup Flow (First Time)

```bash
$ git clone https://github.com/user/CodeButler.git
$ cd CodeButler
$ node coordinator.js
```

**Coordinator detects first-time setup:**

```
🤖 CodeButler Agent Coordinator Starting...
📋 First time setup detected

🔧 Installing dependencies...
   Running: go mod download
   ✅ Dependencies installed

🔨 Building CodeButler...
   Running: go build -o codebutler cmd/codebutler/main.go
   ✅ Binary built

🎉 Now running setup wizard...

🤖 CodeButler - WhatsApp Agent for Claude Code SDK

📋 No config.json found. Starting setup wizard...

⚠️  Voice Transcription (Whisper API)
   Do you want to enable voice message transcription?
   This requires an OpenAI API key.

   Enable voice transcription? (yes/no) [no]: yes

   📝 Enter your OpenAI API key:
   API Key: sk-proj-...

   ✅ Voice transcription enabled

📱 WhatsApp Group Configuration

   What should the WhatsApp group be called?
   Group name [CodeButler Developer]:

   ✅ Using: CodeButler Developer

🤖 Bot Message Prefix

   Bot messages need a prefix to avoid processing its own messages.
   Bot prefix [[BOT]]:

   ✅ Using: [BOT]

📂 Source Code Directory

   Where are your code repositories located?
   Path [./Sources]:

   ✅ Using: ./Sources

📱 Connecting to WhatsApp...

   Scan this QR code with WhatsApp:

   ██████████████████████████████
   ██ ▄▄▄▄▄ █▀ █▀▀██▀█ ▄▄▄▄▄ ██
   ██ █   █ █▀▀▄ ▄▄▀▀█ █   █ ██
   ██ █▄▄▄█ ██▄▄█▄█▀▀█ █▄▄▄█ ██
   [... QR code ...]

   ✅ Connected as: 5491234567890@s.whatsapp.net

📋 Looking for group "CodeButler Developer"...
   Group not found, creating...
   ✅ Created group: CodeButler Developer

💾 Saving configuration...
   ✅ Configuration saved to config.json

🎉 Setup complete!

📱 Building agent binary...
   ✅ Agent binary built

🚀 Starting WhatsApp agent...
✅ Agent running! Waiting for WhatsApp messages...
```

## Integration with Claude Code

### How Claude Code Detects This Project

When you open the CodeButler directory in Claude Code, it reads `CLAUDE.md` and sees:

```markdown
## 🤖 Run as Background Agent

**IMPORTANT**: This project is designed to run as a Claude Code background agent.
```

Claude Code then offers to start the agent in background:

```
🤖 Claude Code detected a background agent project

Would you like to start the CodeButler WhatsApp agent?

[ ] Run in background (recommended)
[ ] Just open the project
[ ] Learn more

[Start Agent] [Skip]
```

If user clicks **Start Agent**, Claude Code runs:
```bash
node coordinator.js &
```

The agent stays running in background, and when WhatsApp messages arrive, they appear in Claude Code's interface like this:

```
📨 New WhatsApp Message

From: John Doe
Content: "add tests to the authentication module"

Execute this prompt?
[Run] [Ignore] [See Full Message]
```

### Handling AskUserQuestion

When Claude Code uses the `AskUserQuestion` tool during execution, the coordinator intercepts it:

**Claude Code calls:**
```javascript
AskUserQuestion({
  questions: [{
    question: "Which testing framework?",
    header: "Framework",
    options: [
      { label: "Jest", description: "Popular, feature-rich" },
      { label: "Vitest", description: "Fast, Vite-powered" },
      { label: "Mocha", description: "Flexible, minimal" }
    ]
  }]
})
```

**Coordinator transforms to WhatsApp:**
```
[BOT] Which testing framework?

1. Jest - Popular, feature-rich
2. Vitest - Fast, Vite-powered
3. Mocha - Flexible, minimal
```

**User responds:** "2"

**Coordinator passes back to Claude Code:**
```javascript
{
  answers: {
    "Framework": "Vitest"
  }
}
```

Claude Code continues execution with the user's choice.

## Commands from WhatsApp

### Repository Commands

- `repos` - List all repositories in Sources/
- `use <name>` - Select a repository
- `status` - Show current repository
- `clear` - Clear repository selection

### Execution Commands

- `run <prompt>` - Execute prompt in active repository
- `<any text>` - If repo is selected, executes as prompt

### Examples

```
User: repos
Bot: 📂 Available repositories:
     1. api-service
     2. mobile-app
     3. shared-lib

User: use api-service
Bot: ✅ Selected: api-service

User: add unit tests for auth
Bot: 🔄 Executing: add unit tests for auth
     [Claude Code processes...]
     ✅ Added 5 test files with 42 tests covering...
```

## Error Handling

### WhatsApp Connection Lost

```
[Agent Error] WhatsApp connection lost
[Agent] Reconnecting in 5 seconds...
[Agent] ✅ Reconnected successfully
```

### Voice Transcription Failure

```
User sends voice message
↓
Agent fails to transcribe (no OpenAI key)
↓
Bot sends: "❌ No pude transcribir el mensaje de voz.
           💡 La transcripción no está configurada."
```

### Claude Code Timeout

```
User: complex refactoring task
↓
Claude Code takes > 5 minutes
↓
Bot sends: "⏳ Esta tarea está tomando más tiempo...
           💡 Te aviso cuando termine."
↓
[Eventually completes]
↓
Bot sends: "✅ Tarea completada: [results]"
```

## Voice Message Support

Voice messages are automatically transcribed using OpenAI Whisper:

```
User sends voice: "Hola CodeButler, agregame tests al módulo de usuarios"
         ↓
Agent downloads audio
         ↓
Calls OpenAI Whisper API
         ↓
Transcription: "Hola CodeButler, agregame tests al módulo de usuarios"
         ↓
Processes as text message
         ↓
Claude Code receives text prompt
```

## Security Model

### Access Control
- Only messages from configured group are processed
- Bot ignores its own messages (prefix detection)
- No personal chat support (only group)

### API Keys
- OpenAI API key in config.json
- ANTHROPIC_API_KEY from environment (optional for intent parsing)
- WhatsApp session encrypted by whatsmeow library

## File Structure

```
CodeButler/
├── CLAUDE.md                 # This tells Claude how to run as agent
├── coordinator.js            # Main coordinator (starts agent, forwards messages)
├── config.json              # Generated by setup wizard
│
├── cmd/
│   ├── codebutler/main.go   # Setup wizard binary
│   └── agent/main.go        # WhatsApp agent binary
│
├── internal/
│   ├── whatsapp/            # WhatsApp client
│   ├── access/              # Access control
│   ├── audio/               # Whisper transcription
│   ├── config/              # Config management
│   ├── commands/            # Command parsing
│   ├── bot/                 # Bot logic
│   └── repo/                # Repository management
│
└── whatsapp-session/        # WhatsApp session data (gitignored)
```

## Future Enhancements

- [ ] Multi-user support (different users, different active repos)
- [ ] Task scheduling ("run tests every hour")
- [ ] Notification preferences (which events trigger WhatsApp messages)
- [ ] Web dashboard for monitoring
- [ ] Support for attachments (send files via WhatsApp)
- [ ] Voice responses (Claude → text-to-speech → WhatsApp)

## Troubleshooting

### Agent won't start
```bash
# Check if Go is installed
go version

# Check if Node.js is installed
node --version

# Rebuild agent
rm codebutler-agent
node coordinator.js
```

### Messages not being received
```bash
# Check agent logs
node coordinator.js

# Should see:
# ✅ Agent running! Waiting for WhatsApp messages...

# If not connected, delete session:
rm -rf whatsapp-session/
node coordinator.js
```

### Questions not reaching WhatsApp
- Check that bot prefix is configured
- Verify group JID in config.json
- Ensure numeric responses (1, 2, 3) are sent

---

**This is the power of bidirectional agent communication!** WhatsApp becomes your Claude Code interface. 🚀
