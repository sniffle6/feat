# Key Files Overlap Detection

## What it does

When you call `get_context` or `get_ready`, docket checks if any in_progress features share the same key_files. If they do, a warning appears in the output so you know about potential conflicts before starting work.

## How it works

- `get_context`: fetches all in_progress features, compares their key_files with the current feature's key_files. Shows overlapping files and which features share them.
- `get_ready`: after listing ready features, checks all in_progress features against each other. Appends a conflict section at the bottom if any files overlap.

Only in_progress features are checked — planned features are excluded.

## Example output

```
⚠ Key file conflicts:
  internal/store/store.go → feature-a, feature-b
```

## Matching

Exact string match on file paths. `internal/store/store.go` only conflicts with the same path, not `internal/store/migrate.go`.

## Key files

- `internal/mcp/tools_feature.go` — overlap helpers and handler integration
- `internal/mcp/tools_test.go` — tests
