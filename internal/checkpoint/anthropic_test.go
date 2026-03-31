package checkpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicSummarizer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		client:  &http.Client{},
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
		client:  &http.Client{},
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
