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
	Summary   string   `json:"summary"`   // human-readable narrative
	Blockers  []string `json:"blockers"`  // discovered blockers
	DeadEnds  []string `json:"dead_ends"` // things tried that didn't work
	NextSteps []string `json:"next_steps"` // intent for next session
	Decisions []string `json:"decisions"` // decisions discussed
	Gotchas   []string `json:"gotchas"`   // non-obvious discoveries
}

// SummarizerBackend processes transcript deltas into structured observations.
type SummarizerBackend interface {
	Summarize(ctx context.Context, input SummarizeInput) (*SummarizeOutput, error)
}
