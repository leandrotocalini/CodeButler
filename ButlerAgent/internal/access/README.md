# Access Package

Controls which messages are processed.

## Usage

```go
if access.IsAllowed(msg, cfg) {
    // Process message
}
```

Only messages from the configured WhatsApp group (`config.WhatsApp.GroupJID`) are allowed. Everything else is blocked.
