package store

import (
	"testing"
)

func TestSearchBasicMatch(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Add a feature with searchable content
	s.AddFeature("Auth Middleware", "Implement JWT authentication for API endpoints")

	results, err := s.Search("authentication", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'authentication'")
	}
	found := false
	for _, r := range results {
		if r.EntityType == "feature" && r.FeatureID == "auth-middleware" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected feature 'auth-middleware' in results, got: %+v", results)
	}
}

func TestSearchScopeFilter(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.AddFeature("Caching Layer", "Redis caching for API responses")
	s.AddDecision("caching-layer", "Use Redis for caching", "accepted", "Low latency")

	// Search with scope=decision only — should NOT find the feature
	results, err := s.Search("caching", SearchOpts{Scope: []string{"decision"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.EntityType != "decision" {
			t.Errorf("scope filter failed: got entity_type %q, want 'decision'", r.EntityType)
		}
	}
	if len(results) == 0 {
		t.Fatal("expected at least one decision result")
	}
}

func TestSearchPathQuery(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Update key_files to include a path
	s.AddFeature("Path Test", "Testing file path search")
	kf := `["internal/store/search.go","internal/mcp/tools.go"]`
	s.UpdateFeature("path-test", FeatureUpdate{KeyFiles: &[]string{"internal/store/search.go", "internal/mcp/tools.go"}})

	_ = kf // key_files stored as JSON by UpdateFeature

	// Path-like query should not error and should find results
	results, err := s.Search("search.go", SearchOpts{})
	if err != nil {
		t.Fatalf("path query should not error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for path query 'search.go'")
	}
}

func TestSearchFeatureIDFilter(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.AddFeature("Auth System", "JWT authentication")
	s.AddFeature("Billing System", "Payment authentication via Stripe")

	results, err := s.Search("authentication", SearchOpts{FeatureID: "auth-system"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.FeatureID != "auth-system" {
			t.Errorf("feature_id filter failed: got %q, want 'auth-system'", r.FeatureID)
		}
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for auth-system")
	}
}

func TestSearchCrossEntity(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	f, _ := s.AddFeature("API Redesign", "Redesign the REST API")
	s.AddDecision(f.ID, "Use GraphQL", "rejected", "Too complex for caching layer")
	s.AddIssue(f.ID, "Cache invalidation broken in staging", nil)
	s.AddNote(f.ID, "Investigated caching — Redis cluster works well")

	results, err := s.Search("caching", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}

	types := map[string]bool{}
	for _, r := range results {
		types[r.EntityType] = true
	}
	// Should find matches in decisions, issues, and notes
	if !types["decision"] {
		t.Error("expected decision in results")
	}
	if !types["issue"] {
		t.Error("expected issue in results")
	}
	if !types["note"] {
		t.Error("expected note in results")
	}
}

func TestSearchNoResults(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.AddFeature("Simple Feature", "Nothing special here")

	results, err := s.Search("xyznonexistent", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchVerbose(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.AddFeature("Verbose Test", "This is a detailed description about authentication middleware")

	// Non-verbose: should get a snippet
	results, err := s.Search("authentication", SearchOpts{Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}

	// Verbose: should get full content
	verboseResults, err := s.Search("authentication", SearchOpts{Verbose: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(verboseResults) == 0 {
		t.Fatal("expected verbose results")
	}
	foundFull := false
	for _, r := range verboseResults {
		if r.FieldName == "description" && r.Snippet == "This is a detailed description about authentication middleware" {
			foundFull = true
		}
	}
	if !foundFull {
		t.Errorf("verbose mode should return full content, got: %+v", verboseResults)
	}
}

func TestSearchPhraseMatch(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.AddFeature("Cache Feature", "Implement cache invalidation strategy")
	s.AddFeature("Other Feature", "Invalidation of user sessions on logout")

	// Phrase search should only match the exact phrase
	results, err := s.Search(`"cache invalidation"`, SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for phrase search")
	}
	for _, r := range results {
		if r.FeatureID == "other-feature" {
			t.Error("phrase search should not match 'other-feature' which has the words separately")
		}
	}
}

func TestSearchPrefixMatch(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.AddFeature("Auth Feature", "Implement authentication and authorization")

	// Prefix search: "auth*" should match authentication, authorization, auth
	results, err := s.Search("auth*", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for prefix search 'auth*'")
	}
	found := false
	for _, r := range results {
		if r.FeatureID == "auth-feature" {
			found = true
			break
		}
	}
	if !found {
		t.Error("prefix search should match auth-feature")
	}
}

func TestSearchTriggerSync(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Insert: should appear in search
	s.AddFeature("Trigger Test", "Synchronization verification")
	results, err := s.Search("synchronization", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected result after insert")
	}

	// Update: search should reflect the change
	newDesc := "Updated to use websockets"
	s.UpdateFeature("trigger-test", FeatureUpdate{Description: &newDesc})
	results, err = s.Search("websockets", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected result after update")
	}
	// Old content should no longer match
	results, err = s.Search("synchronization", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	foundOld := false
	for _, r := range results {
		if r.FieldName == "description" && r.FeatureID == "trigger-test" {
			foundOld = true
		}
	}
	if foundOld {
		t.Error("old description content should not appear in search after update")
	}
}

func TestRebuildSearchIndex(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.AddFeature("Rebuild Test", "Testing index rebuild functionality")

	// Manually corrupt the index by clearing it
	s.db.Exec("DELETE FROM search_index")
	results, err := s.Search("rebuild", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatal("expected empty results after clearing index")
	}

	// Rebuild should restore it
	if err := s.RebuildSearchIndex(); err != nil {
		t.Fatal(err)
	}
	results, err = s.Search("rebuild", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results after rebuild")
	}
}
