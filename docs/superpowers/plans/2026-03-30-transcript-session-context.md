# Transcript-Based Session Context Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Docket's two-phase Stop hook with checkpoint-based context preservation that uses transcript parsing and background LLM summarization to capture semantic session context automatically.

**Architecture:** Hooks extract transcript deltas and enqueue checkpoint jobs into SQLite. A background worker in the MCP server process drains the queue and calls the Anthropic Messages API for semantic summarization. Handoff files are rendered from accumulated checkpoint observations at SessionEnd or /end-session.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Anthropic Messages API (net/http), JSONL parsing

---

## File Structure

### New files

| File | Responsibility |
|------|----------------|
| `internal/transcript/parse.go` | JSONL transcript reader, delta filter, mechanical fact extractor |
| `internal/transcript/parse_test.go` | Tests with fixture JSONL data |
| `internal/transcript/types.go` | `SessionActivity`, `FileAction`, `TestRun`, `ErrorEvent` structs |
| `internal/checkpoint/worker.go` | Background job queue worker, polls `checkpoint_jobs`, orchestrates summarization |
| `internal/checkpoint/worker_test.go` | Worker tests with mock summarizer |
| `internal/checkpoint/summarizer.go` | `SummarizerBackend` interface, `SummarizeInput`/`SummarizeOutput` types |
| `internal/checkpoint/anthropic.go` | Anthropic Messages API implementation of `SummarizerBackend` |
| `internal/checkpoint/anthropic_test.go` | Tests with HTTP test server |
| `internal/checkpoint/config.go` | Config loading from env vars |
| `internal/checkpoint/noop.go` | No-op summarizer for when no API key is configured |
| `internal/store/worksession.go` | Work session CRUD (open, close, get active, mark stale) |
| `internal/store/worksession_test.go` | Work session store tests |
| `internal/store/checkpoint.go` | Checkpoint job + observation CRUD |
| `internal/store/checkpoint_test.go` | Checkpoint store tests |
| `plugin/skills/checkpoint/SKILL.md` | /checkpoint skill definition |
| `plugin/skills/end-session/SKILL.md` | /end-session skill definition |

### Modified files

| File | Changes |
|------|---------|
| `internal/store/migrate.go` | Add schemaV9 with three new tables |
| `cmd/docket/hook.go` | Rewrite Stop, add PreCompact/SessionEnd handlers, update hookInput struct |
| `cmd/docket/handoff.go` | Add "Last Session" section from checkpoint observations |
| `cmd/docket/main.go` | Start checkpoint worker in runServe() |
| `internal/mcp/tools.go` | Remove `log_session`, add `checkpoint` tool |
| `internal/mcp/tools_session.go` | Remove `logSessionHandler`, keep `compactSessionsHandler` |
| `internal/store/store.go` | Remove `MarkSessionLogged`/`WasSessionLogged`/`ClearSessionLogged` |
| `plugin/hooks/hooks.json` | Add PreCompact/SessionEnd hooks, bump Stop timeout |

---

### Task 1: Schema Migration — New Tables

**Files:**
- Modify: `internal/store/migrate.go:87-114`
- Test: `internal/store/feature_test.go` (existing test verifies migration runs)

- [ ] **Step 1: Write the schema migration**

Add `schemaV9` to `internal/store/migrate.go` with the three new tables:

```go
const schemaV9 = `
CREATE TABLE IF NOT EXISTS work_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feature_id TEXT NOT NULL REFERENCES features(id),
    claude_session_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open', 'closed')),
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    ended_at DATETIME,
    handoff_stale INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS checkpoint_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    work_session_id INTEGER NOT NULL REFERENCES work_sessions(id),
    feature_id TEXT NOT NULL,
    reason TEXT NOT NULL CHECK(reason IN ('stop', 'precompact', 'manual_checkpoint', 'manual_end_session')),
    trigger_type TEXT NOT NULL DEFAULT '',
    transcript_start_offset INTEGER NOT NULL DEFAULT 0,
    transcript_end_offset INTEGER NOT NULL DEFAULT 0,
    semantic_text TEXT NOT NULL DEFAULT '',
    mechanical_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued', 'running', 'done', 'failed', 'skipped')),
    error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    started_at DATETIME,
    finished_at DATETIME
);

CREATE TABLE IF NOT EXISTS checkpoint_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    checkpoint_job_id INTEGER NOT NULL REFERENCES checkpoint_jobs(id),
    work_session_id INTEGER NOT NULL REFERENCES work_sessions(id),
    feature_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('summary', 'blocker', 'decision_candidate', 'dead_end', 'next_step', 'gotcha')),
    payload_json TEXT NOT NULL DEFAULT '{}',
    summary_text TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
`
```

- [ ] **Step 2: Wire migration into migrate()**

Add to the `migrate()` function in `internal/store/migrate.go`, after the `db.Exec(schemaV8)` line:

```go
// v9: add work_sessions, checkpoint_jobs, checkpoint_observations tables
db.Exec(schemaV9)
```

- [ ] **Step 3: Run existing tests to verify migration doesn't break anything**

Run: `go test ./internal/store/ -v -run TestAddFeature`
Expected: PASS (the migration runs silently on every store open)

- [ ] **Step 4: Commit**

```bash
git add internal/store/migrate.go
git commit -m "feat: add schema v9 — work_sessions, checkpoint_jobs, checkpoint_observations tables"
```

---

### Task 2: Work Session Store Methods

**Files:**
- Create: `internal/store/worksession.go`
- Create: `internal/store/worksession_test.go`

- [ ] **Step 1: Write failing tests for work session lifecycle**

Create `internal/store/worksession_test.go`:

```go
package store

import (
    "testing"
)

func TestOpenWorkSession(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")

    ws, err := s.OpenWorkSession("auth-system", "session-123")
    if err != nil {
        t.Fatalf("OpenWorkSession: %v", err)
    }
    if ws.FeatureID != "auth-system" {
        t.Errorf("FeatureID = %q, want %q", ws.FeatureID, "auth-system")
    }
    if ws.ClaudeSessionID != "session-123" {
        t.Errorf("ClaudeSessionID = %q, want %q", ws.ClaudeSessionID, "session-123")
    }
    if ws.Status != "open" {
        t.Errorf("Status = %q, want %q", ws.Status, "open")
    }
}

func TestOpenWorkSessionResumesExisting(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")

    ws1, _ := s.OpenWorkSession("auth-system", "session-123")
    ws2, _ := s.OpenWorkSession("auth-system", "session-123")

    if ws1.ID != ws2.ID {
        t.Errorf("expected same work session ID, got %d and %d", ws1.ID, ws2.ID)
    }
}

func TestGetActiveWorkSession(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")

    _, err := s.GetActiveWorkSession()
    if err == nil {
        t.Fatal("expected error when no active work session")
    }

    s.OpenWorkSession("auth-system", "session-123")

    ws, err := s.GetActiveWorkSession()
    if err != nil {
        t.Fatalf("GetActiveWorkSession: %v", err)
    }
    if ws.FeatureID != "auth-system" {
        t.Errorf("FeatureID = %q, want %q", ws.FeatureID, "auth-system")
    }
}

func TestCloseWorkSession(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "session-123")

    err := s.CloseWorkSession(ws.ID)
    if err != nil {
        t.Fatalf("CloseWorkSession: %v", err)
    }

    _, err = s.GetActiveWorkSession()
    if err == nil {
        t.Fatal("expected no active work session after close")
    }
}

func TestMarkHandoffStale(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "session-123")

    s.MarkHandoffStale(ws.ID)
    ws2, _ := s.GetWorkSession(ws.ID)
    if !ws2.HandoffStale {
        t.Error("expected HandoffStale to be true")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -v -run TestOpenWorkSession`
Expected: FAIL — `OpenWorkSession` not defined

- [ ] **Step 3: Implement work session store methods**

Create `internal/store/worksession.go`:

```go
package store

import (
    "fmt"
    "time"
)

type WorkSession struct {
    ID              int64     `json:"id"`
    FeatureID       string    `json:"feature_id"`
    ClaudeSessionID string    `json:"claude_session_id"`
    Status          string    `json:"status"`
    StartedAt       time.Time `json:"started_at"`
    EndedAt         *time.Time `json:"ended_at"`
    HandoffStale    bool      `json:"handoff_stale"`
}

// OpenWorkSession opens a new work session or resumes an existing open one
// for the same feature + claude session.
func (s *Store) OpenWorkSession(featureID, claudeSessionID string) (*WorkSession, error) {
    // Try to resume existing open session for same feature+claude session
    row := s.db.QueryRow(
        `SELECT id, feature_id, claude_session_id, status, started_at, ended_at, handoff_stale
         FROM work_sessions WHERE feature_id = ? AND claude_session_id = ? AND status = 'open'`,
        featureID, claudeSessionID,
    )
    ws, err := scanWorkSession(row)
    if err == nil {
        return ws, nil
    }

    // Close any other open sessions (one active session at a time)
    s.db.Exec(`UPDATE work_sessions SET status = 'closed', ended_at = datetime('now') WHERE status = 'open'`)

    now := time.Now().UTC()
    res, err := s.db.Exec(
        `INSERT INTO work_sessions (feature_id, claude_session_id, status, started_at) VALUES (?, ?, 'open', ?)`,
        featureID, claudeSessionID, now,
    )
    if err != nil {
        return nil, fmt.Errorf("insert work session: %w", err)
    }
    id, _ := res.LastInsertId()
    return s.GetWorkSession(id)
}

func (s *Store) GetWorkSession(id int64) (*WorkSession, error) {
    row := s.db.QueryRow(
        `SELECT id, feature_id, claude_session_id, status, started_at, ended_at, handoff_stale
         FROM work_sessions WHERE id = ?`, id,
    )
    return scanWorkSession(row)
}

// GetActiveWorkSession returns the single open work session, or error if none.
func (s *Store) GetActiveWorkSession() (*WorkSession, error) {
    row := s.db.QueryRow(
        `SELECT id, feature_id, claude_session_id, status, started_at, ended_at, handoff_stale
         FROM work_sessions WHERE status = 'open' ORDER BY id DESC LIMIT 1`,
    )
    return scanWorkSession(row)
}

func (s *Store) CloseWorkSession(id int64) error {
    _, err := s.db.Exec(
        `UPDATE work_sessions SET status = 'closed', ended_at = datetime('now') WHERE id = ?`, id,
    )
    return err
}

func (s *Store) MarkHandoffStale(id int64) {
    s.db.Exec(`UPDATE work_sessions SET handoff_stale = 1 WHERE id = ?`, id)
}

type scannable interface {
    Scan(dest ...any) error
}

func scanWorkSession(row scannable) (*WorkSession, error) {
    var ws WorkSession
    var stale int
    err := row.Scan(&ws.ID, &ws.FeatureID, &ws.ClaudeSessionID, &ws.Status, &ws.StartedAt, &ws.EndedAt, &stale)
    if err != nil {
        return nil, err
    }
    ws.HandoffStale = stale != 0
    return &ws, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -v -run "TestOpenWorkSession|TestGetActiveWorkSession|TestCloseWorkSession|TestMarkHandoffStale"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/worksession.go internal/store/worksession_test.go
git commit -m "feat: add work session store methods — open, close, get active, mark stale"
```

---

### Task 3: Checkpoint Store Methods

**Files:**
- Create: `internal/store/checkpoint.go`
- Create: `internal/store/checkpoint_test.go`

- [ ] **Step 1: Write failing tests for checkpoint job and observation CRUD**

Create `internal/store/checkpoint_test.go`:

```go
package store

import (
    "encoding/json"
    "testing"
)

func TestEnqueueCheckpointJob(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "sess-1")

    job, err := s.EnqueueCheckpointJob(CheckpointJobInput{
        WorkSessionID:        ws.ID,
        FeatureID:            "auth-system",
        Reason:               "stop",
        TriggerType:          "auto",
        TranscriptStartOffset: 0,
        TranscriptEndOffset:  1024,
        SemanticText:         "discussed auth token design",
        MechanicalFacts:      MechanicalFacts{FilesEdited: []FileEdit{{Path: "auth.go", Count: 2}}},
    })
    if err != nil {
        t.Fatalf("EnqueueCheckpointJob: %v", err)
    }
    if job.Status != "queued" {
        t.Errorf("Status = %q, want %q", job.Status, "queued")
    }
    if job.FeatureID != "auth-system" {
        t.Errorf("FeatureID = %q, want %q", job.FeatureID, "auth-system")
    }
}

func TestDequeueCheckpointJob(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "sess-1")

    s.EnqueueCheckpointJob(CheckpointJobInput{
        WorkSessionID:        ws.ID,
        FeatureID:            "auth-system",
        Reason:               "stop",
        TranscriptStartOffset: 0,
        TranscriptEndOffset:  512,
        SemanticText:         "some text",
        MechanicalFacts:      MechanicalFacts{},
    })

    job, err := s.DequeueCheckpointJob()
    if err != nil {
        t.Fatalf("DequeueCheckpointJob: %v", err)
    }
    if job == nil {
        t.Fatal("expected a job, got nil")
    }
    if job.Status != "running" {
        t.Errorf("Status = %q, want %q", job.Status, "running")
    }

    // Second dequeue should return nil (no more queued jobs)
    job2, _ := s.DequeueCheckpointJob()
    if job2 != nil {
        t.Error("expected nil on second dequeue")
    }
}

func TestCompleteCheckpointJob(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "sess-1")

    enqueued, _ := s.EnqueueCheckpointJob(CheckpointJobInput{
        WorkSessionID:        ws.ID,
        FeatureID:            "auth-system",
        Reason:               "stop",
        TranscriptStartOffset: 0,
        TranscriptEndOffset:  512,
        SemanticText:         "text",
        MechanicalFacts:      MechanicalFacts{},
    })

    s.DequeueCheckpointJob()
    err := s.CompleteCheckpointJob(enqueued.ID, nil)
    if err != nil {
        t.Fatalf("CompleteCheckpointJob: %v", err)
    }

    job, _ := s.GetCheckpointJob(enqueued.ID)
    if job.Status != "done" {
        t.Errorf("Status = %q, want %q", job.Status, "done")
    }
}

func TestFailCheckpointJob(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "sess-1")

    enqueued, _ := s.EnqueueCheckpointJob(CheckpointJobInput{
        WorkSessionID:        ws.ID,
        FeatureID:            "auth-system",
        Reason:               "stop",
        TranscriptStartOffset: 0,
        TranscriptEndOffset:  512,
        SemanticText:         "text",
        MechanicalFacts:      MechanicalFacts{},
    })

    s.DequeueCheckpointJob()
    err := s.FailCheckpointJob(enqueued.ID, "api timeout")
    if err != nil {
        t.Fatalf("FailCheckpointJob: %v", err)
    }

    job, _ := s.GetCheckpointJob(enqueued.ID)
    if job.Status != "failed" {
        t.Errorf("Status = %q, want %q", job.Status, "failed")
    }
    if job.Error != "api timeout" {
        t.Errorf("Error = %q, want %q", job.Error, "api timeout")
    }
}

func TestAddCheckpointObservation(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "sess-1")

    job, _ := s.EnqueueCheckpointJob(CheckpointJobInput{
        WorkSessionID:        ws.ID,
        FeatureID:            "auth-system",
        Reason:               "stop",
        TranscriptStartOffset: 0,
        TranscriptEndOffset:  512,
        SemanticText:         "text",
        MechanicalFacts:      MechanicalFacts{},
    })

    obs, err := s.AddCheckpointObservation(CheckpointObservationInput{
        CheckpointJobID: job.ID,
        WorkSessionID:   ws.ID,
        FeatureID:       "auth-system",
        Kind:            "summary",
        PayloadJSON:     `{"goals": ["implement refresh tokens"]}`,
        SummaryText:     "Discussed token refresh design. Decided to use rotating refresh tokens.",
    })
    if err != nil {
        t.Fatalf("AddCheckpointObservation: %v", err)
    }
    if obs.Kind != "summary" {
        t.Errorf("Kind = %q, want %q", obs.Kind, "summary")
    }
}

func TestGetObservationsForWorkSession(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "sess-1")

    job, _ := s.EnqueueCheckpointJob(CheckpointJobInput{
        WorkSessionID: ws.ID, FeatureID: "auth-system", Reason: "stop",
        TranscriptStartOffset: 0, TranscriptEndOffset: 512,
        SemanticText: "text", MechanicalFacts: MechanicalFacts{},
    })

    s.AddCheckpointObservation(CheckpointObservationInput{
        CheckpointJobID: job.ID, WorkSessionID: ws.ID, FeatureID: "auth-system",
        Kind: "summary", SummaryText: "First checkpoint",
    })
    s.AddCheckpointObservation(CheckpointObservationInput{
        CheckpointJobID: job.ID, WorkSessionID: ws.ID, FeatureID: "auth-system",
        Kind: "blocker", SummaryText: "Need API key for external service",
    })

    obs, err := s.GetObservationsForWorkSession(ws.ID)
    if err != nil {
        t.Fatalf("GetObservationsForWorkSession: %v", err)
    }
    if len(obs) != 2 {
        t.Fatalf("got %d observations, want 2", len(obs))
    }
}

func TestCheckpointIdempotency(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "sess-1")

    input := CheckpointJobInput{
        WorkSessionID:        ws.ID,
        FeatureID:            "auth-system",
        Reason:               "stop",
        TranscriptStartOffset: 0,
        TranscriptEndOffset:  512,
        SemanticText:         "text",
        MechanicalFacts:      MechanicalFacts{},
    }

    job1, _ := s.EnqueueCheckpointJob(input)
    job2, _ := s.EnqueueCheckpointJob(input)

    if job1.ID != job2.ID {
        t.Errorf("expected idempotent enqueue, got IDs %d and %d", job1.ID, job2.ID)
    }
}

func TestGetMechanicalFactsForWorkSession(t *testing.T) {
    s := openTestStore(t)
    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "sess-1")

    facts1 := MechanicalFacts{
        FilesEdited: []FileEdit{{Path: "auth.go", Count: 2}},
        Commits:     []CommitFact{{Hash: "abc123", Message: "add auth"}},
    }
    facts2 := MechanicalFacts{
        FilesEdited: []FileEdit{{Path: "middleware.go", Count: 1}},
        TestRuns:    []TestRunFact{{Command: "go test ./...", Passed: true}},
    }

    s.EnqueueCheckpointJob(CheckpointJobInput{
        WorkSessionID: ws.ID, FeatureID: "auth-system", Reason: "stop",
        TranscriptStartOffset: 0, TranscriptEndOffset: 512,
        SemanticText: "text", MechanicalFacts: facts1,
    })
    s.EnqueueCheckpointJob(CheckpointJobInput{
        WorkSessionID: ws.ID, FeatureID: "auth-system", Reason: "stop",
        TranscriptStartOffset: 512, TranscriptEndOffset: 1024,
        SemanticText: "more text", MechanicalFacts: facts2,
    })

    merged, err := s.GetMechanicalFactsForWorkSession(ws.ID)
    if err != nil {
        t.Fatalf("GetMechanicalFactsForWorkSession: %v", err)
    }
    if len(merged.FilesEdited) != 2 {
        t.Errorf("FilesEdited = %d, want 2", len(merged.FilesEdited))
    }
    if len(merged.Commits) != 1 {
        t.Errorf("Commits = %d, want 1", len(merged.Commits))
    }
    if len(merged.TestRuns) != 1 {
        t.Errorf("TestRuns = %d, want 1", len(merged.TestRuns))
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -v -run "TestEnqueueCheckpoint|TestDequeueCheckpoint|TestCompleteCheckpoint|TestFailCheckpoint|TestAddCheckpointObservation|TestGetObservations|TestCheckpointIdempotency|TestGetMechanicalFacts"`
Expected: FAIL — types and methods not defined

- [ ] **Step 3: Implement checkpoint store types and methods**

Create `internal/store/checkpoint.go`:

```go
package store

import (
    "encoding/json"
    "fmt"
    "time"
)

type FileEdit struct {
    Path  string `json:"path"`
    Count int    `json:"count"`
}

type TestRunFact struct {
    Command string `json:"command"`
    Passed  bool   `json:"passed"`
}

type CommitFact struct {
    Hash    string `json:"hash"`
    Message string `json:"message"`
}

type ErrorFact struct {
    Tool    string `json:"tool"`
    Message string `json:"message"`
}

type MechanicalFacts struct {
    FilesEdited []FileEdit    `json:"files_edited,omitempty"`
    TestRuns    []TestRunFact `json:"test_runs,omitempty"`
    Commits     []CommitFact  `json:"commits,omitempty"`
    Errors      []ErrorFact   `json:"errors,omitempty"`
}

type CheckpointJob struct {
    ID                    int64      `json:"id"`
    WorkSessionID         int64      `json:"work_session_id"`
    FeatureID             string     `json:"feature_id"`
    Reason                string     `json:"reason"`
    TriggerType           string     `json:"trigger_type"`
    TranscriptStartOffset int64      `json:"transcript_start_offset"`
    TranscriptEndOffset   int64      `json:"transcript_end_offset"`
    SemanticText          string     `json:"semantic_text"`
    MechanicalJSON        string     `json:"mechanical_json"`
    Status                string     `json:"status"`
    Error                 string     `json:"error"`
    CreatedAt             time.Time  `json:"created_at"`
    StartedAt             *time.Time `json:"started_at"`
    FinishedAt            *time.Time `json:"finished_at"`
}

type CheckpointJobInput struct {
    WorkSessionID         int64
    FeatureID             string
    Reason                string
    TriggerType           string
    TranscriptStartOffset int64
    TranscriptEndOffset   int64
    SemanticText          string
    MechanicalFacts       MechanicalFacts
}

type CheckpointObservation struct {
    ID              int64     `json:"id"`
    CheckpointJobID int64     `json:"checkpoint_job_id"`
    WorkSessionID   int64     `json:"work_session_id"`
    FeatureID       string    `json:"feature_id"`
    Kind            string    `json:"kind"`
    PayloadJSON     string    `json:"payload_json"`
    SummaryText     string    `json:"summary_text"`
    CreatedAt       time.Time `json:"created_at"`
}

type CheckpointObservationInput struct {
    CheckpointJobID int64
    WorkSessionID   int64
    FeatureID       string
    Kind            string
    PayloadJSON     string
    SummaryText     string
}

// EnqueueCheckpointJob inserts a checkpoint job. Idempotent — if a job with
// the same work_session_id + transcript offsets + feature_id already exists,
// returns the existing job.
func (s *Store) EnqueueCheckpointJob(input CheckpointJobInput) (*CheckpointJob, error) {
    // Idempotency check
    row := s.db.QueryRow(
        `SELECT id FROM checkpoint_jobs
         WHERE work_session_id = ? AND feature_id = ?
           AND transcript_start_offset = ? AND transcript_end_offset = ?`,
        input.WorkSessionID, input.FeatureID,
        input.TranscriptStartOffset, input.TranscriptEndOffset,
    )
    var existingID int64
    if err := row.Scan(&existingID); err == nil {
        return s.GetCheckpointJob(existingID)
    }

    mechJSON, _ := json.Marshal(input.MechanicalFacts)
    res, err := s.db.Exec(
        `INSERT INTO checkpoint_jobs
         (work_session_id, feature_id, reason, trigger_type,
          transcript_start_offset, transcript_end_offset,
          semantic_text, mechanical_json, status)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'queued')`,
        input.WorkSessionID, input.FeatureID, input.Reason, input.TriggerType,
        input.TranscriptStartOffset, input.TranscriptEndOffset,
        input.SemanticText, string(mechJSON),
    )
    if err != nil {
        return nil, fmt.Errorf("enqueue checkpoint job: %w", err)
    }
    id, _ := res.LastInsertId()
    return s.GetCheckpointJob(id)
}

// DequeueCheckpointJob atomically picks the oldest queued job and marks it running.
func (s *Store) DequeueCheckpointJob() (*CheckpointJob, error) {
    tx, err := s.db.Begin()
    if err != nil {
        return nil, err
    }
    defer tx.Rollback()

    row := tx.QueryRow(
        `SELECT id FROM checkpoint_jobs WHERE status = 'queued' ORDER BY id ASC LIMIT 1`,
    )
    var id int64
    if err := row.Scan(&id); err != nil {
        return nil, nil // no queued jobs
    }

    _, err = tx.Exec(
        `UPDATE checkpoint_jobs SET status = 'running', started_at = datetime('now') WHERE id = ?`, id,
    )
    if err != nil {
        return nil, err
    }

    if err := tx.Commit(); err != nil {
        return nil, err
    }
    return s.GetCheckpointJob(id)
}

func (s *Store) CompleteCheckpointJob(id int64, errMsg *string) error {
    if errMsg != nil {
        return s.FailCheckpointJob(id, *errMsg)
    }
    _, err := s.db.Exec(
        `UPDATE checkpoint_jobs SET status = 'done', finished_at = datetime('now') WHERE id = ?`, id,
    )
    return err
}

func (s *Store) FailCheckpointJob(id int64, errMsg string) error {
    _, err := s.db.Exec(
        `UPDATE checkpoint_jobs SET status = 'failed', error = ?, finished_at = datetime('now') WHERE id = ?`,
        errMsg, id,
    )
    return err
}

func (s *Store) GetCheckpointJob(id int64) (*CheckpointJob, error) {
    row := s.db.QueryRow(
        `SELECT id, work_session_id, feature_id, reason, trigger_type,
                transcript_start_offset, transcript_end_offset,
                semantic_text, mechanical_json, status, error,
                created_at, started_at, finished_at
         FROM checkpoint_jobs WHERE id = ?`, id,
    )
    var job CheckpointJob
    err := row.Scan(
        &job.ID, &job.WorkSessionID, &job.FeatureID, &job.Reason, &job.TriggerType,
        &job.TranscriptStartOffset, &job.TranscriptEndOffset,
        &job.SemanticText, &job.MechanicalJSON, &job.Status, &job.Error,
        &job.CreatedAt, &job.StartedAt, &job.FinishedAt,
    )
    if err != nil {
        return nil, fmt.Errorf("get checkpoint job %d: %w", id, err)
    }
    return &job, nil
}

func (s *Store) AddCheckpointObservation(input CheckpointObservationInput) (*CheckpointObservation, error) {
    if input.PayloadJSON == "" {
        input.PayloadJSON = "{}"
    }
    res, err := s.db.Exec(
        `INSERT INTO checkpoint_observations
         (checkpoint_job_id, work_session_id, feature_id, kind, payload_json, summary_text)
         VALUES (?, ?, ?, ?, ?, ?)`,
        input.CheckpointJobID, input.WorkSessionID, input.FeatureID,
        input.Kind, input.PayloadJSON, input.SummaryText,
    )
    if err != nil {
        return nil, fmt.Errorf("add checkpoint observation: %w", err)
    }
    id, _ := res.LastInsertId()
    row := s.db.QueryRow(
        `SELECT id, checkpoint_job_id, work_session_id, feature_id, kind, payload_json, summary_text, created_at
         FROM checkpoint_observations WHERE id = ?`, id,
    )
    var obs CheckpointObservation
    err = row.Scan(&obs.ID, &obs.CheckpointJobID, &obs.WorkSessionID, &obs.FeatureID,
        &obs.Kind, &obs.PayloadJSON, &obs.SummaryText, &obs.CreatedAt)
    if err != nil {
        return nil, err
    }
    return &obs, nil
}

func (s *Store) GetObservationsForWorkSession(workSessionID int64) ([]CheckpointObservation, error) {
    rows, err := s.db.Query(
        `SELECT id, checkpoint_job_id, work_session_id, feature_id, kind, payload_json, summary_text, created_at
         FROM checkpoint_observations WHERE work_session_id = ? ORDER BY id ASC`, workSessionID,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var obs []CheckpointObservation
    for rows.Next() {
        var o CheckpointObservation
        if err := rows.Scan(&o.ID, &o.CheckpointJobID, &o.WorkSessionID, &o.FeatureID,
            &o.Kind, &o.PayloadJSON, &o.SummaryText, &o.CreatedAt); err != nil {
            return nil, err
        }
        obs = append(obs, o)
    }
    return obs, nil
}

// GetMechanicalFactsForWorkSession merges mechanical facts from all checkpoint
// jobs in a work session into a single MechanicalFacts.
func (s *Store) GetMechanicalFactsForWorkSession(workSessionID int64) (*MechanicalFacts, error) {
    rows, err := s.db.Query(
        `SELECT mechanical_json FROM checkpoint_jobs WHERE work_session_id = ? ORDER BY id ASC`,
        workSessionID,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    merged := &MechanicalFacts{}
    for rows.Next() {
        var raw string
        if err := rows.Scan(&raw); err != nil {
            continue
        }
        var facts MechanicalFacts
        if err := json.Unmarshal([]byte(raw), &facts); err != nil {
            continue
        }
        merged.FilesEdited = append(merged.FilesEdited, facts.FilesEdited...)
        merged.TestRuns = append(merged.TestRuns, facts.TestRuns...)
        merged.Commits = append(merged.Commits, facts.Commits...)
        merged.Errors = append(merged.Errors, facts.Errors...)
    }
    return merged, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -v -run "TestEnqueueCheckpoint|TestDequeueCheckpoint|TestCompleteCheckpoint|TestFailCheckpoint|TestAddCheckpointObservation|TestGetObservations|TestCheckpointIdempotency|TestGetMechanicalFacts"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/checkpoint.go internal/store/checkpoint_test.go
git commit -m "feat: add checkpoint job queue and observation store methods"
```

---

### Task 4: Transcript Parser

**Files:**
- Create: `internal/transcript/types.go`
- Create: `internal/transcript/parse.go`
- Create: `internal/transcript/parse_test.go`

- [ ] **Step 1: Define transcript types**

Create `internal/transcript/types.go`:

```go
package transcript

import "github.com/sniffle6/claude-docket/internal/store"

// Delta is the result of parsing a transcript range. It contains
// filtered semantic text (user/assistant only) and mechanical facts.
type Delta struct {
    SemanticText    string              // filtered user+assistant text
    MechanicalFacts store.MechanicalFacts
    EndOffset       int64               // byte offset after last processed line
    HasContent      bool                // true if any non-trivial content found
}

// trivialUserMessages are acknowledgment-only messages that don't count
// as meaningful user input.
var trivialUserMessages = map[string]bool{
    "ok":       true,
    "okay":     true,
    "thanks":   true,
    "thank you": true,
    "continue": true,
    "go on":    true,
    "run it":   true,
    "yep":      true,
    "yes":      true,
    "yeah":     true,
    "sure":     true,
    "go":       true,
    "do it":    true,
    "looks good": true,
    "lgtm":     true,
    "sounds good": true,
}
```

- [ ] **Step 2: Write failing tests for transcript parsing**

Create `internal/transcript/parse_test.go`:

```go
package transcript

import (
    "os"
    "path/filepath"
    "testing"
)

func writeFixture(t *testing.T, lines ...string) string {
    t.Helper()
    dir := t.TempDir()
    path := filepath.Join(dir, "transcript.jsonl")
    var content string
    for _, line := range lines {
        content += line + "\n"
    }
    os.WriteFile(path, []byte(content), 0644)
    return path
}

func TestParseAssistantText(t *testing.T) {
    path := writeFixture(t,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I'll fix the auth bug by updating the token validation."}]}}`,
    )
    delta, err := Parse(path, 0)
    if err != nil {
        t.Fatalf("Parse: %v", err)
    }
    if delta.SemanticText == "" {
        t.Error("expected semantic text from assistant message")
    }
    if !delta.HasContent {
        t.Error("expected HasContent=true")
    }
}

func TestParseFiltersToolResults(t *testing.T) {
    path := writeFixture(t,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Let me read the file."},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/tmp/foo.go"}}]}}`,
        `{"type":"tool_result","tool_use_id":"t1","content":"package main\nfunc main() {}"}`,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"The file looks correct."}]}}`,
    )
    delta, err := Parse(path, 0)
    if err != nil {
        t.Fatalf("Parse: %v", err)
    }
    // Should contain assistant text but not the tool result content
    if len(delta.SemanticText) > 200 {
        t.Errorf("semantic text too long (%d chars), likely includes tool result", len(delta.SemanticText))
    }
}

func TestParseExtractsFileEdits(t *testing.T) {
    path := writeFixture(t,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/tmp/store.go","old_string":"foo","new_string":"bar"}}]}}`,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"Edit","input":{"file_path":"/tmp/store.go","old_string":"baz","new_string":"qux"}}]}}`,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t3","name":"Write","input":{"file_path":"/tmp/new.go","content":"package new"}}]}}`,
    )
    delta, err := Parse(path, 0)
    if err != nil {
        t.Fatalf("Parse: %v", err)
    }
    if len(delta.MechanicalFacts.FilesEdited) != 2 {
        t.Errorf("FilesEdited = %d, want 2 (store.go counted once, new.go once)", len(delta.MechanicalFacts.FilesEdited))
    }
}

func TestParseExtractsTestRuns(t *testing.T) {
    path := writeFixture(t,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}`,
        `{"type":"tool_result","tool_use_id":"t1","content":"ok  \tpkg\t0.5s","isError":false}`,
    )
    delta, err := Parse(path, 0)
    if err != nil {
        t.Fatalf("Parse: %v", err)
    }
    if len(delta.MechanicalFacts.TestRuns) != 1 {
        t.Fatalf("TestRuns = %d, want 1", len(delta.MechanicalFacts.TestRuns))
    }
    if !delta.MechanicalFacts.TestRuns[0].Passed {
        t.Error("expected test run to show as passed")
    }
}

func TestParseExtractsErrors(t *testing.T) {
    path := writeFixture(t,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls /nonexistent"}}]}}`,
        `{"type":"tool_result","tool_use_id":"t1","content":"ls: cannot access '/nonexistent'","isError":true}`,
    )
    delta, err := Parse(path, 0)
    if err != nil {
        t.Fatalf("Parse: %v", err)
    }
    if len(delta.MechanicalFacts.Errors) != 1 {
        t.Fatalf("Errors = %d, want 1", len(delta.MechanicalFacts.Errors))
    }
}

func TestParseFromOffset(t *testing.T) {
    line1 := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"First message."}]}}`
    line2 := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Second message."}]}}`
    path := writeFixture(t, line1, line2)

    // Parse from offset after first line
    offset := int64(len(line1) + 1) // +1 for newline
    delta, err := Parse(path, offset)
    if err != nil {
        t.Fatalf("Parse: %v", err)
    }
    if delta.SemanticText != "Second message.\n" {
        t.Errorf("SemanticText = %q, want only second message", delta.SemanticText)
    }
}

func TestParseTrivialUserMessages(t *testing.T) {
    path := writeFixture(t,
        `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"ok"}]}}`,
        `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"thanks"}]}}`,
    )
    delta, err := Parse(path, 0)
    if err != nil {
        t.Fatalf("Parse: %v", err)
    }
    if delta.HasContent {
        t.Error("expected HasContent=false for trivial-only messages")
    }
}

func TestParseSkipsMalformedLines(t *testing.T) {
    path := writeFixture(t,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Good line."}]}}`,
        `this is not json`,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Another good line."}]}}`,
    )
    delta, err := Parse(path, 0)
    if err != nil {
        t.Fatalf("Parse: %v", err)
    }
    if delta.SemanticText == "" {
        t.Error("expected semantic text from non-malformed lines")
    }
}

func TestParseMissingFile(t *testing.T) {
    delta, err := Parse("/nonexistent/transcript.jsonl", 0)
    if err != nil {
        t.Fatalf("expected no error for missing file, got: %v", err)
    }
    if delta.HasContent {
        t.Error("expected empty delta for missing file")
    }
}

func TestParseExtractsCommits(t *testing.T) {
    path := writeFixture(t,
        `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"git commit -m \"fix auth bug\""}}]}}`,
        `{"type":"tool_result","tool_use_id":"t1","content":"[main abc1234] fix auth bug\n 1 file changed","isError":false}`,
    )
    delta, err := Parse(path, 0)
    if err != nil {
        t.Fatalf("Parse: %v", err)
    }
    if len(delta.MechanicalFacts.Commits) != 1 {
        t.Fatalf("Commits = %d, want 1", len(delta.MechanicalFacts.Commits))
    }
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/transcript/ -v`
Expected: FAIL — `Parse` not defined

- [ ] **Step 4: Implement transcript parser**

Create `internal/transcript/parse.go`:

```go
package transcript

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
    "regexp"
    "strings"

    "github.com/sniffle6/claude-docket/internal/store"
)

// transcriptRecord represents one line in the JSONL transcript.
type transcriptRecord struct {
    Type    string          `json:"type"`
    Message *messagePayload `json:"message"`
    // tool_result fields
    ToolUseID string          `json:"tool_use_id"`
    Content   json.RawMessage `json:"content"`
    IsError   bool            `json:"isError"`
}

type messagePayload struct {
    Role    string         `json:"role"`
    Content []contentBlock `json:"content"`
}

type contentBlock struct {
    Type  string          `json:"type"`
    Text  string          `json:"text"`
    Name  string          `json:"name"`
    ID    string          `json:"id"`
    Input json.RawMessage `json:"input"`
}

type toolInput struct {
    FilePath string `json:"file_path"`
    Command  string `json:"command"`
}

var (
    testCmdPattern  = regexp.MustCompile(`(?i)(go test|npm test|pytest|jest|cargo test|make test)`)
    commitPattern   = regexp.MustCompile(`\[[\w/.-]+ ([a-f0-9]{7,40})\] (.+)`)
)

// Parse reads a JSONL transcript from startOffset and returns a Delta
// containing filtered semantic text and mechanical facts.
// Returns an empty Delta (not an error) if the file doesn't exist or is empty.
func Parse(path string, startOffset int64) (*Delta, error) {
    f, err := os.Open(path)
    if err != nil {
        // Missing file = empty delta, not an error
        return &Delta{EndOffset: startOffset}, nil
    }
    defer f.Close()

    if startOffset > 0 {
        if _, err := f.Seek(startOffset, 0); err != nil {
            return &Delta{EndOffset: startOffset}, nil
        }
    }

    delta := &Delta{EndOffset: startOffset}
    var semanticBuf strings.Builder

    // Track pending tool uses for matching with results
    pendingTools := make(map[string]contentBlock) // tool_use_id -> block
    fileEditCounts := make(map[string]int)

    scanner := bufio.NewScanner(f)
    scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // up to 10MB lines

    for scanner.Scan() {
        line := scanner.Bytes()
        delta.EndOffset += int64(len(line)) + 1 // +1 for newline

        var rec transcriptRecord
        if err := json.Unmarshal(line, &rec); err != nil {
            fmt.Fprintf(os.Stderr, "docket transcript: skip malformed line: %v\n", err)
            continue
        }

        switch rec.Type {
        case "assistant":
            if rec.Message == nil {
                continue
            }
            for _, block := range rec.Message.Content {
                switch block.Type {
                case "text":
                    if block.Text != "" {
                        semanticBuf.WriteString(block.Text)
                        semanticBuf.WriteByte('\n')
                    }
                case "tool_use":
                    pendingTools[block.ID] = block
                    processToolUse(block, fileEditCounts)
                }
            }

        case "user":
            if rec.Message == nil {
                continue
            }
            for _, block := range rec.Message.Content {
                if block.Type == "text" && block.Text != "" {
                    normalized := strings.ToLower(strings.TrimSpace(block.Text))
                    if !trivialUserMessages[normalized] {
                        delta.HasContent = true
                    }
                    // Include user text in semantic output regardless
                    semanticBuf.WriteString(block.Text)
                    semanticBuf.WriteByte('\n')
                }
            }

        case "tool_result":
            if pending, ok := pendingTools[rec.ToolUseID]; ok {
                processToolResult(pending, rec, delta)
                delete(pendingTools, rec.ToolUseID)
            }
        }
    }

    // Convert file edit counts to FileEdit slice
    for path, count := range fileEditCounts {
        delta.MechanicalFacts.FilesEdited = append(delta.MechanicalFacts.FilesEdited, store.FileEdit{
            Path: path, Count: count,
        })
    }

    delta.SemanticText = semanticBuf.String()
    if len(delta.SemanticText) >= 300 {
        delta.HasContent = true
    }

    return delta, nil
}

func processToolUse(block contentBlock, fileEditCounts map[string]int) {
    var ti toolInput
    json.Unmarshal(block.Input, &ti)

    switch block.Name {
    case "Edit", "Write":
        if ti.FilePath != "" {
            fileEditCounts[ti.FilePath]++
        }
    }
}

func processToolResult(pending contentBlock, rec transcriptRecord, delta *Delta) {
    var resultText string
    // Content can be a string or an array
    if err := json.Unmarshal(rec.Content, &resultText); err != nil {
        // Try as raw string (already a string)
        resultText = string(rec.Content)
    }

    var ti toolInput
    json.Unmarshal(pending.Input, &ti)

    // Check for errors
    if rec.IsError {
        delta.MechanicalFacts.Errors = append(delta.MechanicalFacts.Errors, store.ErrorFact{
            Tool:    pending.Name,
            Message: truncate(resultText, 200),
        })
    }

    // Bash-specific analysis
    if pending.Name == "Bash" {
        // Test detection
        if testCmdPattern.MatchString(ti.Command) {
            passed := !rec.IsError
            delta.MechanicalFacts.TestRuns = append(delta.MechanicalFacts.TestRuns, store.TestRunFact{
                Command: ti.Command,
                Passed:  passed,
            })
        }

        // Commit detection
        if strings.Contains(ti.Command, "git commit") {
            if matches := commitPattern.FindStringSubmatch(resultText); len(matches) >= 3 {
                delta.MechanicalFacts.Commits = append(delta.MechanicalFacts.Commits, store.CommitFact{
                    Hash:    matches[1],
                    Message: matches[2],
                })
            }
        }
    }
}

func truncate(s string, max int) string {
    if len(s) <= max {
        return s
    }
    return s[:max] + "..."
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/transcript/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/transcript/types.go internal/transcript/parse.go internal/transcript/parse_test.go
git commit -m "feat: add transcript JSONL parser with delta extraction and mechanical facts"
```

---

### Task 5: Summarizer Backend Interface and Anthropic Implementation

**Files:**
- Create: `internal/checkpoint/summarizer.go`
- Create: `internal/checkpoint/config.go`
- Create: `internal/checkpoint/noop.go`
- Create: `internal/checkpoint/anthropic.go`
- Create: `internal/checkpoint/anthropic_test.go`

- [ ] **Step 1: Define the summarizer interface and config**

Create `internal/checkpoint/summarizer.go`:

```go
package checkpoint

import "context"

// SummarizeInput is what the worker sends to the summarizer.
type SummarizeInput struct {
    SemanticText string // filtered user/assistant transcript delta
    FeatureTitle string // for context
    Reason       string // stop, precompact, manual_checkpoint, manual_end_session
}

// SummarizeOutput is what the summarizer returns.
type SummarizeOutput struct {
    Summary   string   // human-readable narrative
    Blockers  []string // discovered blockers
    DeadEnds  []string // things tried that didn't work
    NextSteps []string // intent for next session
    Decisions []string // decisions discussed
    Gotchas   []string // non-obvious discoveries
}

// SummarizerBackend processes transcript deltas into structured observations.
type SummarizerBackend interface {
    Summarize(ctx context.Context, input SummarizeInput) (*SummarizeOutput, error)
}
```

Create `internal/checkpoint/config.go`:

```go
package checkpoint

import "os"

const defaultModel = "claude-haiku-4-5-20251001"

// Config holds summarizer configuration from environment variables.
type Config struct {
    APIKey  string
    Model   string
    Enabled bool
}

// LoadConfig reads summarizer configuration from environment variables.
func LoadConfig() Config {
    apiKey := os.Getenv("ANTHROPIC_API_KEY")
    model := os.Getenv("DOCKET_SUMMARIZER_MODEL")
    if model == "" {
        model = defaultModel
    }

    enabled := apiKey != ""
    if v := os.Getenv("DOCKET_SUMMARIZER_ENABLED"); v == "false" {
        enabled = false
    }

    return Config{
        APIKey:  apiKey,
        Model:   model,
        Enabled: enabled,
    }
}
```

Create `internal/checkpoint/noop.go`:

```go
package checkpoint

import "context"

// NoopSummarizer returns empty output. Used when no API key is configured.
type NoopSummarizer struct{}

func (n *NoopSummarizer) Summarize(ctx context.Context, input SummarizeInput) (*SummarizeOutput, error) {
    return &SummarizeOutput{}, nil
}
```

- [ ] **Step 2: Write failing test for Anthropic summarizer**

Create `internal/checkpoint/anthropic_test.go`:

```go
package checkpoint

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestAnthropicSummarizer(t *testing.T) {
    // Mock Anthropic API server
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify request
        if r.Header.Get("x-api-key") != "test-key" {
            t.Errorf("missing api key header")
        }
        if r.Header.Get("anthropic-version") == "" {
            t.Errorf("missing anthropic-version header")
        }

        resp := map[string]any{
            "content": []map[string]any{
                {
                    "type": "text",
                    "text": `{"summary":"Discussed auth token refresh design. Decided on rotating tokens.","blockers":[],"dead_ends":["Tried stateless refresh but abandoned due to revocation complexity"],"next_steps":["Implement token rotation endpoint"],"decisions":["Use rotating refresh tokens"],"gotchas":["Token expiry must be checked server-side"]}`,
                },
            },
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(resp)
    }))
    defer srv.Close()

    s := &AnthropicSummarizer{
        apiKey:  "test-key",
        model:   "claude-haiku-4-5-20251001",
        baseURL: srv.URL,
    }

    out, err := s.Summarize(context.Background(), SummarizeInput{
        SemanticText: "User: How should we handle token refresh?\nAssistant: I recommend rotating refresh tokens...",
        FeatureTitle: "Auth System",
        Reason:       "stop",
    })
    if err != nil {
        t.Fatalf("Summarize: %v", err)
    }
    if out.Summary == "" {
        t.Error("expected non-empty summary")
    }
    if len(out.DeadEnds) != 1 {
        t.Errorf("DeadEnds = %d, want 1", len(out.DeadEnds))
    }
    if len(out.Decisions) != 1 {
        t.Errorf("Decisions = %d, want 1", len(out.Decisions))
    }
}

func TestAnthropicSummarizerAPIError(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(500)
        w.Write([]byte(`{"error":{"message":"internal error"}}`))
    }))
    defer srv.Close()

    s := &AnthropicSummarizer{
        apiKey:  "test-key",
        model:   "claude-haiku-4-5-20251001",
        baseURL: srv.URL,
    }

    _, err := s.Summarize(context.Background(), SummarizeInput{
        SemanticText: "some text",
        FeatureTitle: "Test",
        Reason:       "stop",
    })
    if err == nil {
        t.Fatal("expected error for 500 response")
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/checkpoint/ -v -run TestAnthropicSummarizer`
Expected: FAIL — `AnthropicSummarizer` not defined

- [ ] **Step 4: Implement Anthropic summarizer**

Create `internal/checkpoint/anthropic.go`:

```go
package checkpoint

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

const defaultBaseURL = "https://api.anthropic.com"
const anthropicVersion = "2023-06-01"

// AnthropicSummarizer calls the Anthropic Messages API to summarize transcript deltas.
type AnthropicSummarizer struct {
    apiKey  string
    model   string
    baseURL string
    client  *http.Client
}

// NewAnthropicSummarizer creates a summarizer using the Anthropic Messages API.
func NewAnthropicSummarizer(cfg Config) *AnthropicSummarizer {
    return &AnthropicSummarizer{
        apiKey:  cfg.APIKey,
        model:   cfg.Model,
        baseURL: defaultBaseURL,
        client:  &http.Client{},
    }
}

type messagesRequest struct {
    Model     string           `json:"model"`
    MaxTokens int              `json:"max_tokens"`
    System    string           `json:"system"`
    Messages  []messageReq     `json:"messages"`
}

type messageReq struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type messagesResponse struct {
    Content []contentResp `json:"content"`
    Error   *apiError     `json:"error"`
}

type contentResp struct {
    Type string `json:"type"`
    Text string `json:"text"`
}

type apiError struct {
    Message string `json:"message"`
}

func (s *AnthropicSummarizer) Summarize(ctx context.Context, input SummarizeInput) (*SummarizeOutput, error) {
    systemPrompt := `You are a session context extractor for a software development feature tracker. Given a conversation excerpt between a developer and an AI assistant, extract structured information about what happened.

Respond with ONLY a JSON object (no markdown, no explanation) with these fields:
- "summary": 2-4 sentence narrative of what was discussed and accomplished
- "blockers": array of strings — anything blocking progress
- "dead_ends": array of strings — approaches tried that didn't work
- "next_steps": array of strings — what should happen next
- "decisions": array of strings — decisions made during the conversation
- "gotchas": array of strings — non-obvious discoveries or warnings

Use empty arrays for fields with no content. Be concise.`

    userContent := fmt.Sprintf("Feature: %s\nCheckpoint reason: %s\n\nConversation excerpt:\n%s",
        input.FeatureTitle, input.Reason, input.SemanticText)

    // Truncate if too long (keep last part which is most relevant)
    if len(userContent) > 30000 {
        userContent = userContent[len(userContent)-30000:]
    }

    reqBody := messagesRequest{
        Model:     s.model,
        MaxTokens: 1024,
        System:    systemPrompt,
        Messages: []messageReq{
            {Role: "user", Content: userContent},
        },
    }

    body, err := json.Marshal(reqBody)
    if err != nil {
        return nil, fmt.Errorf("marshal request: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/v1/messages", bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("x-api-key", s.apiKey)
    req.Header.Set("anthropic-version", anthropicVersion)

    resp, err := s.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("api call: %w", err)
    }
    defer resp.Body.Close()

    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read response: %w", err)
    }

    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(respBody))
    }

    var msgResp messagesResponse
    if err := json.Unmarshal(respBody, &msgResp); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }

    if len(msgResp.Content) == 0 {
        return nil, fmt.Errorf("empty response content")
    }

    var output SummarizeOutput
    if err := json.Unmarshal([]byte(msgResp.Content[0].Text), &output); err != nil {
        // If JSON parsing fails, use raw text as summary
        output.Summary = msgResp.Content[0].Text
    }

    return &output, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/checkpoint/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/checkpoint/summarizer.go internal/checkpoint/config.go internal/checkpoint/noop.go internal/checkpoint/anthropic.go internal/checkpoint/anthropic_test.go
git commit -m "feat: add summarizer backend interface with Anthropic Messages API implementation"
```

---

### Task 6: Background Checkpoint Worker

**Files:**
- Create: `internal/checkpoint/worker.go`
- Create: `internal/checkpoint/worker_test.go`

- [ ] **Step 1: Write failing test for the worker**

Create `internal/checkpoint/worker_test.go`:

```go
package checkpoint

import (
    "context"
    "testing"
    "time"

    "github.com/sniffle6/claude-docket/internal/store"
)

type mockSummarizer struct {
    calls   int
    output  *SummarizeOutput
    err     error
}

func (m *mockSummarizer) Summarize(ctx context.Context, input SummarizeInput) (*SummarizeOutput, error) {
    m.calls++
    if m.err != nil {
        return nil, m.err
    }
    if m.output != nil {
        return m.output, nil
    }
    return &SummarizeOutput{
        Summary:  "Test summary",
        Blockers: []string{},
    }, nil
}

func TestWorkerProcessesJob(t *testing.T) {
    s, err := store.Open(t.TempDir())
    if err != nil {
        t.Fatal(err)
    }
    defer s.Close()

    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "sess-1")

    s.EnqueueCheckpointJob(store.CheckpointJobInput{
        WorkSessionID:        ws.ID,
        FeatureID:            "auth-system",
        Reason:               "stop",
        TranscriptStartOffset: 0,
        TranscriptEndOffset:  512,
        SemanticText:         "discussed auth design",
        MechanicalFacts:      store.MechanicalFacts{},
    })

    mock := &mockSummarizer{}
    w := NewWorker(s, mock)

    // Process one job
    processed := w.ProcessOne()
    if !processed {
        t.Fatal("expected to process a job")
    }
    if mock.calls != 1 {
        t.Errorf("summarizer calls = %d, want 1", mock.calls)
    }

    // Verify observation was written
    obs, _ := s.GetObservationsForWorkSession(ws.ID)
    if len(obs) == 0 {
        t.Fatal("expected at least one observation")
    }
    if obs[0].Kind != "summary" {
        t.Errorf("Kind = %q, want %q", obs[0].Kind, "summary")
    }
}

func TestWorkerSkipsNoopOnEmptyText(t *testing.T) {
    s, err := store.Open(t.TempDir())
    if err != nil {
        t.Fatal(err)
    }
    defer s.Close()

    s.AddFeature("Auth System", "token auth")
    ws, _ := s.OpenWorkSession("auth-system", "sess-1")

    s.EnqueueCheckpointJob(store.CheckpointJobInput{
        WorkSessionID:        ws.ID,
        FeatureID:            "auth-system",
        Reason:               "stop",
        TranscriptStartOffset: 0,
        TranscriptEndOffset:  512,
        SemanticText:         "", // empty
        MechanicalFacts:      store.MechanicalFacts{FilesEdited: []store.FileEdit{{Path: "a.go", Count: 1}}},
    })

    mock := &mockSummarizer{}
    w := NewWorker(s, mock)
    w.ProcessOne()

    // Should skip LLM call but still mark as done
    if mock.calls != 0 {
        t.Errorf("expected 0 summarizer calls for empty text, got %d", mock.calls)
    }

    job, _ := s.GetCheckpointJob(1)
    if job.Status != "done" {
        t.Errorf("Status = %q, want done (skipped)", job.Status)
    }
}

func TestWorkerRunLoop(t *testing.T) {
    s, err := store.Open(t.TempDir())
    if err != nil {
        t.Fatal(err)
    }
    defer s.Close()

    mock := &mockSummarizer{}
    w := NewWorker(s, mock)

    ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
    defer cancel()

    // Run should exit when context is cancelled
    w.Run(ctx, 50*time.Millisecond)
    // No panic = pass
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/checkpoint/ -v -run TestWorker`
Expected: FAIL — `NewWorker` not defined

- [ ] **Step 3: Implement the worker**

Create `internal/checkpoint/worker.go`:

```go
package checkpoint

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "time"

    "github.com/sniffle6/claude-docket/internal/store"
)

// Worker drains checkpoint_jobs and calls the summarizer for each.
type Worker struct {
    store      *store.Store
    summarizer SummarizerBackend
}

func NewWorker(s *store.Store, summarizer SummarizerBackend) *Worker {
    return &Worker{store: s, summarizer: summarizer}
}

// Run polls for queued jobs and processes them until ctx is cancelled.
func (w *Worker) Run(ctx context.Context, pollInterval time.Duration) {
    ticker := time.NewTicker(pollInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            for w.ProcessOne() {
                // drain all queued jobs before waiting
            }
        }
    }
}

// ProcessOne dequeues and processes a single checkpoint job.
// Returns true if a job was processed, false if the queue was empty.
func (w *Worker) ProcessOne() bool {
    job, err := w.store.DequeueCheckpointJob()
    if err != nil || job == nil {
        return false
    }

    // Get feature title for context
    featureTitle := job.FeatureID
    if f, err := w.store.GetFeature(job.FeatureID); err == nil {
        featureTitle = f.Title
    }

    // If semantic text is empty, skip LLM call but still mark done
    if job.SemanticText == "" {
        w.store.CompleteCheckpointJob(job.ID, nil)
        return true
    }

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    output, err := w.summarizer.Summarize(ctx, SummarizeInput{
        SemanticText: job.SemanticText,
        FeatureTitle: featureTitle,
        Reason:       job.Reason,
    })
    if err != nil {
        errMsg := err.Error()
        w.store.FailCheckpointJob(job.ID, errMsg)
        fmt.Fprintf(os.Stderr, "docket checkpoint worker: summarize job %d: %v\n", job.ID, err)
        return true
    }

    // Write observations
    w.writeObservations(job, output)
    w.store.CompleteCheckpointJob(job.ID, nil)

    // Auto-merge key_files from mechanical facts
    w.mergeKeyFiles(job)

    return true
}

func (w *Worker) writeObservations(job *store.CheckpointJob, output *SummarizeOutput) {
    if output.Summary != "" {
        payload, _ := json.Marshal(output)
        w.store.AddCheckpointObservation(store.CheckpointObservationInput{
            CheckpointJobID: job.ID,
            WorkSessionID:   job.WorkSessionID,
            FeatureID:       job.FeatureID,
            Kind:            "summary",
            PayloadJSON:     string(payload),
            SummaryText:     output.Summary,
        })
    }
    for _, b := range output.Blockers {
        w.store.AddCheckpointObservation(store.CheckpointObservationInput{
            CheckpointJobID: job.ID, WorkSessionID: job.WorkSessionID,
            FeatureID: job.FeatureID, Kind: "blocker", SummaryText: b,
        })
    }
    for _, d := range output.DeadEnds {
        w.store.AddCheckpointObservation(store.CheckpointObservationInput{
            CheckpointJobID: job.ID, WorkSessionID: job.WorkSessionID,
            FeatureID: job.FeatureID, Kind: "dead_end", SummaryText: d,
        })
    }
    for _, n := range output.NextSteps {
        w.store.AddCheckpointObservation(store.CheckpointObservationInput{
            CheckpointJobID: job.ID, WorkSessionID: job.WorkSessionID,
            FeatureID: job.FeatureID, Kind: "next_step", SummaryText: n,
        })
    }
    for _, d := range output.Decisions {
        w.store.AddCheckpointObservation(store.CheckpointObservationInput{
            CheckpointJobID: job.ID, WorkSessionID: job.WorkSessionID,
            FeatureID: job.FeatureID, Kind: "decision_candidate", SummaryText: d,
        })
    }
    for _, g := range output.Gotchas {
        w.store.AddCheckpointObservation(store.CheckpointObservationInput{
            CheckpointJobID: job.ID, WorkSessionID: job.WorkSessionID,
            FeatureID: job.FeatureID, Kind: "gotcha", SummaryText: g,
        })
    }
}

func (w *Worker) mergeKeyFiles(job *store.CheckpointJob) {
    var facts store.MechanicalFacts
    if err := json.Unmarshal([]byte(job.MechanicalJSON), &facts); err != nil || len(facts.FilesEdited) == 0 {
        return
    }

    feature, err := w.store.GetFeature(job.FeatureID)
    if err != nil {
        return
    }

    existing := make(map[string]bool, len(feature.KeyFiles))
    for _, f := range feature.KeyFiles {
        existing[f] = true
    }

    merged := append([]string{}, feature.KeyFiles...)
    changed := false
    for _, fe := range facts.FilesEdited {
        if !existing[fe.Path] {
            merged = append(merged, fe.Path)
            existing[fe.Path] = true
            changed = true
        }
    }

    if changed {
        w.store.UpdateFeature(job.FeatureID, store.FeatureUpdate{KeyFiles: &merged})
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/checkpoint/ -v -run TestWorker`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/checkpoint/worker.go internal/checkpoint/worker_test.go
git commit -m "feat: add background checkpoint worker — drains job queue, calls summarizer, writes observations"
```

---

### Task 7: Rewrite Hook Handlers

**Files:**
- Modify: `cmd/docket/hook.go`
- Modify: `cmd/docket/hook_test.go`

This is the largest task. It rewrites Stop, adds PreCompact and SessionEnd, and updates SessionStart to open work sessions.

- [ ] **Step 1: Update hookInput struct**

In `cmd/docket/hook.go`, update the `hookInput` struct to add `TranscriptPath`:

```go
type hookInput struct {
    SessionID      string    `json:"session_id"`
    TranscriptPath string    `json:"transcript_path"`
    CWD            string    `json:"cwd"`
    HookEventName  string    `json:"hook_event_name"`
    ToolName       string    `json:"tool_name"`
    ToolInput      toolInput `json:"tool_input"`
    StopHookActive bool      `json:"stop_hook_active"`
    Trigger        string    `json:"trigger"` // PreCompact: "manual" or "auto"
}
```

- [ ] **Step 2: Update handleSessionStart to open work sessions**

Replace the `handleSessionStart` function. The new version opens a work session for the top active feature:

```go
func handleSessionStart(h *hookInput, w io.Writer) {
    s, err := store.Open(h.CWD)
    if err != nil {
        fmt.Fprintf(os.Stderr, "docket hook: open store: %v\n", err)
        json.NewEncoder(w).Encode(hookOutput{Continue: true})
        return
    }
    defer s.Close()

    // Auto-archive features done >7 days
    var archiveMsg string
    if archived, err := s.AutoArchiveStale(); err == nil && len(archived) > 0 {
        archiveMsg = fmt.Sprintf("[docket] Auto-archived %d features done >7 days: %s\n", len(archived), strings.Join(archived, ", "))
    }

    // Create/clear commits.log
    commitsPath := filepath.Join(h.CWD, ".docket", "commits.log")
    os.WriteFile(commitsPath, []byte{}, 0644)

    // Clear agent-nudged sentinel for new session
    sentinelPath := filepath.Join(h.CWD, ".docket", "agent-nudged")
    os.Remove(sentinelPath)

    // Reset transcript offset for new session
    offsetPath := filepath.Join(h.CWD, ".docket", "transcript-offset")
    os.WriteFile(offsetPath, []byte("0"), 0644)

    features, err := s.ListFeatures("in_progress")
    if err != nil {
        fmt.Fprintf(os.Stderr, "docket hook: list features: %v\n", err)
        json.NewEncoder(w).Encode(hookOutput{Continue: true})
        return
    }

    out := hookOutput{Continue: true}

    if len(features) == 0 {
        out.SystemMessage = archiveMsg + "[docket] No active features. Use docket MCP tools to create one."
        json.NewEncoder(w).Encode(out)
        return
    }

    // Open work session for top feature
    topFeature := features[0]
    s.OpenWorkSession(topFeature.ID, h.SessionID)

    var msg strings.Builder
    handoffPath := filepath.Join(h.CWD, ".docket", "handoff", topFeature.ID+".md")

    if content, err := os.ReadFile(handoffPath); err == nil {
        msg.WriteString("[docket] Session context:\n\n")
        msg.Write(content)
    } else {
        // Fallback: list features with left_off and next task
        msg.WriteString("[docket] Active features:\n")
        msg.WriteString(fmt.Sprintf("- %s (id: %s)", topFeature.Title, topFeature.ID))
        if topFeature.LeftOff != "" {
            msg.WriteString(fmt.Sprintf(" — left off: %s", topFeature.LeftOff))
        }
        msg.WriteString("\n")

        subtasks, err := s.GetSubtasksForFeature(topFeature.ID, false)
        if err == nil {
            for _, st := range subtasks {
                for _, item := range st.Items {
                    if !item.Checked {
                        msg.WriteString(fmt.Sprintf("Next task: %s\n", item.Title))
                        goto doneNextTask
                    }
                }
            }
        }
    doneNextTask:
    }

    // Other features: pointers or one-liners
    for _, f := range features[1:] {
        otherHandoff := filepath.Join(h.CWD, ".docket", "handoff", f.ID+".md")
        if _, err := os.Stat(otherHandoff); err == nil {
            msg.WriteString(fmt.Sprintf("\n[docket] Handoff available: .docket/handoff/%s.md", f.ID))
        } else {
            msg.WriteString(fmt.Sprintf("\n[docket] Also active: %s (id: %s)", f.Title, f.ID))
        }
    }

    out.SystemMessage = archiveMsg + msg.String()
    json.NewEncoder(w).Encode(out)
}
```

- [ ] **Step 3: Rewrite handleStop — single-phase, enqueue checkpoint**

Replace the entire `handleStop` function:

```go
func handleStop(h *hookInput, w io.Writer) {
    s, err := store.Open(h.CWD)
    if err != nil {
        json.NewEncoder(w).Encode(stopHookOutput{})
        return
    }
    defer s.Close()

    ws, err := s.GetActiveWorkSession()
    if err != nil {
        // No active work session — allow stop
        json.NewEncoder(w).Encode(stopHookOutput{})
        return
    }

    // Parse transcript delta
    delta := parseTranscriptDelta(h)

    // Check meaningful delta threshold
    if !isDeltaMeaningful(h.CWD, delta) {
        json.NewEncoder(w).Encode(stopHookOutput{})
        return
    }

    // Enqueue checkpoint job
    s.EnqueueCheckpointJob(store.CheckpointJobInput{
        WorkSessionID:         ws.ID,
        FeatureID:             ws.FeatureID,
        Reason:                "stop",
        TriggerType:           "auto",
        TranscriptStartOffset: getTranscriptOffset(h.CWD),
        TranscriptEndOffset:   delta.EndOffset,
        SemanticText:          delta.SemanticText,
        MechanicalFacts:       delta.MechanicalFacts,
    })

    // Update transcript offset
    saveTranscriptOffset(h.CWD, delta.EndOffset)

    // Never block — always allow stop
    json.NewEncoder(w).Encode(stopHookOutput{})
}
```

- [ ] **Step 4: Add handlePreCompact**

Add the new `handlePreCompact` handler:

```go
func handlePreCompact(h *hookInput, w io.Writer) {
    s, err := store.Open(h.CWD)
    if err != nil {
        json.NewEncoder(w).Encode(hookOutput{Continue: true})
        return
    }
    defer s.Close()

    ws, err := s.GetActiveWorkSession()
    if err != nil {
        json.NewEncoder(w).Encode(hookOutput{Continue: true})
        return
    }

    // Force checkpoint — PreCompact always enqueues regardless of threshold
    delta := parseTranscriptDelta(h)
    startOffset := getTranscriptOffset(h.CWD)

    s.EnqueueCheckpointJob(store.CheckpointJobInput{
        WorkSessionID:         ws.ID,
        FeatureID:             ws.FeatureID,
        Reason:                "precompact",
        TriggerType:           h.Trigger,
        TranscriptStartOffset: startOffset,
        TranscriptEndOffset:   delta.EndOffset,
        SemanticText:          delta.SemanticText,
        MechanicalFacts:       delta.MechanicalFacts,
    })

    saveTranscriptOffset(h.CWD, delta.EndOffset)

    json.NewEncoder(w).Encode(hookOutput{Continue: true})
}
```

- [ ] **Step 5: Add handleSessionEnd**

Add the new `handleSessionEnd` handler:

```go
func handleSessionEnd(h *hookInput, w io.Writer) {
    s, err := store.Open(h.CWD)
    if err != nil {
        json.NewEncoder(w).Encode(stopHookOutput{})
        return
    }
    defer s.Close()

    ws, err := s.GetActiveWorkSession()
    if err != nil {
        json.NewEncoder(w).Encode(stopHookOutput{})
        return
    }

    // Final cheap flush: enqueue any remaining delta
    delta := parseTranscriptDelta(h)
    if delta.HasContent {
        s.EnqueueCheckpointJob(store.CheckpointJobInput{
            WorkSessionID:         ws.ID,
            FeatureID:             ws.FeatureID,
            Reason:                "stop",
            TriggerType:           "auto",
            TranscriptStartOffset: getTranscriptOffset(h.CWD),
            TranscriptEndOffset:   delta.EndOffset,
            SemanticText:          delta.SemanticText,
            MechanicalFacts:       delta.MechanicalFacts,
        })
    }

    // Write handoff files for all active features
    features, _ := s.ListFeatures("in_progress")
    if len(features) > 0 {
        activeIDs := make(map[string]bool)
        for _, f := range features {
            activeIDs[f.ID] = true
            data, err := s.GetHandoffData(f.ID)
            if err == nil {
                if writeErr := writeHandoffFile(h.CWD, data); writeErr != nil {
                    s.MarkHandoffStale(ws.ID)
                }
            }
        }
        cleanStaleHandoffs(h.CWD, activeIDs)
    }

    // Clean up commits.log
    commitsPath := filepath.Join(h.CWD, ".docket", "commits.log")
    os.Remove(commitsPath)

    // Close work session
    s.CloseWorkSession(ws.ID)

    json.NewEncoder(w).Encode(stopHookOutput{})
}
```

- [ ] **Step 6: Add helper functions for transcript offset and delta parsing**

Add these helpers to `cmd/docket/hook.go`:

```go
func parseTranscriptDelta(h *hookInput) *transcript.Delta {
    if h.TranscriptPath == "" {
        return &transcript.Delta{}
    }
    offset := getTranscriptOffset(h.CWD)
    delta, err := transcript.Parse(h.TranscriptPath, offset)
    if err != nil {
        fmt.Fprintf(os.Stderr, "docket hook: parse transcript: %v\n", err)
        return &transcript.Delta{EndOffset: offset}
    }
    return delta
}

func getTranscriptOffset(cwd string) int64 {
    data, err := os.ReadFile(filepath.Join(cwd, ".docket", "transcript-offset"))
    if err != nil {
        return 0
    }
    var offset int64
    fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &offset)
    return offset
}

func saveTranscriptOffset(cwd string, offset int64) {
    os.WriteFile(
        filepath.Join(cwd, ".docket", "transcript-offset"),
        []byte(fmt.Sprintf("%d", offset)),
        0644,
    )
}

func isDeltaMeaningful(cwd string, delta *transcript.Delta) bool {
    // Any commits in commits.log?
    commitsPath := filepath.Join(cwd, ".docket", "commits.log")
    if data, err := os.ReadFile(commitsPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
        return true
    }

    // Transcript-detected commits or failures?
    if len(delta.MechanicalFacts.Commits) > 0 {
        return true
    }
    if len(delta.MechanicalFacts.Errors) > 0 {
        return true
    }
    for _, tr := range delta.MechanicalFacts.TestRuns {
        if !tr.Passed {
            return true
        }
    }

    // Semantic text threshold
    if len(delta.SemanticText) >= 300 {
        return true
    }

    // Non-trivial user input
    if delta.HasContent {
        return true
    }

    return false
}
```

- [ ] **Step 7: Update the switch statement in runHook() to route new events**

Update the switch in `runHook()`:

```go
switch h.HookEventName {
case "PreToolUse":
    handlePreToolUse(&h, os.Stdout)
case "SessionStart":
    handleSessionStart(&h, os.Stdout)
case "PostToolUse":
    handlePostToolUse(&h, os.Stdout)
case "Stop":
    handleStop(&h, os.Stdout)
case "PreCompact":
    handlePreCompact(&h, os.Stdout)
case "SessionEnd":
    handleSessionEnd(&h, os.Stdout)
default:
    json.NewEncoder(os.Stdout).Encode(hookOutput{Continue: true})
}
```

- [ ] **Step 8: Add the import for the transcript package**

Add `"github.com/sniffle6/claude-docket/internal/transcript"` to the imports in `hook.go`.

- [ ] **Step 9: Write tests for the new hook behavior**

Add to `cmd/docket/hook_test.go`:

```go
func TestStopNeverBlocks(t *testing.T) {
    dir := t.TempDir()
    s, _ := store.Open(dir)
    s.AddFeature("Test Feature", "test")
    s.UpdateFeature("test-feature", store.FeatureUpdate{Status: strPtr("in_progress")})
    s.OpenWorkSession("test-feature", "sess-1")

    // Write a commit to commits.log
    commitsPath := filepath.Join(dir, ".docket", "commits.log")
    os.WriteFile(commitsPath, []byte("abc123|||fix bug\n"), 0644)

    // Write transcript offset
    os.WriteFile(filepath.Join(dir, ".docket", "transcript-offset"), []byte("0"), 0644)

    s.Close()

    h := &hookInput{
        SessionID:     "sess-1",
        CWD:           dir,
        HookEventName: "Stop",
    }

    var buf bytes.Buffer
    handleStop(h, &buf)

    var out stopHookOutput
    json.Unmarshal(buf.Bytes(), &out)

    // Must never block
    if out.Decision == "block" {
        t.Error("Stop hook must never block")
    }
}

func TestSessionEndClosesWorkSession(t *testing.T) {
    dir := t.TempDir()
    s, _ := store.Open(dir)
    s.AddFeature("Test Feature", "test")
    s.UpdateFeature("test-feature", store.FeatureUpdate{Status: strPtr("in_progress")})
    s.OpenWorkSession("test-feature", "sess-1")
    s.Close()

    h := &hookInput{
        SessionID:     "sess-1",
        CWD:           dir,
        HookEventName: "SessionEnd",
    }

    var buf bytes.Buffer
    handleSessionEnd(h, &buf)

    // Verify work session is closed
    s2, _ := store.Open(dir)
    defer s2.Close()
    _, err := s2.GetActiveWorkSession()
    if err == nil {
        t.Error("expected no active work session after SessionEnd")
    }
}

func TestSessionStartOpensWorkSession(t *testing.T) {
    dir := t.TempDir()
    s, _ := store.Open(dir)
    s.AddFeature("Test Feature", "test")
    s.UpdateFeature("test-feature", store.FeatureUpdate{Status: strPtr("in_progress")})
    s.Close()

    h := &hookInput{
        SessionID:     "sess-1",
        CWD:           dir,
        HookEventName: "SessionStart",
    }

    var buf bytes.Buffer
    handleSessionStart(h, &buf)

    // Verify work session was opened
    s2, _ := store.Open(dir)
    defer s2.Close()
    ws, err := s2.GetActiveWorkSession()
    if err != nil {
        t.Fatalf("expected active work session, got error: %v", err)
    }
    if ws.FeatureID != "test-feature" {
        t.Errorf("FeatureID = %q, want %q", ws.FeatureID, "test-feature")
    }
}
```

- [ ] **Step 10: Run all hook tests**

Run: `go test ./cmd/docket/ -v -run "TestStop|TestSessionEnd|TestSessionStart"`
Expected: PASS

- [ ] **Step 11: Commit**

```bash
git add cmd/docket/hook.go cmd/docket/hook_test.go
git commit -m "feat: rewrite hook handlers — single-phase stop, PreCompact, SessionEnd, work sessions"
```

---

### Task 8: Remove log_session and Sentinel Logic

**Files:**
- Modify: `internal/mcp/tools.go:49-55` — remove `log_session` registration
- Modify: `internal/mcp/tools_session.go` — remove `logSessionHandler`
- Modify: `internal/store/store.go:125-140` — remove sentinel methods
- Modify: `internal/mcp/tools_test.go` — remove/update log_session tests

- [ ] **Step 1: Remove log_session tool registration from tools.go**

In `internal/mcp/tools.go`, delete lines 49-55 (the `log_session` tool registration block).

- [ ] **Step 2: Remove logSessionHandler from tools_session.go**

In `internal/mcp/tools_session.go`, delete the `logSessionHandler` function (lines 14-58). Keep `compactSessionsHandler`.

- [ ] **Step 3: Remove sentinel methods from store.go**

In `internal/store/store.go`, delete `MarkSessionLogged`, `WasSessionLogged`, and `ClearSessionLogged` (lines 125-140).

- [ ] **Step 4: Update any tests that reference log_session or sentinel methods**

Search for and update tests in `internal/mcp/tools_test.go` that call `log_session`. Remove those test cases.

Run: `grep -n "log_session\|MarkSessionLogged\|WasSessionLogged\|ClearSessionLogged\|session-logged" cmd/docket/hook.go internal/mcp/tools_test.go`

Remove or update all references found.

- [ ] **Step 5: Run all tests to verify nothing breaks**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_session.go internal/store/store.go internal/mcp/tools_test.go cmd/docket/hook.go
git commit -m "feat: remove log_session MCP tool and session-logged sentinel"
```

---

### Task 9: Add checkpoint MCP Tool

**Files:**
- Modify: `internal/mcp/tools.go` — add `checkpoint` tool registration
- Create: `internal/mcp/tools_checkpoint.go` — handler

- [ ] **Step 1: Write the handler**

Create `internal/mcp/tools_checkpoint.go`:

```go
package mcp

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"

    "github.com/sniffle6/claude-docket/internal/store"
    "github.com/sniffle6/claude-docket/internal/transcript"
)

func checkpointHandler(s *store.Store, projectDir string) server.ToolHandlerFunc {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        args := req.GetArguments()
        endSession := false
        if v, ok := args["end_session"]; ok {
            if b, ok := v.(bool); ok {
                endSession = b
            }
        }

        ws, err := s.GetActiveWorkSession()
        if err != nil {
            return mcp.NewToolResultError("no active work session — nothing to checkpoint"), nil
        }

        // Read transcript offset
        offsetPath := filepath.Join(projectDir, ".docket", "transcript-offset")
        var startOffset int64
        if data, err := os.ReadFile(offsetPath); err == nil {
            fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &startOffset)
        }

        // Find transcript path — check common locations
        transcriptPath := findTranscriptPath(ws.ClaudeSessionID)
        if transcriptPath == "" {
            return mcp.NewToolResultText("Checkpoint enqueued (mechanical only — transcript not found)."), nil
        }

        delta, err := transcript.Parse(transcriptPath, startOffset)
        if err != nil {
            return mcp.NewToolResultError(fmt.Sprintf("parse transcript: %v", err)), nil
        }

        reason := "manual_checkpoint"
        if endSession {
            reason = "manual_end_session"
        }

        job, err := s.EnqueueCheckpointJob(store.CheckpointJobInput{
            WorkSessionID:         ws.ID,
            FeatureID:             ws.FeatureID,
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

        // Update offset
        os.WriteFile(offsetPath, []byte(fmt.Sprintf("%d", delta.EndOffset)), 0644)

        // If ending session, write handoff and close work session
        if endSession {
            data, err := s.GetHandoffData(ws.FeatureID)
            if err == nil {
                obs, _ := s.GetObservationsForWorkSession(ws.ID)
                mf, _ := s.GetMechanicalFactsForWorkSession(ws.ID)
                // Write handoff inline since we have the data
                writeHandoffFileWithCheckpointsFromMCP(projectDir, data, obs, mf)
            }
            s.CloseWorkSession(ws.ID)

            return mcp.NewToolResultText(fmt.Sprintf(
                "Work session closed for feature %q. Checkpoint #%d enqueued. Handoff written.",
                ws.FeatureID, job.ID,
            )), nil
        }

        return mcp.NewToolResultText(fmt.Sprintf(
            "Checkpoint #%d enqueued for feature %q. %d chars semantic text, %d files edited.",
            job.ID, ws.FeatureID, len(delta.SemanticText), len(delta.MechanicalFacts.FilesEdited),
        )), nil
    }
}

func findTranscriptPath(claudeSessionID string) string {
    if claudeSessionID == "" {
        return ""
    }
    // Claude stores transcripts in ~/.claude/projects/<hash>/<session-id>.jsonl
    home, err := os.UserHomeDir()
    if err != nil {
        return ""
    }
    projectsDir := filepath.Join(home, ".claude", "projects")
    entries, err := os.ReadDir(projectsDir)
    if err != nil {
        return ""
    }
    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }
        candidate := filepath.Join(projectsDir, entry.Name(), claudeSessionID+".jsonl")
        if _, err := os.Stat(candidate); err == nil {
            return candidate
        }
    }
    return ""
}
```

- [ ] **Step 2: Register the checkpoint tool**

In `internal/mcp/tools.go`, add after the `compact_sessions` registration (around line 70):

```go
srv.AddTool(mcp.NewTool("checkpoint",
    mcp.WithDescription("Force a checkpoint of the current session's semantic and mechanical state. Enqueues a background summarization job. Pass end_session=true to also close the work session and write the handoff file."),
    mcp.WithBoolean("end_session", mcp.Description("If true, close the work session and write handoff after checkpointing. Default: false.")),
), checkpointHandler(s, projectDir))
```

Note: `registerTools` will need a `projectDir` parameter. Update the signature:

```go
func registerTools(srv *server.MCPServer, s *store.Store, projectDir string) {
```

And update the caller in `NewServer`.

- [ ] **Step 3: Update NewServer to pass projectDir**

In `internal/mcp/tools.go`, update `NewServer` to accept and pass `projectDir`. Check how it's currently called and update the call site in `cmd/docket/main.go`.

- [ ] **Step 4: Run tests**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_checkpoint.go cmd/docket/main.go
git commit -m "feat: add checkpoint MCP tool for manual context preservation"
```

---

### Task 10: Start Checkpoint Worker in MCP Server

**Files:**
- Modify: `cmd/docket/main.go:71-103`

- [ ] **Step 1: Update runServe to start the checkpoint worker**

In `cmd/docket/main.go`, update `runServe()` to start the background worker:

```go
func runServe() {
    dir, err := os.Getwd()
    if err != nil {
        log.Fatalf("getwd: %v", err)
    }
    s, err := store.Open(dir)
    if err != nil {
        log.Fatalf("open store: %v", err)
    }
    defer s.Close()

    // Start HTTP dashboard in background on a per-project port
    port := portForDir(dir)

    // Write port file so skills/tools can discover the dashboard URL
    portFile := filepath.Join(dir, ".docket", "port")
    os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0644)

    go func() {
        handler := dashboard.NewHandler(s, staticfiles.StaticFS, dir)
        addr := fmt.Sprintf(":%d", port)
        log.Printf("Dashboard: http://localhost:%d", port)
        if err := http.ListenAndServe(addr, handler); err != nil {
            log.Printf("dashboard error: %v", err)
        }
    }()

    // Start checkpoint worker in background
    cfg := checkpoint.LoadConfig()
    var summarizer checkpoint.SummarizerBackend
    if cfg.Enabled {
        summarizer = checkpoint.NewAnthropicSummarizer(cfg)
        log.Printf("Checkpoint summarizer: enabled (model: %s)", cfg.Model)
    } else {
        summarizer = &checkpoint.NoopSummarizer{}
        log.Printf("Checkpoint summarizer: disabled (no ANTHROPIC_API_KEY)")
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    worker := checkpoint.NewWorker(s, summarizer)
    go worker.Run(ctx, 5*time.Second)

    // Run MCP server on stdio (blocks)
    mcpServer := docketmcp.NewServer(s, dir)
    if err := server.ServeStdio(mcpServer); err != nil {
        log.Fatalf("mcp server: %v", err)
    }
}
```

Add the necessary imports: `"context"`, `"time"`, `"github.com/sniffle6/claude-docket/internal/checkpoint"`.

- [ ] **Step 2: Run build to verify compilation**

Run: `go build -o /dev/null ./cmd/docket/`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add cmd/docket/main.go
git commit -m "feat: start checkpoint worker in MCP server process"
```

---

### Task 11: Update Handoff Renderer

**Files:**
- Modify: `cmd/docket/handoff.go`

- [ ] **Step 1: Update renderHandoff to include checkpoint observations**

Add a new "Last Session" section to the handoff renderer. The function needs access to the store to query observations. Update the signature and add the section:

In `cmd/docket/handoff.go`, update `renderHandoff` to accept optional checkpoint data:

```go
type HandoffCheckpointData struct {
    Observations    []store.CheckpointObservation
    MechanicalFacts *store.MechanicalFacts
}

func renderHandoff(data *store.HandoffData, cpData *HandoffCheckpointData) string {
    var b strings.Builder
    f := data.Feature

    fmt.Fprintf(&b, "# Handoff: %s\n\n", f.Title)

    fmt.Fprintf(&b, "## Status\n")
    fmt.Fprintf(&b, "%s | Progress: %d/%d | Updated: %s\n\n",
        f.Status, data.Done, data.Total, f.UpdatedAt.Format("2006-01-02 15:04"))

    if f.LeftOff != "" {
        fmt.Fprintf(&b, "## Left Off\n%s\n\n", f.LeftOff)
    }

    // New: Last Session section from checkpoint observations
    if cpData != nil && (len(cpData.Observations) > 0 || cpData.MechanicalFacts != nil) {
        b.WriteString("## Last Session\n")

        // Semantic observations
        for _, obs := range cpData.Observations {
            switch obs.Kind {
            case "summary":
                fmt.Fprintf(&b, "%s\n\n", obs.SummaryText)
            case "blocker":
                fmt.Fprintf(&b, "- **Blocker:** %s\n", obs.SummaryText)
            case "dead_end":
                fmt.Fprintf(&b, "- **Dead end:** %s\n", obs.SummaryText)
            case "next_step":
                fmt.Fprintf(&b, "- **Next:** %s\n", obs.SummaryText)
            case "decision_candidate":
                fmt.Fprintf(&b, "- **Decision:** %s\n", obs.SummaryText)
            case "gotcha":
                fmt.Fprintf(&b, "- **Gotcha:** %s\n", obs.SummaryText)
            }
        }

        // Mechanical facts
        if cpData.MechanicalFacts != nil {
            mf := cpData.MechanicalFacts
            if len(mf.FilesEdited) > 0 {
                var parts []string
                for _, fe := range mf.FilesEdited {
                    if fe.Count > 1 {
                        parts = append(parts, fmt.Sprintf("%s (%d×)", fe.Path, fe.Count))
                    } else {
                        parts = append(parts, fe.Path)
                    }
                }
                fmt.Fprintf(&b, "\nFiles: %s\n", strings.Join(parts, ", "))
            }
            if len(mf.TestRuns) > 0 {
                passed := 0
                for _, tr := range mf.TestRuns {
                    if tr.Passed {
                        passed++
                    }
                }
                fmt.Fprintf(&b, "Tests: %d runs (%d passed, %d failed)\n",
                    len(mf.TestRuns), passed, len(mf.TestRuns)-passed)
            }
            if len(mf.Commits) > 0 {
                var msgs []string
                for _, c := range mf.Commits {
                    msgs = append(msgs, fmt.Sprintf("%q", c.Message))
                }
                fmt.Fprintf(&b, "Commits: %s\n", strings.Join(msgs, ", "))
            }
        }
        b.WriteString("\n")
    }

    if len(data.NextTasks) > 0 {
        b.WriteString("## Next Tasks\n")
        for _, task := range data.NextTasks {
            fmt.Fprintf(&b, "- [ ] %s\n", task)
        }
        b.WriteString("\n")
    }

    if len(f.KeyFiles) > 0 {
        b.WriteString("## Key Files\n")
        for _, kf := range f.KeyFiles {
            fmt.Fprintf(&b, "- %s\n", kf)
        }
        b.WriteString("\n")
    }

    if len(data.RecentSessions) > 0 {
        b.WriteString("## Recent Activity\n")
        for _, sess := range data.RecentSessions {
            line := fmt.Sprintf("- %s: %s", sess.CreatedAt.Format("2006-01-02"), sess.Summary)
            if len(sess.Commits) > 0 {
                line += fmt.Sprintf(" [%s]", strings.Join(sess.Commits, ", "))
            }
            fmt.Fprintf(&b, "%s\n", line)
        }
        b.WriteString("\n")
    }

    if len(data.SubtaskSummary) > 0 {
        b.WriteString("## Active Subtasks\n")
        for _, st := range data.SubtaskSummary {
            fmt.Fprintf(&b, "- %s [%d/%d]\n", st.Title, st.Done, st.Total)
        }
        b.WriteString("\n")
    }

    return b.String()
}
```

- [ ] **Step 2: Update writeHandoffFile to load checkpoint data**

Update `writeHandoffFile` to accept and use checkpoint data:

```go
func writeHandoffFile(dir string, data *store.HandoffData) error {
    return writeHandoffFileWithCheckpoints(dir, data, nil)
}

func writeHandoffFileWithCheckpoints(dir string, data *store.HandoffData, cpData *HandoffCheckpointData) error {
    handoffDir := filepath.Join(dir, ".docket", "handoff")
    if err := os.MkdirAll(handoffDir, 0755); err != nil {
        return err
    }
    path := filepath.Join(handoffDir, data.Feature.ID+".md")

    baseline := renderHandoff(data, cpData)

    // Preserve enrichment sections from board-manager if they exist
    if existing, err := os.ReadFile(path); err == nil {
        enrichment := extractEnrichmentSections(string(existing))
        if enrichment != "" {
            baseline = strings.TrimRight(baseline, "\n") + "\n" + enrichment
        }
    }

    return os.WriteFile(path, []byte(baseline), 0644)
}
```

- [ ] **Step 3: Update SessionEnd handler to pass checkpoint data**

In `cmd/docket/hook.go`, update `handleSessionEnd` to query and pass checkpoint data when writing handoffs:

```go
// In handleSessionEnd, replace the handoff writing section:
for _, f := range features {
    activeIDs[f.ID] = true
    data, err := s.GetHandoffData(f.ID)
    if err != nil {
        continue
    }

    // Load checkpoint data for the active work session's feature
    var cpData *HandoffCheckpointData
    if f.ID == ws.FeatureID {
        obs, _ := s.GetObservationsForWorkSession(ws.ID)
        mf, _ := s.GetMechanicalFactsForWorkSession(ws.ID)
        if len(obs) > 0 || mf != nil {
            cpData = &HandoffCheckpointData{
                Observations:    obs,
                MechanicalFacts: mf,
            }
        }
    }

    if writeErr := writeHandoffFileWithCheckpoints(h.CWD, data, cpData); writeErr != nil {
        s.MarkHandoffStale(ws.ID)
    }
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/docket/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/docket/handoff.go cmd/docket/hook.go
git commit -m "feat: add Last Session section to handoff from checkpoint observations"
```

---

### Task 12: Update hooks.json

**Files:**
- Modify: `plugin/hooks/hooks.json`

- [ ] **Step 1: Update hooks configuration**

Replace `plugin/hooks/hooks.json`:

```json
{
  "hooks": {
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
    "SessionStart": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "DOCKET_EXE_PATH hook",
            "timeout": 10
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "DOCKET_EXE_PATH hook",
            "timeout": 5
          }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "DOCKET_EXE_PATH hook",
            "timeout": 30
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "DOCKET_EXE_PATH hook",
            "timeout": 5
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "DOCKET_EXE_PATH hook",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add plugin/hooks/hooks.json
git commit -m "feat: add PreCompact and SessionEnd hooks, bump Stop timeout to 30s"
```

---

### Task 13: Plugin Skills — /checkpoint and /end-session

**Files:**
- Create: `plugin/skills/checkpoint/SKILL.md`
- Create: `plugin/skills/end-session/SKILL.md`

- [ ] **Step 1: Create /checkpoint skill**

Create `plugin/skills/checkpoint/SKILL.md`:

```markdown
---
name: checkpoint
description: Force a checkpoint of the current session's context. Preserves what was discussed, decisions made, and work done since the last checkpoint. Use when you want to save progress without ending the session.
---

# checkpoint: Save Session Context

Force a semantic checkpoint of the current Docket work session.

## Steps

1. **Call the checkpoint MCP tool:**
   Call `mcp__plugin_docket_docket__checkpoint` with no parameters.

2. **Report the result** to the user — how many chars of semantic text and files were captured.

## Notes

- Checkpoints are processed in the background by the Docket summarizer worker.
- If no API key is configured, only mechanical facts (files, tests, commits) are captured.
- The checkpoint is bound to the currently active work session and feature.
- If no work session is active, the tool will return an error.
```

- [ ] **Step 2: Create /end-session skill**

Create `plugin/skills/end-session/SKILL.md`:

```markdown
---
name: end-session
description: End the current Docket work session without closing Claude. Forces a final checkpoint, writes the handoff file, and closes the work session. Use when switching features or finishing work but keeping Claude open.
---

# end-session: End Docket Work Session

End the current Docket work session and write the handoff file.

## Steps

1. **Ask the user** if they want to set a `left_off` note before closing:
   > "Want to set a 'left off' note for the next session? (optional)"

2. **If they provide a note**, call `mcp__plugin_docket_docket__update_feature` with `left_off`.

3. **Close the work session with a final checkpoint:**
   Call `mcp__plugin_docket_docket__checkpoint` with `end_session=true`.
   This forces a checkpoint, writes the handoff file, and closes the work session in one call.

4. **Report** that the work session has been closed and the handoff file has been written.

## Notes

- This does NOT close Claude — only the Docket work session.
- The handoff file will be available at `.docket/handoff/<feature-id>.md`.
- Starting work on a new feature after this will open a new work session.
- This is a user-initiated action — Claude should not call this autonomously.
```

- [ ] **Step 3: Commit**

```bash
git add plugin/skills/checkpoint/SKILL.md plugin/skills/end-session/SKILL.md
git commit -m "feat: add /checkpoint and /end-session plugin skills"
```

---

### Task 14: Update CLAUDE.md and Documentation

**Files:**
- Modify: `cmd/docket/update.go` — update the CLAUDE.md snippet
- Modify: `docs/docket-hooks.md` — update hook documentation

- [ ] **Step 1: Read update.go to understand the snippet**

Read `cmd/docket/update.go` to find the CLAUDE.md snippet template.

- [ ] **Step 2: Update the CLAUDE.md snippet**

Update the snippet in `update.go` to:
- Remove references to `log_session`
- Add `/checkpoint` and `/end-session` commands
- Update the Stop hook description
- Mention that session context is captured automatically

- [ ] **Step 3: Update docs/docket-hooks.md**

Rewrite `docs/docket-hooks.md` to reflect:
- Single-phase Stop (no blocking)
- PreCompact checkpoint preservation
- SessionEnd cheap finalization
- Background summarizer worker
- Removed: two-phase stop, log_session prompting

- [ ] **Step 4: Run tests**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/docket/update.go docs/docket-hooks.md
git commit -m "docs: update CLAUDE.md snippet and hooks documentation for checkpoint-based flow"
```

---

### Task 15: Write Feature Documentation

**Files:**
- Create: `docs/transcript-session-context.md`

- [ ] **Step 1: Write the feature doc**

Create `docs/transcript-session-context.md` following the grug-brain style covering: what it does, why it exists, how to use it, gotchas, and key files.

- [ ] **Step 2: Commit**

```bash
git add docs/transcript-session-context.md
git commit -m "docs: add transcript-based session context feature documentation"
```

---

### Task 16: Full Integration Test

- [ ] **Step 1: Run the full test suite**

Run: `go test ./... -v`
Expected: PASS — all packages

- [ ] **Step 2: Build the binary**

Run: `go build -ldflags="-s -w" -o docket.exe ./cmd/docket/`
Expected: Build succeeds

- [ ] **Step 3: Manual smoke test**

Run: `./docket.exe version`
Expected: `docket v0.1.0`

- [ ] **Step 4: Final commit if any cleanup needed**

Only if previous steps revealed issues that needed fixing.
