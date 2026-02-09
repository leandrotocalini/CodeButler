# CodeButler Developer Group - The Only Communication Channel

## Quick Reference

The "CodeButler Developer" group is automatically created when you first run CodeButler. **It's the ONLY way to communicate with CodeButler** - no personal chat, no other groups.

## Creation Flow

```
First Run:
./butler
  ↓
WhatsApp QR Scan
  ↓
Search for "CodeButler Developer" group
  ↓
  ├─ Found? → Use existing ✅
  └─ Not found? → Create new ✨
  ↓
Add to config.json (groupJID + groupName)
  ↓
Send welcome message
  ↓
Ready! 🚀
```

## Key Features

### 1. Auto-Creation
- Searches for existing group by name "CodeButler Developer"
- Creates new group if not found
- Only you as member (private group)
- Automatically configured as the only channel

### 2. Single Channel Communication
- **Only channel**: No personal chat support
- **No other groups**: Only this group is allowed
- **Simplified**: All messages must come from here
- **Focused**: Your dedicated dev workspace

### 3. Organization
- All dev commands in one place
- Clean command history
- No mixing with personal messages
- Easy to review past work

## Technical Implementation

### Config Structure

```json
{
  "whatsapp": {
    "sessionPath": "./whatsapp-session",
    "personalNumber": "1234567890@s.whatsapp.net",
    "groupJID": "120363123456789012@g.us",
    "groupName": "CodeButler Developer"
  }
}
```
      }
    ]
  }
}
```

### Go Code (WhatsApp Client)

```go
// Find or create CodeButler Developer group
func findOrCreateAlfredDeveloperGroup(wa *whatsapp.Client) (*whatsapp.Group, error) {
    groups, err := wa.GetGroups()
    if err != nil {
        return nil, err
    }

    // Search for existing group
    for _, group := range groups {
        if group.Name == "CodeButler Developer" {
            return &group, nil
        }
    }

    // Create new group
    groupJID, err := wa.CreateGroup("CodeButler Developer", []string{})
    if err != nil {
        return nil, err
    }

    return &whatsapp.Group{
        JID:  groupJID,
        Name: "CodeButler Developer",
    }, nil
}
```

### Message Routing

```go
func handleMessage(msg whatsapp.Message, cfg config.Config) {
    // Check if from dev control group
    isDevControl := false
    for _, group := range cfg.WhatsApp.AllowedGroups {
        if group.JID == msg.Sender && group.IsDevControl {
            isDevControl = true
            break
        }
    }

    if isDevControl {
        // Enable special commands
        handleDevControlCommand(msg)
    } else {
        // Normal command handling
        handleNormalCommand(msg)
    }
}
```

## Commands Available

### Basic Commands (All Chats)
```
@codebutler repos          # List repositories
@codebutler help           # Show help
in <repo>: <message>      # Target specific repo
```

### Dev Control Only (CodeButler Developer Group)
```
@codebutler status         # System status
@codebutler uptime         # System uptime
@codebutler memory         # Memory usage

# Bulk operations
@codebutler run tests in all repos
@codebutler git status in all repos
@codebutler update deps in all repos

# Workflows
@codebutler create workflow <name>
@codebutler list workflows
@codebutler trigger workflow <name>

# Cross-repo
@codebutler find <query> in all repos
@codebutler compare <file> between repos
@codebutler dependency graph

# Group management
@codebutler list groups
@codebutler allow group <name>
@codebutler deny group <name>
```

## Example Workflows

### Morning Standup
```
You: @codebutler morning standup

Bot: 🌅 Good morning! Here's your dev update:

Yesterday's commits:
- api-service: 3 commits, 145 lines changed
- frontend-app: 1 commit, 23 lines changed

Open PRs:
- api-service: "Add user authentication" (#42)

Failed tests: None ✅

TODOs from CLAUDE.md:
- [ ] api-service: Add rate limiting
- [ ] frontend-app: Fix responsive layout

Ready to code! 💪
```

### Pre-Commit Check
```
You: @codebutler pre-commit check api-service

Bot: Running pre-commit checks...

✅ Tests: 45 passed
✅ Linter: No issues
✅ Format: All files formatted
✅ Security: No vulnerabilities
✅ Build: Success

All checks passed! Safe to commit. 🚀
```

### Bulk Testing
```
You: @codebutler run tests in all repos

Bot: Running tests across 3 repositories...

✅ api-service: 45 passed, 0 failed
✅ frontend-app: 123 passed, 2 skipped
❌ mobile-client: 18 passed, 2 failed

Total: 186 passed, 2 failed, 2 skipped
Duration: 12.4s

Failed tests in mobile-client:
- TestUserLogin
- TestDataSync
```

### Cross-Repo Search
```
You: @codebutler find function authenticateUser in all repos

Bot: Found in 3 repositories:

api-service/internal/auth/handler.go:45
  func authenticateUser(email, password string) (*User, error) {

mobile-client/lib/auth/auth_service.dart:123
  Future<User> authenticateUser(String email, String password) async {

frontend-app/src/services/auth.ts:67
  async function authenticateUser(email: string, password: string): Promise<User> {

All implementations follow similar patterns ✅
```

## Why Only One Group?

### Simplicity
- **One place for everything** - No confusion about where to send messages
- **Simple access control** - Just check if message is from this group
- **Easy to understand** - No complex permissions or allow-lists

### Organization
- **Pure development context** - No personal messages mixed in
- **Clean history** - Easy to review commands and responses
- **Focused workspace** - Your dedicated dev command center

### Security
- **Ultra-simple validation** - Only one JID to check
- **No accidental access** - Can't send from wrong chat
- **Private by design** - Only you as member

## Setup Troubleshooting

### Group Already Exists But Not Auto-Detected
```bash
# Manually add to config.json
{
  "whatsapp": {
    "groupJID": "YOUR_GROUP_JID@g.us",
    "groupName": "CodeButler Developer"
  }
}
```

### Can't Create Group
- Check WhatsApp permissions
- Ensure you're not at group limit (256 groups max)
- Try creating manually in WhatsApp, then run `./butler` again

### Multiple "CodeButler Developer" Groups
- CodeButler will use the first one found
- Rename or delete extra groups in WhatsApp

### Group Disappeared
```bash
# Re-run setup
rm config.json
./butler
# Will create new group
```

## Future Enhancements

### Planned Features
- [ ] Scheduled tasks (cron-like)
- [ ] Webhook triggers
- [ ] CI/CD integration
- [ ] Metrics dashboard
- [ ] Team collaboration (add members)
- [ ] Custom workflows
- [ ] Notification rules

### Advanced Commands (Future)
```
@codebutler schedule daily at 9am: run tests
@codebutler when PR merged: deploy to staging
@codebutler alert me if tests fail
@codebutler create dashboard for api-service
```

## Security Considerations

### Privacy
- Group is private (only you as member)
- No external access
- Command history only visible to you

### Permissions
- Same access control as personal chat
- Config modifications require dev control flag
- Bulk operations isolated per repo

### Data Storage
- Group JID stored in config.json (gitignored)
- No conversation history stored by CodeButler
- WhatsApp handles all message encryption

## Best Practices

### Recommended Usage
1. All communication goes through CodeButler Developer group
2. Review command history regularly
3. Set up workflows for repetitive tasks
4. Keep notifications enabled for this group

### Command Organization
```
Morning:
  @butler morning standup
  @butler status

During work:
  in <repo>: <specific commands>

Before commit:
  @butler pre-commit check <repo>

End of day:
  @butler summary
  @butler git status in all repos
```

### Workflow Tips
- Create workflows for common tasks
- Use bulk operations to save time
- Set up alerts for important events
- Review system status periodically

## FAQ

**Q: Can I rename the group?**
A: Yes, but update `groupName` in config.json to match.

**Q: Can I add team members?**
A: CodeButler is designed for single-user use. Adding members would give them full access to all your repositories.

**Q: What if I delete the group?**
A: Run `rm config.json && ./butler` to recreate it.

**Q: Can I use personal chat instead?**
A: No. CodeButler only responds to messages from the CodeButler Developer group. This simplifies the architecture and keeps everything organized.

**Q: Can I have multiple groups?**
A: No. CodeButler only works with one group (CodeButler Developer). This is a design decision for simplicity.

**Q: Does it work on WhatsApp Business?**
A: Yes, same functionality.

**Q: What happens if I send a message from personal chat?**
A: CodeButler will ignore it. Only messages from the CodeButler Developer group are processed.

---

**The CodeButler Developer group is the ONLY way to communicate with CodeButler. Keep it organized!** 🚀
