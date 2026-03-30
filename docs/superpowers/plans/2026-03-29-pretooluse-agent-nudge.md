# PreToolUse Agent Nudge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a PreToolUse hook on the Agent tool that nudges Claude to set up docket tracking before dispatching subagents, plus a conditional CLAUDE.md snippet paragraph for superpowers users.

**Architecture:** New `handlePreToolUse` function in hook.go checks docket state (features, task items) and returns an informational systemMessage. Sentinel file prevents repeat nudges. update.go detects superpowers installation and conditionally includes a plan-execution paragraph.

**Tech Stack:** Go, SQLite (existing store), JSON hook I/O

---

### Task 1: Add PreToolUse handler — no features case

**Files:**
- Modify: `cmd/docket/hook.go:14-36` (add new output struct)
- Modify: `cmd/docket/hook.go:38-67` (add case to switch)
- Modify: `cmd/docket/hook.go:69-100` (add sentinel clear in SessionStart)
- Test: `cmd/docket/hook_test.go`

- [ ] **Step 1: Write the failing test for PreToolUse with no features**

Add to `cmd/docket/hook_test.go`:

```go
func TestPreToolUseNoFeatures(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PreToolUse",
		ToolName:      "Agent",
	}

	var buf bytes.Buffer
	handlePreToolUse(h, &buf)

	var out preToolUseOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.HookSpecificOutput == nil || out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Error("expected permissionDecision=allow")
	}
	if !strings.Contains(out.SystemMessage, "No active docket feature") {
		t.Errorf("expected nudge message, got: %s", out.SystemMessage)
	}
	if !strings.Contains(out.SystemMessage, "get_ready") {
		t.Errorf("expected get_ready instruction, got: %s", out.SystemMessage)
	}

	// Verify sentinel was written
	sentinel := filepath.Join(dir, ".docket", "agent-nudged")
	if _, err := os.Stat(sentinel); os.IsNotExist(err) {
		t.Error("expected agent-nudged sentinel to be created")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd "H:\claude code\tools\docket" && go test ./cmd/docket/ -run TestPreToolUseNoFeatures -v`
Expected: FAIL — `handlePreToolUse` undefined, `preToolUseOutput` undefined

- [ ] **Step 3: Add the preToolUseOutput struct and handlePreToolUse function**

In `cmd/docket/hook.go`, add the new output struct after the existing `stopHookOutput` struct (after line 36):

```go
type preToolUseOutput struct {
	HookSpecificOutput *preToolUseDecision `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
}

type preToolUseDecision struct {
	PermissionDecision string `json:"permissionDecision"`
}
```

Add the `"PreToolUse"` case in the switch statement in `runHook` (after the `"SessionStart"` case):

```go
case "PreToolUse":
	handlePreToolUse(&h, os.Stdout)
```

Add the handler function:

```go
func handlePreToolUse(h *hookInput, w io.Writer) {
	allow := preToolUseOutput{
		HookSpecificOutput: &preToolUseDecision{PermissionDecision: "allow"},
	}

	// Check sentinel — already nudged this session
	sentinelPath := filepath.Join(h.CWD, ".docket", "agent-nudged")
	if _, err := os.Stat(sentinelPath); err == nil {
		json.NewEncoder(w).Encode(allow)
		return
	}

	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(allow)
		return
	}
	defer s.Close()

	features, err := s.ListFeatures("in_progress")
	if err != nil {
		json.NewEncoder(w).Encode(allow)
		return
	}

	if len(features) == 0 {
		os.WriteFile(sentinelPath, []byte{}, 0644)
		allow.SystemMessage = "[docket] No active docket feature. Call get_ready to set up tracking before dispatching subagents."
		json.NewEncoder(w).Encode(allow)
		return
	}

	// Check if top feature has task items
	_, total, err := s.GetFeatureProgress(features[0].ID)
	if err != nil {
		json.NewEncoder(w).Encode(allow)
		return
	}

	if total == 0 {
		os.WriteFile(sentinelPath, []byte{}, 0644)
		allow.SystemMessage = fmt.Sprintf(
			"[docket] Active feature %q (id: %s) has no task items. Add task items from the plan before dispatching subagents.",
			features[0].Title, features[0].ID,
		)
		json.NewEncoder(w).Encode(allow)
		return
	}

	// Feature has task items — all good
	json.NewEncoder(w).Encode(allow)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd "H:\claude code\tools\docket" && go test ./cmd/docket/ -run TestPreToolUseNoFeatures -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd "H:\claude code\tools\docket"
git add cmd/docket/hook.go cmd/docket/hook_test.go
git commit -m "feat: add PreToolUse handler — nudge when no active features"
```

---

### Task 2: Add PreToolUse handler — feature exists, no task items

**Files:**
- Test: `cmd/docket/hook_test.go`

- [ ] **Step 1: Write the failing test for feature with no task items**

Add to `cmd/docket/hook_test.go`:

```go
func TestPreToolUseFeatureNoTaskItems(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.AddFeature("My Feature", "testing")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PreToolUse",
		ToolName:      "Agent",
	}

	var buf bytes.Buffer
	handlePreToolUse(h, &buf)

	var out preToolUseOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.HookSpecificOutput == nil || out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Error("expected permissionDecision=allow")
	}
	if !strings.Contains(out.SystemMessage, "no task items") {
		t.Errorf("expected task items nudge, got: %s", out.SystemMessage)
	}
	if !strings.Contains(out.SystemMessage, f.ID) {
		t.Errorf("expected feature ID in message, got: %s", out.SystemMessage)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

This test should already pass because the implementation in Task 1 handles this case. Run:

Run: `cd "H:\claude code\tools\docket" && go test ./cmd/docket/ -run TestPreToolUseFeatureNoTaskItems -v`
Expected: PASS

- [ ] **Step 3: Write test for feature WITH task items (silent pass-through)**

Add to `cmd/docket/hook_test.go`:

```go
func TestPreToolUseFeatureWithTaskItems(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.AddFeature("My Feature", "testing")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})

	// Add a subtask with a task item
	st, err := s.AddSubtask(f.ID, "Task 1", 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddTaskItem(st.ID, "Step 1: do something", 0)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PreToolUse",
		ToolName:      "Agent",
	}

	var buf bytes.Buffer
	handlePreToolUse(h, &buf)

	var out preToolUseOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.HookSpecificOutput == nil || out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Error("expected permissionDecision=allow")
	}
	if out.SystemMessage != "" {
		t.Errorf("expected no systemMessage when task items exist, got: %s", out.SystemMessage)
	}

	// Verify no sentinel written
	sentinel := filepath.Join(dir, ".docket", "agent-nudged")
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Error("sentinel should NOT be written when feature has task items")
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd "H:\claude code\tools\docket" && go test ./cmd/docket/ -run TestPreToolUseFeatureWithTaskItems -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd "H:\claude code\tools\docket"
git add cmd/docket/hook_test.go
git commit -m "test: add PreToolUse tests for task items and silent pass-through"
```

---

### Task 3: Add sentinel nudge-once behavior and SessionStart cleanup

**Files:**
- Modify: `cmd/docket/hook.go:69-100` (handleSessionStart)
- Test: `cmd/docket/hook_test.go`

- [ ] **Step 1: Write the failing test for sentinel preventing re-nudge**

Add to `cmd/docket/hook_test.go`:

```go
func TestPreToolUseSentinelPreventsReNudge(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Write the sentinel (simulating a prior nudge)
	sentinel := filepath.Join(dir, ".docket", "agent-nudged")
	os.WriteFile(sentinel, []byte{}, 0644)

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PreToolUse",
		ToolName:      "Agent",
	}

	var buf bytes.Buffer
	handlePreToolUse(h, &buf)

	var out preToolUseOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.SystemMessage != "" {
		t.Errorf("expected no nudge when sentinel exists, got: %s", out.SystemMessage)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

This should already pass because the sentinel check is in Task 1's implementation. Run:

Run: `cd "H:\claude code\tools\docket" && go test ./cmd/docket/ -run TestPreToolUseSentinelPreventsReNudge -v`
Expected: PASS

- [ ] **Step 3: Write the failing test for SessionStart clearing the sentinel**

Add to `cmd/docket/hook_test.go`:

```go
func TestSessionStartClearsSentinel(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Write a sentinel from a prior session
	sentinel := filepath.Join(dir, ".docket", "agent-nudged")
	os.WriteFile(sentinel, []byte{}, 0644)

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "SessionStart",
	}

	var buf bytes.Buffer
	handleSessionStart(h, &buf)

	// Verify sentinel was cleared
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Error("expected agent-nudged sentinel to be cleared on SessionStart")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `cd "H:\claude code\tools\docket" && go test ./cmd/docket/ -run TestSessionStartClearsSentinel -v`
Expected: FAIL — sentinel still exists after SessionStart

- [ ] **Step 5: Add sentinel cleanup to handleSessionStart**

In `cmd/docket/hook.go`, in `handleSessionStart`, add after the `commits.log` clear (after line ~87):

```go
// Clear agent-nudged sentinel
os.Remove(filepath.Join(h.CWD, ".docket", "agent-nudged"))
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd "H:\claude code\tools\docket" && go test ./cmd/docket/ -run TestSessionStartClearsSentinel -v`
Expected: PASS

- [ ] **Step 7: Run all hook tests**

Run: `cd "H:\claude code\tools\docket" && go test ./cmd/docket/ -v`
Expected: All tests PASS

- [ ] **Step 8: Commit**

```bash
cd "H:\claude code\tools\docket"
git add cmd/docket/hook.go cmd/docket/hook_test.go
git commit -m "feat: sentinel nudge-once and SessionStart cleanup for PreToolUse"
```

---

### Task 4: Update hooks.json with PreToolUse matcher

**Files:**
- Modify: `plugin/hooks/hooks.json`

- [ ] **Step 1: Add PreToolUse entry to hooks.json**

In `plugin/hooks/hooks.json`, add the PreToolUse entry inside the `"hooks"` object (after the SessionStart block, before PostToolUse):

```json
"PreToolUse": [
  {
    "matcher": "Agent",
    "hooks": [
      {
        "type": "command",
        "command": "DOCKET_EXE_PATH hook",
        "timeout": 5
      }
    ]
  }
],
```

The `DOCKET_EXE_PATH` placeholder is replaced by `install.sh` during installation (line 74 of install.sh).

- [ ] **Step 2: Verify JSON is valid**

Run: `cd "H:\claude code\tools\docket" && python3 -c "import json; json.load(open('plugin/hooks/hooks.json')); print('Valid JSON')"`
Expected: `Valid JSON`

- [ ] **Step 3: Commit**

```bash
cd "H:\claude code\tools\docket"
git add plugin/hooks/hooks.json
git commit -m "feat: add PreToolUse Agent matcher to hooks.json"
```

---

### Task 5: Conditional superpowers paragraph in update.go

**Files:**
- Modify: `cmd/docket/update.go`
- Test: `cmd/docket/update_test.go`

- [ ] **Step 1: Write the failing test for superpowers detection**

Add to `cmd/docket/update_test.go`:

```go
func TestBuildDocketSectionWithSuperpowers(t *testing.T) {
	result := buildDocketSection(true)
	if !strings.Contains(result, "Plan execution (superpowers)") {
		t.Error("expected superpowers paragraph when detected")
	}
	if !strings.Contains(result, "executing-plans") {
		t.Error("expected executing-plans mention")
	}
}

func TestBuildDocketSectionWithoutSuperpowers(t *testing.T) {
	result := buildDocketSection(false)
	if strings.Contains(result, "Plan execution (superpowers)") {
		t.Error("should not include superpowers paragraph when not detected")
	}
	// Should still have core docket content
	if !strings.Contains(result, "Feature Tracking (docket)") {
		t.Error("missing core docket section")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd "H:\claude code\tools\docket" && go test ./cmd/docket/ -run TestBuildDocketSection -v`
Expected: FAIL — `buildDocketSection` undefined

- [ ] **Step 3: Refactor update.go to split snippet and detect superpowers**

Replace the `docketSection` const and `runUpdate` function in `cmd/docket/update.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const docketSectionHead = `## Feature Tracking (docket)

This project uses ` + "`docket`" + ` for feature tracking. Dashboard: http://localhost:<port> (or run ` + "`/docket`" + `).

**Small tasks** (cosmetic changes, one-off fixes, config tweaks): call ` + "`quick_track`" + ` directly — one call, no agent dispatch needed.

**Larger features** (multi-step, plan-driven, complex):

Start of work (after any brainstorming/planning) — call ` + "`get_ready`" + ` to find existing features, then dispatch ` + "`board-manager`" + ` agent (model: sonnet) to create or find a card. Use ` + "`type`" + ` param (feature/bugfix/chore/spike) to auto-generate subtask templates.

Use ` + "`tags`" + ` param (comma-separated) on ` + "`add_feature`" + `/` + "`update_feature`" + ` to categorize work. New tags warn about existing tags to prevent typos.

Done features are auto-archived after 7 days. Use ` + "`list_features(status=\"archived\")`" + ` to see them. ` + "`update_feature(status=\"planned\")`" + ` to unarchive.
`

const docketSectionSuperpowers = `
**Plan execution (superpowers):** When using executing-plans or subagent-driven-development, set up docket BEFORE dispatching the first task — call ` + "`get_ready`" + `, create/find a feature card, and use ` + "`add_task_item`" + ` for each plan task. A PreToolUse hook will remind you if you forget.
`

const docketSectionTail = `
After a commit — use **direct MCP calls**, not agent dispatch:
- ` + "`update_feature`" + ` — set left_off, key_files, status, tags. Completion gate blocks ` + "`done`" + ` with unchecked items — pass ` + "`force=true`" + ` + ` + "`force_reason`" + ` to override.
- ` + "`complete_task_item`" + ` — check off items with outcome and commit_hash (pass ` + "`items`" + ` JSON array for batch)
- ` + "`add_decision`" + ` — record notable decisions (accepted/rejected with reason)
- ` + "`add_issue`" + ` / ` + "`resolve_issue`" + ` — track bugs found during work

Plan files committed during work are auto-imported by hooks. Only dispatch board-manager when the update needs judgment (restructuring imported plans, creating new subtasks).

After subagent work — subagent commits bypass hooks. Use direct MCP calls to batch-update the feature.

Use ` + "`get_context`" + ` (not ` + "`get_feature`" + `) for routine status checks — it's token-efficient (~15 lines).

Session logging and handoff files are handled automatically by the Stop hook.

Carry the feature ID across the session.

**If user rejects a docket update**, fix the issue and retry — don't drop tracking.
`

const sectionHeading = "## Feature Tracking (docket)"

func buildDocketSection(hasSuperpowers bool) string {
	if hasSuperpowers {
		return docketSectionHead + docketSectionSuperpowers + docketSectionTail
	}
	return docketSectionHead + docketSectionTail
}

func detectSuperpowers() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	pluginsFile := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	data, err := os.ReadFile(pluginsFile)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "superpowers")
}

func runUpdate() {
	data, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, "No CLAUDE.md found in current directory.")
		os.Exit(1)
	}

	content := string(data)
	docketSection := buildDocketSection(detectSuperpowers())
	updated := updateDocketSection(content, docketSection)

	if updated == content {
		fmt.Println("CLAUDE.md docket section is already up to date.")
		return
	}

	if err := os.WriteFile("CLAUDE.md", []byte(updated), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write CLAUDE.md: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Updated CLAUDE.md with latest docket section.")
}

func updateDocketSection(content, docketSection string) string {
	// If section exists, replace it in place
	idx := strings.Index(content, sectionHeading)
	if idx >= 0 {
		// Find the end of this section (next ## heading or EOF)
		rest := content[idx+len(sectionHeading):]
		endIdx := strings.Index(rest, "\n## ")
		if endIdx >= 0 {
			// Replace section, keep everything after
			return content[:idx] + docketSection + "\n" + rest[endIdx+1:]
		}
		// Section goes to EOF
		return content[:idx] + docketSection
	}

	// Not found — insert after the first section
	lines := strings.Split(content, "\n")
	var result []string
	inserted := false
	passedFirstHeading := false

	for i, line := range lines {
		if !inserted && strings.HasPrefix(line, "## ") {
			if !passedFirstHeading {
				passedFirstHeading = true
				result = append(result, line)
				continue
			}
			// This is the second ## heading — insert before it
			result = append(result, "")
			result = append(result, strings.Split(strings.TrimRight(docketSection, "\n"), "\n")...)
			result = append(result, "")
			inserted = true
		}
		result = append(result, line)
		_ = i
	}

	if !inserted {
		// No second heading found — append at end
		result = append(result, "")
		result = append(result, strings.Split(strings.TrimRight(docketSection, "\n"), "\n")...)
		result = append(result, "")
	}

	return strings.Join(result, "\n")
}
```

Note: `updateDocketSection` now takes the section content as a parameter instead of using the global const, so `buildDocketSection` controls what gets inserted.

- [ ] **Step 4: Fix existing tests to use new signature**

The existing tests in `update_test.go` call `updateDocketSection(input)` with one arg. Update them to pass `buildDocketSection(false)` as the second argument:

Replace all calls of `updateDocketSection(input)` with `updateDocketSection(input, buildDocketSection(false))`.

For `TestUpdateDocketSectionAlreadyUpToDate`, update the input construction to use `buildDocketSection(false)` instead of `docketSection`:

```go
func TestUpdateDocketSectionAlreadyUpToDate(t *testing.T) {
	section := buildDocketSection(false)
	input := "# My Project\n\n" + section + "\n## Build\n"
	result := updateDocketSection(input, section)

	if result != input {
		t.Error("should not change content that's already up to date")
	}
}
```

- [ ] **Step 5: Run all tests to verify they pass**

Run: `cd "H:\claude code\tools\docket" && go test ./cmd/docket/ -v`
Expected: All tests PASS

- [ ] **Step 6: Commit**

```bash
cd "H:\claude code\tools\docket"
git add cmd/docket/update.go cmd/docket/update_test.go
git commit -m "feat: conditional superpowers paragraph in CLAUDE.md snippet"
```

---

### Task 6: Run full test suite and update docs

**Files:**
- Modify: `docs/pretooluse-agent-nudge.md` (new doc)

- [ ] **Step 1: Run full test suite**

Run: `cd "H:\claude code\tools\docket" && go test ./... -v`
Expected: All tests PASS

- [ ] **Step 2: Build binary**

Run: `cd "H:\claude code\tools\docket" && go build -ldflags="-s -w" -o docket.exe ./cmd/docket/`
Expected: Builds without errors

- [ ] **Step 3: Write feature doc**

Create `docs/pretooluse-agent-nudge.md`:

```markdown
# PreToolUse Agent Nudge

Docket fires a PreToolUse hook whenever Claude dispatches a subagent via the Agent tool. If docket tracking isn't set up — no active feature or no task items on the active feature — it injects a one-time reminder into the conversation.

## Why it exists

Superpowers' plan execution skills (executing-plans, subagent-driven-development) are prescriptive — they tell Claude exactly what steps to follow. The CLAUDE.md docket instructions get ignored. This hook fires at the exact moment work is about to be dispatched, catching the gap.

## How it works

1. PreToolUse fires on `Agent` tool
2. Checks `.docket/` exists (skips if not a docket project)
3. Checks for `.docket/agent-nudged` sentinel (skips if already nudged)
4. Opens store, checks for in_progress features
5. If no features → nudge to call `get_ready`
6. If feature exists but zero task items → nudge to add task items
7. If feature has task items → silent pass-through
8. Writes sentinel after nudging (one nudge per session)

The sentinel is cleared on SessionStart, so each new session gets a fresh check.

## Superpowers detection

When `docket.exe update` runs, it checks `~/.claude/plugins/installed_plugins.json` for superpowers. If found, the CLAUDE.md snippet includes an extra paragraph about setting up docket before plan execution.

## Key files

- `cmd/docket/hook.go` — `handlePreToolUse` function, sentinel logic
- `plugin/hooks/hooks.json` — PreToolUse matcher for Agent
- `cmd/docket/update.go` — `buildDocketSection`, `detectSuperpowers`
- `cmd/docket/hook_test.go` — PreToolUse test cases
```

- [ ] **Step 4: Commit**

```bash
cd "H:\claude code\tools\docket"
git add docs/pretooluse-agent-nudge.md
git commit -m "docs: PreToolUse agent nudge feature doc"
```
