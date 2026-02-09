# Self-Healing Config - Auto-Detect Missing Fields ✅

## What Changed

Config validation now **automatically detects missing fields** and prompts the user to configure them. No need to manually edit config.json when new features are added!

## The Problem

When you add a new feature that requires a config field, existing users have old config.json files without that field:

```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "groupJID": "...",
    "groupName": "CodeButler Developer"
    // botPrefix is MISSING!
  }
}
```

Before, the app would:
- ❌ Crash with "undefined field"
- ❌ Use empty string silently
- ❌ Require manual config edit

## The Solution

Now, the app **detects missing fields** and prompts interactively:

```
🤖 CodeButler - WhatsApp Agent for Claude Code SDK

⚠️  Bot Prefix Not Configured
   Bot messages need a prefix to avoid processing its own messages.

   Bot prefix [[BOT]]: [user presses Enter]

   ✅ Using default: [BOT]

💾 Saving updated configuration...
✅ Configuration updated

📝 Configuration loaded
   Group: CodeButler Developer
   Sources: ./Sources
   Voice: Enabled (Whisper)
   Bot Prefix: [BOT]

✅ Connected to WhatsApp
👂 Listening for messages...
```

**Config.json is automatically updated** with the new field!

## How It Works

### ValidateAndFixConfig() - The Magic Function

```go
func ValidateAndFixConfig(cfg *config.Config) (bool, error) {
    updated := false

    // Check bot prefix
    if cfg.WhatsApp.BotPrefix == "" {
        fmt.Println("⚠️  Bot Prefix Not Configured")
        fmt.Print("   Bot prefix [[BOT]]: ")

        response, _ := reader.ReadString('\n')
        if response == "" {
            cfg.WhatsApp.BotPrefix = "[BOT]"
        } else {
            cfg.WhatsApp.BotPrefix = strings.TrimSpace(response)
        }
        updated = true
    }

    // Check OpenAI API key
    if cfg.OpenAI.APIKey == "" {
        // Prompt for API key...
        updated = true
    }

    return updated, nil
}
```

### Main Startup Flow

```go
// Load config
cfg, _ := config.Load("config.json")

// Validate and fix missing fields
configUpdated, _ := setup.ValidateAndFixConfig(cfg)

if configUpdated {
    // Save updated config
    config.Save(cfg, "config.json")
    fmt.Println("✅ Configuration updated")
}

// Continue startup...
```

## Example: Old Config

```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "personalNumber": "5493764705749:31@s.whatsapp.net",
    "groupJID": "120363405395407771@g.us",
    "groupName": "CodeButler Developer"
  },
  "openai": {
    "apiKey": "sk-proj-..."
  },
  "sources": {
    "rootPath": "./Sources"
  }
}
```

**Missing**: `botPrefix`

### What Happens

```
$ ./codebutler

⚠️  Bot Prefix Not Configured
   Bot messages need a prefix to avoid processing its own messages.

   Bot prefix [[BOT]]:

   ✅ Using default: [BOT]

💾 Saving updated configuration...
✅ Configuration updated
```

### After Validation

```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "personalNumber": "5493764705749:31@s.whatsapp.net",
    "groupJID": "120363405395407771@g.us",
    "groupName": "CodeButler Developer",
    "botPrefix": "[BOT]"  ← ADDED AUTOMATICALLY!
  },
  "openai": {
    "apiKey": "sk-proj-..."
  },
  "sources": {
    "rootPath": "./Sources"
  }
}
```

## Fields Currently Validated

### 1. Bot Prefix (`whatsApp.botPrefix`)

**Check**: Empty string
**Prompt**:
```
⚠️  Bot Prefix Not Configured
   Bot prefix [[BOT]]:
```

**Default**: `[BOT]`
**Required**: Yes (for self-identification)

### 2. OpenAI API Key (`openai.apiKey`)

**Check**: Empty or test key
**Prompt**:
```
⚠️  Voice Transcription Not Configured
   Do you want to enable voice transcription now? (yes/no) [no]:
```

**Default**: Empty (voice disabled)
**Required**: No (optional feature)

## Adding New Fields

To add a new validated field in the future:

```go
func ValidateAndFixConfig(cfg *config.Config) (bool, error) {
    updated := false

    // Existing validations...

    // NEW: Check for new field
    if cfg.NewFeature.Setting == "" {
        fmt.Println("⚠️  New Feature Not Configured")
        fmt.Print("   Setting [[default]]: ")

        response, _ := reader.ReadString('\n')
        if response == "" {
            cfg.NewFeature.Setting = "default"
        } else {
            cfg.NewFeature.Setting = strings.TrimSpace(response)
        }
        updated = true
    }

    return updated, nil
}
```

**That's it!** Users with old configs will be prompted automatically.

## Benefits

### 1. Seamless Updates
- Add new features without breaking old configs
- Users guided through migration
- No manual editing needed

### 2. Progressive Enhancement
- New fields added one at a time
- Each validated individually
- Clear prompts for each missing field

### 3. Self-Documenting
- Prompts explain what each field does
- Default values shown
- Help text included

### 4. Safe Defaults
- Always provides sensible defaults
- Optional fields can be skipped
- Required fields must be configured

### 5. Auto-Save
- Config automatically saved after validation
- No risk of losing changes
- Immediate effect

## User Experience

### First Run (No Config)
```
📋 No config.json found. Starting setup wizard...
[Full wizard with all questions]
```

### Existing Config (Missing Field)
```
⚠️  Bot Prefix Not Configured
   Bot prefix [[BOT]]: [BOT-CUSTOM]
   ✅ Using: [BOT-CUSTOM]

💾 Saving updated configuration...
✅ Configuration updated
```

### Complete Config (No Issues)
```
📝 Configuration loaded
✅ Connected to WhatsApp
```

## Console Output Examples

### Missing Bot Prefix

```
$ ./codebutler

🤖 CodeButler - WhatsApp Agent for Claude Code SDK

⚠️  Bot Prefix Not Configured
   Bot messages need a prefix to avoid processing its own messages.

   Bot prefix [[BOT]]:

   ✅ Using default: [BOT]

💾 Saving updated configuration...
✅ Configuration updated

📝 Configuration loaded
   Group: CodeButler Developer
   Sources: ./Sources
   Voice: Enabled (Whisper)

📱 Connecting to WhatsApp...
✅ Connected to WhatsApp
👂 Listening for messages...
```

### Missing API Key

```
$ ./codebutler

🤖 CodeButler - WhatsApp Agent for Claude Code SDK

⚠️  Voice Transcription Not Configured
   You don't have a valid OpenAI API key for Whisper transcription.
   Voice messages will be ignored.

   Do you want to enable voice transcription now? (yes/no) [no]: yes

   📝 Enter your OpenAI API key:
   API Key: sk-proj-xxxxx

   ✅ API key saved

💾 Saving updated configuration...
✅ Configuration updated

📝 Configuration loaded
   Voice: Enabled (Whisper)

✅ Connected to WhatsApp
👂 Listening for messages...
```

### Multiple Missing Fields

```
$ ./codebutler

⚠️  Bot Prefix Not Configured
   Bot prefix [[BOT]]: [🤖]
   ✅ Using: [🤖]

⚠️  Voice Transcription Not Configured
   Do you want to enable voice transcription now? (yes/no) [no]: no
   ⏭️  Skipping voice transcription

💾 Saving updated configuration...
✅ Configuration updated

[continues startup...]
```

## Testing

To test with an old config:

```bash
# Remove botPrefix from config.json
{
  "whatsapp": {
    "groupName": "CodeButler Developer"
    // Remove "botPrefix" line
  }
}

# Run codebutler
./codebutler

# Should prompt for botPrefix
# Config automatically updated
```

## Error Handling

### User Cancels (Ctrl+C)
App exits gracefully, config not modified.

### Empty Input
Uses default value, continues.

### Invalid Input
Accepted as-is (user choice), can be changed later by deleting field and rerunning.

## Migration Path

### Version 1.0 → 1.1 (Added botPrefix)
- Old configs: Missing `botPrefix`
- On startup: Prompts for `botPrefix`
- After: Config updated, app continues

### Version 1.1 → 1.2 (Added newFeature)
- Old configs: Missing `newFeature`
- On startup: Prompts for `newFeature`
- After: Config updated, app continues

**No breaking changes, ever!**

## Files Modified

- ✅ `internal/setup/validate.go` - Added botPrefix validation
- ✅ `cmd/codebutler/main.go` - Already calls ValidateAndFixConfig
- ✅ Documentation: SELF_HEALING_CONFIG.md (this file)

## Future Enhancements

- Detect deprecated fields (warn about removal)
- Show config diff before saving
- Backup old config before updating
- Validate field types (string, int, bool)
- Validate field values (regex, ranges)

## Conclusion

**Config validation is now extensible and self-healing!**

- ✅ Old configs work seamlessly
- ✅ Missing fields auto-detected
- ✅ Interactive prompts guide user
- ✅ Config automatically saved
- ✅ No breaking changes

**Adding new features is now safe and user-friendly!** 🎉

When you add a new config field:
1. Add it to `types.go`
2. Add validation in `validate.go`
3. Users are automatically prompted on next run
4. Done!
