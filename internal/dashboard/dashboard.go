package dashboard

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

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

			if state, ok := sessionStates[f.ID]; ok {
				fp.SessionState = state
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
