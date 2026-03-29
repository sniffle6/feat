# Issue Tracking

Track bugs and issues found during QA or development on docket feature cards.

## What it does

Issues are bugs logged against a feature card. They have two states: open or resolved. When resolved, the fixing commit hash is recorded for traceability. Issues can optionally link to a specific task item.

## Why it exists

During QA or development, bugs get found. Without a place to log them, they get lost between sessions. Issues give Claude and humans a shared list of known bugs per feature card, visible on the dashboard.

## How to use it

### MCP tools

- `add_issue` — log a bug: `feature_id` (required), `description` (required), `task_item_id` (optional)
- `resolve_issue` — mark fixed: `id` (required), `commit_hash` (optional)
- `list_issues` — see open bugs: `feature_id` (optional, omit for all features)

### Dashboard

- Feature cards show a red `! N issues` badge when there are open issues
- Feature detail panel has an "Issues" section showing all issues (open first, then resolved)

### Workflow

1. Find a bug during QA or development
2. Call `add_issue` with the feature ID and a description of the bug
3. When the bug is fixed, call `resolve_issue` with the issue ID and the commit hash
4. Point Claude at the feature card to fix remaining open issues

## Key files

- `internal/store/issue.go` — Issue struct and store methods
- `internal/store/issue_test.go` — tests
- `internal/mcp/tools.go` — add_issue, resolve_issue, list_issues tool handlers
- `internal/dashboard/dashboard.go` — API endpoints (issue_count on board, issues in detail)
- `dashboard/index.html` — badge on cards, issues section in detail panel
