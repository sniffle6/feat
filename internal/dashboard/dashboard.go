package dashboard

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sniffle6/claude-docket/internal/store"
)

type subtaskProgress struct {
	Title string `json:"title"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

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

func NewHandler(s *store.Store, static fs.FS, projectDir ...string) http.Handler {
	var devDir string
	if len(projectDir) > 0 {
		devDir = projectDir[0]
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/features", func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		features, err := s.ListFeatures(status)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if features == nil {
			features = []store.Feature{}
		}

		sessionStates, _ := s.GetActiveSessionStates()

		var result []featureWithProgress
		for _, f := range features {
			fp := featureWithProgress{Feature: f}
			done, total, _ := s.GetFeatureProgress(f.ID)
			fp.ProgressDone = done
			fp.ProgressTotal = total

			if total > 0 {
				subtasks, _ := s.GetSubtasksForFeature(f.ID, false)
				for _, st := range subtasks {
					stDone := 0
					for _, item := range st.Items {
						if item.Checked {
							stDone++
						} else if fp.NextTask == "" {
							fp.NextTask = item.Title
						}
					}
					fp.SubtaskProgress = append(fp.SubtaskProgress, subtaskProgress{
						Title: st.Title,
						Done:  stDone,
						Total: len(st.Items),
					})
				}
			}
			if fp.SubtaskProgress == nil {
				fp.SubtaskProgress = []subtaskProgress{}
			}
			fp.IssueCount, _ = s.GetOpenIssueCount(f.ID)

			if info, ok := sessionStates[f.ID]; ok {
				fp.SessionState = info.State
				fp.LastHeartbeat = info.LastHeartbeat
			} else {
				fp.SessionState = "idle"
			}

			result = append(result, fp)
		}
		if result == nil {
			result = []featureWithProgress{}
		}
		writeJSON(w, result)
	})

	mux.HandleFunc("GET /api/features/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		f, err := s.GetFeature(id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		includeArchived := r.URL.Query().Get("include_archived") == "true"
		subtasks, _ := s.GetSubtasksForFeature(id, includeArchived)
		sessions, _ := s.GetSessionsForFeature(id)
		if sessions == nil {
			sessions = []store.Session{}
		}
		if subtasks == nil {
			subtasks = []store.Subtask{}
		}
		decisions, _ := s.GetDecisionsForFeature(id)
		if decisions == nil {
			decisions = []store.Decision{}
		}
		issues, _ := s.GetIssuesForFeature(id)
		if issues == nil {
			issues = []store.Issue{}
		}
		writeJSON(w, map[string]any{"feature": f, "subtasks": subtasks, "sessions": sessions, "decisions": decisions, "issues": issues})
	})

	mux.HandleFunc("PATCH /api/features/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var u store.FeatureUpdate
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := s.UpdateFeature(id, u); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	})

	mux.HandleFunc("POST /api/issues", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			FeatureID  string `json:"feature_id"`
			Description string `json:"description"`
			TaskItemID *int64 `json:"task_item_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		issue, err := s.AddIssue(body.FeatureID, body.Description, body.TaskItemID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, issue)
	})

	mux.HandleFunc("PATCH /api/issues/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid issue id", 400)
			return
		}
		var body struct {
			CommitHash string `json:"commit_hash"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := s.ResolveIssue(id, body.CommitHash); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	})

	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("unlinked") == "true" {
			sessions, err := s.GetUnlinkedSessions()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			if sessions == nil {
				sessions = []store.Session{}
			}
			writeJSON(w, sessions)
			return
		}
		http.Error(w, "use ?unlinked=true", 400)
	})

	mux.HandleFunc("PATCH /api/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid session id", 400)
			return
		}
		var body struct {
			FeatureID string `json:"feature_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := s.ReassignSession(id, body.FeatureID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	})

	mux.HandleFunc("POST /api/launch/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		// Check for active session — prevent duplicate launches
		states, _ := s.GetActiveSessionStates()
		if info, ok := states[id]; ok && (info.State == "working" || info.State == "needs_attention") {
			// Allow launching over stale sessions (heartbeat older than 5 minutes)
			stale := info.LastHeartbeat != nil && time.Since(*info.LastHeartbeat) > 5*time.Minute
			if !stale {
				http.Error(w, "session already active for this feature", 409)
				return
			}
		}

		// Get launch data
		data, err := s.GetLaunchData(id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}

		projDir := devDir
		if projDir == "" {
			projDir, _ = os.Getwd()
		}

		// Check for existing handoff file, use it as base if available
		var promptContent string
		handoffPath := filepath.Join(projDir, ".docket", "handoff", id+".md")
		if existing, err := os.ReadFile(handoffPath); err == nil {
			// Append unchecked tasks and open issues not already in the handoff
			promptContent = string(existing)
			extra := renderLaunchExtras(data)
			if extra != "" {
				promptContent = strings.TrimRight(promptContent, "\n") + "\n\n" + extra
			}
		} else {
			promptContent = RenderLaunchPrompt(data)
		}

		// Write launch prompt file
		launchDir := filepath.Join(projDir, ".docket", "launch")
		os.MkdirAll(launchDir, 0755)
		promptPath := filepath.Join(launchDir, id+".md")
		if err := os.WriteFile(promptPath, []byte(promptContent), 0644); err != nil {
			http.Error(w, "failed to write launch prompt: "+err.Error(), 500)
			return
		}

		// Write a .cmd launcher script to avoid nested quoting issues.
		// The script has one command per line with clean quoting.
		cmdScript := fmt.Sprintf("@echo off\r\ncd /d \"%s\"\r\nclaude --dangerously-skip-permissions --append-system-prompt-file \"%s\" \"Resume work on: %s (feature_id: %s). Check get_ready for current status.\"\r\n",
			projDir, promptPath, data.Feature.Title, id)
		cmdPath := filepath.Join(launchDir, id+".cmd")
		if err := os.WriteFile(cmdPath, []byte(cmdScript), 0644); err != nil {
			http.Error(w, "failed to write launch script: "+err.Error(), 500)
			return
		}

		// Open the launcher in a new terminal window
		cmd := exec.Command("cmd")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CmdLine: fmt.Sprintf(`cmd /C start "" wt cmd /k "%s"`, cmdPath),
		}
		cmd.Dir = projDir
		if err := cmd.Start(); err != nil {
			http.Error(w, "failed to launch: "+err.Error(), 500)
			return
		}

		// Don't wait for the process — it's a terminal
		go cmd.Wait()

		writeJSON(w, map[string]any{
			"ok":          true,
			"feature_id":  id,
			"prompt_file": promptPath,
		})
	})

	mux.HandleFunc("GET /api/project", func(w http.ResponseWriter, r *http.Request) {
		name := "docket"
		if devDir != "" {
			name = filepath.Base(devDir)
		}
		writeJSON(w, map[string]string{"name": name})
	})

	// Serve dashboard files — prefer local files on disk for dev, fall back to embedded
	if static != nil {
		fileServer := http.FileServerFS(static)
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			// Dev mode: try serving from disk first (any file, not just index.html)
			dashDir := "dashboard"
			if devDir != "" {
				dashDir = filepath.Join(devDir, "dashboard")
			}
			relPath := r.URL.Path
			if relPath == "/" {
				relPath = "/index.html"
			}
			devPath := filepath.Join(dashDir, filepath.Clean(relPath))
			if data, err := os.ReadFile(devPath); err == nil {
				switch filepath.Ext(devPath) {
				case ".html":
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
				case ".png":
					w.Header().Set("Content-Type", "image/png")
				case ".svg":
					w.Header().Set("Content-Type", "image/svg+xml")
				case ".css":
					w.Header().Set("Content-Type", "text/css")
				case ".js":
					w.Header().Set("Content-Type", "application/javascript")
				}
				w.Write(data)
				return
			}
			fileServer.ServeHTTP(w, r)
		})
	}

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
