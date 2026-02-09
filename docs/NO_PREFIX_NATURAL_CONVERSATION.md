# No More @codebutler Prefix - Natural Conversation! ✅

## What Changed

CodeButler now responds to **plain commands** without requiring `@codebutler` prefix. Just type `repos` instead of `@codebutler repos`.

Bot messages are prefixed with `[BOT]` (configurable) to avoid processing its own messages.

## Before

```
WhatsApp:
@codebutler repos
@codebutler use aurum
@codebutler run add tests
```

**Problem**: Tedious to type `@codebutler` every time.

## After

```
WhatsApp:
repos
use aurum
run add tests
```

**Bot responses:**
```
[BOT] 📂 Found 1 repositor(y/ies):
1. *aurum* ✅
...
```

**Much more natural!**

## How It Works

### 1. Command Parser Updated

```go
// Old: Required @codebutler prefix
func Parse(text string) *Command {
    if !strings.HasPrefix(text, "@codebutler") {
        return nil
    }
    // ...
}

// New: Optional prefix (supports both)
func Parse(text string) *Command {
    text = strings.TrimPrefix(text, "@codebutler") // Remove if present
    text = strings.TrimSpace(text)
    // Parse command...
}
```

Now accepts:
- ✅ `repos` (without prefix)
- ✅ `@codebutler repos` (with prefix, backwards compatible)

### 2. Bot Adds Prefix to All Messages

```go
botPrefix := cfg.WhatsApp.BotPrefix  // Default: "[BOT]"

sendWithPrefix := func(chatID, text string) error {
    return client.SendMessage(chatID, botPrefix+" "+text)
}
```

All bot responses start with `[BOT]` (or configured prefix).

### 3. Ignore Bot's Own Messages

```go
// Ignore bot's own messages
if strings.HasPrefix(msg.Content, botPrefix) {
    fmt.Printf("   🤖 Bot message (%s) - ignoring\n", botPrefix)
    return
}
```

Bot doesn't process messages starting with its own prefix.

### 4. Process All User Messages

```go
// Try to parse as command
cmd := commands.Parse(msg.Content)
if cmd != nil {
    // Execute command
    response := codebutler.HandleCommand(msg.Chat, msg.Content)
    sendWithPrefix(msg.Chat, response)
} else {
    // Not a command, ignore
    fmt.Println("   💬 Not a command - ignoring")
}
```

Every user message is checked for commands. If not a command, silently ignored.

## Configurable Bot Prefix

The bot prefix is **configurable** in `config.json`:

```json
{
  "whatsapp": {
    "botPrefix": "[BOT]"
  }
}
```

### Default: `[BOT]`

```
User: repos
Bot:  [BOT] 📂 Found 1 repositor(y/ies)...
```

### Custom Example: `[🤖]`

```json
{
  "whatsapp": {
    "botPrefix": "[🤖]"
  }
}
```

```
User: repos
Bot:  [🤖] 📂 Found 1 repositor(y/ies)...
```

### Custom Example: `CodeButler:`

```json
{
  "whatsapp": {
    "botPrefix": "CodeButler:"
  }
}
```

```
User: repos
Bot:  CodeButler: 📂 Found 1 repositor(y/ies)...
```

## Setup Wizard Updated

New question during setup:

```
🤖 Bot Message Prefix
   Bot messages will start with this prefix to avoid processing its own messages.
   Bot prefix [[BOT]]:
```

- Press Enter → Uses `[BOT]`
- Type custom → Uses your prefix

Summary shows chosen prefix:

```
📋 Setup Summary
================
   Voice transcription: true
   WhatsApp group: CodeButler Developer
   Bot prefix: [BOT]
   Sources path: ./Sources
```

## Example Conversation

```
User: repos

[BOT] 📂 Found 2 repositor(y/ies):

1. *aurum* ✅
2. *experiment* ❌

✅ Claude-ready: 1/2

💡 Use: use <repo-name>

---

User: use aurum

[BOT] ✅ Now using: *aurum*

💡 Run commands with: run <prompt>

---

User: status

[BOT] 📍 Active: *aurum*
📂 Path: ./Sources/aurum

---

User: run list files

[BOT] 🤖 Executing in *aurum*...
⏳ This may take a few minutes...

[... 2 minutes later ...]

[BOT] ✅ Execution completed in *aurum*
⏱️  Duration: 127.3s

📤 Output:
```
Found 23 files in src/
- main.go
- config.go
...
```
```

## Files Modified

- ✅ `internal/config/types.go` - Added `BotPrefix` field
- ✅ `internal/commands/parser.go` - Made @codebutler prefix optional
- ✅ `internal/setup/wizard.go` - Added bot prefix prompt
- ✅ `cmd/codebutler/main.go` - Process all messages, add prefix to responses
- ✅ `config.json` - Added `"botPrefix": "[BOT]"`
- ✅ `config.sample.json` - Added `"botPrefix": "[BOT]"`

## Backwards Compatibility

✅ Old commands still work:
```
@codebutler repos  ← Still works
repos              ← Also works
```

The parser strips `@codebutler` if present, so both formats work.

## Benefits

### 1. Natural Conversation
- No more typing `@codebutler`
- Feels like chatting with a person
- Faster to type commands

### 2. Customizable Identity
- Change prefix to anything
- Match your team's style
- Can use emojis: `[🤖]`, `[👨‍💻]`, etc.

### 3. Self-Awareness
- Bot knows its own messages
- No infinite loops
- Clean message log

### 4. Flexible
- Still supports `@codebutler` prefix
- Works with voice transcription
- Ignores non-command messages

## Console Output

### Command Message
```
📨 Message received:
   From: 5493764705749:31@s.whatsapp.net
   Chat: 120363405395407771@g.us
   Content: repos
   IsGroup: true
   IsFromMe: false
   ⭐ From authorized group!
   🤖 Command detected
   📤 Sending response...
   ✅ Response sent
```

### Bot Message (Ignored)
```
📨 Message received:
   From: 5493764705749:31@s.whatsapp.net
   Chat: 120363405395407771@g.us
   Content: [BOT] 📂 Found 1 repositor(y/ies)...
   IsGroup: true
   IsFromMe: true
   🤖 Bot message ([BOT]) - ignoring
```

### Not a Command
```
📨 Message received:
   From: 5493764705749:31@s.whatsapp.net
   Chat: 120363405395407771@g.us
   Content: hola como estas?
   IsGroup: true
   IsFromMe: false
   ⭐ From authorized group!
   💬 Not a command - ignoring
```

## Voice Messages

Still work perfectly:

```
User: [voice message]: "repos"

🎤 Processing voice message...
   ✅ Audio downloaded: /tmp/codebutler-audio-1770667595.ogg
   🔄 Transcribing with Whisper API...
   ✅ Transcription: "repos"
   🤖 Command detected
   📤 Sending response...
   ✅ Response sent

[BOT] 📂 Found 1 repositor(y/ies)...
```

## Testing

To test:

```bash
# Build
go build -o codebutler ./cmd/codebutler/

# Run
./codebutler

# In WhatsApp group, try:
repos
use aurum
status
run list files
ping

# All should work without @codebutler prefix
```

## Migration

If you have existing config.json:

```bash
# Add botPrefix manually
{
  "whatsapp": {
    ...
    "botPrefix": "[BOT]"  ← Add this
  }
}

# Or let validation add it automatically
# (Config validator can be updated to add missing fields)
```

## Future Enhancements

- Auto-detect bot prefix from first bot message
- Support multiple prefixes
- Regex-based command detection
- Context-aware responses (remember previous messages)

## Conclusion

**Mucho más natural y fácil de usar!** 🎉

- ✅ No more `@codebutler` prefix needed
- ✅ Bot messages clearly marked with `[BOT]`
- ✅ Configurable prefix
- ✅ Backwards compatible
- ✅ Natural conversation flow

**Just type the command and the bot responds!**
