# Cross-Feature Search

Search across all feature content from Claude Code via the `search` MCP tool.

## What it does

Queries feature descriptions, decisions, issues, notes, session summaries, subtask/task item titles, and checkpoint observations using SQLite FTS5 full-text search. Returns ranked, snippet-highlighted results.

## How to use

Call the `search` MCP tool:

- `search(query: "caching")` — search everything
- `search(query: "auth", scope: "decisions")` — only search decisions
- `search(query: "auth.go", feature_id: "api-redesign")` — search within one feature
- `search(query: "cache", verbose: true)` — get full field values instead of snippets

Supports FTS5 syntax: `"exact phrase"`, `prefix*`, `word1 AND word2`, `word1 OR word2`.

## How it works

A unified FTS5 virtual table (`search_index`) indexes all searchable text. SQLite triggers on each source table (features, decisions, issues, notes, sessions, subtasks, task_items, checkpoint_observations) keep the index in sync on every INSERT/UPDATE/DELETE.

Porter stemming means "caching" matches "cache", "cached", etc.

## Key files

- `internal/store/migrate.go` — schema v16: FTS5 table, triggers, initial population
- `internal/store/search.go` — `Search()` and `RebuildSearchIndex()` methods
- `internal/store/search_test.go` — tests
- `internal/mcp/tools_search.go` — MCP tool handler
- `internal/mcp/tools.go` — tool registration

## Gotchas

- The FTS5 index is denormalized (duplicates text). This is intentional — data volume is tiny for a feature tracker.
- `task_items` don't reference `features` directly. Triggers JOIN through `subtasks` to get the `feature_id`.
- `RebuildSearchIndex()` exists as a safety valve but isn't exposed via MCP. Call it from Go if the index gets out of sync.
