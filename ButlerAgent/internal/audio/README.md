# Audio Package

Transcribes voice messages using OpenAI Whisper API.

## Usage

```go
text, err := audio.TranscribeAudio(audioPath, apiKey)
```

Requires OpenAI API key in `config.json`. Cost: ~$0.006/minute.
