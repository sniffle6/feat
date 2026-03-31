package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/sniffle6/claude-docket/internal/store"
)

// transcriptRecord represents one line in the JSONL transcript.
type transcriptRecord struct {
	Type    string          `json:"type"`
	Message *messagePayload `json:"message"`
	// tool_result fields
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"isError"`
}

type messagePayload struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	ID    string          `json:"id"`
	Input json.RawMessage `json:"input"`
}

type toolInput struct {
	FilePath string `json:"file_path"`
	Command  string `json:"command"`
}

var (
	testCmdPattern = regexp.MustCompile(`(?i)(go test|npm test|pytest|jest|cargo test|make test)`)
	commitPattern  = regexp.MustCompile(`\[[\w/.-]+ ([a-f0-9]{7,40})\] (.+)`)
)

// Parse reads a JSONL transcript from startOffset and returns a Delta
// containing filtered semantic text and mechanical facts.
// Returns an empty Delta (not an error) if the file doesn't exist or is empty.
func Parse(path string, startOffset int64) (*Delta, error) {
	f, err := os.Open(path)
	if err != nil {
		// Missing file = empty delta, not an error
		return &Delta{EndOffset: startOffset}, nil
	}
	defer f.Close()

	if startOffset > 0 {
		if _, err := f.Seek(startOffset, 0); err != nil {
			return &Delta{EndOffset: startOffset}, nil
		}
	}

	delta := &Delta{EndOffset: startOffset}
	var semanticBuf strings.Builder

	// Track pending tool uses for matching with results
	pendingTools := make(map[string]contentBlock) // tool_use_id -> block
	fileEditCounts := make(map[string]int)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // up to 10MB lines

	for scanner.Scan() {
		line := scanner.Bytes()
		delta.EndOffset += int64(len(line)) + 1 // +1 for newline

		var rec transcriptRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			fmt.Fprintf(os.Stderr, "docket transcript: skip malformed line: %v\n", err)
			continue
		}

		switch rec.Type {
		case "assistant":
			if rec.Message == nil {
				continue
			}
			for _, block := range rec.Message.Content {
				switch block.Type {
				case "text":
					if block.Text != "" {
						semanticBuf.WriteString(block.Text)
						semanticBuf.WriteByte('\n')
						delta.HasContent = true
					}
				case "tool_use":
					pendingTools[block.ID] = block
					processToolUse(block, fileEditCounts)
				}
			}

		case "user":
			if rec.Message == nil {
				continue
			}
			for _, block := range rec.Message.Content {
				if block.Type == "text" && block.Text != "" {
					normalized := strings.ToLower(strings.TrimSpace(block.Text))
					if !trivialUserMessages[normalized] {
						delta.HasContent = true
					}
					// Include user text in semantic output regardless
					semanticBuf.WriteString(block.Text)
					semanticBuf.WriteByte('\n')
				}
			}

		case "tool_result":
			if pending, ok := pendingTools[rec.ToolUseID]; ok {
				processToolResult(pending, rec, delta)
				delete(pendingTools, rec.ToolUseID)
			}
		}
	}

	// Convert file edit counts to FileEdit slice
	for p, count := range fileEditCounts {
		delta.MechanicalFacts.FilesEdited = append(delta.MechanicalFacts.FilesEdited, store.FileEdit{
			Path: p, Count: count,
		})
	}

	delta.SemanticText = semanticBuf.String()

	return delta, nil
}

func processToolUse(block contentBlock, fileEditCounts map[string]int) {
	var ti toolInput
	json.Unmarshal(block.Input, &ti)

	switch block.Name {
	case "Edit", "Write":
		if ti.FilePath != "" {
			fileEditCounts[ti.FilePath]++
		}
	}
}

func processToolResult(pending contentBlock, rec transcriptRecord, delta *Delta) {
	var resultText string
	// Content can be a string or an array
	if err := json.Unmarshal(rec.Content, &resultText); err != nil {
		// Try as raw string (already a string)
		resultText = string(rec.Content)
	}

	var ti toolInput
	json.Unmarshal(pending.Input, &ti)

	// Check for errors
	if rec.IsError {
		delta.MechanicalFacts.Errors = append(delta.MechanicalFacts.Errors, store.ErrorFact{
			Tool:    pending.Name,
			Message: truncate(resultText, 200),
		})
	}

	// Bash-specific analysis
	if pending.Name == "Bash" {
		// Test detection
		if testCmdPattern.MatchString(ti.Command) {
			passed := !rec.IsError
			delta.MechanicalFacts.TestRuns = append(delta.MechanicalFacts.TestRuns, store.TestRunFact{
				Command: ti.Command,
				Passed:  passed,
			})
		}

		// Commit detection
		if strings.Contains(ti.Command, "git commit") {
			if matches := commitPattern.FindStringSubmatch(resultText); len(matches) >= 3 {
				delta.MechanicalFacts.Commits = append(delta.MechanicalFacts.Commits, store.CommitFact{
					Hash:    matches[1],
					Message: matches[2],
				})
			}
		}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
