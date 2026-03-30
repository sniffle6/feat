# Tags & Filtering + Feature Archival Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add free-form tags to features (with new-tag warnings) and an `archived` status with auto-archive of stale done features.

**Architecture:** Tags stored as a JSON array column on `features` table (same pattern as `key_files`). Archival is a new status value — no new tables or columns. Auto-archive runs in the SessionStart hook. Dashboard gets tag pills on cards and an archived toggle.

**Tech Stack:** Go, SQLite, vanilla JS (single-file dashboard)

---

### Task 1: Schema migration — add tags column

**Files:**
- Modify: `internal/store/migrate.go`

- [ ] **Step 1: Add schemaV8 constant**

```go
const schemaV8 = `
ALTER TABLE features ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
`
```

Add this after the existing `schemaV7` constant.

- [ ] **Step 2: Call schemaV8 in migrate()**

Add this line after `db.Exec(schemaV7)`:

```go
// v8: add tags column to features (ignore error if already exists)
db.Exec(schemaV8)
```

- [ ] **Step 3: Run tests to verify migration is safe**

Run: `go test ./internal/store/ -run TestFeatureTypeField -v`
Expected: PASS (existing tests still work — migration is additive)

- [ ] **Step 4: Commit**

```bash
git add internal/store/migrate.go
git commit -m "feat: add schemaV8 — tags column on features table"
```

---

### Task 2: Store layer — Tags on Feature structs and scan sites

**Files:**
- Modify: `internal/store/store.go`

- [ ] **Step 1: Write failing test for tags roundtrip**

Create `internal/store/tags_test.go`:

```go
package store

import "testing"

func TestFeatureTagsField(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, err := s.AddFeature("Tagged Feature", "desc")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}
	if len(f.Tags) != 0 {
		t.Fatalf("expected empty tags, got %v", f.Tags)
	}

	tags := []string{"auth", "frontend"}
	s.UpdateFeature(f.ID, FeatureUpdate{Tags: &tags})
	f, _ = s.GetFeature(f.ID)
	if len(f.Tags) != 2 || f.Tags[0] != "auth" || f.Tags[1] != "frontend" {
		t.Fatalf("expected [auth frontend], got %v", f.Tags)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestFeatureTagsField -v`
Expected: FAIL — `Feature` struct has no `Tags` field

- [ ] **Step 3: Add Tags field to Feature struct**

In `store.go`, add to `Feature` struct after `KeyFiles`:

```go
Tags         []string  `json:"tags"`
```

Add to `FeatureUpdate` struct after `KeyFiles`:

```go
Tags         *[]string `json:"tags,omitempty"`
```

- [ ] **Step 4: Update GetFeature to scan tags**

In `GetFeature`, update the SELECT to include `tags`:

```go
row := s.db.QueryRow(
    `SELECT id, title, description, status, type, left_off, notes, key_files, tags, worktree_path, created_at, updated_at FROM features WHERE id = ?`,
    id,
)
```

Add a `tagsJSON` variable alongside `keyFilesJSON`:

```go
var keyFilesJSON, tagsJSON string
err := row.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.Type, &f.LeftOff, &f.Notes, &keyFilesJSON, &tagsJSON, &f.WorktreePath, &f.CreatedAt, &f.UpdatedAt)
```

After the `KeyFiles` unmarshal block, add:

```go
json.Unmarshal([]byte(tagsJSON), &f.Tags)
if f.Tags == nil {
    f.Tags = []string{}
}
```

- [ ] **Step 5: Update ListFeatures to scan tags**

Same pattern — add `tags` after `key_files` in the SELECT, add `tagsJSON` variable, scan it, unmarshal:

```go
query := `SELECT id, title, description, status, type, left_off, notes, key_files, tags, worktree_path, created_at, updated_at FROM features`
```

In the `rows.Next()` loop, change the scan to include `tagsJSON`:

```go
var keyFilesJSON, tagsJSON string
if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.Type, &f.LeftOff, &f.Notes, &keyFilesJSON, &tagsJSON, &f.WorktreePath, &f.CreatedAt, &f.UpdatedAt); err != nil {
```

After keyFiles unmarshal:

```go
json.Unmarshal([]byte(tagsJSON), &f.Tags)
if f.Tags == nil {
    f.Tags = []string{}
}
```

- [ ] **Step 6: Update GetReadyFeatures to scan tags**

Same pattern as ListFeatures — add `tags` to SELECT, add `tagsJSON` to scan, unmarshal.

The SELECT becomes:

```go
`SELECT id, title, description, status, type, left_off, notes, key_files, tags, worktree_path, created_at, updated_at FROM features WHERE status IN ('in_progress', 'planned') ORDER BY CASE WHEN status='in_progress' THEN 0 ELSE 1 END, updated_at DESC`
```

Scan and unmarshal pattern identical to ListFeatures.

- [ ] **Step 7: Add tags handling to UpdateFeature**

After the `KeyFiles` block in `UpdateFeature`, add:

```go
if u.Tags != nil {
    t, _ := json.Marshal(*u.Tags)
    sets = append(sets, "tags = ?")
    args = append(args, string(t))
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestFeatureTagsField -v`
Expected: PASS

- [ ] **Step 9: Run all store tests to check nothing broke**

Run: `go test ./internal/store/ -v`
Expected: All PASS

- [ ] **Step 10: Commit**

```bash
git add internal/store/store.go internal/store/tags_test.go
git commit -m "feat: add Tags field to Feature, update all scan sites"
```

---

### Task 3: Store layer — tags in ListFeatures filter and tag utility methods

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/store/tags_test.go`

- [ ] **Step 1: Write failing test for ListFeatures tag filter**

Append to `internal/store/tags_test.go`:

```go
func TestFeatureTagsInList(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Auth Feature", "desc")
	s.AddFeature("UI Feature", "desc")

	authTags := []string{"auth", "backend"}
	uiTags := []string{"frontend", "ui"}
	s.UpdateFeature("auth-feature", FeatureUpdate{Tags: &authTags})
	s.UpdateFeature("ui-feature", FeatureUpdate{Tags: &uiTags})

	features, _ := s.ListFeatures("")
	if len(features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(features))
	}
	if len(features[0].Tags) == 0 {
		t.Fatal("expected tags in list results")
	}
}

func TestListFeaturesFilterByTag(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Auth Feature", "")
	s.AddFeature("UI Feature", "")
	s.AddFeature("Mixed Feature", "")

	authTags := []string{"auth"}
	uiTags := []string{"ui"}
	mixedTags := []string{"auth", "ui"}
	s.UpdateFeature("auth-feature", FeatureUpdate{Tags: &authTags})
	s.UpdateFeature("ui-feature", FeatureUpdate{Tags: &uiTags})
	s.UpdateFeature("mixed-feature", FeatureUpdate{Tags: &mixedTags})

	features, err := s.ListFeaturesWithTag("", "auth")
	if err != nil {
		t.Fatalf("ListFeaturesWithTag: %v", err)
	}
	if len(features) != 2 {
		t.Fatalf("expected 2 features with auth tag, got %d", len(features))
	}

	// Combined with status filter
	ip := "in_progress"
	s.UpdateFeature("auth-feature", FeatureUpdate{Status: &ip})
	features, _ = s.ListFeaturesWithTag("in_progress", "auth")
	if len(features) != 1 || features[0].ID != "auth-feature" {
		t.Fatalf("expected 1 in_progress feature with auth tag, got %d", len(features))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestListFeaturesFilterByTag -v`
Expected: FAIL — `ListFeaturesWithTag` undefined

- [ ] **Step 3: Implement ListFeaturesWithTag**

Add this method to `store.go` after `ListFeatures`:

```go
func (s *Store) ListFeaturesWithTag(status, tag string) ([]Feature, error) {
	query := `SELECT id, title, description, status, type, left_off, notes, key_files, tags, worktree_path, created_at, updated_at FROM features WHERE tags LIKE ?`
	args := []any{"%" + `"` + tag + `"` + "%"}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY updated_at DESC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list features by tag: %w", err)
	}
	defer rows.Close()
	var features []Feature
	for rows.Next() {
		var f Feature
		var keyFilesJSON, tagsJSON string
		if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.Type, &f.LeftOff, &f.Notes, &keyFilesJSON, &tagsJSON, &f.WorktreePath, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan feature: %w", err)
		}
		json.Unmarshal([]byte(keyFilesJSON), &f.KeyFiles)
		if f.KeyFiles == nil {
			f.KeyFiles = []string{}
		}
		json.Unmarshal([]byte(tagsJSON), &f.Tags)
		if f.Tags == nil {
			f.Tags = []string{}
		}
		features = append(features, f)
	}
	return features, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -run "TestFeatureTagsInList|TestListFeaturesFilterByTag" -v`
Expected: PASS

- [ ] **Step 5: Write failing tests for GetKnownTags and CheckNewTags**

Append to `tags_test.go`:

```go
func TestGetKnownTags(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	// No features — no tags
	tags, _ := s.GetKnownTags()
	if len(tags) != 0 {
		t.Fatalf("expected 0 tags, got %d", len(tags))
	}

	s.AddFeature("Feature A", "")
	s.AddFeature("Feature B", "")
	tagsA := []string{"backend", "auth"}
	tagsB := []string{"auth", "frontend"}
	s.UpdateFeature("feature-a", FeatureUpdate{Tags: &tagsA})
	s.UpdateFeature("feature-b", FeatureUpdate{Tags: &tagsB})

	tags, err := s.GetKnownTags()
	if err != nil {
		t.Fatalf("GetKnownTags: %v", err)
	}
	// Should be sorted and deduplicated: auth, backend, frontend
	if len(tags) != 3 {
		t.Fatalf("expected 3 unique tags, got %d: %v", len(tags), tags)
	}
	if tags[0] != "auth" || tags[1] != "backend" || tags[2] != "frontend" {
		t.Fatalf("expected [auth backend frontend], got %v", tags)
	}
}

func TestCheckNewTags(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Feature A", "")
	existing := []string{"auth", "frontend"}
	s.UpdateFeature("feature-a", FeatureUpdate{Tags: &existing})

	// "auth" exists, "frntend" is new
	newTags := s.CheckNewTags([]string{"auth", "frntend"})
	if len(newTags) != 1 || newTags[0] != "frntend" {
		t.Fatalf("expected [frntend], got %v", newTags)
	}

	// All existing — no new
	newTags = s.CheckNewTags([]string{"auth", "frontend"})
	if len(newTags) != 0 {
		t.Fatalf("expected no new tags, got %v", newTags)
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test ./internal/store/ -run "TestGetKnownTags|TestCheckNewTags" -v`
Expected: FAIL — methods undefined

- [ ] **Step 7: Implement GetKnownTags and CheckNewTags**

Add to `store.go`:

```go
func (s *Store) GetKnownTags() ([]string, error) {
	rows, err := s.db.Query(`SELECT tags FROM features WHERE tags != '[]'`)
	if err != nil {
		return nil, fmt.Errorf("get known tags: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	for rows.Next() {
		var tagsJSON string
		rows.Scan(&tagsJSON)
		var tags []string
		json.Unmarshal([]byte(tagsJSON), &tags)
		for _, t := range tags {
			seen[t] = true
		}
	}

	result := make([]string, 0, len(seen))
	for t := range seen {
		result = append(result, t)
	}
	sort.Strings(result)
	return result, nil
}

func (s *Store) CheckNewTags(tags []string) []string {
	known, _ := s.GetKnownTags()
	knownSet := make(map[string]bool, len(known))
	for _, t := range known {
		knownSet[t] = true
	}
	var newTags []string
	for _, t := range tags {
		if !knownSet[t] {
			newTags = append(newTags, t)
		}
	}
	return newTags
}
```

Add `"sort"` to the imports in `store.go`.

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/store/ -run "TestGetKnownTags|TestCheckNewTags" -v`
Expected: PASS

- [ ] **Step 9: Write and run test for tags in UpdateFeature replacement**

Append to `tags_test.go`:

```go
func TestTagsInUpdateFeature(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Replaceable", "")
	tags1 := []string{"old", "stale"}
	s.UpdateFeature("replaceable", FeatureUpdate{Tags: &tags1})

	tags2 := []string{"new"}
	s.UpdateFeature("replaceable", FeatureUpdate{Tags: &tags2})
	f, _ := s.GetFeature("replaceable")
	if len(f.Tags) != 1 || f.Tags[0] != "new" {
		t.Fatalf("expected [new], got %v", f.Tags)
	}
}
```

Run: `go test ./internal/store/ -run TestTagsInUpdateFeature -v`
Expected: PASS

- [ ] **Step 10: Run full store test suite**

Run: `go test ./internal/store/ -v`
Expected: All PASS

- [ ] **Step 11: Commit**

```bash
git add internal/store/store.go internal/store/tags_test.go
git commit -m "feat: add ListFeaturesWithTag, GetKnownTags, CheckNewTags"
```

---

### Task 4: MCP tool updates — tags param on add_feature, update_feature, list_features

**Files:**
- Modify: `internal/mcp/tools.go` — add `tags` param to tool definitions
- Modify: `internal/mcp/tools_feature.go` — handler logic

- [ ] **Step 1: Add tags param to add_feature tool definition**

In `tools.go`, add after the `type` param on `add_feature`:

```go
mcp.WithString("tags", mcp.Description("Comma-separated tags (e.g., 'auth,frontend'). New tags trigger a warning listing existing tags.")),
```

- [ ] **Step 2: Add tags param to update_feature tool definition**

In `tools.go`, add after the `key_files` param on `update_feature`:

```go
mcp.WithString("tags", mcp.Description("Comma-separated tags — replaces all existing tags. New tags trigger a warning.")),
```

- [ ] **Step 3: Add tag param to list_features tool definition**

In `tools.go`, add after the `status` param on `list_features`:

```go
mcp.WithString("tag", mcp.Description("Filter to features with this tag. Combines with status filter.")),
```

- [ ] **Step 4: Update add_feature handler to handle tags and new-tag warnings**

In `tools_feature.go`, in `addFeatureHandler`, after the `typ` handling block (the `if typ != ""` block that applies templates), add:

```go
var tagWarning string
if tagStr, ok := argString(args, "tags"); ok && tagStr != "" {
    tags := strings.Split(tagStr, ",")
    for i := range tags {
        tags[i] = strings.TrimSpace(tags[i])
    }
    s.UpdateFeature(f.ID, store.FeatureUpdate{Tags: &tags})
    newTags := s.CheckNewTags(tags)
    if len(newTags) > 0 {
        known, _ := s.GetKnownTags()
        tagWarning = fmt.Sprintf("\nNote: new tag(s) %q added. Existing tags: %s", strings.Join(newTags, ", "), strings.Join(known, ", "))
    }
}
```

Then change the return to include the warning. Replace the final `f, _ = s.GetFeature(f.ID)` and return block with:

```go
f, _ = s.GetFeature(f.ID)
data, _ := json.MarshalIndent(f, "", "  ")
result := string(data)
if tagWarning != "" {
    result += tagWarning
}
return mcp.NewToolResultText(result), nil
```

- [ ] **Step 5: Update update_feature handler to handle tags and new-tag warnings**

In `tools_feature.go`, in `updateFeatureHandler`, add tags handling after the `key_files` block:

```go
if v, ok := argString(args, "tags"); ok {
    tags := strings.Split(v, ",")
    for i := range tags {
        tags[i] = strings.TrimSpace(tags[i])
    }
    u.Tags = &tags
}
```

After the `s.UpdateFeature(id, u)` call succeeds, add a new-tag warning:

```go
msg := fmt.Sprintf("Updated feature %q", id)
if u.Tags != nil {
    newTags := s.CheckNewTags(*u.Tags)
    if len(newTags) > 0 {
        known, _ := s.GetKnownTags()
        msg += fmt.Sprintf("\nNote: new tag(s) %q added. Existing tags: %s", strings.Join(newTags, ", "), strings.Join(known, ", "))
    }
}
return mcp.NewToolResultText(msg), nil
```

Remove the old `return mcp.NewToolResultText(fmt.Sprintf("Updated feature %q", id)), nil` line.

- [ ] **Step 6: Update list_features handler to support tag filter**

In `tools_feature.go`, in `listFeaturesHandler`, change to:

```go
return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    args := req.GetArguments()
    status, _ := argString(args, "status")
    tag, _ := argString(args, "tag")

    var features []store.Feature
    var err error
    if tag != "" {
        features, err = s.ListFeaturesWithTag(status, tag)
    } else {
        features, err = s.ListFeatures(status)
    }
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }

    if len(features) == 0 {
        return mcp.NewToolResultText("No features found."), nil
    }

    var lines []string
    for _, f := range features {
        line := fmt.Sprintf("- **%s** [%s] %s", f.ID, f.Status, f.Title)
        if len(f.Tags) > 0 {
            line += fmt.Sprintf(" {%s}", strings.Join(f.Tags, ", "))
        }
        if f.LeftOff != "" {
            snippet := f.LeftOff
            if len(snippet) > 60 {
                snippet = snippet[:60] + "..."
            }
            line += fmt.Sprintf(" — %s", snippet)
        }
        lines = append(lines, line)
    }
    return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}
```

- [ ] **Step 7: Update list_features tool description to mention archived exclusion**

In `tools.go`, update the `list_features` description and status param:

```go
srv.AddTool(mcp.NewTool("list_features",
    mcp.WithDescription("List features. Returns compact summaries: ID, title, status, tags, left_off snippet. Excludes archived by default."),
    mcp.WithString("status", mcp.Description("Filter by status: planned, in_progress, done, blocked, dev_complete, archived. Omit for all (excluding archived).")),
    mcp.WithString("tag", mcp.Description("Filter to features with this tag. Combines with status filter.")),
), listFeaturesHandler(s))
```

- [ ] **Step 8: Build to check compilation**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 9: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 10: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_feature.go
git commit -m "feat: add tags param to add_feature, update_feature, list_features MCP tools"
```

---

### Task 5: Archival — status extension, completion gate bypass, list exclusion

**Files:**
- Modify: `internal/store/store.go`
- Create: `internal/store/archive_test.go`

- [ ] **Step 1: Write failing test for archived status**

Create `internal/store/archive_test.go`:

```go
package store

import "testing"

func TestArchivedStatus(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Old Feature", "")
	archived := "archived"
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &archived})
	if err != nil {
		t.Fatalf("set archived: %v", err)
	}
	f, _ = s.GetFeature(f.ID)
	if f.Status != "archived" {
		t.Fatalf("expected archived, got %q", f.Status)
	}
}

func TestListFeaturesExcludesArchived(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Active", "")
	s.AddFeature("Old", "")
	archived := "archived"
	s.UpdateFeature("old", FeatureUpdate{Status: &archived})

	features, _ := s.ListFeatures("")
	if len(features) != 1 || features[0].ID != "active" {
		t.Fatalf("expected only active feature, got %v", features)
	}
}

func TestListFeaturesShowArchived(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Old", "")
	archived := "archived"
	s.UpdateFeature("old", FeatureUpdate{Status: &archived})

	features, _ := s.ListFeatures("archived")
	if len(features) != 1 || features[0].ID != "old" {
		t.Fatalf("expected archived feature, got %v", features)
	}
}

func TestCompletionGateBypassForArchived(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Archivable", "")
	s.ApplyTemplate(f.ID, "bugfix") // creates unchecked items

	archived := "archived"
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &archived})
	if err != nil {
		t.Fatalf("archiving with unchecked items should work, got: %v", err)
	}
	f, _ = s.GetFeature(f.ID)
	if f.Status != "archived" {
		t.Fatalf("expected archived, got %q", f.Status)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -run "TestListFeaturesExcludesArchived" -v`
Expected: FAIL — archived features still appear in unfiltered list

- [ ] **Step 3: Modify ListFeatures to exclude archived by default**

In `store.go`, `ListFeatures` method, change the query building:

```go
func (s *Store) ListFeatures(status string) ([]Feature, error) {
	query := `SELECT id, title, description, status, type, left_off, notes, key_files, tags, worktree_path, created_at, updated_at FROM features`
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	} else {
		query += " WHERE status != 'archived'"
	}
	query += " ORDER BY updated_at DESC"
```

Also update `ListFeaturesWithTag` to exclude archived by default:

```go
func (s *Store) ListFeaturesWithTag(status, tag string) ([]Feature, error) {
	query := `SELECT id, title, description, status, type, left_off, notes, key_files, tags, worktree_path, created_at, updated_at FROM features WHERE tags LIKE ?`
	args := []any{"%" + `"` + tag + `"` + "%"}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	} else {
		query += " AND status != 'archived'"
	}
	query += " ORDER BY updated_at DESC"
```

- [ ] **Step 4: Run archive tests**

Run: `go test ./internal/store/ -run "TestArchived|TestListFeaturesExcludesArchived|TestListFeaturesShowArchived|TestCompletionGateBypass" -v`
Expected: `TestCompletionGateBypassForArchived` may still FAIL — completion gate blocks non-done too if it fires

- [ ] **Step 5: Bypass completion gate for archived status**

The completion gate in `UpdateFeature` only fires for `status=done`. Since `archived` is not `done`, it already bypasses. Run the test to confirm:

Run: `go test ./internal/store/ -run TestCompletionGateBypassForArchived -v`
Expected: PASS (the gate only checks `*u.Status == "done"`)

- [ ] **Step 6: Run full test suite**

Run: `go test ./internal/store/ -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/store.go internal/store/archive_test.go
git commit -m "feat: archived status — exclude from list by default, bypass completion gate"
```

---

### Task 6: Auto-archive in SessionStart hook

**Files:**
- Modify: `internal/store/store.go` — new `AutoArchiveStale` method
- Modify: `cmd/docket/hook.go`
- Modify: `internal/store/archive_test.go`
- Modify: `cmd/docket/hook_test.go`

- [ ] **Step 1: Write failing test for AutoArchiveStale**

Append to `internal/store/archive_test.go`:

```go
import (
	"testing"
	"time"
)
```

Update the package imports (replace the existing `import "testing"` with the above). Then append:

```go
func TestAutoArchiveStale(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Old Done", "")
	done := "done"
	s.UpdateFeature("old-done", FeatureUpdate{Status: &done})

	// Backdate updated_at to 8 days ago
	s.db.Exec(`UPDATE features SET updated_at = datetime('now', '-8 days') WHERE id = 'old-done'`)

	s.AddFeature("Recent Done", "")
	s.UpdateFeature("recent-done", FeatureUpdate{Status: &done})

	archived, err := s.AutoArchiveStale()
	if err != nil {
		t.Fatalf("AutoArchiveStale: %v", err)
	}
	if len(archived) != 1 || archived[0] != "old-done" {
		t.Fatalf("expected [old-done], got %v", archived)
	}

	f, _ := s.GetFeature("old-done")
	if f.Status != "archived" {
		t.Fatalf("expected archived, got %q", f.Status)
	}

	f, _ = s.GetFeature("recent-done")
	if f.Status != "done" {
		t.Fatalf("recent should still be done, got %q", f.Status)
	}
}

func TestAutoArchiveSkipsRecent(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Fresh Done", "")
	done := "done"
	s.UpdateFeature("fresh-done", FeatureUpdate{Status: &done})

	archived, _ := s.AutoArchiveStale()
	if len(archived) != 0 {
		t.Fatalf("expected no archival, got %v", archived)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -run "TestAutoArchive" -v`
Expected: FAIL — `AutoArchiveStale` undefined

- [ ] **Step 3: Implement AutoArchiveStale**

Add to `store.go`:

```go
func (s *Store) AutoArchiveStale() ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM features WHERE status = 'done' AND updated_at < datetime('now', '-7 days')`)
	if err != nil {
		return nil, fmt.Errorf("query stale features: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}

	archived := "archived"
	for _, id := range ids {
		s.UpdateFeature(id, FeatureUpdate{Status: &archived})
	}
	return ids, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -run "TestAutoArchive" -v`
Expected: PASS

- [ ] **Step 5: Add auto-archive call to SessionStart hook**

In `hook.go`, in `handleSessionStart`, right after `defer s.Close()`, add:

```go
// Auto-archive features done >7 days
if archived, err := s.AutoArchiveStale(); err == nil && len(archived) > 0 {
    fmt.Fprintf(os.Stderr, "[docket] Auto-archived %d features: %s\n", len(archived), strings.Join(archived, ", "))
}
```

This logs to stderr (visible in debug output) but doesn't inject into the systemMessage — keeping the session start message focused on active work.

- [ ] **Step 6: Write hook test for auto-archive**

Append to `cmd/docket/hook_test.go`:

```go
func TestSessionStartAutoArchives(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a done feature backdated 8 days
	s.AddFeature("Old Done Feature", "")
	done := "done"
	s.UpdateFeature("old-done-feature", store.FeatureUpdate{Status: &done})
	s.DB().Exec(`UPDATE features SET updated_at = datetime('now', '-8 days') WHERE id = 'old-done-feature'`)

	// Create an active feature
	s.AddFeature("Active Feature", "")
	ip := "in_progress"
	s.UpdateFeature("active-feature", store.FeatureUpdate{Status: &ip})
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "SessionStart",
	}

	var buf bytes.Buffer
	handleSessionStart(h, &buf)

	// Verify old feature was archived
	s2, _ := store.Open(dir)
	defer s2.Close()
	f, _ := s2.GetFeature("old-done-feature")
	if f.Status != "archived" {
		t.Fatalf("expected archived, got %q", f.Status)
	}
}
```

- [ ] **Step 7: Expose db for testing**

The hook test needs `s.DB()` to backdate. Add this accessor to `store.go`:

```go
func (s *Store) DB() *sql.DB {
	return s.db
}
```

- [ ] **Step 8: Run hook test**

Run: `go test ./cmd/docket/ -run TestSessionStartAutoArchives -v`
Expected: PASS

- [ ] **Step 9: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 10: Commit**

```bash
git add internal/store/store.go internal/store/archive_test.go cmd/docket/hook.go cmd/docket/hook_test.go
git commit -m "feat: auto-archive done features >7 days old in SessionStart hook"
```

---

### Task 7: Dashboard — tag pills on cards and archived toggle

**Files:**
- Modify: `dashboard/index.html`

- [ ] **Step 1: Add tag pill CSS**

In the `<style>` section, after the `.issue-badge` light-mode rule (around line 209), add:

```css
.tag-pill {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 10px;
  font-size: 11px;
  border: 1px solid #4ECDC440;
  color: var(--teal);
  margin: 2px 2px 2px 0;
}
html.light .tag-pill { border-color: #1A7A7240; color: #1A7A72; }
```

- [ ] **Step 2: Add archived column CSS**

Add after the `.col-done` styles:

```css
.col-archived .column-header { color: var(--muted); }
.col-archived .dot { background: var(--muted); }
.col-archived .card { opacity: 0.45; }
.col-archived .card:hover { opacity: 0.8; }
```

- [ ] **Step 3: Add tag pills to card rendering**

In the JavaScript `render()` function, after the issue-badge block (after `card.appendChild(issueBadge)` closing brace), add:

```javascript
// Tag pills
var tags = f.tags || [];
if (tags.length > 0) {
  var tagWrap = document.createElement('div');
  tagWrap.style.marginTop = '6px';
  for (var ti = 0; ti < tags.length; ti++) {
    var pill = document.createElement('span');
    pill.className = 'tag-pill';
    pill.textContent = tags[ti];
    tagWrap.appendChild(pill);
  }
  card.appendChild(tagWrap);
}
```

- [ ] **Step 4: Add tag pills to panel detail view**

In the `showDetail()` function, after the type badge block (after `slugDiv.appendChild(typeBadge)`), add:

```javascript
if (f.tags && f.tags.length > 0) {
  for (var ti = 0; ti < f.tags.length; ti++) {
    var tagPill = document.createElement('span');
    tagPill.className = 'tag-pill';
    tagPill.style.marginLeft = '6px';
    tagPill.textContent = f.tags[ti];
    slugDiv.appendChild(tagPill);
  }
}
```

- [ ] **Step 5: Add archived toggle and column**

Update the `STATUSES` array and related constants at the top of the script:

```javascript
var STATUSES = ['planned', 'in_progress', 'blocked', 'dev_complete', 'done'];
var ALL_STATUSES = ['planned', 'in_progress', 'blocked', 'dev_complete', 'done', 'archived'];
var STATUS_LABELS = { planned: 'Planned', in_progress: 'In Progress', blocked: 'Blocked', dev_complete: 'Dev Complete', done: 'Done', archived: 'Archived' };
var STATUS_CSS = { planned: 'col-planned', in_progress: 'col-in-progress', blocked: 'col-blocked', dev_complete: 'col-dev-complete', done: 'col-done', archived: 'col-archived' };
var showArchived = false;
```

Add archived badge to header, in `<div class="header-right">`, before the unlinked badge:

```html
<button class="theme-toggle" id="archiveToggle" onclick="toggleArchived()" title="Show archived features" style="font-size:13px">&#x1F4E6;</button>
```

Add the toggle function:

```javascript
function toggleArchived() {
  showArchived = !showArchived;
  document.getElementById('archiveToggle').style.borderColor = showArchived ? 'var(--primary)' : '';
  load();
}
```

Update the `load()` function to fetch with or without archived:

```javascript
async function load() {
  try {
    var url = API + '/api/features';
    if (showArchived) url += '?status=';
    var res = await fetch(url);
    features = await res.json();
  } catch(e) { features = []; }
  render();
  loadUnlinked();
}
```

Wait — `?status=` with empty value will match `""` in the handler and exclude archived. We need a different approach. The dashboard API uses `GET /api/features` which calls `s.ListFeatures(status)`. With no status, it excludes archived. We need to load archived separately.

Better approach: update `render()` to use the right statuses list:

```javascript
function render() {
  var board = document.getElementById('board');
  board.innerHTML = '';
  var statuses = showArchived ? ALL_STATUSES : STATUSES;
  board.style.gridTemplateColumns = 'repeat(' + statuses.length + ', 1fr)';
  for (var si = 0; si < statuses.length; si++) {
```

And update `load()` to fetch archived features when the toggle is on:

```javascript
async function load() {
  try {
    var res = await fetch(API + '/api/features');
    features = await res.json();
    if (showArchived) {
      var res2 = await fetch(API + '/api/features?status=archived');
      var archived = await res2.json();
      features = features.concat(archived);
    }
  } catch(e) { features = []; }
  render();
  loadUnlinked();
}
```

- [ ] **Step 6: Update board grid for 6 columns when archived visible**

The existing CSS has `grid-template-columns: repeat(5, 1fr)`. The JS override in `render()` handles this dynamically with `board.style.gridTemplateColumns`.

- [ ] **Step 7: Add archived badge style to panel**

Add to the badge styles:

```css
.badge-archived { background: #8A887810; color: var(--muted); }
html.light .badge-archived { background: #EAEAEA; color: var(--muted); }
```

- [ ] **Step 8: Build and visually verify**

Run: `go build -ldflags="-s -w" -o docket.exe ./cmd/docket/`
Expected: Compiles. Open dashboard to verify tag pills render and archived toggle works.

- [ ] **Step 9: Commit**

```bash
git add dashboard/index.html
git commit -m "feat: dashboard tag pills and archived feature toggle"
```

---

### Task 8: CLAUDE.md snippet update

**Files:**
- Modify: `cmd/docket/update.go`

- [ ] **Step 1: Update docketSection constant**

In `update.go`, update the `docketSection` constant. After the `type` param line in the "Start of work" paragraph, add a new line about tags:

In the "After a commit" section, update the `update_feature` bullet to mention tags:

Replace the `update_feature` bullet line:
```
- ` + "`update_feature`" + ` — set left_off, key_files, status. Completion gate blocks ` + "`done`" + ` with unchecked items — pass ` + "`force=true`" + ` + ` + "`force_reason`" + ` to override.
```

With:
```
- ` + "`update_feature`" + ` — set left_off, key_files, status, tags. Completion gate blocks ` + "`done`" + ` with unchecked items — pass ` + "`force=true`" + ` + ` + "`force_reason`" + ` to override.
```

Add after the "Start of work" paragraph (after the `type` line):

```
Use ` + "`tags`" + ` param (comma-separated) on ` + "`add_feature`" + `/` + "`update_feature`" + ` to categorize work. New tags warn about existing tags to prevent typos.

Done features are auto-archived after 7 days. Use ` + "`list_features(status=\"archived\")`" + ` to see them. ` + "`update_feature(status=\"planned\")`" + ` to unarchive.
```

- [ ] **Step 2: Run update command to verify**

Run: `go run ./cmd/docket/ update` (in project root)
Expected: Updates CLAUDE.md with new snippet

- [ ] **Step 3: Commit**

```bash
git add cmd/docket/update.go
git commit -m "feat: update CLAUDE.md snippet with tags and archival docs"
```

---

### Task 9: Update project CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Run the update command to sync the snippet**

Run: `go run ./cmd/docket/ update`

- [ ] **Step 2: Verify CLAUDE.md has the new tags and archival info**

Read `CLAUDE.md` and verify the docket section mentions tags, auto-archive, and archived status.

- [ ] **Step 3: Update the update_feature status list in tools.go description**

In `tools.go`, update the `update_feature` status description:

```go
mcp.WithString("status", mcp.Description("New status: planned, in_progress, done, blocked, dev_complete, archived")),
```

- [ ] **Step 4: Build final binary**

Run: `go build -ldflags="-s -w" -o docket.exe ./cmd/docket/`
Expected: Clean build

- [ ] **Step 5: Run complete test suite**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md internal/mcp/tools.go
git commit -m "docs: update CLAUDE.md snippet and status descriptions for tags/archival"
```

---

### Task 10: Write feature doc

**Files:**
- Create: `docs/tags-and-archival.md`

- [ ] **Step 1: Write the doc**

```markdown
# Tags and Archival

## What it does

Two additions to feature tracking:

1. **Tags** — free-form string tags on features for categorization. Comma-separated input, stored as JSON array in SQLite. New-tag warnings help catch typos by listing existing tags when you add one that doesn't exist yet.

2. **Archival** — `archived` is a feature status. Done features auto-archive after 7 days (checked on SessionStart). Archived features are hidden from `list_features` and the dashboard by default. No special tool needed — use `update_feature(status="planned")` to unarchive.

## How to use

**Tags:**
- `add_feature(title="...", tags="auth,frontend")` — set tags on creation
- `update_feature(id="...", tags="backend,api")` — replace all tags
- `list_features(tag="auth")` — filter by tag

**Archival:**
- Features marked `done` for 7+ days auto-archive on next session start
- `list_features(status="archived")` — see archived features
- `update_feature(id="...", status="planned")` — unarchive
- Dashboard has an archive toggle button in the header

## Gotchas

- Tags are exact-match only. No fuzzy matching. The new-tag warning is your typo detector.
- `tags` param on `update_feature` replaces all tags, not appends. Send the full list.
- No `list_tags` or `delete_tag` tool. Tags are derived from what features have — unused tags disappear naturally.
- Auto-archive is hardcoded to 7 days. No config file.
- Archiving bypasses the completion gate — you can archive features with unchecked items.
- `get_feature` and `get_context` return any feature regardless of status, including archived.

## Key files

- `internal/store/migrate.go` — schemaV8 (tags column)
- `internal/store/store.go` — Tags field, GetKnownTags, CheckNewTags, ListFeaturesWithTag, AutoArchiveStale
- `internal/store/tags_test.go` — tag tests
- `internal/store/archive_test.go` — archival tests
- `internal/mcp/tools.go` — tags/tag params on MCP tools
- `internal/mcp/tools_feature.go` — tag handling in handlers
- `cmd/docket/hook.go` — auto-archive in SessionStart
- `dashboard/index.html` — tag pills, archived toggle
```

- [ ] **Step 2: Commit**

```bash
git add docs/tags-and-archival.md
git commit -m "docs: tags and archival feature doc"
```
