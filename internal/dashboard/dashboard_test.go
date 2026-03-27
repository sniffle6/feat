package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sniffyanimal/feat/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestListFeaturesAPI(t *testing.T) {
	s := testStore(t)
	s.AddFeature("Feature A", "desc a")
	s.AddFeature("Feature B", "desc b")

	handler := NewHandler(s, nil)
	req := httptest.NewRequest("GET", "/api/features", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var features []store.Feature
	json.NewDecoder(w.Body).Decode(&features)
	if len(features) != 2 {
		t.Fatalf("got %d features", len(features))
	}
}

func TestGetFeatureAPI(t *testing.T) {
	s := testStore(t)
	s.AddFeature("Web Browser", "w3m")

	handler := NewHandler(s, nil)
	req := httptest.NewRequest("GET", "/api/features/web-browser", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestPatchFeatureAPI(t *testing.T) {
	s := testStore(t)
	s.AddFeature("Settings", "prefs")

	handler := NewHandler(s, nil)
	body := `{"status":"in_progress","left_off":"need save button"}`
	req := httptest.NewRequest("PATCH", "/api/features/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	f, _ := s.GetFeature("settings")
	if f.Status != "in_progress" {
		t.Errorf("Status = %q", f.Status)
	}
}

func TestGetUnlinkedSessionsAPI(t *testing.T) {
	s := testStore(t)
	s.LogSession(store.SessionInput{Summary: "orphan"})

	handler := NewHandler(s, nil)
	req := httptest.NewRequest("GET", "/api/sessions?unlinked=true", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var sessions []store.Session
	json.NewDecoder(w.Body).Decode(&sessions)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions", len(sessions))
	}
}

func TestReassignSessionAPI(t *testing.T) {
	s := testStore(t)
	s.AddFeature("Feature A", "")
	sess, _ := s.LogSession(store.SessionInput{Summary: "orphan"})

	handler := NewHandler(s, nil)
	body := fmt.Sprintf(`{"feature_id":"feature-a"}`)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/sessions/%d", sess.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
