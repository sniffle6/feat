package checkpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// CLISummarizer shells out to the Claude Code CLI for summarization.
// Zero config — uses whatever auth Claude Code already has.
type CLISummarizer struct {
	model string
}

func NewCLISummarizer(model string) *CLISummarizer {
	return &CLISummarizer{model: model}
}

// CLIAvailable returns true if the claude CLI is in PATH.
func CLIAvailable() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

func (s *CLISummarizer) Summarize(ctx context.Context, input SummarizeInput) (*SummarizeOutput, error) {
	instruction := `You are a session context extractor for a software development feature tracker. Given a conversation excerpt between a developer and an AI assistant, extract structured information about what happened.

Respond with ONLY a JSON object (no markdown, no explanation) with these fields:
- "summary": 2-4 sentence narrative of what was discussed and accomplished
- "blockers": array of strings — anything blocking progress
- "dead_ends": array of strings — approaches tried that didn't work
- "next_steps": array of strings — what should happen next
- "decisions": array of strings — decisions made during the conversation
- "gotchas": array of strings — non-obvious discoveries or warnings

Use empty arrays for fields with no content. Be concise.`

	userContent := fmt.Sprintf("Feature: %s\nCheckpoint reason: %s\n\nConversation excerpt:\n%s",
		input.FeatureTitle, input.Reason, input.SemanticText)

	if len(userContent) > 30000 {
		userContent = userContent[len(userContent)-30000:]
	}

	out, err := s.run(ctx, instruction, userContent)
	if err != nil {
		return nil, err
	}

	var output SummarizeOutput
	if err := json.Unmarshal(out, &output); err != nil {
		output.Summary = strings.TrimSpace(string(out))
	}

	return &output, nil
}

func (s *CLISummarizer) Synthesize(ctx context.Context, input SynthesizeInput) (*SynthesizeOutput, error) {
	instruction := `You are updating a knowledge summary for a software feature being tracked in a project management tool. You receive all observations from work sessions on this feature. Produce a concise, evolving narrative (3-8 sentences) that captures: what has been built, what approaches were tried and failed (dead ends), key decisions made, active blockers, and non-obvious gotchas. Drop stale information (resolved blockers, superseded next-steps). Prioritize what a developer starting a new session needs to know to be productive immediately.

Respond with ONLY the narrative text. No markdown headers, no bullet points, no JSON.`

	var obsLines []string
	for _, o := range input.Observations {
		obsLines = append(obsLines, fmt.Sprintf("[%s] %s", o.Kind, o.SummaryText))
	}

	userContent := fmt.Sprintf("Feature: %s\n\nObservations (newest first):\n%s",
		input.FeatureTitle, strings.Join(obsLines, "\n"))

	out, err := s.run(ctx, instruction, userContent)
	if err != nil {
		return nil, err
	}

	return &SynthesizeOutput{Text: strings.TrimSpace(string(out))}, nil
}

func (s *CLISummarizer) run(ctx context.Context, instruction, stdinContent string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "claude", "-p", instruction, "--model", s.model, "--output-format", "text")
	cmd.Stdin = strings.NewReader(stdinContent)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("claude cli: %w: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}
