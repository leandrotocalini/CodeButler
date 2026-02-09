# CodeButler - WhatsApp Interface for Claude Code

> **Instructions for Claude Code when working in this project**

## 🚀 Setup Flow

When user opens this project for the first time:

1. Check if `config.json` exists
2. If NOT exists, run: `./setup.sh`
3. This opens a **web browser** at `http://localhost:3000`
4. User completes setup in browser (scan QR, enter keys)
5. Setup wizard returns JSON status
6. Start agent in background

## 📡 JSON Protocol (Agent ↔ Claude)

All communication between the Go agent and Claude Code happens via **JSON files** in `/tmp/codebutler/`.

### Directory Structure

```
/tmp/codebutler/
├── incoming.json       # Agent writes, Claude reads (WhatsApp → Claude)
├── outgoing.json       # Claude writes, Agent reads (Claude → WhatsApp)
├── question.json       # Claude writes when asking user
└── answer.json         # Agent writes when user responds
```

### 1. Incoming Message (WhatsApp → Claude)

**File:** `/tmp/codebutler/incoming.json`

```json
{
  "type": "message",
  "timestamp": "2025-02-09T20:00:00Z",
  "message_id": "msg_abc123",
  "from": {
    "jid": "5491234567890@s.whatsapp.net",
    "name": "Leandro"
  },
  "chat": {
    "jid": "120363123456789012@g.us",
    "name": "CodeButler Developer"
  },
  "content": "add authentication to the API",
  "is_voice": false,
  "transcript": null
}
```

**What Claude should do:**
1. Read `/tmp/codebutler/incoming.json`
2. Process the prompt in `content`
3. Write response to `/tmp/codebutler/outgoing.json`
4. Delete incoming.json (consumed)

### 2. Outgoing Response (Claude → WhatsApp)

**File:** `/tmp/codebutler/outgoing.json`

```json
{
  "type": "response",
  "timestamp": "2025-02-09T20:05:00Z",
  "reply_to": "msg_abc123",
  "chat_jid": "120363123456789012@g.us",
  "content": "✅ Authentication added successfully!\n\nModified files:\n- src/auth/jwt.js (new)\n- src/middleware/auth.js (new)\n- src/routes/api.js (updated)\n\nTotal: 127 lines added"
}
```

**What Agent does:**
1. Poll `/tmp/codebutler/outgoing.json` every 1s
2. When found, send to WhatsApp
3. Delete file (consumed)

### 3. Ask Question (Claude → User)

**File:** `/tmp/codebutler/question.json`

```json
{
  "type": "question",
  "timestamp": "2025-02-09T20:02:00Z",
  "question_id": "q_xyz789",
  "chat_jid": "120363123456789012@g.us",
  "text": "Which database ORM?",
  "options": [
    "Sequelize",
    "Prisma",
    "Mongoose"
  ],
  "timeout": 30
}
```

**What Agent does:**
1. Send question to WhatsApp as: `[BOT] Which database ORM?\n1. Sequelize\n2. Prisma\n3. Mongoose`
2. Wait for user response
3. Write answer to `/tmp/codebutler/answer.json`

### 4. Answer (User → Claude)

**File:** `/tmp/codebutler/answer.json`

```json
{
  "type": "answer",
  "timestamp": "2025-02-09T20:02:15Z",
  "question_id": "q_xyz789",
  "selected": 2,
  "text": "Prisma"
}
```

**What Claude does:**
1. Poll `/tmp/codebutler/answer.json`
2. When found, read answer
3. Continue processing
4. Delete file (consumed)

### 5. Setup Status (Wizard → Claude)

**File:** `/tmp/codebutler/setup-status.json`

```json
{
  "type": "setup_complete",
  "timestamp": "2025-02-09T19:55:00Z",
  "success": true,
  "user": {
    "jid": "5493764705749@s.whatsapp.net",
    "name": "Leandro"
  },
  "group": {
    "jid": "120363405395407771@g.us",
    "name": "CodeButler Developer"
  },
  "voice_enabled": true,
  "config_path": "./config.json"
}
```

## 🔧 Commands for Claude

### Run Setup
```bash
./setup.sh
```
- Builds binaries (wizard + agent)
- Starts web server at http://localhost:3000
- Opens browser automatically
- Returns when setup complete
- Outputs: `/tmp/codebutler/setup-status.json`

### Start Agent
```bash
./start-agent.sh
```
- Runs agent in background
- Creates `/tmp/codebutler/` directory
- Starts monitoring for messages

### Send Response
```bash
# Claude writes JSON, agent picks it up automatically
cat > /tmp/codebutler/outgoing.json <<EOF
{
  "type": "response",
  "chat_jid": "120363405395407771@g.us",
  "content": "Done!"
}
EOF
```

### Ask Question
```bash
cat > /tmp/codebutler/question.json <<EOF
{
  "type": "question",
  "question_id": "q1",
  "chat_jid": "120363405395407771@g.us",
  "text": "Which option?",
  "options": ["A", "B", "C"],
  "timeout": 30
}
EOF

# Wait for answer
while [ ! -f /tmp/codebutler/answer.json ]; do sleep 1; done
cat /tmp/codebutler/answer.json
```

## 📝 Claude's Workflow

### When project opens:

```bash
if [ ! -f config.json ]; then
  echo "👋 Welcome to CodeButler! Setting up..."
  ./setup.sh

  # Read setup status
  STATUS=$(cat /tmp/codebutler/setup-status.json)

  # Show user what happened
  echo "✅ Setup complete!"
  echo "   User: $(echo $STATUS | jq -r '.user.name')"
  echo "   Group: $(echo $STATUS | jq -r '.group.name')"

  # Start agent
  ./start-agent.sh
  echo "✅ Agent running in background"
else
  echo "👋 Welcome back! Starting agent..."
  ./start-agent.sh
fi
```

### When incoming message arrives:

```bash
# Agent writes incoming.json when message arrives
if [ -f /tmp/codebutler/incoming.json ]; then
  MSG=$(cat /tmp/codebutler/incoming.json)
  CONTENT=$(echo $MSG | jq -r '.content')
  CHAT=$(echo $MSG | jq -r '.chat.jid')
  MSG_ID=$(echo $MSG | jq -r '.message_id')

  # Process the prompt
  echo "📨 WhatsApp: $CONTENT"

  # ... do work ...

  # Send response
  cat > /tmp/codebutler/outgoing.json <<EOF
{
  "type": "response",
  "reply_to": "$MSG_ID",
  "chat_jid": "$CHAT",
  "content": "✅ Task completed!"
}
EOF

  # Clean up
  rm /tmp/codebutler/incoming.json
fi
```

### When asking a question:

```bash
# Write question
cat > /tmp/codebutler/question.json <<EOF
{
  "type": "question",
  "question_id": "$(uuidgen)",
  "chat_jid": "120363405395407771@g.us",
  "text": "Which database?",
  "options": ["PostgreSQL", "MySQL", "MongoDB"],
  "timeout": 30
}
EOF

# Wait for answer (max 30s)
TIMEOUT=30
ELAPSED=0
while [ ! -f /tmp/codebutler/answer.json ] && [ $ELAPSED -lt $TIMEOUT ]; do
  sleep 1
  ELAPSED=$((ELAPSED + 1))
done

if [ -f /tmp/codebutler/answer.json ]; then
  ANSWER=$(cat /tmp/codebutler/answer.json)
  SELECTED=$(echo $ANSWER | jq -r '.selected')
  TEXT=$(echo $ANSWER | jq -r '.text')

  echo "User selected: $TEXT"
  rm /tmp/codebutler/answer.json
else
  echo "⏱️  Question timed out"
fi
```

## 🗂️ Project Structure

```
CodeButler/
├── CLAUDE.md                    # This file (instructions for Claude)
├── README.md                    # User documentation
├── setup.sh                     # Setup script (builds + starts wizard)
├── start-agent.sh               # Start agent in background
│
├── ButlerAgent/                 # Go source code
│   ├── cmd/
│   │   ├── setup-wizard/        # Web-based setup wizard
│   │   └── agent/               # WhatsApp agent
│   └── internal/
│       ├── whatsapp/            # WhatsApp client
│       ├── protocol/            # JSON protocol handlers
│       └── config/              # Config management
│
├── config.json                  # Runtime config (gitignored)
├── Sources/                     # User's code repos (gitignored)
└── /tmp/codebutler/            # JSON communication files
```

## 🚫 What Claude Should NOT Do

- ❌ Don't parse terminal output or logs
- ❌ Don't "guess" what happened
- ❌ Don't use ask-question.sh or send-response.sh (old scripts)
- ❌ Don't run the agent directly (`./codebutler-agent`)
- ✅ Always use JSON files for communication
- ✅ Always use ./setup.sh and ./start-agent.sh

## 🎯 Example: Full Workflow

```bash
# User opens project for first time
$ claude

# Claude detects no config.json
echo "👋 Setting up CodeButler..."

./setup.sh
# → Opens browser at http://localhost:3000
# → User scans QR, enters OpenAI key
# → Wizard writes /tmp/codebutler/setup-status.json
# → Browser shows "Setup complete!"

# Claude reads setup status
cat /tmp/codebutler/setup-status.json
# {
#   "success": true,
#   "user": {"name": "Leandro", ...},
#   "group": {"name": "CodeButler Developer", ...}
# }

echo "✅ Setup complete! Starting agent..."

./start-agent.sh
# → Agent runs in background
# → Monitors WhatsApp
# → Writes incoming.json when messages arrive

echo "✅ CodeButler ready! Send messages from WhatsApp."

# --- Later: Message arrives from WhatsApp ---

# Agent writes incoming.json
cat /tmp/codebutler/incoming.json
# {
#   "content": "add JWT authentication",
#   "chat": {"jid": "120363...@g.us"},
#   ...
# }

echo "📨 Task: add JWT authentication"

# Claude does the work...
# ... reads files, makes changes, etc ...

# Claude sends response
cat > /tmp/codebutler/outgoing.json <<EOF
{
  "type": "response",
  "chat_jid": "120363405395407771@g.us",
  "content": "✅ JWT authentication added!\n\nFiles created:\n- src/auth/jwt.js\n- src/middleware/auth.js"
}
EOF

rm /tmp/codebutler/incoming.json

echo "✅ Response sent to WhatsApp"
```

---

**That's it!** Clean, simple, JSON-based communication. No guessing, no parsing logs.
