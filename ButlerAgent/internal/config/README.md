# Config Package

Loads and saves CodeButler configuration.

## Usage

```go
cfg, err := config.Load("config.json")
err := config.Save(cfg, "config.json")
```

## Structure

```go
type Config struct {
    WhatsApp WhatsAppConfig  // Session, group JID, bot prefix
    OpenAI   OpenAIConfig    // API key for Whisper
    Sources  SourcesConfig   // Path to repositories
}
```

Config file has 0600 permissions and is gitignored.
