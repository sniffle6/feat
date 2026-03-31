package transcript

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	var content string
	for _, line := range lines {
		content += line + "\n"
	}
	os.WriteFile(path, []byte(content), 0644)
	return path
}

func TestParseAssistantText(t *testing.T) {
	path := writeFixture(t,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I'll fix the auth bug by updating the token validation."}]}}`,
	)
	delta, err := Parse(path, 0)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if delta.SemanticText == "" {
		t.Error("expected semantic text from assistant message")
	}
	if !delta.HasContent {
		t.Error("expected HasContent=true")
	}
}

func TestParseFiltersToolResults(t *testing.T) {
	path := writeFixture(t,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Let me read the file."},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/tmp/foo.go"}}]}}`,
		`{"type":"tool_result","tool_use_id":"t1","content":"package main\nfunc main() {}"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"The file looks correct."}]}}`,
	)
	delta, err := Parse(path, 0)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Should contain assistant text but not the tool result content
	if len(delta.SemanticText) > 200 {
		t.Errorf("semantic text too long (%d chars), likely includes tool result", len(delta.SemanticText))
	}
}

func TestParseExtractsFileEdits(t *testing.T) {
	path := writeFixture(t,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/tmp/store.go","old_string":"foo","new_string":"bar"}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"Edit","input":{"file_path":"/tmp/store.go","old_string":"baz","new_string":"qux"}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t3","name":"Write","input":{"file_path":"/tmp/new.go","content":"package new"}}]}}`,
	)
	delta, err := Parse(path, 0)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(delta.MechanicalFacts.FilesEdited) != 2 {
		t.Errorf("FilesEdited = %d, want 2 (store.go counted once, new.go once)", len(delta.MechanicalFacts.FilesEdited))
	}
}

func TestParseExtractsTestRuns(t *testing.T) {
	path := writeFixture(t,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}`,
		`{"type":"tool_result","tool_use_id":"t1","content":"ok  \tpkg\t0.5s","isError":false}`,
	)
	delta, err := Parse(path, 0)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(delta.MechanicalFacts.TestRuns) != 1 {
		t.Fatalf("TestRuns = %d, want 1", len(delta.MechanicalFacts.TestRuns))
	}
	if !delta.MechanicalFacts.TestRuns[0].Passed {
		t.Error("expected test run to show as passed")
	}
}

func TestParseExtractsErrors(t *testing.T) {
	path := writeFixture(t,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls /nonexistent"}}]}}`,
		`{"type":"tool_result","tool_use_id":"t1","content":"ls: cannot access '/nonexistent'","isError":true}`,
	)
	delta, err := Parse(path, 0)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(delta.MechanicalFacts.Errors) != 1 {
		t.Fatalf("Errors = %d, want 1", len(delta.MechanicalFacts.Errors))
	}
}

func TestParseFromOffset(t *testing.T) {
	line1 := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"First message."}]}}`
	line2 := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Second message."}]}}`
	path := writeFixture(t, line1, line2)

	// Parse from offset after first line
	offset := int64(len(line1) + 1) // +1 for newline
	delta, err := Parse(path, offset)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if delta.SemanticText != "Second message.\n" {
		t.Errorf("SemanticText = %q, want only second message", delta.SemanticText)
	}
}

func TestParseTrivialUserMessages(t *testing.T) {
	path := writeFixture(t,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"ok"}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"thanks"}]}}`,
	)
	delta, err := Parse(path, 0)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if delta.HasContent {
		t.Error("expected HasContent=false for trivial-only messages")
	}
}

func TestParseSkipsMalformedLines(t *testing.T) {
	path := writeFixture(t,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Good line."}]}}`,
		`this is not json`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Another good line."}]}}`,
	)
	delta, err := Parse(path, 0)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if delta.SemanticText == "" {
		t.Error("expected semantic text from non-malformed lines")
	}
}

func TestParseMissingFile(t *testing.T) {
	delta, err := Parse("/nonexistent/transcript.jsonl", 0)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if delta.HasContent {
		t.Error("expected empty delta for missing file")
	}
}

func TestParseExtractsCommits(t *testing.T) {
	path := writeFixture(t,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"git commit -m \"fix auth bug\""}}]}}`,
		`{"type":"tool_result","tool_use_id":"t1","content":"[main abc1234] fix auth bug\n 1 file changed","isError":false}`,
	)
	delta, err := Parse(path, 0)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(delta.MechanicalFacts.Commits) != 1 {
		t.Fatalf("Commits = %d, want 1", len(delta.MechanicalFacts.Commits))
	}
}
