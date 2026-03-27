package dashboard

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/sniffyanimal/feat/internal/store"
)

func NewHandler(s *store.Store, static fs.FS) http.Handler {
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
		writeJSON(w, features)
	})

	mux.HandleFunc("GET /api/features/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		f, err := s.GetFeature(id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		sessions, _ := s.GetSessionsForFeature(id)
		if sessions == nil {
			sessions = []store.Session{}
		}
		writeJSON(w, map[string]any{"feature": f, "sessions": sessions})
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

	// Serve static dashboard files if provided
	if static != nil {
		mux.Handle("GET /", http.FileServerFS(static))
	}

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
