# Multi-Session MCP Server Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow multiple concurrent Claude Code sessions per project, each with its own MCP server, sharing a single dashboard that survives any individual session closing.

**Architecture:** Each `docket.exe serve` process runs MCP + checkpoint worker independently. Dashboard uses leader election via port binding. Session identity flows through `bind_session` MCP tool (manual sessions) or `DOCKET_LAUNCH_FEATURE` env var (dashboard launches). Work sessions are feature-scoped (not global close-all).

**Tech Stack:** Go, SQLite, mcp-go library

**Spec:** `docs/superpowers/specs/2026-04-03-multi-session-mcp-lifecycle-design.md`

---

### Task 1: Schema Migration — add `mcp_pid` column

**Files:**
- Modify: `internal/store/migrate.go:365-413`

- [ ] **Step 1: Add the migration constant**

After `schemaV18` in `migrate.go`, add:

```go
const schemaV19 = `ALTER TABLE work_sessions ADD COLUMN mcp_pid INTEGER;`
```

- [ ] **Step 2: Register in migrate()**

In the `migrate()` function, after `db.Exec(schemaV18)`, add:

```go
db.Exec(schemaV19)
```

- [ ] **Step 3: Run tests to verify migration is idempotent**

Run: `go test ./internal/store/ -run TestOpen -v`
Expected: PASS (store.Open runs migrate on every open; ALTER TABLE errors are ignored for idempotency)

- [ ] **Step 4: Commit**

```bash
git add internal/store/migrate.go
git commit -m "feat(store): add mcp_pid column to work_sessions (migration v19)"
```

---

### Task 2: Store — Feature-Scoped `OpenWorkSession`

**Files:**
- Modify: `internal/store/worksession.go:52-53`
- Modify: `internal/store/worksession_test.go`

- [ ] **Step 1: Write failing test — two features stay open concurrently**

In `worksession_test.go`, add:

```go
func TestOpenWorkSessionFeatureScoped(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	s.CreateFeature("feat-a", "Feature A")
	s.CreateFeature("feat-b", "Feature B")

	wsA, err := s.OpenWorkSession("feat-a", "session-1")
	if err != nil {
		t.Fatal(err)
	}

	wsB, err := s.OpenWorkSession("feat-b", "session-2")
	if err != nil {
		t.Fatal(err)
	}

	// Session A should still be open
	reloaded, err := s.GetWorkSession(wsA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != "open" {
		t.Errorf("session A should still be open, got %q", reloaded.Status)
	}

	// Both sessions open
	if wsB.Status != "open" {
		t.Errorf("session B should be open, got %q", wsB.Status)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestOpenWorkSessionFeatureScoped -v`
Expected: FAIL — `session A should still be open, got "closed"` (because current code closes ALL open sessions)

- [ ] **Step 3: Write failing test — same feature supersedes**

```go
func TestOpenWorkSessionSupersedes(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	s.CreateFeature("feat-a", "Feature A")

	wsA, err := s.OpenWorkSession("feat-a", "session-1")
	if err != nil {
		t.Fatal(err)
	}

	wsB, err := s.OpenWorkSession("feat-a", "session-2")
	if err != nil {
		t.Fatal(err)
	}

	// Session A should be closed (superseded)
	reloaded, err := s.GetWorkSession(wsA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != "closed" {
		t.Errorf("session A should be closed, got %q", reloaded.Status)
	}
	if wsB.Status != "open" {
		t.Errorf("session B should be open, got %q", wsB.Status)
	}
}
```

- [ ] **Step 4: Implement feature-scoped closing**

In `worksession.go`, replace line 52-53:

```go
// Close any other open sessions (one active session at a time)
s.db.Exec(`UPDATE work_sessions SET status = 'closed', ended_at = datetime('now') WHERE status = 'open'`)
```

With:

```go
// Close any open sessions for the same feature (one active session per feature)
s.db.Exec(`UPDATE work_sessions SET status = 'closed', ended_at = datetime('now') WHERE feature_id = ? AND status = 'open'`, featureID)
```

- [ ] **Step 5: Run both tests**

Run: `go test ./internal/store/ -run "TestOpenWorkSession(FeatureScoped|Supersedes)" -v`
Expected: PASS

- [ ] **Step 6: Run full store test suite**

Run: `go test ./internal/store/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/worksession.go internal/store/worksession_test.go
git commit -m "feat(store): feature-scoped OpenWorkSession — no longer closes all sessions"
```

---

### Task 3: Store — `mcp_pid` Methods

**Files:**
- Modify: `internal/store/worksession.go`
- Modify: `internal/store/worksession_test.go`

- [ ] **Step 1: Update `WorkSession` struct and `scanWorkSession`**

Add `McpPid *int64` field to `WorkSession`:

```go
type WorkSession struct {
	ID              int64      `json:"id"`
	FeatureID       string     `json:"feature_id"`
	ClaudeSessionID string     `json:"claude_session_id"`
	Status          string     `json:"status"`
	SessionState    string     `json:"session_state"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at"`
	HandoffStale    bool       `json:"handoff_stale"`
	LastHeartbeat   *time.Time `json:"last_heartbeat"`
	McpPid          *int64     `json:"mcp_pid"`
}
```

Update `scanWorkSession` to scan the new column:

```go
func scanWorkSession(row scannable) (*WorkSession, error) {
	var ws WorkSession
	var stale int
	err := row.Scan(&ws.ID, &ws.FeatureID, &ws.ClaudeSessionID, &ws.Status, &ws.SessionState, &ws.StartedAt, &ws.EndedAt, &stale, &ws.LastHeartbeat, &ws.McpPid)
	if err != nil {
		return nil, err
	}
	ws.HandoffStale = stale != 0
	return &ws, nil
}
```

Update ALL SELECT queries in worksession.go to include `mcp_pid` in the column list. Every query that uses `scanWorkSession` must select the column. The affected functions and their query strings:

- `OpenWorkSession` (line 25): add `, mcp_pid` after `last_heartbeat`
- `GetWorkSession` (line 87): add `, mcp_pid` after `last_heartbeat`
- `GetActiveWorkSession` (line 96): add `, mcp_pid` after `last_heartbeat`
- `GetWorkSessionByClaudeSession` (line 108): add `, mcp_pid` after `last_heartbeat`
- `GetOpenWorkSessionForFeature` (line 118): add `, mcp_pid` after `last_heartbeat`

- [ ] **Step 2: Write failing test for SetMcpPid**

```go
func TestSetMcpPid(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	s.CreateFeature("feat-a", "Feature A")

	ws, _ := s.OpenWorkSession("feat-a", "session-1")
	if ws.McpPid != nil {
		t.Fatalf("expected nil McpPid, got %v", *ws.McpPid)
	}

	pid := int64(12345)
	if err := s.SetMcpPid(ws.ID, &pid); err != nil {
		t.Fatal(err)
	}

	reloaded, _ := s.GetWorkSession(ws.ID)
	if reloaded.McpPid == nil || *reloaded.McpPid != 12345 {
		t.Errorf("expected McpPid=12345, got %v", reloaded.McpPid)
	}

	// Clear it
	if err := s.SetMcpPid(ws.ID, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, _ = s.GetWorkSession(ws.ID)
	if reloaded.McpPid != nil {
		t.Errorf("expected nil McpPid after clear, got %v", *reloaded.McpPid)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestSetMcpPid -v`
Expected: FAIL — `SetMcpPid` not defined

- [ ] **Step 4: Implement `SetMcpPid`**

```go
// SetMcpPid sets or clears the MCP server PID claim on a work session.
func (s *Store) SetMcpPid(id int64, pid *int64) error {
	_, err := s.db.Exec(
		`UPDATE work_sessions SET mcp_pid = ? WHERE id = ?`, pid, id,
	)
	return err
}
```

- [ ] **Step 5: Run test**

Run: `go test ./internal/store/ -run TestSetMcpPid -v`
Expected: PASS

- [ ] **Step 6: Run full store test suite**

Run: `go test ./internal/store/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/worksession.go internal/store/worksession_test.go
git commit -m "feat(store): add mcp_pid field and SetMcpPid method"
```

---

### Task 4: Store — Atomic `DequeueCheckpointJob`

**Files:**
- Modify: `internal/store/checkpoint.go:120-147`

- [ ] **Step 1: Replace `DequeueCheckpointJob` with atomic version**

Replace lines 120-147 of `checkpoint.go`:

```go
// DequeueCheckpointJob atomically picks the oldest queued job and marks it running.
func (s *Store) DequeueCheckpointJob() (*CheckpointJob, error) {
	row := s.db.QueryRow(
		`UPDATE checkpoint_jobs
		 SET status = 'running', started_at = datetime('now')
		 WHERE id = (
		     SELECT id FROM checkpoint_jobs
		     WHERE status = 'queued'
		     ORDER BY id ASC LIMIT 1
		 )
		 RETURNING id, work_session_id, feature_id, reason, trigger_type,
		           transcript_start_offset, transcript_end_offset,
		           semantic_text, mechanical_json, status, error,
		           retry_count, created_at, started_at, finished_at`,
	)
	return scanCheckpointJob(row)
}
```

Note: You need a `scanCheckpointJob` helper. Check if one already exists. If `GetCheckpointJob` uses inline scanning, extract it. The RETURNING columns must match the order used in `GetCheckpointJob`'s SELECT.

If `scanCheckpointJob` doesn't exist, look at `GetCheckpointJob` to see the column order and create the helper, then refactor `GetCheckpointJob` to use it too.

Also update the nil return: when no queued jobs exist, `QueryRow` returns `sql.ErrNoRows`. Handle it:

```go
func (s *Store) DequeueCheckpointJob() (*CheckpointJob, error) {
	row := s.db.QueryRow(/* ... as above ... */)
	job, err := scanCheckpointJob(row)
	if err != nil {
		return nil, nil // no queued jobs (or scan error — treat as empty)
	}
	return job, nil
}
```

- [ ] **Step 2: Run checkpoint tests**

Run: `go test ./internal/store/ -run Checkpoint -v`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/store/checkpoint.go
git commit -m "fix(store): atomic DequeueCheckpointJob — prevents multi-worker double-claim"
```

---

### Task 5: MCP Server Identity — Cached Binding State

**Files:**
- Modify: `internal/mcp/server.go`

- [ ] **Step 1: Add binding state to server creation**

Replace `server.go` entirely:

```go
package mcp

import (
	"os"
	"sync"

	"github.com/mark3labs/mcp-go/server"

	"github.com/sniffle6/claude-docket/internal/store"
)

// Binding holds the cached session identity for this MCP server instance.
type Binding struct {
	mu              sync.RWMutex
	WorkSessionID   int64
	FeatureID       string
	ClaudeSessionID string
	Bound           bool
}

func (b *Binding) Get() (wsID int64, featureID, claudeSessionID string, ok bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.WorkSessionID, b.FeatureID, b.ClaudeSessionID, b.Bound
}

func (b *Binding) Set(wsID int64, featureID, claudeSessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.WorkSessionID = wsID
	b.FeatureID = featureID
	b.ClaudeSessionID = claudeSessionID
	b.Bound = true
}

func (b *Binding) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.WorkSessionID = 0
	b.FeatureID = ""
	b.ClaudeSessionID = ""
	b.Bound = false
}

func NewServer(s *store.Store, projectDir string, onCheckpoint func()) *server.MCPServer {
	srv := server.NewMCPServer("docket", "0.1.0",
		server.WithToolCapabilities(true),
	)

	binding := &Binding{}

	// Auto-bind for dashboard launches via env var
	launchFeature := os.Getenv("DOCKET_LAUNCH_FEATURE")

	registerTools(srv, s, projectDir, onCheckpoint, binding, launchFeature)
	return srv
}
```

- [ ] **Step 2: Update `registerTools` signature**

In `tools.go`, update the function signature:

```go
func registerTools(srv *server.MCPServer, s *store.Store, projectDir string, onCheckpoint func(), binding *Binding, launchFeature string) {
```

Pass `binding` and `launchFeature` to handlers that need them: `checkpointHandler` and the new `bindSessionHandler` (added in Task 6). Note: Task 11 will add an `ensureDashboard func()` parameter to both `NewServer` and `registerTools` — plan for that in the signature.

- [ ] **Step 3: Build to verify compilation**

Run: `go build ./cmd/docket/`
Expected: SUCCESS

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/server.go internal/mcp/tools.go
git commit -m "feat(mcp): add Binding state for MCP server session identity"
```

---

### Task 6: `bind_session` MCP Tool

**Files:**
- Modify: `internal/mcp/tools.go` (registration)
- Create: `internal/mcp/tools_bind.go` (handler)

- [ ] **Step 1: Register the tool**

In `tools.go`, within `registerTools()`, add after the `checkpoint` tool registration:

```go
srv.AddTool(mcp.NewTool("bind_session",
	mcp.WithDescription("Bind this MCP server to a specific work session. Call at session start with the session_id and feature_id from the system message. Required before checkpoint can work. Safe to call multiple times — idempotent if already bound."),
	mcp.WithString("feature_id", mcp.Required(), mcp.Description("Feature slug ID to bind to")),
	mcp.WithString("session_id", mcp.Required(), mcp.Description("Claude session ID from the session context system message")),
), bindSessionHandler(s, binding, launchFeature))
```

- [ ] **Step 2: Write the handler**

Create `internal/mcp/tools_bind.go`:

```go
package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/sniffle6/claude-docket/internal/store"
)

func bindSessionHandler(s *store.Store, binding *Binding, launchFeature string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		featureID, _ := args["feature_id"].(string)
		sessionID, _ := args["session_id"].(string)
		if featureID == "" || sessionID == "" {
			return mcp.NewToolResultError("feature_id and session_id are required"), nil
		}

		// Already bound to this session? Return cached context.
		if _, cachedFeature, cachedSession, ok := binding.Get(); ok && cachedSession == sessionID {
			brief, err := s.GetContextBrief(cachedFeature)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Bound to feature %q (already bound).", cachedFeature)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Bound to feature %q (already bound).\n\n%s", cachedFeature, brief)), nil
		}

		// Try to find existing open session for this Claude session
		ws, err := s.GetWorkSessionByClaudeSession(sessionID)
		if err != nil {
			// No existing session — open a new one
			ws, err = s.OpenWorkSession(featureID, sessionID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to open work session: %v", err)), nil
			}
		}

		// Claim with mcp_pid
		pid := int64(os.Getpid())
		s.SetMcpPid(ws.ID, &pid)

		// Cache identity
		binding.Set(ws.ID, ws.FeatureID, ws.ClaudeSessionID)

		// Return context
		brief, err := s.GetContextBrief(ws.FeatureID)
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("Bound to feature %q.", ws.FeatureID)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Bound to feature %q.\n\n%s", ws.FeatureID, brief)), nil
	}
}
```

Note: `s.GetContextBrief` may not exist yet as a store method. Check how `getContextHandler` in `tools.go` builds its context string — it likely constructs it inline. Extract that logic into a `Store.GetContextBrief(featureID string) (string, error)` method so both `get_context` and `bind_session` can use it. If extraction is too complex, just return a simple "Bound to feature X" message without the full context.

- [ ] **Step 3: Build to verify compilation**

Run: `go build ./cmd/docket/`
Expected: SUCCESS

- [ ] **Step 4: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_bind.go
git commit -m "feat(mcp): add bind_session tool for explicit session-to-feature binding"
```

---

### Task 7: Checkpoint Tool — Session Isolation and Cache Clearing

**Files:**
- Modify: `internal/mcp/tools_checkpoint.go`
- Modify: `internal/mcp/tools.go` (pass binding to checkpoint handler)

- [ ] **Step 1: Update checkpoint handler signature**

In `tools.go`, update the checkpoint registration to pass `binding`:

```go
), checkpointHandler(s, projectDir, onCheckpoint, binding))
```

- [ ] **Step 2: Rewrite checkpoint handler to use cached binding**

In `tools_checkpoint.go`, update the handler:

```go
func checkpointHandler(s *store.Store, projectDir string, onCheckpoint func(), binding *Binding) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		endSession := false
		if v, ok := args["end_session"]; ok {
			if b, ok := v.(bool); ok {
				endSession = b
			}
		}

		// Get cached identity
		wsID, featureID, claudeSessionID, bound := binding.Get()
		if !bound {
			// Try env var auto-bind for dashboard launches
			if lf := os.Getenv("DOCKET_LAUNCH_FEATURE"); lf != "" {
				ws, err := s.GetOpenWorkSessionForFeature(lf)
				if err == nil && ws != nil {
					pid := int64(os.Getpid())
					s.SetMcpPid(ws.ID, &pid)
					binding.Set(ws.ID, ws.FeatureID, ws.ClaudeSessionID)
					wsID, featureID, claudeSessionID, bound = ws.ID, ws.FeatureID, ws.ClaudeSessionID, true
				}
			}
			if !bound {
				return mcp.NewToolResultError(
					"No session bound. Call bind_session(feature_id=\"...\", session_id=\"...\") first — see your session context for the IDs.",
				), nil
			}
		}

		// Read transcript offset — scoped by claudeSessionID
		offsetPath := filepath.Join(projectDir, ".docket", "transcript-offset-"+claudeSessionID)
		var startOffset int64
		if data, err := os.ReadFile(offsetPath); err == nil {
			fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &startOffset)
		}

		// Find transcript path
		transcriptPath := findTranscriptPath(claudeSessionID)

		var delta *transcript.Delta
		if transcriptPath != "" {
			var parseErr error
			delta, parseErr = transcript.Parse(transcriptPath, startOffset)
			if parseErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("parse transcript: %v", parseErr)), nil
			}
		} else {
			delta = &transcript.Delta{EndOffset: startOffset}
		}

		reason := "manual_checkpoint"
		if endSession {
			reason = "manual_end_session"
		}

		job, err := s.EnqueueCheckpointJob(store.CheckpointJobInput{
			WorkSessionID:         wsID,
			FeatureID:             featureID,
			Reason:                reason,
			TriggerType:           "manual",
			TranscriptStartOffset: startOffset,
			TranscriptEndOffset:   delta.EndOffset,
			SemanticText:          delta.SemanticText,
			MechanicalFacts:       delta.MechanicalFacts,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("enqueue checkpoint: %v", err)), nil
		}
		if onCheckpoint != nil {
			onCheckpoint()
		}

		// Update offset
		os.WriteFile(offsetPath, []byte(fmt.Sprintf("%d", delta.EndOffset)), 0644)

		if endSession {
			data, err := s.GetHandoffData(featureID)
			if err == nil {
				handoff.WriteFile(projectDir, data, nil)
			}
			s.SetMcpPid(wsID, nil)
			s.CloseWorkSession(wsID)
			binding.Clear()

			return mcp.NewToolResultText(fmt.Sprintf(
				"Work session closed for feature %q. Checkpoint #%d enqueued. Handoff written. Call bind_session to start a new session.",
				featureID, job.ID,
			)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(
			"Checkpoint #%d enqueued for feature %q. %d chars semantic text, %d files edited.",
			job.ID, featureID, len(delta.SemanticText), len(delta.MechanicalFacts.FilesEdited),
		)), nil
	}
}
```

- [ ] **Step 3: Build to verify compilation**

Run: `go build ./cmd/docket/`
Expected: SUCCESS

- [ ] **Step 4: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/tools_checkpoint.go internal/mcp/tools.go
git commit -m "feat(mcp): checkpoint uses cached binding, session-scoped offsets, clears on end_session"
```

---

### Task 8: Hook — SessionStart Binding Logic

**Files:**
- Modify: `cmd/docket/hook.go:153-244`

This is the largest change. SessionStart needs three new behaviors:
1. Read `DOCKET_LAUNCH_FEATURE` env var for dashboard launches
2. Occupancy-aware selection for manual sessions (skip occupied features, skip placeholders, reclaim 24h zombies)
3. Include session ID and `bind_session` instruction in system message

- [ ] **Step 1: Rewrite SessionStart feature selection**

Replace the feature selection logic in `handleSessionStart` (lines 181-202) with:

```go
	features, err := s.ListFeatures("in_progress")
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: list features: %v\n", err)
		json.NewEncoder(w).Encode(hookOutput{Continue: true})
		return
	}

	out := hookOutput{Continue: true}

	if len(features) == 0 {
		out.SystemMessage = archiveMsg + fmt.Sprintf("[docket] Session: %s | No active features. Use docket MCP tools to create one.", h.SessionID)
		json.NewEncoder(w).Encode(out)
		return
	}

	var topFeature store.Feature
	var ws *store.WorkSession
	var bindFailed bool

	// Dashboard launch — bind to specific feature via env var
	if launchFeature := os.Getenv("DOCKET_LAUNCH_FEATURE"); launchFeature != "" {
		for _, f := range features {
			if f.ID == launchFeature {
				topFeature = f
				break
			}
		}
		if topFeature.ID == "" {
			// Feature not found or not in_progress — fall through to occupancy-aware
			topFeature = store.Feature{}
		}
	}

	// Occupancy-aware selection for manual sessions (or if dashboard feature not found)
	if topFeature.ID == "" {
		for _, f := range features {
			openSess, _ := s.GetOpenWorkSessionForFeature(f.ID)
			if openSess == nil {
				topFeature = f
				break
			}
			// Reclaim zombie: unclaimed session with stale heartbeat (>24h)
			if openSess.McpPid == nil &&
				openSess.ClaudeSessionID != "dashboard-launch" &&
				openSess.LastHeartbeat != nil &&
				time.Since(*openSess.LastHeartbeat) > 24*time.Hour {
				topFeature = f
				break
			}
		}
	}

	// Supersession fallback — only supersede real sessions, never placeholders
	if topFeature.ID == "" {
		for _, f := range features {
			openSess, _ := s.GetOpenWorkSessionForFeature(f.ID)
			if openSess != nil && openSess.ClaudeSessionID != "dashboard-launch" {
				topFeature = f
				break
			}
		}
	}

	// Could not bind — all features occupied by placeholders
	if topFeature.ID == "" {
		bindFailed = true
		out.SystemMessage = archiveMsg + fmt.Sprintf(
			"[docket] Session: %s | All in-progress features are occupied.\nUse get_ready to see features, then bind_session(feature_id=\"...\", session_id=\"%s\") to pick one.",
			h.SessionID, h.SessionID,
		)
		json.NewEncoder(w).Encode(out)
		return
	}

	ws, wsErr := s.OpenWorkSession(topFeature.ID, h.SessionID)
	if wsErr == nil {
		s.SetSessionState(ws.ID, "working")
	}
```

- [ ] **Step 2: Update system message to include session ID and bind_session**

Replace the message building section (after the work session is opened) — the existing handoff/fallback logic stays, but wrap the output to include session ID and bind instruction:

After the existing message building (handoff read or fallback), before `json.NewEncoder(w).Encode(out)`, prepend the session and bind info:

```go
	// Prepend session binding info
	bindInfo := fmt.Sprintf("[docket] Session: %s | Feature: %s (id: %s)\nBind docket: bind_session(feature_id=%q, session_id=%q)\n\n",
		h.SessionID, topFeature.Title, topFeature.ID, topFeature.ID, h.SessionID)

	out.SystemMessage = archiveMsg + bindInfo + msg.String()
```

- [ ] **Step 3: Build to verify compilation**

Run: `go build ./cmd/docket/`
Expected: SUCCESS

- [ ] **Step 4: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/docket/hook.go
git commit -m "feat(hooks): SessionStart with DOCKET_LAUNCH_FEATURE binding, occupancy-aware selection, bind_session instruction"
```

---

### Task 9: Hook — Session-Scoped State Files

**Files:**
- Modify: `cmd/docket/hook.go`

- [ ] **Step 1: Add helper for session-scoped file paths**

Add near the existing path helpers (after line 151):

```go
func commitsPath(cwd, sessionID string) string {
	return filepath.Join(cwd, ".docket", "commits-"+sessionID+".log")
}

func transcriptOffsetPath(cwd, sessionID string) string {
	return filepath.Join(cwd, ".docket", "transcript-offset-"+sessionID)
}

func agentNudgedPath(cwd, sessionID string) string {
	return filepath.Join(cwd, ".docket", "agent-nudged-"+sessionID)
}
```

- [ ] **Step 2: Update SessionStart to use scoped paths**

Replace lines 168-179 (clearing state files):

```go
	// Create/clear session-scoped commits log
	os.WriteFile(commitsPath(h.CWD, h.SessionID), []byte{}, 0644)

	// Clear sentinels for new session
	os.Remove(agentNudgedPath(h.CWD, h.SessionID))
	os.Remove(agentPendingPath(h.CWD, h.SessionID))

	// Reset session-scoped transcript offset
	os.WriteFile(transcriptOffsetPath(h.CWD, h.SessionID), []byte("0"), 0644)
```

- [ ] **Step 3: Update `getTranscriptOffset` to accept sessionID**

Change the signature and implementation:

```go
func getTranscriptOffset(cwd, sessionID string) int64 {
	data, err := os.ReadFile(transcriptOffsetPath(cwd, sessionID))
	if err != nil {
		return 0
	}
	var offset int64
	fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &offset)
	return offset
}
```

- [ ] **Step 4: Update all callers of getTranscriptOffset**

Search for `getTranscriptOffset(h.CWD)` and replace with `getTranscriptOffset(h.CWD, h.SessionID)`. There are calls in:
- `handleStop` (enqueuing checkpoint)
- `handleSessionEnd` (enqueuing checkpoint)
- `parseTranscriptDelta` helper

For `parseTranscriptDelta`, update signature to accept sessionID:

```go
func parseTranscriptDelta(h *hookInput) *transcript.Delta {
	if h.TranscriptPath == "" {
		return &transcript.Delta{}
	}
	offset := getTranscriptOffset(h.CWD, h.SessionID)
	delta, err := transcript.Parse(h.TranscriptPath, offset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: parse transcript: %v\n", err)
		return &transcript.Delta{EndOffset: offset}
	}
	return delta
}
```

- [ ] **Step 5: Update offset writes after checkpoint enqueue**

Search for writes to the old `transcript-offset` path. In Stop and SessionEnd handlers, after enqueuing a checkpoint, the offset is updated. Update these to use the scoped path:

```go
os.WriteFile(transcriptOffsetPath(h.CWD, h.SessionID), []byte(fmt.Sprintf("%d", delta.EndOffset)), 0644)
```

- [ ] **Step 6: Update `commits.log` references**

In PostToolUse handler, replace the commits path:

```go
commitsFile := commitsPath(h.CWD, h.SessionID)
```

In SessionEnd handler, replace the commits cleanup:

```go
os.Remove(commitsPath(h.CWD, h.SessionID))
```

- [ ] **Step 7: Update PreToolUse `agent-nudged` sentinel**

Replace `filepath.Join(h.CWD, ".docket", "agent-nudged")` with `agentNudgedPath(h.CWD, h.SessionID)` in all occurrences in `handlePreToolUse`.

- [ ] **Step 8: Build and test**

Run: `go build ./cmd/docket/ && go test ./... -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add cmd/docket/hook.go
git commit -m "feat(hooks): session-scoped state files — commits, transcript-offset, agent-nudged"
```

---

### Task 10: Hook — SessionEnd Handoff Scoping

**Files:**
- Modify: `cmd/docket/hook.go:381-408`

- [ ] **Step 1: Replace the all-features handoff loop**

In `handleSessionEnd`, replace lines 381-408 (the loop that writes handoffs for ALL features) with:

```go
	// Only write handoff for this session's feature
	if ws != nil {
		data, err := s.GetHandoffData(ws.FeatureID)
		if err == nil {
			var cpData *HandoffCheckpointData
			obs, _ := s.GetObservationsForWorkSession(ws.ID)
			mf, _ := s.GetMechanicalFactsForWorkSession(ws.ID)
			if len(obs) > 0 || mf != nil {
				cpData = &HandoffCheckpointData{
					Observations:    obs,
					MechanicalFacts: mf,
				}
			}
			if writeErr := writeHandoffFileWithCheckpoints(h.CWD, data, cpData); writeErr != nil {
				s.MarkHandoffStale(ws.ID)
			}
		}
	}
```

Remove the `cleanStaleHandoffs()` call that follows (around line 407).

- [ ] **Step 2: Build and test**

Run: `go build ./cmd/docket/ && go test ./... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/docket/hook.go
git commit -m "feat(hooks): SessionEnd only writes handoff for own feature, removes cleanStaleHandoffs"
```

---

### Task 11: Dashboard — Leader Election

**Files:**
- Modify: `cmd/docket/main.go:74-123`

- [ ] **Step 1: Replace the dashboard startup with opportunistic leader election**

In `runServe()`, replace lines 85-99 (the dashboard goroutine and port file write) with:

```go
	port := portForDir(dir)
	portStr := fmt.Sprintf("%d", port)

	// Write port file (all instances write the same deterministic port)
	os.WriteFile(filepath.Join(dir, ".docket", "port"), []byte(portStr), 0644)

	handler := dashboard.NewHandler(s, dir, "")

	// dashboardReady is closed once this process is serving HTTP (or gives up)
	dashboardReady := make(chan struct{})
	var dashboardOnce sync.Once

	// tryServeDashboard attempts to bind the port and serve. Returns true if serving.
	tryServeDashboard := func() bool {
		ln, err := net.Listen("tcp", ":"+portStr)
		if err != nil {
			return false // port taken — another instance is the leader
		}
		go func() {
			if err := http.Serve(ln, handler); err != nil {
				fmt.Fprintf(os.Stderr, "docket: dashboard serve error: %v\n", err)
			}
		}()
		dashboardOnce.Do(func() { close(dashboardReady) })
		return true
	}

	// Try to become leader immediately
	if !tryServeDashboard() {
		// Standby — probe every 3 seconds
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if tryServeDashboard() {
					return
				}
			}
		}()
		dashboardOnce.Do(func() { close(dashboardReady) })
	}

	// ensureDashboard is called before each MCP tool call
	ensureDashboard := func() {
		conn, err := net.DialTimeout("tcp", "localhost:"+portStr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return // dashboard is alive
		}
		// Dashboard is down — try to take over
		tryServeDashboard()
	}
	_ = ensureDashboard // will be wired into MCP tool handlers
```

Add required imports: `"net"`, `"sync"`, `"time"`.

- [ ] **Step 2: Wire ensureDashboard into MCP server**

Pass `ensureDashboard` to `NewServer` and have it called before each tool handler. Update `NewServer` signature:

```go
func NewServer(s *store.Store, projectDir string, onCheckpoint func(), ensureDashboard func()) *server.MCPServer {
```

In `registerTools`, wrap each handler with an `ensureDashboard` call. The cleanest way is a middleware wrapper:

```go
func withDashboard(ensureDashboard func(), handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if ensureDashboard != nil {
			ensureDashboard()
		}
		return handler(ctx, req)
	}
}
```

Then wrap each handler in `registerTools`:

```go
), withDashboard(ensureDashboard, addFeatureHandler(s)))
```

Update `main.go` to pass `ensureDashboard` to `NewServer`:

```go
mcpServer := docketmcp.NewServer(s, dir, worker.Notify, ensureDashboard)
```

- [ ] **Step 3: Build**

Run: `go build ./cmd/docket/`
Expected: SUCCESS

- [ ] **Step 4: Commit**

```bash
git add cmd/docket/main.go internal/mcp/server.go internal/mcp/tools.go
git commit -m "feat: opportunistic dashboard leader election with ensureDashboard guard"
```

---

### Task 12: Dashboard — Three-Valued Session Liveness

**Files:**
- Modify: `internal/dashboard/dashboard.go`

- [ ] **Step 1: Add `sessionLiveness` function and type**

Add near the top of `dashboard.go` (after imports):

```go
type sessionLiveness int

const (
	sessionDead    sessionLiveness = iota
	sessionAlive
	sessionUnknown
)

func checkSessionLiveness(ws *store.WorkSession, projDir string) sessionLiveness {
	if ws.McpPid != nil && *ws.McpPid > 0 {
		if isPIDAlive(fmt.Sprintf("%d", *ws.McpPid)) {
			return sessionAlive
		}
		return sessionDead
	}
	if ws.ClaudeSessionID == "dashboard-launch" {
		if isWindowAlive(projDir, ws.FeatureID) {
			return sessionAlive
		}
		return sessionDead
	}
	return sessionUnknown
}
```

- [ ] **Step 2: Update GET /api/features handler**

Replace the liveness check block (dashboard.go:89-105) with:

```go
			if openSess, err := s.GetOpenWorkSessionForFeature(f.ID); err == nil && openSess != nil {
				if fp.SessionState == "" {
					fp.SessionState = "idle"
				}
				pDir := devDir
				if pDir == "" {
					pDir, _ = os.Getwd()
				}
				switch checkSessionLiveness(openSess, pDir) {
				case sessionDead:
					s.CloseWorkSession(openSess.ID)
					fp.SessionState = ""
					fp.LastHeartbeat = nil
				case sessionUnknown:
					fp.SessionState = "unlinked"
				case sessionAlive:
					// keep existing state
				}
			}
```

- [ ] **Step 3: Update POST /api/launch/{id} handler**

Replace the liveness check in the launch handler (dashboard.go:309-318) with:

```go
		openSession, _ := s.GetOpenWorkSessionForFeature(id)
		if openSession != nil {
			switch checkSessionLiveness(openSession, projDir) {
			case sessionDead:
				s.CloseWorkSession(openSession.ID)
				openSession = nil
			case sessionUnknown:
				// Reclaim if stale (>24h heartbeat)
				if openSession.LastHeartbeat != nil && time.Since(*openSession.LastHeartbeat) > 24*time.Hour {
					s.CloseWorkSession(openSession.ID)
					openSession = nil
				}
				// else: leave it, show stale badge
			case sessionAlive:
				// keep openSession — will try to focus
			}
		}
```

- [ ] **Step 4: Build**

Run: `go build ./cmd/docket/`
Expected: SUCCESS

- [ ] **Step 5: Commit**

```bash
git add internal/dashboard/dashboard.go
git commit -m "feat(dashboard): three-valued session liveness with mcp_pid, both call sites"
```

---

### Task 13: Launch Scripts — `DOCKET_LAUNCH_FEATURE` Env Var

**Files:**
- Modify: `internal/dashboard/launch_exec_windows.go:43-48`
- Modify: `internal/dashboard/launch_exec_unix.go:21-27`

- [ ] **Step 1: Update Windows launch script**

In `launch_exec_windows.go`, update the `cmdScript` format string (line 44) to add `set DOCKET_LAUNCH_FEATURE=` before the title line:

```go
cmdScript := fmt.Sprintf("@echo off\r\nset DOCKET_LAUNCH_FEATURE=%s\r\ntitle docket-%s\r\npowershell -NoProfile -Command \"(Get-CimInstance Win32_Process -Filter ('ProcessId='+$PID)).ParentProcessId | Out-File -Encoding ascii -NoNewline '%s'\"\r\ncd /d \"%s\"\r\nclaude --dangerously-skip-permissions --append-system-prompt-file \"%s\" \"Resume work on: %s (feature_id: %s). Check get_ready for current status.\"\r\ndel \"%s\" 2>nul\r\n",
	featureID, featureID, pidPath, projDir, promptPath, featureTitle, featureID, pidPath)
```

- [ ] **Step 2: Update Unix launch script**

In `launch_exec_unix.go`, update the script format string (line 22) to add `export DOCKET_LAUNCH_FEATURE=`:

```go
script := fmt.Sprintf("#!/bin/sh\nexport DOCKET_LAUNCH_FEATURE=%s\ncd %q\nclaude --dangerously-skip-permissions --append-system-prompt-file %q \"Resume work on: %s (feature_id: %s). Check get_ready for current status.\"\n",
	featureID, projDir, promptPath, featureTitle, featureID)
```

- [ ] **Step 3: Build**

Run: `go build ./cmd/docket/`
Expected: SUCCESS

- [ ] **Step 4: Commit**

```bash
git add internal/dashboard/launch_exec_windows.go internal/dashboard/launch_exec_unix.go
git commit -m "feat(dashboard): add DOCKET_LAUNCH_FEATURE env var to launch scripts"
```

---

### Task 14: Downstream Prompt and Skill Updates

**Files:**
- Modify: `plugin/skills/end-session/SKILL.md`
- Modify: `cmd/docket/update.go`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update end-session skill**

In `plugin/skills/end-session/SKILL.md`, replace line 27:

```
- Starting work on a new feature after this will open a new work session.
```

With:

```
- After closing, call `bind_session(feature_id="...", session_id="...")` to start tracking a new feature. The session ID is shown in the session context.
```

- [ ] **Step 2: Update CLAUDE.md snippet in update.go**

In `cmd/docket/update.go`, update `docketSectionHead` to mention `bind_session`. Replace the "Larger features" line:

```go
**Larger features**: call ` + "`get_ready`" + `, then dispatch ` + "`board-manager`" + ` agent (model: sonnet) to create or find a card. Use ` + "`type`" + ` (feature/bugfix/chore/spike) for auto-generated subtask templates. Always pass ` + "`tags`" + ` when calling ` + "`add_feature`" + ` — use existing tags from ` + "`list_features`" + `.
```

With:

```go
**Larger features**: call ` + "`get_ready`" + `, then dispatch ` + "`board-manager`" + ` agent (model: sonnet) to create or find a card. Call ` + "`bind_session(feature_id, session_id)`" + ` to bind the session (session ID is in the session context message). Use ` + "`type`" + ` (feature/bugfix/chore/spike) for auto-generated subtask templates. Always pass ` + "`tags`" + ` when calling ` + "`add_feature`" + ` — use existing tags from ` + "`list_features`" + `.
```

- [ ] **Step 3: Update project CLAUDE.md**

Run `docket.exe update` to regenerate the snippet, or manually update the `## Feature Tracking (docket)` section in `CLAUDE.md` to match the updated `docketSectionHead`.

- [ ] **Step 4: Build and install**

Run: `go build ./cmd/docket/ && bash dev-build.sh`
Expected: SUCCESS

- [ ] **Step 5: Commit**

```bash
git add plugin/skills/end-session/SKILL.md cmd/docket/update.go CLAUDE.md
git commit -m "docs: update prompts and skills for bind_session flow"
```

---

### Task 15: Verification Tests

**Files:**
- Modify: `internal/store/worksession_test.go`

- [ ] **Step 1: Test bind_session reopening after end_session**

```go
func TestBindSessionReopenAfterEndSession(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	s.CreateFeature("feat-a", "Feature A")
	s.CreateFeature("feat-b", "Feature B")

	// Open session on feat-a
	ws1, _ := s.OpenWorkSession("feat-a", "session-1")
	pid := int64(99999)
	s.SetMcpPid(ws1.ID, &pid)

	// Simulate end_session: close and clear mcp_pid
	s.SetMcpPid(ws1.ID, nil)
	s.CloseWorkSession(ws1.ID)

	// Rebind to feat-b (same claude session)
	ws2, err := s.OpenWorkSession("feat-b", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if ws2.FeatureID != "feat-b" {
		t.Errorf("expected feat-b, got %q", ws2.FeatureID)
	}
	if ws2.Status != "open" {
		t.Errorf("expected open, got %q", ws2.Status)
	}

	// Original session should still be closed
	reloaded, _ := s.GetWorkSession(ws1.ID)
	if reloaded.Status != "closed" {
		t.Errorf("original session should be closed, got %q", reloaded.Status)
	}
}
```

- [ ] **Step 2: Test 24h zombie reclaim**

```go
func TestZombieSessionReclaim(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	s.CreateFeature("feat-a", "Feature A")

	// Create a zombie: open session, no mcp_pid, stale heartbeat
	ws, _ := s.OpenWorkSession("feat-a", "old-session")
	// Manually set heartbeat to 25 hours ago
	s.db.Exec(`UPDATE work_sessions SET last_heartbeat = datetime('now', '-25 hours') WHERE id = ?`, ws.ID)

	// Reload to verify stale heartbeat
	reloaded, _ := s.GetWorkSession(ws.ID)
	if reloaded.McpPid != nil {
		t.Fatal("expected nil McpPid for zombie")
	}

	// New session supersedes the zombie
	ws2, err := s.OpenWorkSession("feat-a", "new-session")
	if err != nil {
		t.Fatal(err)
	}
	if ws2.ClaudeSessionID != "new-session" {
		t.Errorf("expected new-session, got %q", ws2.ClaudeSessionID)
	}

	// Zombie should be closed
	reloaded, _ = s.GetWorkSession(ws.ID)
	if reloaded.Status != "closed" {
		t.Errorf("zombie should be closed, got %q", reloaded.Status)
	}
}
```

- [ ] **Step 3: Test manual sessions refuse placeholders**

```go
func TestManualSessionSkipsPlaceholder(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	s.CreateFeature("feat-a", "Feature A")
	s.CreateFeature("feat-b", "Feature B")

	// Create placeholder for feat-a (dashboard launch pending)
	s.CreatePlaceholderSession("feat-a")

	// Manual session should skip feat-a (has placeholder) and bind to feat-b
	// Simulate occupancy check: feat-a has open session, feat-b doesn't
	openA, _ := s.GetOpenWorkSessionForFeature("feat-a")
	if openA == nil {
		t.Fatal("expected placeholder session for feat-a")
	}
	openB, _ := s.GetOpenWorkSessionForFeature("feat-b")
	if openB != nil {
		t.Fatal("expected no session for feat-b")
	}

	// Open manual session on feat-b (the first unoccupied feature)
	ws, err := s.OpenWorkSession("feat-b", "manual-session")
	if err != nil {
		t.Fatal(err)
	}
	if ws.FeatureID != "feat-b" {
		t.Errorf("expected feat-b, got %q", ws.FeatureID)
	}

	// Placeholder should still be intact
	openA, _ = s.GetOpenWorkSessionForFeature("feat-a")
	if openA == nil || openA.ClaudeSessionID != "dashboard-launch" {
		t.Error("placeholder for feat-a should still exist")
	}
}
```

- [ ] **Step 4: Run all tests**

Run: `go test ./internal/store/ -run "TestBindSessionReopen|TestZombieSession|TestManualSessionSkips" -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/worksession_test.go
git commit -m "test(store): verification tests for bind_session lifecycle, zombie reclaim, placeholder protection"
```

---

### Task 16: Final Build and Integration Verify

**Files:** None new

- [ ] **Step 1: Full build**

Run: `go build -ldflags="-s -w" -o docket.exe ./cmd/docket/`
Expected: SUCCESS

- [ ] **Step 2: Full test suite**

Run: `go test ./... -v`
Expected: ALL PASS

- [ ] **Step 3: Dev build and reload**

Run: `bash dev-build.sh`
Then: `/reload-plugins`

- [ ] **Step 4: Smoke test**

Open the dashboard (check `.docket/port` for the port). Verify:
- Dashboard loads
- Features display
- Session state shows correctly

- [ ] **Step 5: Update feature doc**

Write or update `docs/multi-session-mcp.md` documenting what was built, key files, and how multi-session works.

- [ ] **Step 6: Commit doc**

```bash
git add docs/multi-session-mcp.md
git commit -m "docs: add multi-session MCP lifecycle feature doc"
```
