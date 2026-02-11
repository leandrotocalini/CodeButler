# CodeButler Journey

Detailed record of architecture decisions and features implemented.

---

## 2026-02-11 — TUI local input and intelligent session management

### TUI: Local terminal input

The daemon had a clean TUI with a fixed header and scrolling logs, but there was
no way to type messages without switching to WhatsApp. A text input row (`> `)
was added to the bottom of the terminal.

**Screen architecture:**
```
Row 1-4:    Fixed header (group name, web port)
Row 5..N-1: Scroll region (logs, messages, Claude responses)
Row N:      Input prompt "> _"
```

**Changes in `logger.go`:**
- Added `golang.org/x/term` dependency to detect terminal size
- `NewLogger()` now checks if both stdin AND stderr are TTYs (`go-isatty`). If
  both are, it activates `inputMode`
- `Header()` uses `term.GetSize()` to set the scroll region to `[5; height-1]`,
  reserving the last row for the input prompt
- New core method `printLines()`: ALL TUI output goes through this. It moves the
  cursor to the bottom of the scroll region, prints (which scrolls up), and
  redraws the prompt. This prevents log output from clobbering the input line
- `outMu` (mutex) protects cursor operations so multiple goroutines don't
  interleave terminal escape sequences
- `UserMsg()` changed first parameter from `from` to `label` — now accepts
  "WhatsApp" or "TUI" as a generic source label

**Changes in `daemon.go`:**
- `startInput(ctx)`: goroutine with `bufio.Scanner(os.Stdin)`. For each line:
  1. Echo to WhatsApp as `[BOT] [TUI] text` (visible in the group but filtered
     by the bot prefix, so it won't be re-processed)
  2. Insert into SQLite as a regular message (`From: "TUI"`)
  3. Log with "TUI" label + notify poll loop
- Launched in `Run()` only if `InputMode()` returns true

**Why this approach:** The alternative was an interactive input with readline/libedit,
but that adds unnecessary complexity. A simple `bufio.Scanner` is enough — the
user doesn't need history or autocomplete to send prompts to Claude.

---

### Sessions: Automatic continuation with `[CONTINUING]`

**Problem:** `claude -p` with `--max-turns 10` can take a long time on large tasks
and the user sees nothing until it finishes. There's no partial output because
`--output-format json` returns a single JSON at the end.

**Solution:** Lower `maxTurns` to 5 and add a marker to the system prompt:

> "If you hit the turn limit and still have more work to do, summarize what you
> accomplished so far and what you will do next, then end with [CONTINUING]"

In `processBatch()`, the Claude loop now:
1. Runs `claude -p` with `--max-turns 5`
2. If the response contains `[CONTINUING]`:
   - Strips the marker
   - Sends the partial response to WhatsApp (user sees progress)
   - Auto-resumes with "Continue from where you left off."
   - Repeats
3. If no marker present — exits the loop

**Token trade-off:** Each `--resume` reloads the entire session (input tokens).
With `maxTurns=5`, a 20-turn task generates 4 reloads instead of 1. But the
visibility benefit justifies the cost. If the overhead matters, increase
`maxTurns` in `config.json`.

---

### Sessions: `[NEED_USER_INPUT]` for indefinite wait

**Problem:** When Claude proposes a plan and waits for confirmation ("Reply *yes*
to implement"), the compact watchdog could clean the session if the user takes
time to respond (went to the bathroom, busy with something else, could even
take days).

**Solution:** Another marker in the system prompt:

> "When you need user confirmation or any input before you can proceed, end with
> [NEED_USER_INPUT]"

The daemon tracks this with `convWaitingInput` (protected by `convMu`):
- Set in `startConversation(chatJID, waitingInput)` after each response
- The compact watchdog checks `isWaitingInput()` — if true, it won't compact
- When the user replies, Claude processes. If it still needs input, it returns
  the marker again. If not, the flag clears naturally

**Elegance:** There's no special logic to "clear" the flag. Claude puts it when
it needs input and doesn't when it doesn't. The flag simply reflects Claude's
latest state.

---

### Sessions: Compact watchdog with 10-minute idle timer

**Problem:** Sessions grew indefinitely. Each `--resume` loaded the entire history.
Compacting on `endConversation()` (60s no reply) was too aggressive — sometimes
the user comes back in 2 minutes with a follow-up.

**Solution:** An independent `compactWatchdog` goroutine:
1. Waits for activity signal (`activityNotify` channel)
2. After the last activity, waits 10 minutes of total silence
3. Any activity (message in, Claude response, TUI input) resets the timer
4. When 10 minutes pass with nothing:
   - Checks: not busy, no active conversation, not waiting for input
   - Resumes Claude with a compaction prompt: "Summarize this conversation:
     key decisions, what was done, current state, pending items"
   - Saves the summary text in `sessions.context_summary`
   - Clears the `session_id` with `ResetSession()` (Claude's session is discarded)

**Next message after compaction:** Since there's no `session_id`, it doesn't use
`--resume`. Instead, `processBatch` looks up the `context_summary` and prepends it:
```
<previous-context>
{compacted summary}
</previous-context>

[2026-02-11T...] Leandro: new message
```

Claude starts a fresh session but with context from before. Bounded growth.

**`/cleanSession` command:** Deletes the entire DB row (session + summary).
The next message starts 100% clean. Useful when previous context is irrelevant
or confuses Claude.

**SQLite schema:**
```sql
sessions (
  chat_jid        TEXT PRIMARY KEY,
  session_id      TEXT DEFAULT '',    -- Claude session for --resume
  context_summary TEXT DEFAULT '',    -- compacted summary
  updated_at      TEXT
)
```

`ResetSession()` clears only `session_id`. `ClearSession()` deletes the entire row.

---

## 2026-02-11 — Why the terminal couldn't handle typing, and teaching Claude to send photos

### The TUI input was broken in a way that looked correct

The previous entry described a TUI with a scroll region and a `> ` prompt pinned to
the bottom row. It sounded great on paper. In practice, typing "Test" and pressing
Enter did nothing — the text appeared in the middle of the screen, never reached
WhatsApp, and Claude never saw it.

Two things were broken, and they conspired to make debugging confusing:

**The rogue print.** Deep inside `whatsapp/client.go`, the `Connect()` function had
a `fmt.Fprintln(os.Stderr, "✅ Connected to WhatsApp")` — a leftover from early
development. This single line bypassed the Logger's scroll region management and
wrote directly to the prompt row (the last terminal row). The trailing newline then
scrolled the *entire* screen by one line, shifting the scroll region out of alignment.
Every subsequent `printLines` call wrote to positions that no longer matched what was
on screen. The prompt was being redrawn, but at a row that was now invisible.

**Cooked mode fundamentally can't work here.** Even without the rogue print, the
standard terminal line discipline (cooked mode) echoes typed characters at whatever
position the cursor happens to be. If a log line arrives while you're typing, the
cursor jumps to the scroll region, and your next keystroke echoes there instead of
the prompt. Worse: pressing Enter at the last row of the screen generates a newline
that scrolls everything — the header, the scroll region, all of it. Each Enter press
would degrade the layout further.

The fix was to switch to **raw mode** (`term.MakeRaw`). This disables terminal echo
entirely — no character appears on screen unless we explicitly draw it. Each keystroke
goes through the Logger's cursor management, protected by the same mutex that handles
log output. The prompt, the typed text, and the log lines never fight over cursor
position.

The implementation handles backspace (with proper UTF-8 multi-byte awareness),
Ctrl+U (clear line), Ctrl+W (delete word), Ctrl+C/D (shutdown), and silently
swallows escape sequences from arrow keys. It's not readline — there's no cursor
movement within the line, no history — but it's solid enough for sending prompts
to Claude.

One subtlety: raw mode disables signal generation, so Ctrl+C no longer produces
SIGINT automatically. The input handler catches byte `0x03`, restores the terminal
state, and sends SIGINT to itself. This preserves the existing signal-based shutdown
flow without special-casing raw mode in the daemon.

---

### Claude can now send you images over WhatsApp

Until now, the communication between Claude and WhatsApp was text-only in the
outbound direction. Claude could *receive* images (via `<attached-image>`), but
if it wanted to share something visual — a screenshot, a chart, a test result —
it had no way to do it. The response was always plain text.

This matters more than it sounds. Consider: you ask Claude to run your test suite,
and it finds failures with visual diffs. Or it generates a diagram to explain an
architecture decision. Or it takes a screenshot of a UI it just modified. Without
image support, it has to *describe* what it sees instead of just showing you.

The solution is a symmetric counterpart to the existing `<attached-image>` tag.
Claude's system prompt now includes an instruction:

> To send the user an image file, wrap the absolute path in your response:
> `<send-image path="/absolute/path/to/file.png">optional caption</send-image>`

When the daemon receives Claude's response, before sending it as text, it scans for
`<send-image>` tags with a regex. For each match, it reads the file at the given
path, sends it to WhatsApp as an image message (with the caption, if any), and strips
the tag from the text. Whatever text remains gets sent as a normal message. If the
response was *only* an image tag with no surrounding text, no text message is sent
at all — just the image.

This means Claude can now do things like:

- Run tests, take a screenshot of the failure, and send it alongside a summary
- Generate a diagram with a tool and send it directly
- Show you what a UI looks like after a change
- Send multiple images in one response, interleaved with explanations

**Why a tag instead of a tool?** Claude already has tool access to the filesystem —
it could write files. But it has no "send to WhatsApp" tool. The tag approach
is lighter: no MCP extension needed, no new tool registration, no extra round trips.
Claude just includes the tag in its normal text response, and the daemon handles
the rest on the way out. It's a post-processing step, not an interaction.

**Mimetype detection.** The original `SendImage` in the WhatsApp client hardcoded
`image/png` as the mimetype. Since Claude might reference JPEGs, PNGs, or anything
else, `SendImage` now uses `http.DetectContentType` to sniff the first bytes and
pick the right mimetype automatically. If detection fails, it falls back to JPEG
(the most common format for screenshots and photos).
