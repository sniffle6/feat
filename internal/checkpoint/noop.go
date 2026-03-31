package checkpoint

import "context"

// NoopSummarizer returns empty output. Used when no API key is configured.
type NoopSummarizer struct{}

func (n *NoopSummarizer) Summarize(ctx context.Context, input SummarizeInput) (*SummarizeOutput, error) {
	return &SummarizeOutput{}, nil
}
