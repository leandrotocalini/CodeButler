# Setup Package

This package provides an interactive wizard for first-time CodeButler setup.

## Features

- Interactive command-line wizard
- Configures all CodeButler settings
- Auto-detects WhatsApp group
- Generates complete config.json
- User-friendly prompts with defaults

## Usage

The setup package provides two main flows:

### 1. First-time Setup (No config.json)

The wizard is automatically invoked by `cmd/codebutler/main.go` when no config.json exists.

```go
package main

import (
    "github.com/leandrotocalini/CodeButler/internal/setup"
)

func main() {
    // Check if config exists
    if !configExists() {
        // Run wizard
        wizardCfg, _ := setup.RunWizard()

        // Create group
        setup.CreateGroupIfNeeded(wizardCfg.GroupName)

        // Connect to WhatsApp and get group JID
        // ... connect ...

        // Save config
        setup.SaveConfig(wizardCfg, groupJID, userJID)
    }

    // Continue with normal startup
}
```

### 2. Config Validation (Existing config.json)

When config.json exists but has incomplete settings, validation prompts to fix them.

```go
package main

import (
    "github.com/leandrotocalini/CodeButler/internal/setup"
    "github.com/leandrotocalini/CodeButler/internal/config"
)

func main() {
    // Load existing config
    cfg, _ := config.Load("config.json")

    // Validate and fix if needed
    updated, _ := setup.ValidateAndFixConfig(cfg)

    if updated {
        // Save updated config
        config.Save(cfg, "config.json")
    }

    // Continue with normal startup
}
```

## Config Validation Flow

When starting CodeButler with an existing config.json, the system checks for incomplete settings:

### 1. Missing or Invalid API Key

```
⚠️  Voice Transcription Not Configured
   You don't have a valid OpenAI API key for Whisper transcription.
   Voice messages will be ignored.

   Do you want to enable voice transcription now? (yes/no) [no]: yes

   📝 Enter your OpenAI API key:
   API Key: sk-proj-...

   ✅ API key saved

💾 Saving updated configuration...
✅ Configuration updated
```

**Triggers when**:
- `openai.apiKey` is empty (`""`)
- `openai.apiKey` is test key (`"sk-test-key-for-phase-testing"`)

### 2. Missing OAuth Token

```
💡 Tip: Claude OAuth Token
   You can set CLAUDE_CODE_OAUTH_TOKEN environment variable
   or add it to config.json for convenience.

   Do you want to add it to config.json now? (yes/no) [no]: yes

   OAuth Token: sk-ant-...

   ✅ OAuth token saved

💾 Saving updated configuration...
✅ Configuration updated
```

**Triggers when**:
- `claudeCode.oauthToken` is empty (`""`)

If user says "no" to any prompt, the setting remains unchanged and CodeButler starts normally.

## Wizard Flow

### 1. Voice Transcription```
📢 Voice Message Transcription
   Do you want to enable voice message transcription with OpenAI Whisper?
   Enable Whisper? (yes/no) [no]:
```

- Default: **no** (disabled)
- If **yes**, asks for OpenAI API key
- If **no**, voice messages ignored

### 2. Group Name

```
📱 WhatsApp Group Configuration
   CodeButler listens to commands from a single WhatsApp group.
   Group name [CodeButler Developer]:
```

- Default: **CodeButler Developer**
- Can enter custom name
- Press Enter for default

### 3. Sources Path

```
📂 Repositories Configuration
   Where are your code repositories located?
   Sources path [./Sources]:
```

- Default: **./Sources**
- Can use absolute path: `/Users/you/Projects`
- Can use home shortcut: `~/Projects`

### 4. Claude OAuth Token

```
🔑 Claude Code OAuth Token
   You can:
   1. Enter token now (will be saved to config.json)
   2. Use environment variable CLAUDE_CODE_OAUTH_TOKEN (leave empty)
   OAuth token (or press Enter to use env):
```

Two options:
1. Enter token → Saved to config.json
2. Press Enter → Uses environment variable

### 5. Summary

```
📋 Setup Summary
================
   Voice transcription: true
   WhatsApp group: My Dev Team
   Sources path: ~/Projects
   Claude OAuth: from environment
```

Shows all configured values before proceeding.

### 6. Group Creation Prompt

```
📱 WhatsApp Group Setup
======================
   Please create a WhatsApp group named: "My Dev Team"

   Steps:
   1. Open WhatsApp on your phone
   2. Create a new group
   3. Name it: My Dev Team
   4. Add yourself only (or trusted team members)

   Press Enter when you've created the group...
```

Waits for user to create group in WhatsApp.

## WizardConfig Structure

```go
type WizardConfig struct {
    UseWhisper      bool   // Enable voice transcription
    WhisperAPIKey   string // OpenAI API key (if Whisper enabled)
    GroupName       string // WhatsApp group name
    SourcesPath     string // Path to repositories
    ClaudeOAuthToken string // OAuth token (empty = use env)
}
```

## Functions

### RunWizard()

Runs the interactive wizard and returns configuration.

```go
cfg, err := setup.RunWizard()
if err != nil {
    // Handle error
}
```

**Returns**: `*WizardConfig`, `error`

### ValidateAndFixConfig(cfg)

Validates existing configuration and prompts to fix issues.

```go
updated, err := setup.ValidateAndFixConfig(cfg)
if err != nil {
    // Handle error
}

if updated {
    // Save config
    config.Save(cfg, "config.json")
}
```

**Checks**:
- OpenAI API key (empty or test key) → Offers to add real key
- Claude OAuth token (empty) → Offers to add token

**Returns**: `bool` (whether config was updated), `error`

### CreateGroupIfNeeded(groupName)

Displays instructions for creating the WhatsApp group.

```go
err := setup.CreateGroupIfNeeded("My Dev Team")
if err != nil {
    // Handle error
}
```

Waits for user to press Enter after creating group.

### SaveConfig(wizardCfg, groupJID, personalNumber)

Saves the wizard configuration to config.json.

```go
err := setup.SaveConfig(wizardCfg, groupJID, userJID)
if err != nil {
    // Handle error
}
```

**Parameters**:
- `wizardCfg`: Configuration from wizard
- `groupJID`: WhatsApp group JID (e.g., "120363...@g.us")
- `personalNumber`: User's WhatsApp JID (e.g., "549...@s.whatsapp.net")

## Generated config.json

### With Whisper
```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "personalNumber": "5491134567890@s.whatsapp.net",
    "groupJID": "120363123456789012@g.us",
    "groupName": "My Dev Team"
  },
  "openai": {
    "apiKey": "sk-..."
  },
  "claudeCode": {
    "oauthToken": ""
  },
  "sources": {
    "rootPath": "~/Projects"
  }
}
```

### Without Whisper
```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "personalNumber": "5491134567890@s.whatsapp.net",
    "groupJID": "120363123456789012@g.us",
    "groupName": "CodeButler Developer"
  },
  "openai": {
    "apiKey": ""
  },
  "claudeCode": {
    "oauthToken": ""
  },
  "sources": {
    "rootPath": "./Sources"
  }
}
```

## Input Validation

- **Empty inputs**: Use defaults
- **Whitespace**: Trimmed automatically
- **Case**: Whisper yes/no is case-insensitive
- **Paths**: Accepted as-is (validated later)

## Error Handling

Wizard errors are rare:
- File I/O errors during save
- User Ctrl+C (interrupts gracefully)

All errors are returned to caller for handling.

## Design Decisions

### 1. Defaults for Everything
Every question has a sensible default:
- Whisper: **disabled** (no API key needed)
- Group: **CodeButler Developer** (clear purpose)
- Sources: **./Sources** (standard location)
- OAuth: **environment variable** (more secure)

### 2. No Validation During Input
- Don't validate API keys (slow, unnecessary)
- Don't validate paths (user knows best)
- Don't validate tokens (checked at runtime)
- Trust user, fail gracefully later

### 3. Group Creation Outside Wizard
- Can't create group programmatically
- User must do it manually
- Clear instructions provided
- Wait for confirmation

### 4. Summary Before Proceeding
- Shows all choices
- User can verify
- No confirmation prompt (assumes correct)
- If wrong, can re-run wizard

## User Experience Goals

1. **Fast**: < 2 minutes to complete
2. **Clear**: Obvious what each question means
3. **Safe**: Defaults work for most users
4. **Flexible**: Can customize everything

## Integration Example

Full integration in `cmd/codebutler/main.go`:

```go
func main() {
    // Check for existing config
    _, err := os.Stat("config.json")
    if os.IsNotExist(err) {
        // Run wizard
        wizardCfg, _ := setup.RunWizard()

        // Prompt for group creation
        setup.CreateGroupIfNeeded(wizardCfg.GroupName)

        // Connect to WhatsApp
        client, _ := whatsapp.Connect("./whatsapp-session")
        info, _ := client.GetInfo()
        groups, _ := client.GetGroups()

        // Find group
        var groupJID string
        for _, g := range groups {
            if g.Name == wizardCfg.GroupName {
                groupJID = g.JID
                break
            }
        }

        // Save config
        setup.SaveConfig(wizardCfg, groupJID, info.JID)

        client.Disconnect()
    }

    // Normal startup continues...
}
```

## Future Enhancements

- Validate API keys during setup
- Check Claude CLI installation
- Create Sources directory automatically
- Offer to generate CLAUDE.md template
- Multi-step wizard with back/forward navigation
- Config file preview before saving
