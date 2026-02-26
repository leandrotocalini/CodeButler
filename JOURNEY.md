# CodeButler Journey

Detailed record of architecture decisions and features implemented.

---

## 2026-02-25 — M1: Project Bootstrap

### What was built

The Go project foundation: module initialization, CLI entrypoint, and the full
internal package layout that all future milestones build on.

### Decisions

**stdlib `flag` over cobra** — M1 only needs `--role`. Cobra adds a dependency
and boilerplate for something one `flag.String` call handles. If future milestones
need subcommands (`init`, `start`, `stop`, `validate`), cobra can be introduced
then. No point paying the complexity cost now.

**doc.go per package** — Each internal package gets a `doc.go` with a package
declaration and doc comment. This makes every package a valid Go package that
`go build`, `go test`, and `go vet` recognize, even before any real code exists.
It also serves as lightweight documentation of each package's purpose.

**Role validation in main** — The valid roles map lives in `main.go` rather than
a separate package. It's a simple string set that validates CLI input. Moving it
to `internal/models` would be premature — when the agent runner needs role types,
it'll define its own (likely an enum-style `type Role string`).

### Package layout

```
cmd/codebutler/main.go          — Entrypoint, --role flag, role validation
internal/
  slack/                         — Slack client, Socket Mode, dedup
  github/                        — PR operations via gh CLI
  models/                        — Agent interfaces and shared types
  provider/openrouter/           — OpenRouter chat completions client
  provider/openai/               — OpenAI image gen (Artist)
  tools/                         — Tool interface, registry, executor
  mcp/                           — Model Context Protocol client
  skills/                        — Skill parser, validator, index
  multimodel/                    — Multi-model fan-out
  router/                        — Per-agent message routing
  conflicts/                     — File overlap detection
  worktree/                      — Git worktree management
  config/                        — Config loading and validation
  decisions/                     — Decision log (JSONL)
```

### What's next

M2 (Config System) — load global and per-repo config files with typed structs
and environment variable resolution. This unblocks M3 (OpenRouter Client) and
M4 (Tool System).

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

---

## 2026-02-25 — M2: Two config files, one truth

### The problem: secrets and settings don't belong in the same place

CodeButler runs six agent processes, all parameterized from the same binary. Each
needs to know which Slack channel to listen on, which LLM model to use, and how to
authenticate with OpenRouter and Slack. The obvious approach — one config file with
everything — immediately hits a wall: API keys can't be committed to git, but model
assignments and channel IDs should be.

The spec defines two config files for this reason. `~/.codebutler/config.json` holds
secrets (Slack tokens, OpenRouter key, OpenAI key) and lives on each machine, never
committed. `.codebutler/config.json` inside the repo holds everything else — models,
limits, channel — and gets committed so the whole team shares the same agent
configuration.

### Finding the repo

Before loading anything, the loader needs to find the repo root. CodeButler uses
`.codebutler/` as its marker directory (similar to how git uses `.git/`). The loader
walks up from the current working directory, checking each level for a `.codebutler/`
subdirectory. This means you can run `codebutler --role pm` from any subdirectory
of the project and it will find the right config.

The alternative was requiring an explicit path flag, but that's friction for every
invocation. The walk-up approach matches developer expectations — `git` works the
same way.

### Environment variable resolution happens before JSON parsing

MCP server configs reference secrets with `${VAR}` syntax — for example,
`"${GITHUB_TOKEN}"` in the server's env block. These need to resolve to actual
values from the process environment, since secrets come from the service unit or
shell, not from committed files.

The resolution is a simple regex replacement on the raw JSON string *before*
`json.Unmarshal`. This is intentional: doing it at the string level means it works
for any field in any config file, not just specifically annotated fields. The MCP
config (loaded by a different package in a later milestone) will get the same
resolution for free because it uses the same `loadJSON` helper.

Unset environment variables resolve to empty strings rather than causing an error
at the resolution stage. The validation step catches the problem downstream with
a clear message ("slack.botToken is required") rather than a cryptic "SLACK_BOT_TOKEN
is not set." This way the user knows *which config field* is broken, not just which
env var is missing.

### Typed structs mirror the spec exactly

Each agent role has its own model configuration shape. The PM gets a `default` model
plus a `pool` map for hot-swapping (`/pm claude`, `/pm kimi`). Standard agents (Coder,
Reviewer, Researcher, Lead) get a single `model` plus an optional `fallbackModel` for
circuit breaker recovery. The Artist is different again — separate `uxModel` (for
design reasoning via Claude) and `imageModel` (for generation via OpenAI).

These could have been a single generic struct with optional fields, but that would
push validation complexity into every consumer. With distinct types, the compiler
catches mistakes: you can't accidentally read `Model` on a PM config that only has
`Default` and `Pool`.

### Validation: fail fast, fail all at once

The validator collects all missing required fields before returning, rather than
failing on the first one. When you're setting up a fresh machine and three fields
are missing, you want to see all three — not fix one, re-run, fix the next, re-run.

Only truly required fields are validated: the three authentication keys (Slack bot
token, Slack app token, OpenRouter API key) and the Slack channel ID. The OpenAI
key is optional — only the Artist agent needs it, and not every team uses the Artist.
Model assignments are optional too; defaults can be applied at the agent runner level
when that code exists.

### Testing with real file layouts

The tests create temporary directory trees that mirror the actual file structure:
`tmpdir/.codebutler/config.json` for the repo config, a separate temp directory
for the global config. This exercises the full `Load` path including repo root
discovery, file reading, env var resolution, and validation — not just individual
functions in isolation.

The `globalDir` parameter on `Load` exists specifically for testing. In production
it's empty and defaults to `~/.codebutler/`. In tests it points to a temp directory.
This avoids the need to mock `os.UserHomeDir` or pollute the real home directory
during tests.

---

## 2026-02-25 — M3: Not all errors deserve a retry

### The problem with "just retry on failure"

CodeButler runs six agent processes, all hitting the same OpenRouter account. When
you naively retry on any error, six processes backing off at the same rate create
a thundering herd — they all retry at the same moment, get rate-limited together,
back off the same duration, and slam the API again in sync. Worse, retrying errors
that will *never* succeed (bad API key, content policy violation) burns money and
delays the real error message.

The OpenRouter client needed to treat different errors differently. Not as an
afterthought, but as the core design decision.

### Eight error types, eight strategies

The classifier examines HTTP status codes and response bodies to sort every failure
into one of eight categories. Each has its own retry policy:

**Retry aggressively:** rate limits (429) and provider overloads (502/503) get up
to 5 retries with exponential backoff. Rate limits respect the `Retry-After` header
when present — the server is telling you exactly when to come back.

**Retry cautiously:** context length exceeded gets exactly 1 retry (the caller can
compact the conversation and try again), malformed JSON responses get 3 retries
(the model might produce valid JSON on the next attempt), and timeouts get 1 retry.

**Never retry:** auth errors (wrong API key), content filter violations, and unknown
errors fail immediately. No amount of retrying will fix a bad API key. Failing fast
means the agent can post a useful error message to the Slack thread instead of
silently burning through retry budgets.

The interesting case is 400 errors. OpenRouter returns 400 for at least three
different situations — context too long, content filtered, and generic bad request.
The classifier examines the error body's `code`, `type`, and `message` fields with
substring matching across all three (lowercased), because different upstream
providers format these fields inconsistently. A Claude error might put
`context_length_exceeded` in the `code` field; an OpenAI model might say "too many
tokens" in the `message`. The classifier catches both.

### Jitter: the anti-thundering-herd weapon

Every retry delay gets multiplied by `0.5 + rand.Float64()`, producing a value
between 50% and 150% of the calculated delay. This is mandatory, not optional.
Six agent processes sharing one API key will naturally synchronize their retry
cadences without jitter — they hit the same rate limit at the same time, wait the
same duration, and collide again. Jitter breaks the synchronization.

### Per-model circuit breakers

The circuit breaker wraps the entire retry loop. If a model fails 3 consecutive
times (after retries), the breaker opens and every subsequent call returns
immediately with an error — no HTTP request made. After 30 seconds, it allows
one probe request. If that succeeds, normal operation resumes.

The key design decision: **breakers are per-model, not per-client.** If the Coder's
Claude Opus model is down but the PM's cheaper model is fine, only the Coder's
circuit opens. The PM continues working. This required a `map[string]*CircuitBreaker`
keyed by model name, lazily created on first use, protected by a mutex.

Another subtlety: not all errors should trip the breaker. Auth errors and content
filter violations are client-side problems, not provider outages. The `IsSuccessful`
callback tells gobreaker to treat these as "successful" from a circuit-breaking
perspective — the provider responded correctly, the request was just invalid. Without
this, a bad API key would trip the breaker and block all requests for 30 seconds,
when the real fix is to update the config.

### Testing retry logic without waiting

The first test run took over 2 minutes. The circuit breaker test alone was 99 seconds
— it needed to exhaust 5 retries × 3 calls before the breaker tripped, with real
exponential backoff delays reaching 16+ seconds each.

The fix was an injectable sleep function. The client accepts a `WithSleepFunc` option
that replaces `time.Sleep` with any function matching `func(context.Context,
time.Duration)`. Tests pass `noSleep` (an immediate return), bringing the entire
suite from 2+ minutes to 60ms. The production default is a proper context-aware
sleep using `select` on both `ctx.Done()` and `time.After`.

This pattern — making time-dependent behavior injectable rather than mockable — is
cleaner than mocking `time.After` globally. The sleep function is part of the client's
configuration, not a test hack.

### The sole external dependency

`sony/gobreaker` is CodeButler's first external dependency beyond the standard
library. The alternative was implementing a circuit breaker from scratch, but
gobreaker is battle-tested, has a clean generic API (Go 1.18+ type parameters),
and does exactly one thing well. The generics-based `CircuitBreaker[*ChatResponse]`
means Execute returns a typed response without casting — a nice improvement over the
pre-generics version that returned `interface{}`.

---

## 2026-02-25 — M4: Every tool is a sandbox escape waiting to happen

### The problem: agents that can do anything will eventually do the wrong thing

An LLM with full filesystem access is a liability. When the Coder agent gets a
task like "refactor the auth module," it needs Write, Edit, and Bash. But the PM
agent — which only plans and delegates — should never touch a file. The Reviewer
reads diffs and comments; if it could also edit files, a hallucinated "quick fix"
could land in the codebase. And any agent running `rm -rf /` in a bash tool call
(which models do occasionally hallucinate) would be catastrophic.

The tool system needed three layers of defense: what a tool *is* (interface and
registry), who can *use* it (role restrictions), and where it can *act* (sandbox).

### Risk tiers: not all side effects are equal

The spec defines four risk tiers, and the interesting design question was where to
draw the lines. `Read`, `Grep`, and `Glob` are pure reads — no side effects, no
approval needed. `Write` and `Edit` change files but only in the worktree, which
is a disposable git branch — everything is reversible with `git checkout`. `Bash`
is the tricky one: `go test` is safe, `rm -rf` is destructive, and `npm install`
falls somewhere in between.

The classifier uses two lists: a safe list (test runners, linters, build tools,
read-only commands like `ls` and `cat`) and a dangerous pattern list (`rm -rf`,
`sudo`, `docker`, `DROP TABLE`, piping to `sh`). Everything not on either list
defaults to `WRITE_LOCAL` — conservative but not blocking. The alternative was
defaulting unknowns to `DESTRUCTIVE` and requiring approval for every unfamiliar
command, but that would make the Coder agent unusable for any project with custom
scripts.

One subtle case: `curl https://example.com | sh`. The original dangerous patterns
included `"curl | sh"` as a literal substring, which doesn't match when there's a
URL between `curl` and the pipe. The fix was simpler: match `"| sh"` and `"| bash"`
as standalone patterns. Piping *anything* to a shell interpreter is dangerous,
regardless of what's producing the output.

### Role restrictions: structural, not behavioral

The spec is explicit: role restrictions are *structural*, enforced at the executor
level, not just in the system prompt. A prompt saying "you should not write files"
is a suggestion. The executor checking `roleRestrictions[role][toolName]` and
returning an error is a wall.

The restriction map follows the spec table exactly: PM can't Write, Edit, or do
git operations (PM never writes code). Researcher can't Write, Edit, Bash, or
push (reads web, writes to research/ via a dedicated tool in a future milestone).
Reviewer can't Write, Edit, or Bash (reads and comments only). Lead can't Bash
(writes to MDs but never runs commands). Artist can't Bash or push (produces
designs, not code). Coder has no restrictions — full access within the sandbox.

### Sandbox: the path is a lie

File paths from an LLM can't be trusted. The model might return `../../etc/passwd`,
a symlink that resolves outside the worktree, or an absolute path to a sensitive
location. The sandbox validates every path before any tool touches the filesystem.

The validation resolves the path to an absolute form, then checks it starts with
the sandbox root. But `filepath.Clean` alone isn't enough — a symlink inside the
worktree could point to `/etc/`. So the sandbox calls `filepath.EvalSymlinks` to
resolve the actual target. If the resolved path escapes the root, the operation
is rejected.

There's a wrinkle for new files: `EvalSymlinks` fails if the file doesn't exist
yet (which is the normal case for Write). The fallback is to resolve the parent
directory instead — if the parent is inside the sandbox, the new file will be too.

### Idempotency: crash recovery without double execution

When an agent crashes and restarts, it resumes from the last saved conversation.
The last tool call might execute twice. For Read and Grep, that's harmless. For
Write, re-execution produces the same file (atomic write: temp file + rename).
For Edit, the tool checks whether `old_string` still exists — if it doesn't but
`new_string` is present, the edit was already applied.

But the general case is handled at the registry level: every tool call has an ID
(from the LLM response), and the registry caches results keyed by that ID. On
replay, the cached result is returned without re-executing the tool. This means
even Bash commands — which are inherently non-idempotent — won't run twice after
a crash recovery.

### Atomic writes: the temp-file-rename dance

Both Write and Edit use the same pattern: write to a temp file in the same
directory, then `os.Rename` to the target path. This guarantees that the file is
either fully written or not written at all — no partial writes if the process
crashes mid-operation. The temp file is created with `os.CreateTemp` in the same
directory as the target to ensure the rename is an atomic filesystem operation
(same filesystem, no cross-device rename).

### What's next

M4 unblocks M5 (Agent Loop), which is the core prompt → LLM → tool-call →
execute → repeat cycle. With the tool system in place, the agent runner can
dispatch tool calls to actual executors instead of mocks. The registry's
`Execute` method is already shaped to match the `ToolExecutor` interface defined
in `ARCHITECTURE.md` — the agent loop will accept it directly via dependency
injection.

---

## 2026-02-25 — M6: What happens when an agent dies mid-thought

### The problem: agents are mortal, conversations are not

The Coder agent is 50 tool calls into a complex refactoring. It's read files,
written code, run tests, fixed errors, and is about to make the final commit.
Then the process crashes — OOM, deployment restart, machine reboot. All 50
rounds of context live in a Go slice that just vanished from memory. When the
agent restarts, it has no idea what it was doing. It starts fresh, re-reads
the same files, re-writes the same code, burns the same tokens.

The agent loop (M5) was designed to be stateless — given a system prompt and
some messages, it runs until completion. That's clean for the loop itself, but
it means everything above the loop needs to handle the messy reality that
processes die. M6 adds the persistence layer: after every model round, the
full conversation is written to disk. On restart, the agent loads it and
continues from where it left off.

### Two layers of state, and why only one needs persistence

The architecture has a deliberate separation. Slack threads hold the curated,
public communication — what agents *said* to each other and to the user.
Conversation files hold the private, full transcript — every tool call, every
tool result, every intermediate reasoning step. The Coder might make 20 tool
calls before posting "PR ready" in Slack. Those 20 rounds live in the
conversation file, not in the thread.

This matters for crash recovery. You don't need to replay the Slack thread
to rebuild the model's context. The conversation file has everything: the
system prompt, the user's request, every assistant response, every tool
invocation and its result. Load the file, feed it to the model, and the
agent picks up exactly where it left off.

### Crash-safe writes: the rename trick

The write protocol is simple: serialize to a temp file, then rename. On POSIX
systems, `rename(2)` is atomic within a filesystem — it either completes fully
or not at all. If the process crashes while writing the temp file, the original
conversation file is untouched. If it crashes between writing and renaming,
you have a stale `.tmp` file that gets overwritten on the next save. The
conversation file is never in a half-written state.

This is the same pattern used by the tool system's Write and Edit tools (M4),
which makes sense — it's the standard approach for crash-safe file updates.
The alternative was append-only JSONL (one line per message), which would be
more efficient for incremental writes but harder to load and replay. Since
the full conversation needs to be sent to the model on every LLM call anyway,
writing the complete array each time is the natural choice.

### Where the store lives in the architecture

The `ConversationStore` interface lives in the agent package alongside
`LLMProvider`, `ToolExecutor`, and `MessageSender` — consumer-defined
interfaces that the runner depends on without knowing the implementation:

```go
type ConversationStore interface {
    Load(ctx context.Context) ([]Message, error)
    Save(ctx context.Context, messages []Message) error
}
```

The `FileStore` implementation lives in a separate `conversation` package.
This follows the "extract, don't embed" principle from the architecture doc:
the conversation store is generic enough to be useful outside CodeButler
(any LLM app that needs persistent conversations), so it's designed with
clean boundaries from the start. The `conversation` package imports
`agent.Message` but nothing else from the project.

The store is injected into the runner as an optional dependency via
`WithConversationStore()`. When not set, the runner behaves exactly as
before — pure in-memory conversation, no persistence. This keeps all
existing tests passing without modification.

### Resume logic: counting where you left off

The tricky part of resume isn't loading the messages — it's figuring out how
many turns were already completed. The runner needs to know because the
`MaxTurns` budget applies to the entire conversation, not just one
activation. If the Coder's limit is 100 turns and it crashed at turn 50, it
should have 50 turns remaining, not start over with 100.

The solution: count assistant messages in the loaded conversation. Each
assistant message corresponds to exactly one LLM call. If the loaded
conversation has 50 assistant messages, set `startTurn = 50` and begin the
loop from there. The remaining budget is `MaxTurns - startTurn`.

One edge case needs special handling: if the last message in the loaded
conversation is an assistant text response (no tool calls), the conversation
already completed. The runner returns immediately without making another LLM
call. Without this check, loading a completed conversation would send it to
the model again, which would produce a confused response to an already-finished
task.

### Save errors don't stop the loop

A deliberate decision: if `Save()` fails (disk full, permissions, I/O error),
the error is logged but the agent loop continues. The alternative — failing the
entire run on a save error — would mean a transient disk issue kills a
30-minute coding session. The agent can still complete its task; it just loses
crash recovery for that round. If the disk stays broken, every round will log
an error, giving the operator visibility without blocking progress.

### What's next

M6 provides the persistence layer that M7 (Agent Loop Safety) and M8 (Slack
Client) will build on. M7 needs conversation state for stuck detection (same
tool call 3 times = loop), and context compaction (when conversations grow too
long, summarize old messages). M8 needs it to reconcile Slack thread state with
agent state on restart — loading the conversation file tells the agent which
messages it already processed.

---

## 2026-02-26 — M7: Agent Loop Safety

### The problem: agents that loop forever

An LLM agent with tools will sometimes get stuck. It reads a file, gets an
error, reads the same file again, gets the same error, and repeats until it
exhausts its turn budget. Worse, it might not even be getting errors — it
might call the same tool with the same arguments three times in a row because
the model's response is deterministic for that context. Without detection,
this burns tokens and wastes time.

### Three detection signals

The `ProgressTracker` watches for three patterns, each with a threshold of 3
consecutive occurrences:

1. **Same tool + same params** — Hash the tool name and arguments with SHA-256.
   If the last 3 hashes in the rolling window are identical, the agent is
   calling the same thing repeatedly. "Same tool, different file" doesn't
   trigger this — only exact parameter matches.

2. **Same error repeated** — Track error messages from tool results. If the
   last 3 are identical strings, the agent is hitting the same wall. Different
   errors (even from the same tool) don't trigger — that indicates the agent
   is trying different things and getting different failures, which is progress.

3. **No progress** — Hash the assistant's text responses. If 3 consecutive
   responses are identical, the agent is saying the same thing without making
   observable progress.

The rolling window is 5 entries (the last 5 tool calls, errors, or responses).
Detection runs before every LLM call, so it can inject escape prompts before
the model repeats itself again.

### Four escalating escape strategies

When a stuck condition is detected, the runtime applies strategies in order.
Each strategy gets 2 turns to break the loop before escalating:

1. **Inject reflection** — Append a user message: "You appear to be in a loop.
   Stop and reflect." This often works because it changes the context enough
   that the model generates a different response. Cost: 1 turn.

2. **Force reasoning** — If reflection didn't help, inject: "List every approach
   you've tried and why each failed. Propose an approach you haven't tried."
   This forces explicit novelty in the next action.

3. **Reduce tools** — Remove the tool that's causing the loop from the active
   tool list. If the model was stuck calling Read on the same file, removing
   Read forces it to try something else (Grep, Bash, or just respond with text).
   Tools are restored when progress is detected.

4. **Escalate** — Post to the thread: "I'm stuck. Here's what I tried. I need
   help." Coder escalates to PM. PM and Lead escalate to the user. The agent
   stops its current activation.

Total cost: max 6 extra turns before the user is asked for help.

### Context compaction

Conversations grow. A Coder working through a 50-file refactor can easily hit
100K tokens. When the cumulative token usage approaches 80% of the model's
context window, the runner triggers compaction:

1. Keep the system prompt (never summarized)
2. Keep the last N assistant+tool pairs verbatim (recent context matters most)
3. Summarize everything in between via a single-shot LLM call
4. Replace the middle with the summary as a user message

The summary is a user message, not a system message — per the architecture
doc. This matters because system messages have special weight in most models,
and a progress summary shouldn't override the agent's identity prompt.

The compaction config is injectable: `WithCompaction(cfg)` sets the context
window size, threshold, and how many recent pairs to keep. When not configured,
no compaction happens — the runner behaves exactly as before.

### Integration with the runner

The runner now creates a `ProgressTracker` by default. Before each LLM call,
it checks for stuck conditions. After each tool execution, it records tool
call hashes and errors. When an escape sequence resets (progress detected),
any removed tools are restored.

The `Result` type gained two new fields: `LoopsDetected` (count of stuck
signals fired) and `Escalated` (true if all escape strategies were exhausted).
These feed into the decision log and thread reports in later milestones.

### What's next

M7 completes Phase 2 (Agent Core). Phase 3 starts with M8 (Slack Client),
connecting the agent loop to Slack via Socket Mode. The safety features from
M7 will be exercised end-to-end once real conversations flow through the
system.

---

## 2026-02-26 — M8: Slack Client

### What was built

The Slack integration layer: Socket Mode connection, message sending with
per-agent identity, file uploads for code snippets, emoji reactions, and
event deduplication. This is the bridge between the agent loop (M5) and the
outside world.

### Three components

**DedupSet** — A bounded, TTL-based set that prevents duplicate event
processing. Slack Socket Mode can deliver the same event multiple times
during reconnects and retries. The set tracks 10,000 event IDs with a
5-minute TTL, using a simple `map[string]time.Time` with mutex protection.
The time source is injectable for testing.

**Client** — Wraps `slack-go/slack` with Socket Mode event handling.
Receives events, acks them, filters through dedup, extracts message
content, and dispatches to a registered handler. Sends messages with
per-agent display name and icon emoji. Handles code snippets (inline for
<20 lines, file upload for longer). Manages emoji reactions for status
signaling.

**AgentIdentity** — Maps each agent role to a display name and icon.
PM gets :clipboard:, Coder gets :hammer_and_wrench:, Reviewer gets :mag:,
etc. All agents share one Slack bot app but post with distinct identities.

### Design decisions

**Skip bot messages** — The client filters out messages with a `bot_id` to
prevent self-loops. Without this, an agent posting a response would trigger
all agents to process their own messages, creating an infinite loop.

**Skip message subtypes** — Slack uses subtypes for edits, deletions, joins,
and other non-message events. The client only processes plain user messages
(`SubType == ""`), avoiding wasted LLM calls on thread meta-events.

**Thread fallback** — When a message has no `ThreadTimeStamp`, the client
uses the message's own timestamp as the thread. This handles top-level
messages correctly: they become the root of their own thread.

### What's next

M9 (Message Routing & Thread Registry) builds on this client to add
per-agent message filtering and goroutine-per-thread dispatch. M10 adds
Block Kit interactive messages for approval flows.

---

## 2026-02-26 — M9: Message Routing & Thread Registry

### What was built

Three components in the `router` package: message filtering (who processes
what), thread registry (goroutine-per-thread dispatch), and message
redaction (sensitive content filtering).

### Message filter: simple rules, no model

The routing rules are string matches — no LLM call needed:
- PM processes messages containing `@codebutler.pm` OR messages with no
  `@codebutler.*` mention at all (PM is the default handler)
- All other agents only process messages containing their specific
  `@codebutler.<role>` mention

A single message can route to multiple agents: `@codebutler.coder
@codebutler.reviewer` reaches both. This enables the PM to delegate
review and coding in one message.

### Thread registry: goroutine-per-thread

Each active Slack thread gets its own goroutine via `ThreadRegistry`.
The design follows the architecture doc: spawn on first message, die
after 60 seconds of inactivity (~2KB stack cost per idle goroutine),
respawn on next message.

Workers have a buffered channel inbox (10 messages). Messages are
processed sequentially within a thread to maintain ordering. Panic
recovery wraps both the worker's run loop and individual message
handling, so a panic in one message doesn't kill the worker or affect
other threads.

### Redaction: microsecond filtering

The `Redactor` runs regex patterns against every outbound message,
replacing sensitive content with `[REDACTED]`. Built-in patterns catch:
API keys (OpenAI, Slack, GitHub, AWS, Google), JWTs, private keys,
database connection strings, and private IP addresses (10.x, 172.16-31.x,
192.168.x).

Custom patterns can be added via `AddPattern`/`AddPatterns` for per-repo
overrides from `policy.json`. The redactor is a pure function on text —
no LLM, no I/O, microsecond latency.

### What's next

M10 (Block Kit & Interactions) adds interactive Slack messages for
approval flows. Then Phase 4 (Worktree Management) for isolated git
workspaces.

---

## 2026-02-26 — M10: Block Kit & Interactions

### What was built

Interactive Slack messages using Block Kit for decision points: plan
approval, destructive tool confirmation, and workflow selection. Plus
emoji reaction handling for lightweight signals.

### Block Kit builder

`BlockKitMessage` is a simple builder that produces Slack Block Kit JSON:
header section (bold text), body section (markdown), and an action block
with buttons. Each button has an `ActionID`, display text, value, and
optional style ("primary" for green, "danger" for red).

Two presets are provided: `PlanApproval` (Approve/Modify/Reject) and
`DestructiveToolApproval` (Approve/Reject with the command in a code
block).

### Fallback to plain text

Every `BlockKitMessage` generates a plain-text version with numbered
options. If the Block Kit API call fails (missing scope, rendering error),
`SendBlockKit` falls back to this text automatically. Users reply with
the option number instead of clicking buttons.

### Interaction routing

`InteractionRouter` dispatches button clicks by `ActionID` to registered
handlers. `ParseInteractionPayload` extracts the first block action from
Slack's callback JSON. Emoji reactions are converted to `Interaction`
values with helper functions: `IsStopSignal` (🛑) and `IsApproveSignal`
(👍 or approve button).

### What's next

Phase 3 (Slack & Communication) is complete. Phase 4 starts with M11
(Worktree Management) for isolated git workspaces per coding task.

---

## 2026-02-26 — M11: Worktree Management

### What was built

Git worktree lifecycle management: create isolated workspaces per coding
task, initialize them per platform, list active worktrees, and clean up
when done.

### Worktree Manager

The `Manager` struct wraps git worktree commands with a clean API:
`Create`, `Remove`, `List`, `Init`, `Exists`, `Path`. All git operations
go through an injectable `CommandRunner` — production uses
`exec.CommandContext`, tests use a mock that records calls.

`Create` tries `git worktree add -b` first (new branch), then falls back
to `git worktree add` with an existing branch. This handles both fresh
tasks and resumed work. If the worktree directory already exists, it
returns immediately — idempotent on retry.

`Remove` is aggressive: `--force` remove, manual cleanup if git fails,
prune stale entries, delete local branch, optionally delete remote
branch. Each step is best-effort — a failure in one doesn't block the
others.

### Platform detection and init

`DetectPlatform` checks for marker files: `go.mod` (Go), `package.json`
(Node), `requirements.txt`/`pyproject.toml` (Python), `Cargo.toml`
(Rust). Go and Rust need no explicit init. Node runs `npm ci`. Python
creates a venv and installs from `requirements.txt` if present.

### Branch naming

`BranchSlug` generates deterministic branch names from task descriptions:
lowercase, special chars to hyphens, collapsed doubles, truncated to 50
chars. Convention: `codebutler/<slug>`. This makes branches self-
documenting without needing to cross-reference thread IDs.

### What's next

M12 (Git & GitHub Tools) adds commit, push, and PR creation as agent
tools. M13 adds worktree garbage collection for orphan cleanup.

---

## 2026-02-25 — M12: Git & GitHub Tools

### What was built

Two structs in `internal/github/` that give agents full git + GitHub PR
capabilities, all idempotent on retry:

**GitOps** (`git.go`) — wraps git CLI commands:
- `Commit(ctx, files, message)` — stages files, checks for staged changes,
  commits. No-op if nothing staged (idempotent).
- `Push(ctx)` — pushes current branch. No-op if "Everything up-to-date".
- `Pull(ctx)` — pulls with rebase.
- `HasChanges(ctx)` — checks `git status --porcelain`.
- `CurrentBranch(ctx)` — reads current branch name.

**GHOps** (`pr.go`) — wraps `gh` CLI for pull request operations:
- `PRExists(ctx, head)` — checks if a PR exists for a head branch via
  `gh pr list --head`. Returns nil if none found.
- `CreatePR(ctx, input)` — creates a PR. **Idempotent**: checks PRExists
  first, returns existing PR if one already exists for the branch.
- `EditPR(ctx, input)` — updates title/body via `gh pr edit`.
- `MergePR(ctx, number)` — squash merges with branch deletion.
  **Idempotent**: handles "already been merged" gracefully.
- `PRStatus(ctx, number)` — fetches PR info via `gh pr view`.

### Design decisions

**Injectable CommandRunner pattern.** Both structs accept a `CommandRunner`
function (`func(ctx, dir, name, args) (string, error)`) via options.
Production uses `exec.CommandContext`; tests inject a mock that replays
recorded outputs. This avoids real git/gh calls in tests while verifying
the exact command sequences.

**Shared CommandRunner type.** `git.go` defines the `CommandRunner` type
and `defaultRunner` function. `pr.go` reuses them since both are in the
same package. This avoids duplication while keeping the package cohesive.

**Idempotency as a first-class concern.** Every operation that could be
replayed on agent restart handles the "already done" case:
- Commit: `git diff --cached --quiet` returns exit 0 = no changes = skip
- Push: "Everything up-to-date" in stderr = skip
- CreatePR: PRExists check before creation = return existing
- MergePR: "already been merged" in output = skip

This matches the idempotent tool execution requirement from ARCHITECTURE.md.

**JSON structured output from gh.** PRExists and PRStatus use
`--json number,url,title,state,headRefName` to get structured data instead
of parsing human-readable output. This is more reliable and avoids
locale-dependent formatting issues.

### What's next

M13 adds worktree garbage collection — orphan detection, warn/wait/clean
cycle, and restart recovery. M14 adds seed loading and prompt building.

---

## 2026-02-25 — M13: Worktree Garbage Collection

### What was built

Two modules in `internal/worktree/` for orphan detection and startup recovery:

**GarbageCollector** (`gc.go`) — periodic orphan detection and cleanup:
- `RunOnce(ctx)` — single GC pass: list worktrees, check each against
  three orphan criteria, warn or clean as appropriate.
- `Run(ctx)` — blocking loop that runs RunOnce immediately then every 6h.
- Orphan detection requires ALL three conditions: 48h+ inactivity, not in
  coder phase, no open PR for the branch.
- Warn-wait-clean cycle: first detection posts a warning in the thread,
  records the warn time. Next pass after 24h grace period triggers cleanup
  (worktree removal + remote branch deletion + mapping cleanup).
- Warning resets automatically if the thread becomes active again.

**RecoveryHandler** (`recovery.go`) — startup reconciliation:
- `Reconcile(ctx)` — compares local worktrees against thread mappings.
  If thread is gone → immediate cleanup. If worktree has no mapping →
  flagged as orphaned (left for GC). Active threads left alone.
- Returns `RecoveryResult` with counts for monitoring.

### Design decisions

**Five interfaces for dependency injection.** The GC needs to check Slack
threads, PR status, thread phases, send notifications, and read/write
mappings. Rather than coupling to concrete Slack/GitHub clients, five
small interfaces (`ThreadChecker`, `PRChecker`, `PhaseChecker`,
`GCNotifier`, `MappingStore`) allow testing with simple mocks.

**Injectable clock.** The `WithGCClock` option injects `func() time.Time`
so tests can control time deterministically. This avoids flaky tests that
depend on wall-clock timing and lets us test the 48h inactivity threshold
and 24h grace period precisely.

**In-memory GC state.** Warned-at timestamps are kept in a `map[string]time.Time`
protected by a mutex. On restart, this state is lost — which is fine because
the recovery handler runs first and handles the immediate cases, while GC
will re-detect orphans and re-warn them. No persistent state file needed.

**Mapping cleanup for stale entries.** If a mapping exists but no local
worktree is found, the mapping is cleaned up immediately. This handles
cases where a worktree was manually deleted or cleaned by another process.

### What's next

M14 adds seed loading and system prompt building — parsing agent MD files
and assembling the system prompt for each role. M15 adds skill file
parsing and validation.

---

## 2026-02-25 — M14: Seed Loading & Prompt Building

### What was built

New `internal/prompt/` package with four files:

**seed.go** — seed file loading:
- `LoadSeed(seedsDir, filename)` — reads a single MD file, strips archived
  learnings section.
- `LoadSeedFiles(seedsDir, role)` — loads role seed + global.md + workflows.md
  (PM only). Returns `SeedFiles` struct.
- `ExcludeArchivedLearnings(content)` — finds `## Archived Learnings` marker
  and removes everything from that point onward.

**builder.go** — system prompt assembly:
- `BuildSystemPrompt(seeds, skillIndex)` — pure function that joins
  seed + global + workflows + skill index with `---` separators.
  Only includes non-empty sections.

**skillindex.go** — skill directory scanning:
- `ScanSkillIndex(skillsDir)` — reads all `.md` files, extracts name
  (from `#` header or filename), description (first paragraph), and
  triggers (from `## Trigger` section).
- `FormatSkillIndex(skills)` — formats as markdown for the PM's prompt.

**watcher.go** — hot-reload cache:
- `PromptCache` with `Get()` that checks file modification times and
  rebuilds the prompt when any watched file changes. Thread-safe with
  RWMutex. `Invalidate()` forces rebuild on next call.

### Design decisions

**Pure builder function.** `BuildSystemPrompt` is a pure function — same
inputs always produce the same output. No file I/O, no state. This makes
it trivially testable and composable.

**PM-only sections.** Workflows and skill index are only included for the
PM role. Other agents don't need workflow knowledge or skill listings —
they receive specific instructions from PM via @mentions.

**Mod-time based change detection.** The cache checks `os.Stat` mod times
rather than computing file hashes. This is simpler, faster, and
sufficient — we only need to know "did anything change", not "what changed".

**Archived learnings exclusion.** Uses simple string scanning for the
`## Archived Learnings` marker. Everything from that point is stripped.
This keeps the system prompt focused on current learnings while preserving
archived content in the file for git history.

### What's next

M15 adds full skill file parsing and validation — structured extraction
of all skill sections, variable detection, and `codebutler validate`.
