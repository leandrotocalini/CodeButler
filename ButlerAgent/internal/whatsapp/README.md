# WhatsApp Package

This package handles all WhatsApp communication for CodeButler using the `whatsmeow` library.

## Files

### client.go
- **Connect()**: Establishes WhatsApp connection with QR code authentication
- **Disconnect()**: Closes the connection
- **GetInfo()**: Returns account information (JID, name)
- **IsConnected()**: Checks connection status
- **GetJID()**: Returns the user's JID

### auth.go
- **displayQR()**: Displays QR code in terminal for pairing

### handler.go
- **Message struct**: Represents a WhatsApp message
- **OnMessage()**: Registers a callback for incoming messages
- **SendMessage()**: Sends a text message to a chat
- **DownloadAudio()**: Downloads voice messages to temp file

### groups.go
- **Group struct**: Represents a WhatsApp group
- **GetGroups()**: Returns all groups the user is in
- **CreateGroup()**: Creates a new group
- **GetGroupInfo()**: Gets detailed group information
- **AddParticipants()**: Adds members to a group
- **RemoveParticipants()**: Removes members from a group

## Usage Example

```go
package main

import (
    "fmt"
    "github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

func main() {
    // Connect to WhatsApp
    client, err := whatsapp.Connect("./whatsapp-session")
    if err != nil {
        panic(err)
    }
    defer client.Disconnect()

    // Get account info
    info, _ := client.GetInfo()
    fmt.Printf("Connected as: %s\n", info.JID)

    // Register message handler
    client.OnMessage(func(msg whatsapp.Message) {
        fmt.Printf("Message from %s: %s\n", msg.From, msg.Content)

        // Reply
        client.SendMessage(msg.Chat, "Hello!")
    })

    // Get all groups
    groups, _ := client.GetGroups()
    for _, group := range groups {
        fmt.Printf("Group: %s (%s)\n", group.Name, group.JID)
    }

    // Keep alive
    select {}
}
```

## Dependencies

- `go.mau.fi/whatsmeow` - Modern WhatsApp Web client
- `github.com/mattn/go-sqlite3` - SQLite for session storage
- `github.com/skip2/go-qrcode` - QR code generation

## Session Storage

Sessions are stored in SQLite database at `{sessionPath}/session.db`. This allows the client to reconnect without re-scanning the QR code.

## Notes

- QR codes expire after ~20 seconds, so scan quickly
- The client auto-reconnects if the connection drops
- Voice messages are downloaded to `/tmp/codebutler-audio-{timestamp}.ogg`
- Only processes messages that are not from ourselves (except in groups)
