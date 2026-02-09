# Config Package

This package handles all configuration for CodeButler.

## Files

### types.go
Defines all configuration structs:
- **Config**: Main configuration container
- **WhatsAppConfig**: WhatsApp session and group settings
- **OpenAIConfig**: OpenAI API credentials (for Whisper transcription)
- **ClaudeConfig**: Claude Code SDK OAuth token
- **SourcesConfig**: Root path for managed repositories

### load.go
- **Load()**: Reads and validates config.json
- **validate()**: Ensures all required fields are present
- Loads Claude OAuth token from `CLAUDE_CODE_OAUTH_TOKEN` env variable if not in config

### save.go
- **Save()**: Writes config to JSON file with 0600 permissions (secure)
- Uses indented JSON for human readability

## Configuration File

Create `config.json` in the project root:

```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "personalNumber": "1234567890@s.whatsapp.net",
    "groupJID": "120363123456789012@g.us",
    "groupName": "CodeButler Developer"
  },
  "openai": {
    "apiKey": "sk-..."
  },
  "claudeCode": {
    "oauthToken": ""
  },
  "sources": {
    "rootPath": "./Sources"
  }
}
```

## Usage Example

```go
package main

import (
    "fmt"
    "github.com/leandrotocalini/CodeButler/internal/config"
)

func main() {
    // Load configuration
    cfg, err := config.Load("config.json")
    if err != nil {
        panic(err)
    }

    fmt.Printf("WhatsApp Group: %s\n", cfg.WhatsApp.GroupName)
    fmt.Printf("Sources Path: %s\n", cfg.Sources.RootPath)

    // Modify and save
    cfg.WhatsApp.GroupName = "New Name"
    if err := config.Save(cfg, "config.json"); err != nil {
        panic(err)
    }
}
```

## Security Notes

- `config.json` is in `.gitignore` - never commit it!
- File permissions are 0600 (owner read/write only)
- Claude OAuth token can be loaded from environment variable
- Keep your OpenAI API key secret

## Required Fields

All fields are required except:
- `claudeCode.oauthToken` - can be loaded from env variable

Missing required fields will cause Load() to fail with a descriptive error.

## Environment Variables

- `CLAUDE_CODE_OAUTH_TOKEN`: Claude Code SDK OAuth token (optional if in config)
