# Audio Package

This package handles audio transcription for CodeButler using OpenAI's Whisper API.

## Files

### transcribe.go
- **TranscribeAudio()**: Transcribes audio files to text using Whisper API

## Features

- Downloads WhatsApp voice messages
- Transcribes audio to text using OpenAI Whisper
- Supports all formats Whisper accepts (ogg, mp3, wav, etc.)
- Returns plain text transcription

## Usage Example

```go
package main

import (
    "fmt"
    "github.com/leandrotocalini/CodeButler/internal/audio"
    "github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

func handleVoiceMessage(client *whatsapp.Client, evt *events.Message, apiKey string) {
    // Download voice message
    audioPath, err := client.DownloadAudio(evt)
    if err != nil {
        log.Printf("Failed to download audio: %v", err)
        return
    }
    defer os.Remove(audioPath) // Clean up

    // Transcribe
    text, err := audio.TranscribeAudio(audioPath, apiKey)
    if err != nil {
        log.Printf("Failed to transcribe: %v", err)
        return
    }

    fmt.Printf("Transcription: %s\n", text)

    // Process as text command
    processCommand(text)
}
```

## OpenAI Whisper API

### Configuration

Get your API key from: https://platform.openai.com/api-keys

Add to `config.json`:
```json
{
  "openai": {
    "apiKey": "sk-..."
  }
}
```

### Pricing (as of 2024)

- **$0.006 per minute** of audio
- Most voice messages are 5-30 seconds
- Very affordable for personal use

### Supported Audio Formats

- ogg (WhatsApp default)
- mp3
- mp4
- mpeg
- mpga
- m4a
- wav
- webm

### File Size Limits

- Max 25 MB per file
- Most voice messages are < 1 MB

## Error Handling

The function returns errors for:
- File not found or unreadable
- Network errors
- API errors (invalid key, rate limits, etc.)
- Invalid audio format
- Decoding errors

## Security Notes

- API key is sent via HTTPS
- Audio files are temporary and deleted after processing
- No audio data is stored permanently
- Transcriptions are not logged by default

## Performance

- Typical transcription time: 1-3 seconds
- Depends on audio length and API load
- Network latency matters
- Can process multiple audios concurrently

## Testing

For testing without spending API credits:
1. Use a short test audio file (< 5 seconds)
2. Set a small budget limit in OpenAI dashboard
3. Mock the API in unit tests (see test examples)

## Common Issues

### "API returned status 401"
- Invalid or expired API key
- Check your key at https://platform.openai.com/api-keys

### "API returned status 429"
- Rate limit exceeded
- Wait and retry
- Upgrade your OpenAI plan if needed

### "API returned status 400"
- Invalid audio file
- Check file format and size
- Ensure file is not corrupted

## Future Enhancements

Possible improvements:
- Retry logic with exponential backoff
- Audio format conversion
- Language detection
- Speaker diarization
- Timestamp support
