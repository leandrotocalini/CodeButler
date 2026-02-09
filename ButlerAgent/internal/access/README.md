# Access Control Package

This package handles access control for CodeButler, ensuring only authorized messages are processed.

## Simplified Architecture

CodeButler uses a **single-group access control model**. Only messages from the configured "CodeButler Developer" group are allowed.

## Files

### control.go
- **IsAllowed()**: Validates if a message is from the authorized group

### control_test.go
- Comprehensive tests for access control logic

## Usage Example

```go
package main

import (
    "github.com/leandrotocalini/CodeButler/internal/access"
    "github.com/leandrotocalini/CodeButler/internal/config"
    "github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

func handleMessage(msg whatsapp.Message, cfg *config.Config) {
    // Check if message is allowed
    if !access.IsAllowed(msg, cfg) {
        log.Printf("⛔ Blocked message from: %s", msg.Chat)
        return
    }

    // Process the message
    log.Printf("✅ Processing message: %s", msg.Content)
    processCommand(msg)
}
```

## Access Rules

### ✅ Allowed
- Messages from the configured group JID in `config.json`
- Only the "CodeButler Developer" group

### ⛔ Blocked
- Messages from other groups
- Personal chat messages
- Any message when no group is configured

## Why Single-Group?

The original design considered multi-group support with per-group permissions, but we simplified to:

1. **Easier to understand**: One group = one control point
2. **Simpler code**: No arrays, no loops, just one comparison
3. **More secure**: Fewer configuration mistakes
4. **Easier setup**: Less config required
5. **Future-proof**: Can add more groups later if needed

## Configuration

In `config.json`:

```json
{
  "whatsapp": {
    "groupJID": "120363405395407771@g.us",
    "groupName": "CodeButler Developer"
  }
}
```

If `groupJID` is empty, **all messages are blocked**.

## Testing

Run tests:
```bash
go test ./internal/access/... -v
```

Tests cover:
- ✅ Message from correct group → Allowed
- ⛔ Message from different group → Blocked
- ⛔ Message from personal chat → Blocked
- ⛔ Message when no group configured → Blocked
- ✅ Multiple group JIDs tested against config

## Security Notes

- Access control is enforced **before any command processing**
- Empty or missing `groupJID` blocks everything (fail-safe)
- No wildcards, no patterns - exact JID match only
- Cannot be bypassed by spoofing (WhatsApp validates JIDs)

## Performance

- **O(1)** complexity: Single string comparison
- No database lookups
- No network calls
- Instant validation
