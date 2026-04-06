package checkpoint

import (
	"context"
	"testing"
	"time"

	"github.com/sniffle6/claude-docket/internal/store"
)

type mockSummarizer struct {
	calls           int
	synthesizeCalls int
	output          *SummarizeOutput
	synthesizeText  string
	err             error
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

func (m *mockSummarizer) Synthesize(ctx context.Context, input SynthesizeInput) (*SynthesizeOutput, error) {
	m.synthesizeCalls++
	if m.err != nil {
		return nil, m.err
	}
	text := m.synthesizeText
	if text == "" {
		text = "Synthesized summary of all observations."
	}
	return &SynthesizeOutput{Text: text}, nil
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
		WorkSessionID:         ws.ID,
		FeatureID:             "auth-system",
		Reason:                "stop",
		TranscriptStartOffset: 0,
		TranscriptEndOffset:   512,
		SemanticText:          "discussed auth design",
		MechanicalFacts:       store.MechanicalFacts{},
	})

	mock := &mockSummarizer{}
	w := NewWorker(s, mock)

	processed := w.ProcessOne()
	if !processed {
		t.Fatal("expected to process a job")
	}
	if mock.calls != 1 {
		t.Errorf("summarizer calls = %d, want 1", mock.calls)
	}

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
		WorkSessionID:         ws.ID,
		FeatureID:             "auth-system",
		Reason:                "stop",
		TranscriptStartOffset: 0,
		TranscriptEndOffset:   512,
		SemanticText:          "", // empty
		MechanicalFacts:       store.MechanicalFacts{FilesEdited: []store.FileEdit{{Path: "a.go", Count: 1}}},
	})

	mock := &mockSummarizer{}
	w := NewWorker(s, mock)
	w.ProcessOne()

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

	w.Run(ctx, 50*time.Millisecond)
	// No panic = pass
}

func TestWorkerSynthesizesAfterSessionEnd(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.AddFeature("Auth System", "token auth")
	ws, _ := s.OpenWorkSession("auth-system", "sess-1")

	s.EnqueueCheckpointJob(store.CheckpointJobInput{
		WorkSessionID:         ws.ID,
		FeatureID:             "auth-system",
		Reason:                "session_end",
		TranscriptStartOffset: 0,
		TranscriptEndOffset:   512,
		SemanticText:          "implemented token refresh",
		MechanicalFacts:       store.MechanicalFacts{},
	})

	mock := &mockSummarizer{synthesizeText: "Auth system with refresh tokens implemented."}
	w := NewWorker(s, mock)
	w.ProcessOne()

	if mock.calls != 1 {
		t.Errorf("summarize calls = %d, want 1", mock.calls)
	}
	if mock.synthesizeCalls != 1 {
		t.Errorf("synthesize calls = %d, want 1", mock.synthesizeCalls)
	}

	f, _ := s.GetFeature("auth-system")
	if f.Synthesis != "Auth system with refresh tokens implemented." {
		t.Errorf("Synthesis = %q, want expected text", f.Synthesis)
	}
	if f.SynthesisObsID == 0 {
		t.Error("SynthesisObsID should be > 0 after synthesis")
	}
}

func TestWorkerSkipsSynthesisForNonSessionEnd(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.AddFeature("Auth System", "token auth")
	ws, _ := s.OpenWorkSession("auth-system", "sess-1")

	s.EnqueueCheckpointJob(store.CheckpointJobInput{
		WorkSessionID:         ws.ID,
		FeatureID:             "auth-system",
		Reason:                "stop",
		TranscriptStartOffset: 0,
		TranscriptEndOffset:   512,
		SemanticText:          "some work",
		MechanicalFacts:       store.MechanicalFacts{},
	})

	mock := &mockSummarizer{}
	w := NewWorker(s, mock)
	w.ProcessOne()

	if mock.synthesizeCalls != 0 {
		t.Errorf("synthesize calls = %d, want 0 for non-session_end", mock.synthesizeCalls)
	}
}
