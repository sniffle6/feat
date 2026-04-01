# Session Heartbeat & Staleness Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect crashed/killed Claude sessions by tracking heartbeat timestamps, showing "Stale (Xm)" on the dashboard instead of falsely showing "Working" on dead sessions.

**Architecture:** Hook-driven heartbeat with dashboard-side staleness evaluation. Every hook event updates a `last_heartbeat` column. The dashboard JS compares heartbeat age against a 5-minute threshold and renders a "Stale" indicator when too old. No background workers needed.

**Tech Stack:** Go, SQLite, vanilla JS

**Spec:** `docs/superpowers/specs/2026-04-01-session-heartbeat-staleness-design.md`

---

### Task 1: Schema Migration V13

**Files:**
- Modify: `internal/store/migrate.go:162-193`

- [ ] **Step 1: Write the V13 migration constant**

Add after the existing `schemaV12` constant:

```go
const schemaV13 = `
CREATE TABLE IF NOT EXISTS work_sessions_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feature_id TEXT NOT NULL REFERENCES features(id),
    claude_session_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open', 'closed')),
    session_state TEXT NOT NULL DEFAULT 'idle' CHECK(session_state IN ('idle', 'working', 'needs_attention', 'stale')),
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    ended_at DATETIME,
    handoff_stale INTEGER NOT NULL DEFAULT 0,
    last_heartbeat DATETIME
);

INSERT OR IGNORE INTO work_sessions_new SELECT id, feature_id, claude_session_id, status, session_state, started_at, ended_at, handoff_stale, NULL FROM work_sessions;
DROP TABLE IF EXISTS work_sessions;
ALTER TABLE work_sessions_new RENAME TO work_sessions;
`
```

- [ ] **Step 2: Register V13 in the migrate function**

Add at the end of `migrate()`, before `return nil`:

```go
// v13: add last_heartbeat column, add 'stale' to session_state CHECK constraint
db.Exec(schemaV13)
```

- [ ] **Step 3: Run tests to verify migration doesn't break anything**

Run: `go test ./internal/store/... -v -run TestOpen`
Expected: PASS — existing tests still work with the new schema.

- [ ] **Step 4: Commit**

```bash
git add internal/store/migrate.go
git commit -m "feat: add V13 migration for last_heartbeat column and stale state"
```

---

### Task 2: TouchHeartbeat Store Method + Tests

**Files:**
- Modify: `internal/store/worksession.go`
- Modify: `internal/store/worksession_test.go`

- [ ] **Step 1: Write the failing test for TouchHeartbeat**

Add to `internal/store/worksession_test.go`:

```go
func TestTouchHeartbeat(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")
	ws, _ := s.OpenWorkSession("auth-system", "session-123")

	s.TouchHeartbeat(ws.ID)

	ws2, _ := s.GetWorkSession(ws.ID)
	if ws2.LastHeartbeat == nil {
		t.Fatal("expected LastHeartbeat to be set after TouchHeartbeat")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -v -run TestTouchHeartbeat`
Expected: FAIL — `TouchHeartbeat` method and `LastHeartbeat` field don't exist yet.

- [ ] **Step 3: Add LastHeartbeat field to WorkSession struct**

In `internal/store/worksession.go`, update the `WorkSession` struct:

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
}
```

- [ ] **Step 4: Update scanWorkSession to read the new column**

Replace the existing `scanWorkSession` function:

```go
func scanWorkSession(row scannable) (*WorkSession, error) {
	var ws WorkSession
	var stale int
	err := row.Scan(&ws.ID, &ws.FeatureID, &ws.ClaudeSessionID, &ws.Status, &ws.SessionState, &ws.StartedAt, &ws.EndedAt, &stale, &ws.LastHeartbeat)
	if err != nil {
		return nil, err
	}
	ws.HandoffStale = stale != 0
	return &ws, nil
}
```

- [ ] **Step 5: Update all SELECT queries to include last_heartbeat**

Every query that selects from `work_sessions` needs the new column. Update these methods:
- `OpenWorkSession` — the SELECT in the resume check
- `GetWorkSession`
- `GetActiveWorkSession`

All three use the same column list. Update each to:

```sql
SELECT id, feature_id, claude_session_id, status, session_state, started_at, ended_at, handoff_stale, last_heartbeat
FROM work_sessions ...
```

- [ ] **Step 6: Implement TouchHeartbeat**

Add to `internal/store/worksession.go`:

```go
// TouchHeartbeat updates the last_heartbeat timestamp for a work session.
func (s *Store) TouchHeartbeat(id int64) {
	s.db.Exec(`UPDATE work_sessions SET last_heartbeat = datetime('now') WHERE id = ?`, id)
}
```

- [ ] **Step 7: Run test to verify it passes**

Run: `go test ./internal/store/... -v -run TestTouchHeartbeat`
Expected: PASS

- [ ] **Step 8: Run all store tests to check nothing broke**

Run: `go test ./internal/store/... -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add internal/store/worksession.go internal/store/worksession_test.go
git commit -m "feat: add TouchHeartbeat method and LastHeartbeat field"
```

---

### Task 3: Set Heartbeat on OpenWorkSession + Tests

**Files:**
- Modify: `internal/store/worksession.go`
- Modify: `internal/store/worksession_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/store/worksession_test.go`:

```go
func TestOpenWorkSessionSetsHeartbeat(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")

	ws, err := s.OpenWorkSession("auth-system", "session-123")
	if err != nil {
		t.Fatalf("OpenWorkSession: %v", err)
	}
	if ws.LastHeartbeat == nil {
		t.Fatal("expected LastHeartbeat to be set on new work session")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -v -run TestOpenWorkSessionSetsHeartbeat`
Expected: FAIL — `LastHeartbeat` is nil because the INSERT doesn't set it.

- [ ] **Step 3: Update OpenWorkSession INSERT to set last_heartbeat**

In `OpenWorkSession`, change the INSERT statement from:

```go
res, err := s.db.Exec(
    `INSERT INTO work_sessions (feature_id, claude_session_id, status, started_at) VALUES (?, ?, 'open', ?)`,
    featureID, claudeSessionID, now,
)
```

to:

```go
res, err := s.db.Exec(
    `INSERT INTO work_sessions (feature_id, claude_session_id, status, started_at, last_heartbeat) VALUES (?, ?, 'open', ?, ?)`,
    featureID, claudeSessionID, now, now,
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/... -v -run TestOpenWorkSessionSetsHeartbeat`
Expected: PASS

- [ ] **Step 5: Run all store tests**

Run: `go test ./internal/store/... -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/worksession.go internal/store/worksession_test.go
git commit -m "feat: set last_heartbeat on work session creation"
```

---

### Task 4: Update GetActiveSessionStates to Return Heartbeat

**Files:**
- Modify: `internal/store/worksession.go`
- Modify: `internal/store/worksession_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/store/worksession_test.go`:

```go
func TestGetActiveSessionStatesReturnsHeartbeat(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")
	ws, _ := s.OpenWorkSession("feature-a", "session-1")
	s.SetSessionState(ws.ID, "working")
	s.TouchHeartbeat(ws.ID)

	states, err := s.GetActiveSessionStates()
	if err != nil {
		t.Fatalf("GetActiveSessionStates: %v", err)
	}
	info, ok := states["feature-a"]
	if !ok {
		t.Fatal("expected feature-a in active session states")
	}
	if info.State != "working" {
		t.Errorf("State = %q, want %q", info.State, "working")
	}
	if info.LastHeartbeat == nil {
		t.Fatal("expected LastHeartbeat to be set")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -v -run TestGetActiveSessionStatesReturnsHeartbeat`
Expected: FAIL — `GetActiveSessionStates` returns `map[string]string`, not a struct.

- [ ] **Step 3: Create SessionStateInfo struct and update GetActiveSessionStates**

In `internal/store/worksession.go`, add the struct and update the method:

```go
// SessionStateInfo holds session state and heartbeat for dashboard consumption.
type SessionStateInfo struct {
	State         string     `json:"state"`
	LastHeartbeat *time.Time `json:"last_heartbeat"`
}

// GetActiveSessionStates returns feature_id → SessionStateInfo for all open
// work sessions with a non-idle state.
func (s *Store) GetActiveSessionStates() (map[string]SessionStateInfo, error) {
	rows, err := s.db.Query(
		`SELECT feature_id, session_state, last_heartbeat FROM work_sessions
		 WHERE status = 'open' AND session_state != 'idle'`,
	)
	if err != nil {
		return nil, fmt.Errorf("get active session states: %w", err)
	}
	defer rows.Close()

	states := make(map[string]SessionStateInfo)
	for rows.Next() {
		var featureID string
		var info SessionStateInfo
		if err := rows.Scan(&featureID, &info.State, &info.LastHeartbeat); err != nil {
			return nil, err
		}
		states[featureID] = info
	}
	return states, nil
}
```

- [ ] **Step 4: Fix all callers of GetActiveSessionStates**

The return type changed from `map[string]string` to `map[string]SessionStateInfo`. Update callers:

**`internal/dashboard/dashboard.go`** — in the `GET /api/features` handler, change:

```go
if state, ok := sessionStates[f.ID]; ok {
    fp.SessionState = state
} else {
    fp.SessionState = "idle"
}
```

to:

```go
if info, ok := sessionStates[f.ID]; ok {
    fp.SessionState = info.State
    fp.LastHeartbeat = info.LastHeartbeat
} else {
    fp.SessionState = "idle"
}
```

And add `LastHeartbeat` to the `featureWithProgress` struct:

```go
type featureWithProgress struct {
	store.Feature
	ProgressDone    int               `json:"progress_done"`
	ProgressTotal   int               `json:"progress_total"`
	NextTask        string            `json:"next_task"`
	SubtaskProgress []subtaskProgress `json:"subtask_progress"`
	IssueCount      int               `json:"issue_count"`
	SessionState    string            `json:"session_state"`
	LastHeartbeat   *time.Time        `json:"last_heartbeat,omitempty"`
}
```

Add the `"time"` import if not already present.

**`internal/dashboard/dashboard.go`** — in the `POST /api/launch/{id}` handler, change:

```go
states, _ := s.GetActiveSessionStates()
if state, ok := states[id]; ok && (state == "working" || state == "needs_attention") {
```

to:

```go
states, _ := s.GetActiveSessionStates()
if info, ok := states[id]; ok && (info.State == "working" || info.State == "needs_attention") {
```

- [ ] **Step 5: Fix existing GetActiveSessionStates tests**

Update `TestGetActiveSessionStates` in `internal/store/worksession_test.go` — change:

```go
if states["feature-b"] != "needs_attention" {
    t.Errorf("feature-b state = %q, want %q", states["feature-b"], "needs_attention")
}

// C should be absent
if _, ok := states["feature-c"]; ok {
    t.Error("feature-c should not be in active session states")
}
```

to:

```go
if states["feature-b"].State != "needs_attention" {
    t.Errorf("feature-b state = %q, want %q", states["feature-b"].State, "needs_attention")
}

// C should be absent
if _, ok := states["feature-c"]; ok {
    t.Error("feature-c should not be in active session states")
}
```

Update `TestGetActiveSessionStates_ExcludesIdle` — no change needed (it only checks `_, ok`).

- [ ] **Step 6: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/worksession.go internal/store/worksession_test.go internal/dashboard/dashboard.go
git commit -m "feat: return heartbeat data from GetActiveSessionStates"
```

---

### Task 5: Add Heartbeat Calls to Hook Handlers + Tests

**Files:**
- Modify: `cmd/docket/hook.go`
- Modify: `cmd/docket/hook_test.go`

- [ ] **Step 1: Write the failing test for Stop hook heartbeat**

Add to `cmd/docket/hook_test.go`:

```go
func TestStopHookTouchesHeartbeat(t *testing.T) {
	dir := t.TempDir()
	s, _ := store.Open(dir)
	f, _ := s.AddFeature("Heartbeat Feature", "testing")
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: strPtr("in_progress")})
	ws, _ := s.OpenWorkSession(f.ID, "sess-1")
	s.SetSessionState(ws.ID, "working")
	s.Close()

	h := &hookInput{
		SessionID:     "sess-1",
		CWD:           dir,
		HookEventName: "Stop",
	}

	var buf bytes.Buffer
	handleStop(h, &buf)

	s2, _ := store.Open(dir)
	defer s2.Close()
	ws2, _ := s2.GetWorkSession(ws.ID)
	if ws2.LastHeartbeat == nil {
		t.Error("expected LastHeartbeat to be set after Stop hook")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/docket/... -v -run TestStopHookTouchesHeartbeat`
Expected: FAIL — `handleStop` doesn't call `TouchHeartbeat`.

- [ ] **Step 3: Add TouchHeartbeat calls to all 4 hook handlers**

In `cmd/docket/hook.go`:

**handleSessionStart** — after `s.SetSessionState(ws.ID, "working")` (line 190), heartbeat is already set by `OpenWorkSession`, so no additional call needed here. The INSERT sets `last_heartbeat` to `now`.

**handleStop** — add after `s.SetSessionState(ws.ID, "needs_attention")` (line 249):

```go
s.TouchHeartbeat(ws.ID)
```

**handlePreCompact** — add after `ws, err := s.GetActiveWorkSession()` (line 284), before the delta parsing:

```go
s.TouchHeartbeat(ws.ID)
```

**handlePostToolUse** — add inside the sentinel check block. After `s.SetSessionState(ws.ID, "working")` (line 470):

```go
s.TouchHeartbeat(ws.ID)
```

But PostToolUse also needs a heartbeat when there's NO sentinel file (normal tool use). The sentinel path only opens the store when `needs-attention` exists. For the common case (tool use while working), we need to touch the heartbeat too. Add after the sentinel block (after line 474), before the git commit check:

```go
// Touch heartbeat on every PostToolUse to prove the session is alive.
// Only open SQLite if we haven't already (sentinel block above handles that case).
if _, statErr := os.Stat(filepath.Join(h.CWD, ".docket", "needs-attention")); statErr != nil {
    // Sentinel was already removed above, or never existed — either way,
    // we haven't touched heartbeat yet in this call.
}
```

Wait — this gets complicated. The sentinel block already opens and closes the store. For the non-sentinel case, we'd need to open the store again just for heartbeat. That's expensive on every tool call.

Better approach: use a lightweight file-based heartbeat instead of SQLite for PostToolUse. Write the current time to `.docket/last-heartbeat` as a file. Then in `GetActiveSessionStates`, also check this file.

Actually, that overcomplicates things. The simpler approach: only touch heartbeat in the sentinel block (when state flips) and in the git commit block (which already opens the store). For the common-case PostToolUse (no sentinel, no git commit), the Stop hook provides the heartbeat. Since Stop fires whenever Claude stops generating, it covers the "is the session alive?" question adequately.

So the PostToolUse heartbeat is: **only when the store is already open** (sentinel flip or git commit paths).

Update the sentinel block to add heartbeat:

```go
sentinelPath := filepath.Join(h.CWD, ".docket", "needs-attention")
if _, statErr := os.Stat(sentinelPath); statErr == nil {
    os.Remove(sentinelPath)
    if s, err := store.Open(h.CWD); err == nil {
        if ws, wsErr := s.GetActiveWorkSession(); wsErr == nil {
            s.SetSessionState(ws.ID, "working")
            s.TouchHeartbeat(ws.ID)
        }
        s.Close()
    }
}
```

And in the git commit path, after opening the store (line 503-508 area), add heartbeat. The store is already open there. After `features, err := s.ListFeatures("in_progress")`:

```go
if ws, wsErr := s.GetActiveWorkSession(); wsErr == nil {
    s.TouchHeartbeat(ws.ID)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/docket/... -v -run TestStopHookTouchesHeartbeat`
Expected: PASS

- [ ] **Step 5: Write test for PreCompact hook heartbeat**

Add to `cmd/docket/hook_test.go`:

```go
func TestPreCompactHookTouchesHeartbeat(t *testing.T) {
	dir := t.TempDir()
	s, _ := store.Open(dir)
	f, _ := s.AddFeature("Compact Feature", "testing")
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: strPtr("in_progress")})
	ws, _ := s.OpenWorkSession(f.ID, "sess-1")
	s.SetSessionState(ws.ID, "working")
	s.Close()

	h := &hookInput{
		SessionID:     "sess-1",
		CWD:           dir,
		HookEventName: "PreCompact",
	}

	var buf bytes.Buffer
	handlePreCompact(h, &buf)

	s2, _ := store.Open(dir)
	defer s2.Close()
	ws2, _ := s2.GetWorkSession(ws.ID)
	if ws2.LastHeartbeat == nil {
		t.Error("expected LastHeartbeat to be set after PreCompact hook")
	}
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./cmd/docket/... -v -run TestPreCompactHookTouchesHeartbeat`
Expected: PASS

- [ ] **Step 7: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add cmd/docket/hook.go cmd/docket/hook_test.go
git commit -m "feat: add heartbeat touches to Stop, PreCompact, and PostToolUse hooks"
```

---

### Task 6: Dashboard Staleness Rendering

**Files:**
- Modify: `dashboard/index.html`

- [ ] **Step 1: Add the stale CSS class**

In `dashboard/index.html`, after the `.status-attention .pulse` block (after line 239), add:

```css
.status-stale {
    color: var(--muted);
}
.status-stale .dot {
    width: 6px; height: 6px; border-radius: 50%;
    background: var(--muted);
}
```

Note: static dot (no animation), using the `.dot` class instead of `.pulse` to make the distinction clear.

- [ ] **Step 2: Update the session status rendering in JS**

Replace the session status indicator block (lines 719-731) with staleness-aware logic:

```javascript
// Session status indicator
var state = f.session_state || 'idle';
var lastHeartbeat = f.last_heartbeat ? new Date(f.last_heartbeat) : null;
var isStale = false;
var staleMinutes = 0;

if ((state === 'working' || state === 'needs_attention') && lastHeartbeat) {
    staleMinutes = Math.floor((Date.now() - lastHeartbeat.getTime()) / 60000);
    isStale = staleMinutes >= 5;
}

if (isStale) {
    var statusSpan = document.createElement('span');
    statusSpan.className = 'session-status status-stale';
    var staleText = staleMinutes < 60
        ? 'Stale (' + staleMinutes + 'm)'
        : staleMinutes < 1440
            ? 'Stale (' + Math.floor(staleMinutes / 60) + 'h)'
            : 'Stale (' + Math.floor(staleMinutes / 1440) + 'd)';
    statusSpan.innerHTML = '<span class="dot"></span> ' + staleText;
    headerRow.appendChild(statusSpan);
} else if (state === 'working') {
    var statusSpan = document.createElement('span');
    statusSpan.className = 'session-status status-working';
    statusSpan.innerHTML = '<span class="pulse"></span> Working';
    headerRow.appendChild(statusSpan);
} else if (state === 'needs_attention') {
    var statusSpan = document.createElement('span');
    statusSpan.className = 'session-status status-attention';
    statusSpan.innerHTML = '<span class="pulse"></span> Waiting';
    headerRow.appendChild(statusSpan);
}
```

- [ ] **Step 3: Update launch button logic for stale sessions**

The launch button (lines 734-741) currently disables when `state === 'working'`. Update it to also allow launching when stale:

```javascript
var launchBtn = document.createElement('button');
launchBtn.className = 'launch-btn';
launchBtn.innerHTML = '&#9654;'; // ▶
if (state === 'working' && !isStale) {
    launchBtn.title = 'Session active';
    launchBtn.disabled = true;
} else {
    launchBtn.title = 'Launch Claude session';
}
```

- [ ] **Step 4: Verify manually**

Build and check the dashboard renders correctly:

Run: `go build -ldflags="-s -w" -o docket.exe ./cmd/docket/`
Expected: Build succeeds. Open dashboard in browser, verify stale indicator renders with gray static dot when a session has been idle for >5 minutes.

- [ ] **Step 5: Commit**

```bash
git add dashboard/index.html
git commit -m "feat: render stale session indicator on dashboard"
```

---

### Task 7: Update Launch Endpoint for Stale Sessions

**Files:**
- Modify: `internal/dashboard/dashboard.go`

- [ ] **Step 1: Update the launch endpoint's active session check**

In the `POST /api/launch/{id}` handler, the current check blocks launching when a session is active. Update it to also allow launching over stale sessions. Replace:

```go
states, _ := s.GetActiveSessionStates()
if info, ok := states[id]; ok && (info.State == "working" || info.State == "needs_attention") {
    http.Error(w, "session already active for this feature", 409)
    return
}
```

with:

```go
states, _ := s.GetActiveSessionStates()
if info, ok := states[id]; ok && (info.State == "working" || info.State == "needs_attention") {
    // Allow launching over stale sessions (heartbeat older than 5 minutes)
    stale := info.LastHeartbeat != nil && time.Since(*info.LastHeartbeat) > 5*time.Minute
    if !stale {
        http.Error(w, "session already active for this feature", 409)
        return
    }
}
```

Add `"time"` to the imports if not already present.

- [ ] **Step 2: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 3: Build to verify compilation**

Run: `go build -ldflags="-s -w" -o docket.exe ./cmd/docket/`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add internal/dashboard/dashboard.go
git commit -m "feat: allow launching sessions over stale ones"
```

---

### Task 8: Documentation

**Files:**
- Modify: `docs/session-heartbeat-staleness.md` (create)

- [ ] **Step 1: Write the feature doc**

Create `docs/session-heartbeat-staleness.md`:

```markdown
# Session Heartbeat & Staleness Detection

Detects crashed or killed Claude Code sessions and shows them as "Stale" on the dashboard instead of falsely showing "Working."

## How It Works

Every hook event (Stop, PreCompact, PostToolUse with state flip or git commit) updates a `last_heartbeat` timestamp on the work session in SQLite. When the dashboard renders, it compares the heartbeat age against a 5-minute threshold. If the heartbeat is older than 5 minutes and the session is still marked as "working" or "needs_attention," the dashboard shows a gray "Stale (Xm)" indicator instead.

Staleness is a display-only concern — the DB stays in its last known state. This is because the MCP server (which writes to the DB) dies with the Claude session, so it can't update its own state when it crashes.

## Dashboard Indicators

| State | Indicator | Action |
|-------|-----------|--------|
| Working | Green pulsing dot | Session is live, launch disabled |
| Waiting | Yellow pulsing dot | Claude needs input, launch enabled |
| Stale | Gray static dot + "(Xm)" | Session probably dead, launch enabled |
| Idle | No indicator | No active session |

## Cleanup

Stale sessions auto-resolve when a new session starts — `OpenWorkSession` closes all other open sessions. No manual cleanup needed.

## Key Files

- `internal/store/migrate.go` — V13 migration (last_heartbeat column, stale CHECK value)
- `internal/store/worksession.go` — `TouchHeartbeat`, `SessionStateInfo`, updated `GetActiveSessionStates`
- `cmd/docket/hook.go` — heartbeat calls in Stop, PreCompact, PostToolUse hooks
- `dashboard/index.html` — staleness evaluation and rendering
- `internal/dashboard/dashboard.go` — passes `last_heartbeat` through API, stale-aware launch check
```

- [ ] **Step 2: Commit**

```bash
git add docs/session-heartbeat-staleness.md
git commit -m "docs: add session heartbeat & staleness detection doc"
```
