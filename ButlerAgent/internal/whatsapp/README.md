# WhatsApp Package

Handles WhatsApp connection using `whatsmeow` library.

## Key Functions

- `Connect(sessionPath)` - Connect with existing session
- `ConnectWithQR(sessionPath)` - Connect with QR code channel
- `SendMessage(jid, text)` - Send text message
- `OnMessage(callback)` - Register message handler
- `GetGroups()` - List all groups
- `GetInfo()` - Get account info
- `IsConnected()` - Check connection status
- `DownloadAudioFromMessage(msg)` - Download voice message

## Session

Sessions stored in SQLite at `{sessionPath}/session.db`. Reconnects automatically without QR after first scan.
