# CodeButler Journey

Detailed record of architecture decisions and features implemented.

---

## 2026-02-22 — Vector DB evaluation for agent memory

### The question

Should CodeButler use a vector database for agent memory? The idea is natural —
agents accumulate knowledge over time, vector DBs enable semantic search over
that knowledge, and RAG (retrieval-augmented generation) is the standard pattern
for LLM memory systems. But the standard pattern isn't always the right one.

### Why the current design doesn't need it

The memory system is built around a specific principle: **the agent MD IS the
memory.** Each agent's `.md` file is loaded entirely into the system prompt on
every LLM call. This means 100% recall — the model sees every learning, every
project map entry, every convention, on every activation. There's no retrieval
step, and therefore no retrieval error.

A vector DB introduces a retrieval step. You embed a query, search for the top-K
most similar chunks, and hope the relevant knowledge is in those chunks. This
works well when you have millions of documents and can't fit them all in context.
But CodeButler's memory is intentionally small and curated — one MD per agent,
one global.md, a handful of research files. The entire memory fits comfortably
in a fraction of the context window.

### Five reasons against

**1. Breaks the "no database" decision.** The spec explicitly says: "No shared
database. The Slack thread is the source of truth." A vector DB (Qdrant,
ChromaDB, pgvector, even embedded options like sqlite-vss) adds infrastructure
that needs installation, configuration, backups, and maintenance. CodeButler
is one Go binary — a vector DB contradicts this.

**2. Distributed agents break.** The architecture supports agents on different
machines, coordinating only via git and Slack. A vector DB would need to be
either centralized (adding a network dependency and single point of failure) or
replicated across machines (consistency problems). Git already handles knowledge
distribution — MDs are committed, pushed, and pulled. No extra coordination.

**3. The Lead already curates knowledge.** The re-learn workflow is explicitly
designed as "knowledge garbage collection." The Lead compacts, removes outdated
info, and keeps MDs clean. This is better than accumulating everything in a
vector store, because a curated MD is always relevant. A vector DB grows
monotonically unless you build a separate garbage collection process — which is
exactly what the Lead already does, but for markdown files with git versioning.

**4. Embeddings cost money and add latency.** Every write to memory needs an
embedding API call. Every agent activation needs a vector search query before
the LLM call. That's latency and cost on every single interaction, in a system
designed to be cost-conscious (the PM uses cheap models specifically to minimize
cost per message).

**5. Git gives versioning for free.** You can `git blame` to see when a learning
was added, `git log` to trace its evolution, `git revert` to undo a bad one,
and review all changes in PRs before they land on main. Vector DBs have no
native equivalent. You'd need to build a versioning layer on top.

### The Research Index pattern already works

For accumulated research findings (which is the closest thing to "lots of
documents that need retrieval"), the spec uses a simple pattern: the Researcher
adds one-line entries to a `## Research Index` section in `global.md`, with
references to individual files in `research/`. All agents see the index via
their system prompt. When they need depth, they read the full file.

This is effectively manual RAG without the overhead: the index is the "search
results," the full files are the "retrieved documents," and the agent's judgment
replaces the embedding similarity score. It works because the number of research
files is small enough for a linear index. If it grew to hundreds, this pattern
would need to evolve — but that's a problem for later, not now.

### When it might actually be needed

The right time to revisit this decision is when:

1. **Agent MDs grow beyond ~50K tokens** — a single project with 100+
   retrospectives, each adding learnings, could push MDs past the point where
   full-context loading is wasteful. Not there yet.
2. **Cross-project knowledge transfer** — if the same team runs CodeButler on
   multiple repos and wants learnings from one project to inform another, a shared
   knowledge base with semantic search could help. Not a current requirement.
3. **Historical conversation queries** — "what did we decide about auth 3 months
   ago?" requires searching across closed threads whose conversation files have
   been deleted. A vector DB of conversation summaries could serve this.
4. **Research accumulation** — if `research/` grows to 50+ files, the linear
   index in global.md becomes unwieldy.

### The progression if scaling becomes a problem

1. **More aggressive compaction by the Lead** — already designed (re-learn
   workflow). First lever to pull.
2. **BM25 / keyword search over MDs** — no embeddings, no infrastructure. Just
   `strings.Contains` or a lightweight Go search library. Handles the "find
   relevant learning" case without vector overhead.
3. **Embedded vector store** — only as a last resort, local-only (no external
   service), as an optional feature. Maybe sqlite-vss or a pure Go option.

### Decision

**No vector DB.** The MD-based memory with Lead curation, git versioning, and
full-context loading is the right design for CodeButler's scale and architecture.
Revisit when there's evidence of actual memory scaling problems, not before.

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

---

## 2026-02-11 — Coding from the beach: the self-improving daemon

### The setup: three daemons, one phone

Picture this. You're going on vacation. Back home, your Mac has three CodeButler
daemons running — one for your iOS app, one for your backend API, and one for the
CodeButler repo itself. Each daemon has its own WhatsApp group, its own database,
its own Claude sessions. Three terminals in a `while true` loop, three WhatsApp
groups on your phone.

From the beach, you message the "iOS app" group: "add haptic feedback to the
purchase button." Claude builds it, runs the tests, sends you screenshots. You
switch to the "API" group: "add rate limiting to the auth endpoint." Done.

Then you hit a wall. You ask Claude to run the UI tests and send you the failure
screenshots, but the images arrive in reverse order — screenshot 6 first,
screenshot 1 last. Annoying, but not a bug in your project. It's a bug in Butler.

This is where things get interesting.

### The self-improvement loop

You switch to the third WhatsApp group — the one connected to the CodeButler repo.
You say: "fix the image send order, they arrive reversed." The CodeButler daemon
running on its own codebase receives the message, Claude finds the bug (images were
processed in reverse to keep string indices valid during removal), fixes it, creates
a PR. You merge it from your phone.

Now you need the fix to reach the other two daemons. You message the CodeButler
group: "install the new version." Claude runs `go install`, the new binary lands
in `$PATH`. Then you send `/exit` to all three groups.

Each daemon says "Bye!", exits, and the `while true` loop restarts it with the
fresh binary. Each one announces: "I am back. I am version 8." You go back to the
iOS app group, ask for the screenshots again, and this time they arrive in order.

The whole cycle — discover a limitation, fix it in Butler, deploy, continue working
— happened from a phone on the beach. You never opened a laptop.

### `/exit` and the respawn loop

The deployment model is deliberately primitive:

```bash
while true; do codebutler; sleep 2; done
```

No systemd, no Docker, no process manager. When you want to deploy, you just need
the process to die. The loop restarts it with whatever binary is in `$PATH`.

`/exit` is the WhatsApp command that triggers this. The daemon sends "Bye!", cleans
up the terminal, and calls `os.Exit(0)`. Two seconds later, it's back. On startup
it announces "I am back. I am version N" — only on the initial connection, not on
WiFi reconnects, so you don't get spammed every time the network hiccups.

### Version = PR number

The version number is just the PR number. PR #7 merged? Version 7. PR #12?
Version 12. It's monotonically increasing, maps directly to a GitHub URL
(`/pull/7`), and tells you exactly how many changes have landed.

The version is baked into the binary with `go:embed`:

```go
//go:embed VERSION
var Version string
```

This was a deliberate choice over `-ldflags`. Ldflags require the builder to pass
the right flag — which works locally but breaks for `go install github.com/...@latest`.
With `go:embed`, the version is part of the source. Anyone who builds the binary,
from anywhere, gets the correct version.

### Why this matters

The interesting thing isn't any single feature here. It's the closed loop. When
you're coding on a project through Butler and you run into something Butler can't
do — maybe you need it to handle a new file format, or you want a new command, or
the output is confusing — you don't have to stop, open a laptop, fix Butler, rebuild,
and redeploy. You just switch to the Butler group, describe what you need, and the
tool improves itself. Then you restart all instances and keep going.

The tool and the workflow evolve together, from the same interface.

---

## 2026-02-11 — Cheap thinking before expensive doing

### The cost of a messy prompt

Here's something that happens when you're coding from your phone: you send Claude
a stream-of-consciousness message. "Add a thing where when I write stuff it doesn't
go to Claude yet, like a draft, and then some AI cleans it up, and then I choose
whether to send it or not." Claude Opus receives this, parses it, thinks about it,
asks clarifying questions, and burns through tokens — all because the prompt was
sloppy. The intent was clear, but the expression wasn't.

This is expensive. Claude Opus costs real money per token, and most of that cost
is in the *understanding* phase — figuring out what you actually want. If the prompt
were clearer, Claude would spend fewer turns asking questions and more turns building.

The idea: what if a cheaper, faster model could clean up your thinking *before*
Claude sees it? You brain-dump freely, a lightweight model restructures it into a
proper prompt, and Claude gets a clean, actionable request on the first try.

### Draft mode: a buffer between you and Claude

The implementation is `/draft-mode` — a WhatsApp command that creates a buffer.
While draft mode is active, your messages accumulate locally. Nothing goes to Claude.
You can ramble, contradict yourself, think out loud. The daemon just stores each
message in a list.

When you're done thinking, you send `/draft-done`. The daemon concatenates all your
messages and sends them to Kimi (Moonshot AI's model, via their OpenAI-compatible
API at `api.moonshot.ai`). Kimi's system prompt is focused: "You're a prompt
engineer. Take these raw notes and turn them into a clear, structured, actionable
prompt for a coding assistant. Preserve intent, eliminate ambiguity, output only the
refined prompt."

Kimi responds with the cleaned-up version, and Butler presents it with three options:

> **1** — Send to Claude
> **2** — Iterate (give feedback, Kimi refines again)
> **3** — Discard

Option 2 is the interesting one. If the refinement missed something or went in the
wrong direction, you send feedback. Kimi gets the full conversation history — the
original draft, its first attempt, and your correction — and produces a new version.
You can iterate as many times as you want. When you're satisfied, option 1 injects
the refined prompt into the message store as if you'd typed it directly, and the
daemon's normal poll loop picks it up and sends it to Claude.

### Why Kimi and not Claude itself?

The obvious question: why not just use Claude to refine the prompt and then use
Claude again to execute it? Two reasons.

First, cost. Moonshot's `moonshot-v1-32k` model costs a fraction of what Claude
Opus charges. The refinement step is pure text transformation — no tool use, no
file access, no code execution. You don't need the most powerful model in the world
to restructure a paragraph.

Second, separation of concerns. The refinement model doesn't need repo context,
Claude sessions, or permission modes. It's a stateless HTTP call with a system
prompt and user text. Keeping it outside the Claude pipeline means draft mode
doesn't touch sessions, doesn't affect the conversation state machine, and can't
accidentally trigger tools or file changes.

### The model name saga

An amusing debugging story: the first version used `kimi-k2` as the model name,
because that's what most documentation sites listed. The API returned a 404:
"Not found the model kimi-k2 or Permission denied." Turns out the Moonshot API
doesn't use the marketing model names. The actual model identifiers are
`moonshot-v1-8k`, `moonshot-v1-32k`, and `moonshot-v1-128k` — named after context
window size, not the model generation. The 32k variant is plenty for prompt
refinement.

### Architecture: following the pattern

Draft mode follows the same pattern as `/create-image`: a handler struct with a
mutex-protected map of pending states per chat, command detection functions checked
before messages reach the normal pipeline, and numeric confirmations (1/2/3) that
are intercepted only when there's a pending state for that chat.

The key integration point is in `setupClient()`, where incoming messages are
filtered. Draft mode checks happen after image commands but before messages reach
the store. If draft mode is active, messages are accumulated instead of persisted —
they never enter the queue that feeds Claude. This is important: draft messages
don't trigger the poll loop, don't affect the conversation state machine, and don't
show up as pending messages. They exist only in memory until the user decides what
to do with them.

### Voice messages in draft mode: an ordering bug

The first version had a subtle bug: voice messages sent during draft mode wouldn't
get transcribed. The reason was the message processing order in `setupClient()`.
Voice transcription happened *after* the draft mode interceptor, so when the draft
check saw an incoming voice message, it accumulated the raw audio placeholder
instead of the transcribed text.

The fix was to move voice transcription earlier in the pipeline — right after the
command checks but before draft mode. Now the flow is: filter commands → transcribe
audio → check draft mode → normal pipeline. This means voice notes in draft mode
arrive as clean text, exactly like typed messages. The same Whisper transcription
that works in normal mode works in draft mode, because the draft handler never
sees the difference — it just receives a string.
