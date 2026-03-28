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
		writeJSON(w, map[string]any{"feature": f, "subtasks": subtasks, "sessions": sessions})
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

	// Serve dashboard HTML — prefer local file on disk for dev, fall back to embedded
	if static != nil {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.FileServerFS(static).ServeHTTP(w, r)
				return
			}
			devPath := "dashboard/index.html"
			if devDir != "" {
				devPath = filepath.Join(devDir, "dashboard", "index.html")
			}
			if devHTML, err := os.ReadFile(devPath); err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(devHTML)
				return
			}
			http.FileServerFS(static).ServeHTTP(w, r)
		})
	}

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
