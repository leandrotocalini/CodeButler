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
