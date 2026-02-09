# Phase 7: Config Validation - UPDATE ✅

## What Was Added

Enhanced Phase 7 with automatic config validation on startup.

## New Feature: ValidateAndFixConfig()

When CodeButler starts with an existing config.json, it now checks for incomplete settings and offers to fix them interactively.

### Checks Performed

#### 1. OpenAI API Key Validation

**Triggers when**:
- `openai.apiKey` is empty (`""`)
- `openai.apiKey` is test key (`"sk-test-key-for-phase-testing"`)

**Prompt**:
```
⚠️  Voice Transcription Not Configured
   You don't have a valid OpenAI API key for Whisper transcription.
   Voice messages will be ignored.

   Do you want to enable voice transcription now? (yes/no) [no]:
```

If user says **yes**:
- Asks for real OpenAI API key
- Updates config.json automatically
- Voice transcription enabled

If user says **no**:
- Voice messages ignored
- Can enable later by running again

#### 2. Claude OAuth Token Validation

**Triggers when**:
- `claudeCode.oauthToken` is empty (`""`)

**Prompt**:
```
💡 Tip: Claude OAuth Token
   You can set CLAUDE_CODE_OAUTH_TOKEN environment variable
   or add it to config.json for convenience.

   Do you want to add it to config.json now? (yes/no) [no]:
```

If user says **yes**:
- Asks for OAuth token
- Updates config.json
- Token available for Claude Code execution

If user says **no**:
- Uses environment variable if set
- Falls back to OAuth flow if needed

## User Experience

### Before

```bash
$ ./codebutler

📝 Configuration loaded
   Group: CodeButler Developer
   Sources: ./Sources
   Voice: Disabled

# Voice messages silently ignored
# No indication why or how to fix
```

### After

```bash
$ ./codebutler

⚠️  Voice Transcription Not Configured
   You don't have a valid OpenAI API key for Whisper transcription.
   Voice messages will be ignored.

   Do you want to enable voice transcription now? (yes/no) [no]: yes

   📝 Enter your OpenAI API key:
   API Key: sk-proj-xxxxxxxxxxxxx

   ✅ API key saved

💾 Saving updated configuration...
✅ Configuration updated

📝 Configuration loaded
   Group: CodeButler Developer
   Sources: ./Sources
   Voice: Enabled (Whisper)

✅ Connected to WhatsApp
👂 Listening for messages...

# Voice messages now work!
```

## Implementation

### New File: `internal/setup/validate.go`

```go
func ValidateAndFixConfig(cfg *config.Config) (bool, error) {
    // Check OpenAI API key
    if cfg.OpenAI.APIKey == "" || cfg.OpenAI.APIKey == "sk-test-key-for-phase-testing" {
        // Prompt user to add key
    }

    // Check Claude OAuth token
    if cfg.Claude.OAuthToken == "" {
        // Prompt user to add token
    }

    return updated, nil
}
```

### Updated: `cmd/codebutler/main.go`

```go
// Load config
cfg, _ := config.Load("config.json")

// Validate and fix if needed
configUpdated, _ := setup.ValidateAndFixConfig(cfg)

if configUpdated {
    // Save updated config
    config.Save(cfg, "config.json")
}

// Continue startup...
```

## Files Modified

- ✅ `internal/setup/validate.go` (78 LOC) - New validation logic
- ✅ `cmd/codebutler/main.go` - Integrated validation
- ✅ `internal/setup/README.md` - Documentation updated

## Build

```bash
go build -o codebutler ./cmd/codebutler/
✅ SUCCESS
```

## Testing

### Test with invalid API key:

1. Edit config.json:
```json
{
  "openai": {
    "apiKey": "sk-test-key-for-phase-testing"
  },
  ...
}
```

2. Run:
```bash
./codebutler
```

3. Should prompt:
```
⚠️  Voice Transcription Not Configured
   Do you want to enable voice transcription now? (yes/no) [no]:
```

### Test with empty OAuth token:

1. Edit config.json:
```json
{
  "claudeCode": {
    "oauthToken": ""
  },
  ...
}
```

2. Run:
```bash
./codebutler
```

3. Should prompt:
```
💡 Tip: Claude OAuth Token
   Do you want to add it to config.json now? (yes/no) [no]:
```

## Benefits

### 1. Self-Healing Configuration
- Detects incomplete settings automatically
- Offers to fix interactively
- No manual config.json editing needed

### 2. Better User Experience
- Clear explanation of what's missing
- Offers solution immediately
- Can skip and continue if wanted

### 3. Backwards Compatible
- Works with old config.json files
- Doesn't break existing setups
- Only prompts when needed

### 4. Non-Blocking
- User can say "no" and continue
- Doesn't force configuration
- Can enable features later

## Edge Cases Handled

1. **Test API Key** - Detects `sk-test-key-for-phase-testing` and prompts
2. **Empty String** - Treats `""` as missing
3. **User Cancels** - Pressing Ctrl+C exits gracefully
4. **Invalid Input** - Empty responses skip that setting
5. **Multiple Issues** - Checks all settings sequentially

## Future Enhancements

Possible additions:
- Validate API key by testing with OpenAI
- Check Claude CLI installation
- Verify OAuth token validity
- Detect Sources directory missing
- Offer to create CLAUDE.md template

## Conclusion

Config validation makes CodeButler more user-friendly by:
- ✅ Detecting configuration issues automatically
- ✅ Offering to fix them interactively
- ✅ Explaining what's missing and why
- ✅ Allowing users to skip if wanted
- ✅ Saving changes automatically

**Phase 7 enhanced successfully!**
