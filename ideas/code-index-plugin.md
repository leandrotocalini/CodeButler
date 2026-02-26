# Code Index Plugin — Semantic Codebase Understanding for AI Agents

**Status**: Idea (not yet planned)
**Date**: 2026-02-26

## Problem

When AI agents (Claude Code, CodeButler, etc.) land on a new project, they
spend significant time doing iterative Grep/Glob/Read to understand the
codebase. This works but is slow, keyword-dependent, and wastes LLM turns.

Grep can find `func HandleAuth` but cannot answer:
- "Where is authentication handled?" (semantic)
- "What does the `internal/agent/` module do?" (summary)
- "What depends on the config package?" (relationships)
- "How does a Slack message flow through the system?" (trace)

## Vision

A standalone CLI that **pre-computes codebase understanding** and exposes it
to AI agents via MCP or file-based consumption.

```
codeindex init .                     # index the repo
codeindex summary internal/agent/    # what does this module do?
codeindex relations internal/slack/  # who uses it, what it calls
codeindex search "message routing"   # semantic search
codeindex trace "HTTP request"       # data flow through the system
codeindex serve                      # MCP server mode
```

## Output Structure

```
.codeindex/
  manifest.json          # repo metadata, languages, size, last indexed
  summary.md             # executive summary (~2 paragraphs, inject into system prompt)
  modules/               # one file per package/module
    internal-agent.json  # summary, exports, imports, key types, responsibilities
    internal-slack.json
    ...
  graph.json             # dependency graph (imports + call graph)
  embeddings.db          # vector store (SQLite-vss or chromem-go)
```

## Key Insight: Embed Summaries, Not Code

Greptile's research found that **semantic similarity between a query and a
natural language description of code is 12% higher than between the query and
the code itself**. The pipeline should be:

1. Parse code with **tree-sitter** into AST nodes (functions, types, methods)
2. Generate natural language summaries for each node (cheap LLM, one-time)
3. Embed the summaries (not the raw code)
4. Search over summary embeddings

This is why naive "chunk files and embed them" approaches underperform.

## Architecture

### Parsing Layer

- **tree-sitter** for AST extraction (supports 100+ languages)
- Extract: functions, types, interfaces, methods, imports, exports
- Language-specific queries for Go, TypeScript, Python, Rust, etc.

### Summary Generation

- One-time batch pass with a cheap model (Haiku/Sonnet)
- Per-function: one-line summary of what it does
- Per-module: 2-3 sentence summary of responsibilities
- Per-repo: executive summary with architecture overview
- Incremental: only re-summarize changed files (track mtime/hash)

### Embedding & Storage

Best options for an embeddable, single-binary approach:

| Component | Recommended | Alternative |
|-----------|-------------|-------------|
| **Embeddings** | Voyage Code 3 (best for code) | nomic-embed-text via Ollama (fully local) |
| **Vector store** | chromem-go (pure Go, zero deps) | SQLite-vss, LanceDB |
| **Text search** | bleve (pure Go BM25) | — |

Hybrid retrieval: BM25 candidates re-ranked by vector similarity.

### Agent Interface

Two consumption modes:

1. **File-based**: `summary.md` injected into system prompt (like Aider's repo-map)
2. **MCP server**: exposes tools for on-demand search and exploration

MCP tools:
- `semantic_search(query, topK)` — find code by intent
- `explain_module(path)` — what does this module do?
- `show_relations(path)` — imports, dependents, call graph
- `trace_flow(description)` — follow data through the system

## Existing Solutions (Landscape)

### MCP Servers (plug-and-play, no code needed)

| Server | Vector DB | Notes |
|--------|-----------|-------|
| **Claude Context (Zilliz)** | Milvus | AST via tree-sitter, Go supported, hybrid BM25+dense |
| **CodeGrok MCP** | ChromaDB | 100% local, tree-sitter + symbol extraction |
| **Smart Coding MCP** | SQLite | Fully local, nomic-embed-text, Cursor-inspired |
| **VectorCode** | ChromaDB | Python-based, tree-sitter chunking |
| **DeepContext MCP** | Internal | Deep semantic graph, offline indexing |

### Integrated Products

| Product | Approach | Embeddable? |
|---------|----------|-------------|
| **Greptile** | AST + recursive docstring generation + embedding | API only ($30/dev/mo) |
| **Sourcegraph Cody** | Code graph + semantic index | Platform ($59/user/mo) |
| **Continue.dev** | LanceDB + tree-sitter + all-MiniLM | TypeScript, not embeddable in Go |
| **Cursor** | tree-sitter + custom embeddings + Turbopuffer | Proprietary |

### Embedding Models for Code

| Model | Provider | Dims | Notes |
|-------|----------|------|-------|
| **Voyage Code 3** | Voyage AI | 1024 | Best code-specific quality |
| **text-embedding-3-large** | OpenAI | 3072 | Strong general purpose |
| **Jina Embeddings v4** | Jina AI | 3840 | Multimodal, 30+ languages |
| **Nomic Embed Code** | Nomic | 768 | Best open-source for code |
| **nomic-embed-text** | Ollama | 768 | Fully local via Ollama |

## Implementation Strategy

### Phase 0: Evaluate existing MCP servers

Before building anything, try Claude Context (Zilliz) or CodeGrok as MCP
servers on a real project. Measure: does semantic search actually reduce the
number of tool calls and improve agent accuracy?

### Phase 1: Summary generator only (no vectors)

Build the tree-sitter parser + LLM summarizer. Output `summary.md` and
`modules/*.json`. This alone is high-value — agents read it once and
understand the project. No embeddings, no vector DB, no infrastructure.

### Phase 2: Add semantic search

Layer chromem-go + Voyage Code 3 embeddings on top of the summaries.
Expose as MCP server. Now agents can search by intent.

### Phase 3: Relationship graph

Build the import/call graph from AST data. Enable `trace_flow` and
`show_relations` queries. This is what moves it from "search" to
"understanding."

## Relation to CodeButler

This is **not** a CodeButler feature — it's a standalone tool that any AI
coding agent can use. However, it could integrate with CodeButler as:

- An MCP server in `.codebutler/mcp.json` (available to all agents)
- Part of the Learn workflow (index the repo when agents first explore it)
- A tool available to PM, Coder, and Reviewer

The existing CodeButler decision "no vector DB for memory" (SPEC.md, JOURNEY.md)
is about agent memory (learnings, research), not about code understanding.
These are different problems with different solutions.
