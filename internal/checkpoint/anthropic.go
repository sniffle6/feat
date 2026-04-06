package checkpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultBaseURL = "https://api.anthropic.com"
const anthropicVersion = "2023-06-01"

// AnthropicSummarizer calls the Anthropic Messages API to summarize transcript deltas.
type AnthropicSummarizer struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewAnthropicSummarizer creates a summarizer using the Anthropic Messages API.
func NewAnthropicSummarizer(cfg Config) *AnthropicSummarizer {
	return &AnthropicSummarizer{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: defaultBaseURL,
		client:  &http.Client{},
	}
}

type messagesRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system"`
	Messages  []messageReq `json:"messages"`
}

type messageReq struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	Content []contentResp `json:"content"`
	Error   *apiError     `json:"error"`
}

type contentResp struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type apiError struct {
	Message string `json:"message"`
}

func (s *AnthropicSummarizer) Summarize(ctx context.Context, input SummarizeInput) (*SummarizeOutput, error) {
	systemPrompt := `You are a session context extractor for a software development feature tracker. Given a conversation excerpt between a developer and an AI assistant, extract structured information about what happened.

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

	// Truncate if too long (keep last part which is most relevant)
	if len(userContent) > 30000 {
		userContent = userContent[len(userContent)-30000:]
	}

	reqBody := messagesRequest{
		Model:     s.model,
		MaxTokens: 1024,
		System:    systemPrompt,
		Messages: []messageReq{
			{Role: "user", Content: userContent},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var msgResp messagesResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(msgResp.Content) == 0 {
		return nil, fmt.Errorf("empty response content")
	}

	var output SummarizeOutput
	if err := json.Unmarshal([]byte(msgResp.Content[0].Text), &output); err != nil {
		// If JSON parsing fails, use raw text as summary
		output.Summary = msgResp.Content[0].Text
	}

	return &output, nil
}

func (s *AnthropicSummarizer) Synthesize(ctx context.Context, input SynthesizeInput) (*SynthesizeOutput, error) {
	systemPrompt := `You are updating a knowledge summary for a software feature being tracked in a project management tool. You receive all observations from work sessions on this feature. Produce a concise, evolving narrative (3-8 sentences) that captures: what has been built, what approaches were tried and failed (dead ends), key decisions made, active blockers, and non-obvious gotchas. Drop stale information (resolved blockers, superseded next-steps). Prioritize what a developer starting a new session needs to know to be productive immediately.

Respond with ONLY the narrative text. No markdown headers, no bullet points, no JSON.`

	var obsLines []string
	for _, o := range input.Observations {
		obsLines = append(obsLines, fmt.Sprintf("[%s] %s", o.Kind, o.SummaryText))
	}

	userContent := fmt.Sprintf("Feature: %s\n\nObservations (newest first):\n%s",
		input.FeatureTitle, strings.Join(obsLines, "\n"))

	if len(userContent) > 30000 {
		userContent = userContent[len(userContent)-30000:]
	}

	reqBody := messagesRequest{
		Model:     s.model,
		MaxTokens: 512,
		System:    systemPrompt,
		Messages:  []messageReq{{Role: "user", Content: userContent}},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var msgResp messagesResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(msgResp.Content) == 0 {
		return nil, fmt.Errorf("empty response content")
	}

	return &SynthesizeOutput{Text: msgResp.Content[0].Text}, nil
}
