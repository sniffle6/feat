# Feature Templates & Acceptance Criteria Gate — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add feature type templates for consistent feature creation, and a soft gate preventing features from being marked done with unchecked task items or open issues.

**Architecture:** Two independent changes to the store layer, exposed through existing MCP tools. Feature type is a new column + template application logic. Completion gate is validation logic in `UpdateFeature` with force bypass that auto-logs decisions.

**Tech Stack:** Go, SQLite, mcp-go

---

### Task 1: Schema Migration — Add `type` Column

**Files:**
- Modify: `internal/store/migrate.go:86-102`

- [ ] **Step 1: Write the migration constant**

Add after `schemaV6`:

```go
const schemaV7 = `
ALTER TABLE features ADD COLUMN type TEXT NOT NULL DEFAULT '';
`
```

- [ ] **Step 2: Wire it into migrate()**

Add after `db.Exec(schemaV6)`:

```go
// v7: add type column to features (ignore error if already exists)
db.Exec(schemaV7)
```

- [ ] **Step 3: Run tests to verify migration is idempotent**

Run: `cd "H:/claude code/tools/docket" && go test ./internal/store/...`
Expected: All existing tests pass (migration runs on every Open, errors ignored for existing columns).

- [ ] **Step 4: Commit**

```bash
git add internal/store/migrate.go
git commit -m "feat: add type column to features table (schema v7)"
```

---

### Task 2: Feature Struct — Add `Type` Field

**Files:**
- Modify: `internal/store/store.go:16-27` (Feature struct)
- Modify: `internal/store/store.go:85-96` (AddFeature)
- Modify: `internal/store/store.go:98-114` (GetFeature)
- Modify: `internal/store/store.go:166-193` (ListFeatures)
- Modify: `internal/store/store.go:306-328` (GetReadyFeatures)

- [ ] **Step 1: Write the test**

Create `internal/store/type_test.go`:

```go
package store

import "testing"

func TestFeatureTypeField(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, err := s.AddFeature("My Feature", "desc")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}
	if f.Type != "" {
		t.Fatalf("expected empty type, got %q", f.Type)
	}

	// Update type
	typ := "bugfix"
	s.UpdateFeature(f.ID, FeatureUpdate{Type: &typ})
	f, _ = s.GetFeature(f.ID)
	if f.Type != "bugfix" {
		t.Fatalf("expected bugfix, got %q", f.Type)
	}
}

func TestFeatureTypeInList(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Typed Feature", "desc")
	typ := "feature"
	s.UpdateFeature("typed-feature", FeatureUpdate{Type: &typ})

	features, _ := s.ListFeatures("")
	if len(features) != 1 || features[0].Type != "feature" {
		t.Fatalf("expected type=feature in list, got %q", features[0].Type)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "H:/claude code/tools/docket" && go test ./internal/store/ -run TestFeatureType -v`
Expected: FAIL — `Type` field doesn't exist on Feature struct yet.

- [ ] **Step 3: Add Type field to Feature struct**

In `internal/store/store.go`, add `Type` field to the `Feature` struct after `Status`:

```go
type Feature struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Status       string    `json:"status"`
	Type         string    `json:"type"`
	LeftOff      string    `json:"left_off"`
	Notes        string    `json:"notes"`
	KeyFiles     []string  `json:"key_files"`
	WorktreePath string    `json:"worktree_path"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
```

Add `Type` field to `FeatureUpdate`:

```go
type FeatureUpdate struct {
	Title        *string   `json:"title,omitempty"`
	Description  *string   `json:"description,omitempty"`
	Status       *string   `json:"status,omitempty"`
	Type         *string   `json:"type,omitempty"`
	LeftOff      *string   `json:"left_off,omitempty"`
	Notes        *string   `json:"notes,omitempty"`
	KeyFiles     *[]string `json:"key_files,omitempty"`
	WorktreePath *string   `json:"worktree_path,omitempty"`
	Force        *bool     `json:"force,omitempty"`
	ForceReason  *string   `json:"force_reason,omitempty"`
}
```

- [ ] **Step 4: Update all SQL queries that read Feature fields**

Every SELECT that reads features needs `type` added. There are 4 scan sites:

**GetFeature** — add `type` to SELECT and Scan:
```go
row := s.db.QueryRow(
    `SELECT id, title, description, status, type, left_off, notes, key_files, worktree_path, created_at, updated_at FROM features WHERE id = ?`,
    id,
)
var f Feature
var keyFilesJSON string
err := row.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.Type, &f.LeftOff, &f.Notes, &keyFilesJSON, &f.WorktreePath, &f.CreatedAt, &f.UpdatedAt)
```

**ListFeatures** — same pattern:
```go
query := `SELECT id, title, description, status, type, left_off, notes, key_files, worktree_path, created_at, updated_at FROM features`
```
And in the scan:
```go
if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.Type, &f.LeftOff, &f.Notes, &keyFilesJSON, &f.WorktreePath, &f.CreatedAt, &f.UpdatedAt); err != nil {
```

**GetReadyFeatures** — same pattern:
```go
`SELECT id, title, description, status, type, left_off, notes, key_files, worktree_path, created_at, updated_at FROM features WHERE status IN ('in_progress', 'planned') ORDER BY CASE WHEN status='in_progress' THEN 0 ELSE 1 END, updated_at DESC`
```
And in the scan:
```go
if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.Type, &f.LeftOff, &f.Notes, &keyFilesJSON, &f.WorktreePath, &f.CreatedAt, &f.UpdatedAt); err != nil {
```

**UpdateFeature** — add Type handling:
```go
if u.Type != nil {
    sets = append(sets, "type = ?")
    args = append(args, *u.Type)
}
```

- [ ] **Step 5: Run tests**

Run: `cd "H:/claude code/tools/docket" && go test ./internal/store/ -v`
Expected: ALL tests pass, including the new type tests.

- [ ] **Step 6: Commit**

```bash
git add internal/store/store.go internal/store/type_test.go
git commit -m "feat: add Type field to Feature struct and all queries"
```

---

### Task 3: Feature Templates

**Files:**
- Create: `internal/store/templates.go`
- Create: `internal/store/templates_test.go`

- [ ] **Step 1: Write the test**

Create `internal/store/templates_test.go`:

```go
package store

import "testing"

func TestApplyTemplate(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Login Bug", "broken")
	err := s.ApplyTemplate(f.ID, "bugfix")
	if err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}

	subtasks, _ := s.GetSubtasksForFeature(f.ID, false)
	if len(subtasks) != 2 {
		t.Fatalf("expected 2 subtasks, got %d", len(subtasks))
	}
	if subtasks[0].Title != "Investigation" {
		t.Fatalf("expected Investigation, got %q", subtasks[0].Title)
	}
	if len(subtasks[0].Items) != 2 {
		t.Fatalf("expected 2 items in Investigation, got %d", len(subtasks[0].Items))
	}
	if subtasks[0].Items[0].Title != "Reproduce the bug" {
		t.Fatalf("expected 'Reproduce the bug', got %q", subtasks[0].Items[0].Title)
	}
	if subtasks[1].Title != "Fix" {
		t.Fatalf("expected Fix, got %q", subtasks[1].Title)
	}
}

func TestApplyTemplateUnknownType(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Something", "desc")
	err := s.ApplyTemplate(f.ID, "unknown")
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestApplyTemplateAllTypes(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	for _, typ := range []string{"feature", "bugfix", "chore", "spike"} {
		f, _ := s.AddFeature("Test "+typ, "desc")
		err := s.ApplyTemplate(f.ID, typ)
		if err != nil {
			t.Fatalf("ApplyTemplate(%s): %v", typ, err)
		}
		subtasks, _ := s.GetSubtasksForFeature(f.ID, false)
		if len(subtasks) == 0 {
			t.Fatalf("type %s: expected subtasks, got 0", typ)
		}
		for _, st := range subtasks {
			if len(st.Items) == 0 {
				t.Fatalf("type %s, subtask %q: expected items, got 0", typ, st.Title)
			}
		}
	}
}

func TestValidFeatureTypes(t *testing.T) {
	valid := ValidFeatureTypes()
	if len(valid) != 4 {
		t.Fatalf("expected 4 valid types, got %d", len(valid))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "H:/claude code/tools/docket" && go test ./internal/store/ -run TestApplyTemplate -v`
Expected: FAIL — `ApplyTemplate` doesn't exist.

- [ ] **Step 3: Write templates.go**

Create `internal/store/templates.go`:

```go
package store

import (
	"fmt"
	"sort"
)

type TemplateSubtask struct {
	Title string
	Items []string
}

var featureTemplates = map[string][]TemplateSubtask{
	"feature": {
		{Title: "Planning", Items: []string{"Define acceptance criteria", "Identify key files"}},
		{Title: "Implementation", Items: []string{"Implement core logic", "Add tests"}},
		{Title: "Polish", Items: []string{"Update documentation", "Final review"}},
	},
	"bugfix": {
		{Title: "Investigation", Items: []string{"Reproduce the bug", "Identify root cause"}},
		{Title: "Fix", Items: []string{"Implement fix", "Add regression test"}},
	},
	"chore": {
		{Title: "Work", Items: []string{"Implement changes", "Verify no regressions"}},
	},
	"spike": {
		{Title: "Research", Items: []string{"Explore approaches", "Document findings"}},
	},
}

func ValidFeatureTypes() []string {
	keys := make([]string, 0, len(featureTemplates))
	for k := range featureTemplates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *Store) ApplyTemplate(featureID, typ string) error {
	tmpl, ok := featureTemplates[typ]
	if !ok {
		return fmt.Errorf("unknown feature type %q (valid: %v)", typ, ValidFeatureTypes())
	}

	for pos, st := range tmpl {
		subtask, err := s.AddSubtask(featureID, st.Title, pos+1)
		if err != nil {
			return fmt.Errorf("add subtask %q: %w", st.Title, err)
		}
		for itemPos, itemTitle := range st.Items {
			if _, err := s.AddTaskItem(subtask.ID, itemTitle, itemPos+1); err != nil {
				return fmt.Errorf("add task item %q: %w", itemTitle, err)
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd "H:/claude code/tools/docket" && go test ./internal/store/ -run TestApplyTemplate -v`
Expected: ALL pass.

- [ ] **Step 5: Run all store tests**

Run: `cd "H:/claude code/tools/docket" && go test ./internal/store/ -v`
Expected: ALL pass.

- [ ] **Step 6: Commit**

```bash
git add internal/store/templates.go internal/store/templates_test.go
git commit -m "feat: add feature templates with ApplyTemplate store method"
```

---

### Task 4: Wire `type` Into `add_feature` MCP Tool

**Files:**
- Modify: `internal/mcp/tools.go:16-22` (tool registration)
- Modify: `internal/mcp/tools.go:152-178` (handler)

- [ ] **Step 1: Write the test**

Add to `internal/mcp/tools_test.go`:

```go
func TestAddFeatureWithType(t *testing.T) {
	s, _ := store.Open(t.TempDir())
	defer s.Close()

	f, err := s.AddFeature("Typed Bug", "a bug")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}

	// Set type and apply template (mimics what the MCP handler will do)
	typ := "bugfix"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Type: &typ})
	s.ApplyTemplate(f.ID, "bugfix")

	f, _ = s.GetFeature(f.ID)
	if f.Type != "bugfix" {
		t.Fatalf("expected type=bugfix, got %q", f.Type)
	}

	subtasks, _ := s.GetSubtasksForFeature(f.ID, false)
	if len(subtasks) != 2 {
		t.Fatalf("expected 2 subtasks from bugfix template, got %d", len(subtasks))
	}
}

func TestAddFeatureWithInvalidType(t *testing.T) {
	s, _ := store.Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Bad Type", "desc")
	err := s.ApplyTemplate(f.ID, "invalid")
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}
```

- [ ] **Step 2: Run test to verify it passes** (these test the store directly, already implemented)

Run: `cd "H:/claude code/tools/docket" && go test ./internal/mcp/ -run TestAddFeatureWith -v`
Expected: PASS.

- [ ] **Step 3: Update add_feature tool registration**

In `registerTools`, update the `add_feature` tool to include the `type` parameter:

```go
srv.AddTool(mcp.NewTool("add_feature",
    mcp.WithDescription("Create a new feature to track. Returns the generated slug ID."),
    mcp.WithString("title", mcp.Required(), mcp.Description("Feature title (e.g., 'Bluetooth Panel')")),
    mcp.WithString("description", mcp.Description("What the feature is")),
    mcp.WithString("status", mcp.Description("Initial status: planned (default), in_progress, blocked, dev_complete")),
    mcp.WithString("notes", mcp.Description("User notes — thoughts, ideas, context for Claude to read when picking up this feature")),
    mcp.WithString("type", mcp.Description("Feature type: feature, bugfix, chore, spike. Auto-creates subtasks from template when set.")),
), addFeatureHandler(s))
```

- [ ] **Step 4: Update addFeatureHandler to apply template**

```go
func addFeatureHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		title, ok := argString(args, "title")
		if !ok || title == "" {
			return mcp.NewToolResultError("missing required parameter: title"), nil
		}
		desc, _ := argString(args, "description")
		status, _ := argString(args, "status")
		notes, _ := argString(args, "notes")
		typ, _ := argString(args, "type")

		f, err := s.AddFeature(title, desc)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if status != "" && status != "planned" {
			s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})
		}
		if notes != "" {
			s.UpdateFeature(f.ID, store.FeatureUpdate{Notes: &notes})
		}
		if typ != "" {
			if err := s.ApplyTemplate(f.ID, typ); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("feature created but template failed: %v", err)), nil
			}
			s.UpdateFeature(f.ID, store.FeatureUpdate{Type: &typ})
		}
		f, _ = s.GetFeature(f.ID)

		data, _ := json.MarshalIndent(f, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
```

- [ ] **Step 5: Run all tests**

Run: `cd "H:/claude code/tools/docket" && go test ./... -v`
Expected: ALL pass.

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat: add type parameter to add_feature MCP tool with template application"
```

---

### Task 5: Completion Gate in `UpdateFeature`

**Files:**
- Modify: `internal/store/store.go:116-163` (UpdateFeature)

- [ ] **Step 1: Write the test**

Add to `internal/store/type_test.go`:

```go
func TestCompletionGateBlocksDone(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Gated Feature", "desc")
	s.ApplyTemplate(f.ID, "bugfix")

	// Try to mark done with unchecked items
	done := "done"
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &done})
	if err == nil {
		t.Fatal("expected error when marking done with unchecked items")
	}

	// Verify feature is NOT done
	f, _ = s.GetFeature(f.ID)
	if f.Status == "done" {
		t.Fatal("feature should not be done")
	}
}

func TestCompletionGateAllowsForce(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Force Feature", "desc")
	s.ApplyTemplate(f.ID, "chore")

	done := "done"
	force := true
	reason := "Decided items are not needed"
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &done, Force: &force, ForceReason: &reason})
	if err != nil {
		t.Fatalf("force completion should succeed: %v", err)
	}

	f, _ = s.GetFeature(f.ID)
	if f.Status != "done" {
		t.Fatalf("expected done, got %q", f.Status)
	}

	// Check decision was logged
	decisions, _ := s.GetDecisionsForFeature(f.ID)
	if len(decisions) == 0 {
		t.Fatal("expected a decision logged for force completion")
	}
	if decisions[0].Outcome != "accepted" {
		t.Fatalf("expected accepted, got %q", decisions[0].Outcome)
	}
}

func TestCompletionGatePassesWhenAllDone(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Complete Feature", "desc")
	s.ApplyTemplate(f.ID, "chore") // 1 subtask, 2 items

	subtasks, _ := s.GetSubtasksForFeature(f.ID, false)
	for _, st := range subtasks {
		for _, item := range st.Items {
			s.CompleteTaskItem(item.ID, TaskItemCompletion{Outcome: "done"})
		}
	}

	done := "done"
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &done})
	if err != nil {
		t.Fatalf("should pass gate when all items checked: %v", err)
	}
}

func TestCompletionGateNoSubtasks(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Empty Feature", "desc")

	done := "done"
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &done})
	if err != nil {
		t.Fatalf("should pass gate with no subtasks: %v", err)
	}
}

func TestCompletionGateOpenIssues(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Issue Feature", "desc")
	s.AddIssue(f.ID, "something is broken", nil)

	done := "done"
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &done})
	if err == nil {
		t.Fatal("expected error when marking done with open issues")
	}
}

func TestCompletionGateNonDoneStatusSkipsCheck(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Status Feature", "desc")
	s.ApplyTemplate(f.ID, "bugfix")

	// Moving to in_progress should work even with unchecked items
	inProgress := "in_progress"
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &inProgress})
	if err != nil {
		t.Fatalf("non-done status should skip gate: %v", err)
	}
}

func TestCompletionGateForceNoReason(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("No Reason", "desc")
	s.ApplyTemplate(f.ID, "spike")

	done := "done"
	force := true
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &done, Force: &force})
	if err != nil {
		t.Fatalf("force without reason should succeed: %v", err)
	}

	decisions, _ := s.GetDecisionsForFeature(f.ID)
	if len(decisions) == 0 {
		t.Fatal("expected decision logged")
	}
	if decisions[0].Reason != "No reason given" {
		t.Fatalf("expected 'No reason given', got %q", decisions[0].Reason)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd "H:/claude code/tools/docket" && go test ./internal/store/ -run TestCompletionGate -v`
Expected: FAIL — `Force` and `ForceReason` fields don't exist on `FeatureUpdate` yet (added in Task 2 Step 3 but the gate logic isn't implemented).

Note: The `Force` and `ForceReason` fields were already added to `FeatureUpdate` in Task 2 Step 3. The tests fail because the gate logic in `UpdateFeature` doesn't exist yet.

- [ ] **Step 3: Add completion gate logic to UpdateFeature**

Replace the `UpdateFeature` method in `internal/store/store.go`:

```go
func (s *Store) UpdateFeature(id string, u FeatureUpdate) error {
	// Completion gate: check prerequisites before allowing status=done
	if u.Status != nil && *u.Status == "done" {
		unchecked, total, _ := s.GetFeatureProgress(id)
		uncheckedCount := total - unchecked
		openIssues, _ := s.GetOpenIssueCount(id)

		if (uncheckedCount > 0 || openIssues > 0) && (u.Force == nil || !*u.Force) {
			return fmt.Errorf("cannot mark feature %q as done: %d unchecked task items, %d open issues (use force=true to override)", id, uncheckedCount, openIssues)
		}

		if (uncheckedCount > 0 || openIssues > 0) && u.Force != nil && *u.Force {
			reason := "No reason given"
			if u.ForceReason != nil && *u.ForceReason != "" {
				reason = *u.ForceReason
			}
			approach := fmt.Sprintf("Force-completed with %d unchecked items, %d open issues", uncheckedCount, openIssues)
			s.AddDecision(id, approach, "accepted", reason)
		}
	}

	sets := []string{}
	args := []any{}
	if u.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *u.Title)
	}
	if u.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *u.Description)
	}
	if u.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *u.Status)
	}
	if u.Type != nil {
		sets = append(sets, "type = ?")
		args = append(args, *u.Type)
	}
	if u.LeftOff != nil {
		sets = append(sets, "left_off = ?")
		args = append(args, *u.LeftOff)
	}
	if u.Notes != nil {
		sets = append(sets, "notes = ?")
		args = append(args, *u.Notes)
	}
	if u.KeyFiles != nil {
		kf, _ := json.Marshal(*u.KeyFiles)
		sets = append(sets, "key_files = ?")
		args = append(args, string(kf))
	}
	if u.WorktreePath != nil {
		sets = append(sets, "worktree_path = ?")
		args = append(args, *u.WorktreePath)
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().UTC())
	args = append(args, id)
	query := fmt.Sprintf("UPDATE features SET %s WHERE id = ?", strings.Join(sets, ", "))
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update feature: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("feature %q not found", id)
	}
	return nil
}
```

- [ ] **Step 4: Run completion gate tests**

Run: `cd "H:/claude code/tools/docket" && go test ./internal/store/ -run TestCompletionGate -v`
Expected: ALL pass.

- [ ] **Step 5: Run all tests**

Run: `cd "H:/claude code/tools/docket" && go test ./... -v`
Expected: ALL pass. The existing `QuickTrack` tests that set status=done should still pass because those features have no subtasks (gate passes with 0 items).

- [ ] **Step 6: Commit**

```bash
git add internal/store/store.go internal/store/type_test.go
git commit -m "feat: add completion gate — blocks done with unchecked items or open issues"
```

---

### Task 6: Wire `force`/`force_reason` Into `update_feature` MCP Tool

**Files:**
- Modify: `internal/mcp/tools.go:24-34` (tool registration)
- Modify: `internal/mcp/tools.go:181-222` (handler)

- [ ] **Step 1: Update update_feature tool registration**

Add force and force_reason parameters:

```go
srv.AddTool(mcp.NewTool("update_feature",
    mcp.WithDescription("Update a feature's status, description, left_off note, notes, worktree_path, or key_files."),
    mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
    mcp.WithString("status", mcp.Description("New status: planned, in_progress, done, blocked, dev_complete")),
    mcp.WithString("title", mcp.Description("New title")),
    mcp.WithString("description", mcp.Description("New description")),
    mcp.WithString("left_off", mcp.Description("Where work stopped — free text")),
    mcp.WithString("notes", mcp.Description("User notes — thoughts, ideas, context for Claude")),
    mcp.WithString("worktree_path", mcp.Description("Absolute path to git worktree")),
    mcp.WithString("key_files", mcp.Description("Comma-separated list of key file paths for this feature")),
    mcp.WithBoolean("force", mcp.Description("Force status=done even with unchecked task items or open issues. Logs a decision.")),
    mcp.WithString("force_reason", mcp.Description("Reason for force-completing (logged as a decision)")),
), updateFeatureHandler(s))
```

- [ ] **Step 2: Update updateFeatureHandler to pass force/force_reason**

In the handler, add after the key_files handling:

```go
if v, ok := args["force"]; ok {
    if b, ok := v.(bool); ok && b {
        force := true
        u.Force = &force
    }
}
if v, ok := argString(args, "force_reason"); ok && v != "" {
    u.ForceReason = &v
}
```

- [ ] **Step 3: Run all tests**

Run: `cd "H:/claude code/tools/docket" && go test ./... -v`
Expected: ALL pass.

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/tools.go
git commit -m "feat: add force/force_reason params to update_feature MCP tool"
```

---

### Task 7: Dashboard — Show Feature Type

**Files:**
- Modify: `dashboard/index.html`

- [ ] **Step 1: Find the feature card rendering in the dashboard HTML**

Search for where feature title/status are rendered. Look for the status badge pattern. The type badge should go next to it.

- [ ] **Step 2: Add type badge rendering**

In the feature card template/rendering code, after the status badge, add a type badge when `feature.type` is non-empty:

```javascript
// After status badge rendering, add:
if (f.type) {
    // Add a type badge with a distinct style (e.g., outline badge)
    badge += ` <span class="type-badge">${f.type}</span>`;
}
```

Add CSS for the type badge — a simple outlined pill that doesn't compete with the status badge:

```css
.type-badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 12px;
    font-size: 0.75em;
    border: 1px solid #888;
    color: #888;
    margin-left: 4px;
}
```

The exact implementation depends on the dashboard's current HTML structure — read the file first and follow the existing pattern.

- [ ] **Step 3: Run dashboard tests**

Run: `cd "H:/claude code/tools/docket" && go test ./internal/dashboard/ -v`
Expected: ALL pass.

- [ ] **Step 4: Commit**

```bash
git add dashboard/index.html
git commit -m "feat: show feature type badge in dashboard"
```

---

### Task 8: Update Documentation

**Files:**
- Modify: `docs/feature-templates.md` (create if not exists)

- [ ] **Step 1: Check existing docs**

Run: `ls "H:/claude code/tools/docket/docs/"` to see what docs exist.

- [ ] **Step 2: Write the doc**

Create `docs/feature-templates.md`:

```markdown
# Feature Templates & Completion Gate

## What It Does

Two features that make docket's feature tracking more consistent and reliable:

1. **Feature types with templates** — when creating a feature, pass a `type` (feature, bugfix, chore, spike) to auto-generate a standard subtask structure.
2. **Completion gate** — features can't be marked `done` until all task items are checked and all issues are resolved. Override with `force=true` (logs a decision).

## How to Use

### Creating a typed feature (MCP tool)

```json
{
    "title": "Fix login timeout",
    "type": "bugfix"
}
```

This creates the feature AND generates subtasks:
- Investigation: Reproduce the bug, Identify root cause
- Fix: Implement fix, Add regression test

### Available types and their templates

- **feature** — Planning → Implementation → Polish
- **bugfix** — Investigation → Fix
- **chore** — Work (single phase)
- **spike** — Research (single phase)

### Completion gate

Marking `status=done` checks:
- All task items on non-archived subtasks must be checked
- All issues must be resolved

If either fails, the update is rejected with a message listing what's outstanding.

To override: pass `force=true` and optionally `force_reason="why"`. This logs an accepted decision on the feature for audit.

## Gotchas

- Templates are fire-and-forget. Once created, subtasks are independent of the type.
- `import_plan` archives template-generated subtasks (same as any existing subtasks).
- Features with no subtasks pass the completion gate (nothing to check).
- `quick_track` bypasses the gate because it calls `UpdateFeature` directly — but quick-tracked features rarely have subtasks.

## Key Files

- `internal/store/templates.go` — template definitions and ApplyTemplate
- `internal/store/store.go` — Feature struct (Type field), UpdateFeature (completion gate)
- `internal/store/migrate.go` — schema v7 (type column)
- `internal/mcp/tools.go` — add_feature (type param), update_feature (force/force_reason params)
```

- [ ] **Step 3: Commit**

```bash
git add docs/feature-templates.md
git commit -m "docs: feature templates and completion gate"
```

---

### Task 9: Build and Verify

- [ ] **Step 1: Run full test suite**

Run: `cd "H:/claude code/tools/docket" && go test ./... -v`
Expected: ALL pass, zero failures.

- [ ] **Step 2: Build the binary**

Run: `cd "H:/claude code/tools/docket" && go build -ldflags="-s -w" -o docket.exe ./cmd/docket/`
Expected: Builds successfully with no errors.

- [ ] **Step 3: Verify binary runs**

Run: `cd "H:/claude code/tools/docket" && ./docket.exe version`
Expected: Prints version info without errors.
