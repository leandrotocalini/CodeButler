# CodeButler - WhatsApp Interface for Claude Code

> **Instructions for Claude Code when working in this project**

## 🤖 What Claude Should Do on Project Open

**ALWAYS run this immediately when project opens:**

```bash
./butler.sh
```

This single command:
- Builds CodeButler binary
- Opens web UI at http://localhost:3000
- Shows setup wizard (if first time) OR dashboard (if configured)
- Runs in background

**Then tell the user:**

```
🤖 CodeButler is running at http://localhost:3000

[First time]
→ Open the browser and complete the 3-step setup
→ I'll monitor for WhatsApp messages automatically

[Already configured]
→ Dashboard is open - agent is running
→ Send messages from your WhatsApp group
```

## 🚀 Quick Start (for users reading this)

```bash
./butler.sh
```

That's it. One command.

## 📡 JSON Protocol (Agent ↔ Claude)

All communication happens via JSON files in `/tmp/codebutler/`:

```
/tmp/codebutler/
├── incoming.json    # Agent → Claude (WhatsApp messages)
├── outgoing.json    # Claude → Agent (responses)
├── question.json    # Claude → Agent (ask user)
└── answer.json      # Agent → Claude (user response)
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
2. Process the prompt
3. Write response to `/tmp/codebutler/outgoing.json`
4. Delete incoming.json

### 2. Outgoing Response (Claude → WhatsApp)

**File:** `/tmp/codebutler/outgoing.json`

```json
{
  "type": "response",
  "timestamp": "2025-02-09T20:05:00Z",
  "reply_to": "msg_abc123",
  "chat_jid": "120363123456789012@g.us",
  "content": "✅ Authentication added!\n\nFiles:\n- src/auth/jwt.js (new)\n- src/middleware/auth.js (new)"
}
```

**What Agent does:**
1. Polls every 1s for outgoing.json
2. Sends to WhatsApp
3. Deletes file

### 3. Question (Claude → User)

**File:** `/tmp/codebutler/question.json`

```json
{
  "type": "question",
  "timestamp": "2025-02-09T20:02:00Z",
  "question_id": "q_xyz789",
  "chat_jid": "120363123456789012@g.us",
  "text": "Which database ORM?",
  "options": ["Sequelize", "Prisma", "Mongoose"],
  "timeout": 30
}
```

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

## 🔧 Commands for Claude

### Run CodeButler

```bash
./butler.sh
```

- First time: Opens wizard → scan QR → configure → starts agent
- Already configured: Opens dashboard → shows status
- Always shows web UI at `http://localhost:3000`

### Read Setup Status

After first-time setup completes:

```bash
cat /tmp/codebutler/setup-status.json
```

Returns:
```json
{
  "success": true,
  "user": {"jid": "...", "name": "Leandro"},
  "group": {"jid": "...", "name": "CodeButler Developer"},
  "voice_enabled": true
}
```

### Send Response to WhatsApp

```bash
cat > /tmp/codebutler/outgoing.json <<EOF
{
  "type": "response",
  "chat_jid": "120363405395407771@g.us",
  "content": "✅ Task completed!"
}
EOF
```

### Ask Question

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

# Wait for answer
while [ ! -f /tmp/codebutler/answer.json ]; do sleep 1; done

# Read answer
ANSWER=$(cat /tmp/codebutler/answer.json | jq -r '.text')
echo "User chose: $ANSWER"

# Clean up
rm /tmp/codebutler/answer.json
```

## 📝 Claude's Workflow

### On project open:

```bash
# Check if configured
if [ ! -f config.json ]; then
  echo "👋 Welcome to CodeButler! Running setup..."
  ./butler.sh
  # → Opens browser
  # → User completes setup
  # → Returns when done

  # Read status
  cat /tmp/codebutler/setup-status.json

  echo "✅ Setup complete! Agent is running."
else
  echo "👋 CodeButler already configured."
  echo "   Web UI: http://localhost:3000"

  # Optionally start if not running
  ./butler.sh &
fi
```

### When incoming message arrives:

```bash
# Check for incoming
if [ -f /tmp/codebutler/incoming.json ]; then
  MSG=$(cat /tmp/codebutler/incoming.json)
  CONTENT=$(echo $MSG | jq -r '.content')
  CHAT=$(echo $MSG | jq -r '.chat.jid')

  echo "📨 WhatsApp: $CONTENT"

  # Process task
  # ... do work ...

  # Send response
  cat > /tmp/codebutler/outgoing.json <<EOF
{
  "type": "response",
  "chat_jid": "$CHAT",
  "content": "✅ Done!"
}
EOF

  # Clean up
  rm /tmp/codebutler/incoming.json
fi
```

## 🗂️ Project Structure

```
CodeButler/
├── CLAUDE.md                    # This file
├── butler.sh                    # Build & run script
│
├── ButlerAgent/                 # Go source
│   ├── cmd/codebutler/          # Unified binary (setup + agent + web UI)
│   │   ├── main.go
│   │   └── templates/
│   │       ├── setup.html       # Setup wizard UI
│   │       └── dashboard.html   # Dashboard UI
│   └── internal/
│       ├── whatsapp/            # WhatsApp client
│       ├── protocol/            # JSON protocol
│       ├── config/              # Config management
│       ├── access/              # Access control
│       └── audio/               # Voice transcription
│
├── config.json                  # Runtime config (gitignored)
├── whatsapp-session/            # WhatsApp session (gitignored)
├── Sources/                     # User's repos (gitignored)
└── /tmp/codebutler/            # JSON protocol files
```

## 🌐 Web UI Features

### Setup Mode (no config.json)

1. **Step 1: QR Code**
   - Shows QR via WebSocket
   - User scans with WhatsApp

2. **Step 2: Configure**
   - Group name
   - Bot prefix
   - Sources directory
   - OpenAI API key (optional)

3. **Step 3: Complete**
   - Shows success
   - Auto-starts agent

### Dashboard Mode (config.json exists)

- Shows agent status (running/stopped)
- Displays current config
- Edit config inline
- Start/Stop agent buttons
- Shows protocol info

## 🚫 What Claude Should NOT Do

- ❌ Don't parse stdout/logs from agent
- ❌ Don't guess what happened
- ❌ Don't use old scripts (they don't exist anymore)
- ✅ Always use JSON protocol
- ✅ Trust the web UI for setup
- ✅ Read setup-status.json after setup

## 🎯 Example: Full Workflow

```bash
# User clones repo
git clone github.com:leandrotocalini/CodeButler.git
cd CodeButler

# User (or Claude) runs setup
./butler.sh
# → Browser opens at http://localhost:3000
# → Shows setup wizard (no config.json)
# → User scans QR
# → User fills form
# → Setup completes
# → Writes /tmp/codebutler/setup-status.json
# → Agent starts automatically
# → Dashboard now shows

# Read setup result
cat /tmp/codebutler/setup-status.json
# {
#   "success": true,
#   "user": {"jid": "...", "name": "Leandro"},
#   "group": {"jid": "...", "name": "CodeButler Developer"}
# }

echo "✅ CodeButler is running!"
echo "   Web UI: http://localhost:3000"

# --- Later: WhatsApp message arrives ---

# Agent writes incoming.json
cat /tmp/codebutler/incoming.json
# {
#   "content": "add JWT authentication",
#   "chat": {"jid": "120363...@g.us"}
# }

echo "📨 Task: add JWT authentication"

# Process task...

# Write response
cat > /tmp/codebutler/outgoing.json <<'EOF'
{
  "type": "response",
  "chat_jid": "120363405395407771@g.us",
  "content": "✅ JWT added!\n- src/auth/jwt.js\n- src/middleware/auth.js"
}
EOF

rm /tmp/codebutler/incoming.json

echo "✅ Response sent to WhatsApp"
```

---

**One binary. One UI. JSON protocol. No magic.** 🎯
