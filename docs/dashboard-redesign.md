# Dashboard Redesign

Redesign the docket dashboard to better communicate the Feature → Task → Subtask hierarchy. Replace the current flat kanban + modal with a structured board + slide-out panel, themed with Memphis Dark.

## What it does

The dashboard is the human-facing view of docket's feature tracker. It shows all features organized by status, lets you drill into any feature to see its tasks/subtasks/sessions, and gives you a place to write notes that Claude reads when picking up work.

## Why it exists

The current dashboard has problems:
- Cards are too flat — just title + single progress bar, can't tell what tasks make up a feature without clicking
- Detail view is a cramped 640px modal that doesn't have room for complex features
- Sessions are disconnected from the tasks they relate to
- Status dropdowns on cards suggest human control, but status should be Claude-controlled
- No way for humans to write notes/context that Claude reads when resuming work
- Missing `dev_complete` status for code-done-but-not-merged features

## Design Decisions

| Decision | Choice | Why |
|---|---|---|
| Card density | Per-task progress bars | See task-level completion at a glance without clicking |
| Detail view | Slide-out panel (right, 60%) | Board stays visible for context, can swap between features |
| Sessions | Stay feature-level | Task item outcomes already capture per-task context |
| Left off | Non-editable text, top of panel | Claude-owned context for session resumption |
| User notes | New editable field | Human-owned context Claude reads on pickup |
| Board layout | 5-column kanban | Spatial position = status, instant recognition |
| Status control | Read-only, Claude-controlled | No dropdowns, no drag-and-drop |
| Theme | Memphis Noir (dark + light) | Tomato red, lime green, sunny yellow on warm olive-grey (dark) / cream (light) |

## Board Layout

5 columns, left to right following the feature lifecycle:

**Planned** → **In Progress** → **Blocked** → **Dev Complete** → **Done**

Columns are always visible (even empty). Each column has a colored status dot + label + count.

### Feature Cards

Each card shows:
- Feature title (bold)
- Per-task progress rows: task name, count (e.g. 3/4), mini progress bar
- Left-off snippet (truncated, separated by a top border)
- No status dropdown, no drag handles

Done column cards render at reduced opacity (0.65), full opacity on hover.

Card hover: border highlights with `border.strong` (#FF6B9D), subtle glow.

## Slide-out Panel

Clicking a card opens a panel that slides in from the right, covering ~60% of the screen (min 480px, max 800px). The board is dimmed behind with an overlay. ESC or clicking the overlay closes it. Clicking another card while the panel is open swaps the content without close/reopen.

### Panel sections, top to bottom:

1. **Header** — Feature title (large), slug ID + status badge + type badge
2. **Tags** — Editable tag pills with remove buttons + input with autocomplete from known tags. Saves inline via PATCH.
3. **Description** — If present
3. **Left Off** — Callout block with teal left border. Non-editable. Shows Claude's last session context.
4. **Your Notes** — Editable textarea with save button. Placeholder: "Add notes, thoughts, or ideas for Claude..."
5. **Tasks** — Each task (subtask in schema) is a collapsible section:
   - Header: task title + progress count
   - Expanded: subtask items as checklist (checked items muted, unchecked items bright)
   - Under completed items: outcome text + commit hash
   - Archived tasks collapsed by default, dimmed
6. **Key Files** — Monospace file tags
7. **Activity** — Collapsible section (collapsed by default), shows session timeline: date, summary, file tags

## Data Model Changes

### Schema v4 migration

```sql
ALTER TABLE features ADD COLUMN notes TEXT NOT NULL DEFAULT '';
```

`dev_complete` is a new valid value for the existing `status` column — no schema change needed.

### Feature struct

Add `Notes string` to `Feature` and `FeatureUpdate` in `store.go`. Include in all SELECT/UPDATE queries.

### MCP tool updates

- `add_feature` — accept optional `notes` param
- `update_feature` — accept optional `notes` param
- `get_context` / `get_ready` / `get_full_context` — return `notes` so Claude reads user notes on pickup
- Status validation — add `dev_complete` as valid

### API updates (implemented)

- `GET /api/features` — includes `notes`, `subtask_progress` array (title/done/total per subtask)
- `GET /api/features/{id}` — includes `notes` via Feature struct
- `PATCH /api/features/{id}` — accepts `notes` and `tags` in body (dashboard save button / tag editor uses this)
- `GET /api/tags` — returns all known tags across features (for autocomplete)

## Theme: Memphis Noir

Source files: `themes/memphis-noir-dark.json`, `themes/memphis-noir-light.json`

Adopted from the theme-enforcer global library. Authentic Memphis Design — warm olive-grey surfaces with tomato red (#FF6B6B), lime green (#95E06C), and sunny yellow (#FFD166). No purple, no blue.

### Dark mode surfaces (warm olive-grey)

| Token | Hex | Role |
|---|---|---|
| surface.background | #2E2E28 | Page background |
| surface.card | #3C3C34 | Column background, panel background |
| surface.elevated | #4A4A40 | Feature cards, subtask headers |
| surface.muted-foreground | #8A8878 | Column headers, muted text, left-off snippets |
| surface.foreground | #EDEADF | Primary text (warm cream) |
| border.default | #4A4A40 | Card borders, dividers |

### Light mode surfaces (warm cream, heavy outlines)

| Token | Hex | Role |
|---|---|---|
| surface.background | #F3EDE0 | Page background (warm cream) |
| surface.card | #FFF0E0 | Column/panel background (peach tint) |
| border.default | #2A2A24 | Heavy dark outlines (Memphis poster style) |

Light mode uses dark text for column headers (vibrant colors fail 4.5:1 on cream), with colored dots for visual identification. Badges use opaque tinted backgrounds with dark text for contrast.

### Status colors

| Status | Dark | Light | Token |
|---|---|---|---|
| Planned | #8A8878 | #6B6B60 | surface.muted-foreground |
| In Progress | #FF6B6B | #FF6B6B | palette.primary.base |
| Blocked | #C73A3A | #B83030 | semantic.danger.base |
| Dev Complete | #FFD166 | #FFD166 | palette.accent.base |
| Done | #95E06C | #6EBA45 | palette.secondary.base |

### Accent usage

| Element | Color | Token |
|---|---|---|
| Card hover border | #FF6B6B | palette.primary.base |
| Progress bars | #FF6B6B | palette.primary.base |
| Focus indicators | #FF6B6B (dark) / #E04545 (light) | border.focus |
| Left-off callout border | #4ECDC4 | semantic.info.base |
| File tags / commits | #4ECDC4 (dark) / #1A7A72 (light) | semantic.info |
| Save button | #FF6B6B bg, #1A1A16 text | action.primary |
| Panel left border | #FF6B6B | palette.primary.base |

All dark mode contrast pairs verified WCAG AA minimum. Lowest ratio: 5.0:1 (danger/destructive). Light mode uses accessible text overrides for colored elements. See theme JSON files for full contrast_pairs audit.

## Behavior

- **Auto-refresh**: Poll every 10s so the board updates while Claude works in the background
- **Theme toggle**: Keep dark/light toggle in header (light theme TBD, dark is primary)
- **Keyboard**: ESC closes panel, tab navigation works with focus indicators
- **Panel swap**: Click a different card while panel is open → swap content, no close animation

## Key Files

- `dashboard/index.html` — Frontend (full rewrite)
- `themes/memphis-noir-dark.json` — Dark theme definition
- `themes/memphis-noir-light.json` — Light theme definition
- `internal/store/store.go` — Feature struct + queries (add notes field)
- `internal/store/migrate.go` — Schema v4 migration
- `internal/dashboard/dashboard.go` — API handlers (add notes to responses/updates)
- `internal/mcp/tools.go` — MCP tool updates (notes param, dev_complete status)

## Mockup

Interactive mockup at `.superpowers/brainstorm/7163-1774717832/content/memphis-dashboard-mockup.html` — open locally to see the full board + slide-out panel with sample data.
