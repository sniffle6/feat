# Spec Viewer — Design Spec

## Problem

Specs and features are completely decoupled. Specs live as markdown files in `docs/superpowers/specs/`, features live in SQLite, and there's no link between them. New Claude sessions picking up a feature have no way to discover the design spec that drove it. Humans browsing the dashboard can't see the spec either.

## Approach

Add a `spec_path` field to features. When set, Claude sessions see it in MCP responses and can read the file. The dashboard shows a teal "Spec" badge on feature cards that opens a modal rendering the markdown.

## Data Model

### New column

```sql
ALTER TABLE features ADD COLUMN spec_path TEXT DEFAULT '';
```

Added as a new migration step in `internal/store/migrate.go`. Empty string means no spec (consistent with other optional text fields like `notes`, `left_off`).

### Store layer

`Feature` struct gets `SpecPath string` field. `AddFeature` and `UpdateFeature` accept it. Standard nullable-to-empty-string handling like the other text columns.

## MCP Tools

### `add_feature` / `update_feature`

New optional `spec_path` string parameter. Accepts a project-relative path (e.g., `docs/superpowers/specs/2026-04-01-foo-design.md`). No validation that the file exists — it may be created later or moved.

### `get_feature` / `get_context`

Include `spec_path` in the response when non-empty. `get_context` is the lightweight status check — it already returns key fields like `left_off`, `key_files`, etc. Adding `spec_path` keeps it useful for sessions that need to know a spec exists without fetching the full feature.

## HTTP API

### `GET /api/spec?path=<relative-path>`

Reads the spec file from disk relative to the project root. Returns raw markdown with `Content-Type: text/plain; charset=utf-8`.

**Safety:** Reject paths that:
- Contain `..` segments (directory traversal)
- Are absolute paths
- Point outside the project root

Returns 400 for invalid paths, 404 if file doesn't exist, 200 with the markdown content otherwise.

## Dashboard

### Spec badge on feature cards

A teal badge (matching the existing tag pill aesthetic) appears on feature cards when `spec_path` is non-empty. Positioned alongside the issue badge row.

```
📄 Spec
```

Styled like the issue badge but with teal coloring (`--teal` / `#4ECDC4`). Only rendered when `spec_path` is present on the feature data.

### Spec viewer modal

Clicking the badge opens a modal overlay:

- **Backdrop:** Semi-transparent overlay (`var(--overlay-bg)`) covering the full viewport. Clicking it closes the modal.
- **Modal container:** Centered, max-width 720px, max-height 80vh, scrollable body.
- **Header:** Feature title, file path in monospace, close button (×).
- **Body:** Markdown rendered client-side via `marked` library loaded from CDN (`https://cdn.jsdelivr.net/npm/marked/marked.min.js`).
- **Close triggers:** × button, Escape key, backdrop click.

### Markdown styling

Basic styles for rendered markdown within the modal:
- Headings use `--primary` color
- Body text uses `--text` / `--secondary`
- Code blocks use `--card-bg` background with monospace font
- Links use `--teal`
- Lists get standard indentation

No syntax highlighting for code blocks — keeps it simple and avoids another dependency.

## Key Files

- `internal/store/migrate.go` — new migration adding `spec_path` column
- `internal/store/store.go` — `Feature` struct, `AddFeature`, `UpdateFeature`, queries
- `internal/mcp/tools_feature.go` — `add_feature`, `update_feature`, `get_feature` handlers
- `internal/mcp/tools_context.go` — `get_context` handler (if separate file)
- `dashboard/index.html` — badge rendering, modal, markdown loading, CSS
- `cmd/docket/main.go` — new `/api/spec` HTTP endpoint

## What This Does NOT Include

- No auto-detection of specs by filename/title matching
- No spec editing from the dashboard
- No spec creation from the dashboard
- No spec versioning or history
- No full-text search of specs
- No spec browser independent of features (specs are accessed through their feature)
