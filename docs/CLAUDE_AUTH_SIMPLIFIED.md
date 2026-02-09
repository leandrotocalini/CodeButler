# Claude Code Authentication - SIMPLIFIED ✅

## What Changed

Simplified CodeButler to use Claude Code CLI's built-in authentication instead of manually managing OAuth tokens.

## Before

```json
{
  "claudeCode": {
    "oauthToken": ""
  }
}
```

Wizard asked for OAuth token, config validation checked it, executor injected it via environment variable.

## After

```json
{
  // claudeCode section removed
}
```

Claude Code CLI handles its own authentication automatically.

## How It Works Now

### 1. Claude Code CLI Authentication

When you first use Claude Code:
```bash
$ claude

# First time, it will prompt:
Please visit this URL to authenticate:
https://claude.ai/oauth/...

# Opens browser, you login, done!
```

The token is saved in `~/.claude/` (or similar) and automatically used for all future commands.

### 2. CodeButler Just Executes

```go
// Old way (unnecessary)
cmd := exec.Command("claude", prompt)
cmd.Env = append(cmd.Env, "CLAUDE_CODE_OAUTH_TOKEN="+token)

// New way (simpler)
cmd := exec.Command("claude", prompt)
// That's it! Claude CLI handles auth
```

## Files Modified

- ✅ `internal/claude/executor.go` - Removed `oauthToken` parameter
- ✅ `internal/bot/handler.go` - Updated to call `NewExecutor()` without token
- ✅ `internal/config/types.go` - Removed `ClaudeConfig` struct
- ✅ `internal/config/load.go` - Removed token env variable loading
- ✅ `internal/setup/wizard.go` - Removed OAuth token prompt
- ✅ `internal/setup/validate.go` - Removed OAuth token validation
- ✅ `config.sample.json` - Removed `claudeCode` section
- ✅ `config.json` - Removed `claudeCode` section

## User Setup

### Prerequisites

```bash
# Install Claude Code CLI
brew install claude
```

### First Time Use

Option 1: **Manual login** (recommended):
```bash
$ claude login
# Opens browser, login with Claude account, done!
```

Option 2: **Let CodeButler trigger it**:
```bash
$ ./codebutler
# When you run your first @codebutler run command,
# Claude CLI will prompt for authentication
```

### That's It!

No config files to edit, no tokens to manage, no environment variables. Claude Code just works.

## Benefits

### 1. Simpler Setup
- **Before**: Copy OAuth token, paste in config or env variable
- **After**: `claude login` once, never think about it again

### 2. More Secure
- Token stored in Claude CLI's secure location
- Not in config.json or environment variables
- Standard OAuth flow via browser

### 3. Consistent with Claude Code
- Same authentication method as using Claude CLI directly
- No custom token management
- Follows Claude's best practices

### 4. Less Code
- Removed ~80 LOC related to token management
- No OAuth prompts in wizard
- No validation checks
- Simpler executor

## Documentation Updated

All docs updated to reflect:
- No OAuth token configuration needed
- Just install Claude CLI and login
- CodeButler uses system Claude authentication

## Testing

To test:
```bash
# 1. Ensure Claude Code is installed
brew list claude || brew install claude

# 2. Login if not already
claude login

# 3. Run CodeButler
./codebutler

# 4. Test command
@codebutler run list files
# Should work without any OAuth prompts
```

## Migration

If you have an existing config.json with `claudeCode` section:
- The field is ignored (backwards compatible)
- Or delete it manually
- No migration script needed

## Future

Claude Code CLI will handle:
- Token refresh
- Re-authentication if expired
- Multi-account support (if they add it)

CodeButler doesn't need to worry about any of this.

## Conclusion

**Eliminamos complejidad innecesaria.** Claude Code CLI ya maneja autenticación perfectamente. ¿Para qué reinventar la rueda?

✅ Más simple
✅ Más seguro
✅ Menos código
✅ Mejor UX
