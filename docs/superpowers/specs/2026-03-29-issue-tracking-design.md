# Issue Tracking Design

## Purpose

Allow humans and Claude to log bugs found during QA or development against feature cards. Issues are visible on the card with an open count badge, and Claude can be pointed at a card to read and fix the listed bugs. Claude can also file issues against other features when it notices problems during unrelated work.

## Data Model

New `issues` table (schema V6):

```sql
CREATE TABLE IF NOT EXISTS issues (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feature_id TEXT NOT NULL REFERENCES features(id),
    task_item_id INTEGER REFERENCES task_items(id),
    description TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open', 'resolved')),
    resolved_commit TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    resolved_at DATETIME
);
```

Fields:
- `feature_id` (required) — which feature card the issue belongs to
- `task_item_id` (optional) — links to a specific task item if the issue relates to one
- `description` — free text describing the bug
- `status` — `open` or `resolved`
- `resolved_commit` — commit SHA that fixed it, set when resolving
- `resolved_at` — timestamp when resolved

## MCP Tools

### `add_issue`

Log a bug on a feature card.

| Param | Required | Description |
|-------|----------|-------------|
| `feature_id` | yes | Feature slug ID |
| `description` | yes | What's wrong |
| `task_item_id` | no | Link to specific task item ID |

Returns: issue ID and confirmation.

### `resolve_issue`

Mark an issue as fixed.

| Param | Required | Description |
|-------|----------|-------------|
| `id` | yes | Issue ID |
| `commit_hash` | no | Commit that fixed it |

Sets status to `resolved`, `resolved_at` to now, and `resolved_commit` if provided.

### `list_issues`

List open issues.

| Param | Required | Description |
|-------|----------|-------------|
| `feature_id` | no | Filter to one feature. Omit for all open issues across all features. |

Returns issues sorted by creation date (newest first). Only shows open issues by default.

## Dashboard Changes

### Board view (feature cards)

Red badge on cards showing open issue count: `! 3`. Only rendered when open count > 0. Positioned near the progress indicator.

The board API response (`GET /api/features`) adds an `issue_count` field to each feature summary representing the number of open issues.

### Detail panel (feature detail)

New "Issues" section below decisions. Each issue shows:
- Description text
- Status (open or resolved)
- Linked task item title (if `task_item_id` is set)
- Resolved commit hash (if resolved)

Open issues listed first, resolved issues below them.

The detail API response (`GET /api/features/{id}`) adds an `issues` array to the response.

## Store Layer

New file `internal/store/issue.go` following the pattern of `decision.go`:
- `Issue` struct
- `AddIssue(featureID, description string, taskItemID *int64) (*Issue, error)`
- `ResolveIssue(id int64, commitHash string) error`
- `GetIssuesForFeature(featureID string) ([]Issue, error)`
- `GetOpenIssueCount(featureID string) (int, error)`
- `GetAllOpenIssues() ([]Issue, error)`

## Files Changed

- `internal/store/migrate.go` — add schemaV6 with issues table
- `internal/store/issue.go` — new file, Issue struct and store methods
- `internal/store/issue_test.go` — new file, tests
- `internal/mcp/tools.go` — register add_issue, resolve_issue, list_issues tools and handlers
- `internal/dashboard/dashboard.go` — add issue_count to board API, add issues to detail API
- `dashboard/index.html` — badge on cards, issues section in detail panel
