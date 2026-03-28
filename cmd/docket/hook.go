package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sniffle6/claude-docket/internal/store"
)

type hookInput struct {
	SessionID     string    `json:"session_id"`
	CWD           string    `json:"cwd"`
	HookEventName string    `json:"hook_event_name"`
	ToolName      string    `json:"tool_name"`
	ToolInput     toolInput `json:"tool_input"`
}

type toolInput struct {
	Command string `json:"command"`
}

type hookOutput struct {
	Continue      bool   `json:"continue"`
	SystemMessage string `json:"systemMessage,omitempty"`
}

func runHook() {
	var h hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&h); err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: decode stdin: %v\n", err)
		os.Exit(1)
	}

	switch h.HookEventName {
	case "SessionStart":
		handleSessionStart(&h, os.Stdout)
	case "PostToolUse":
		handlePostToolUse(&h)
	}
}

func handleSessionStart(h *hookInput, w io.Writer) {
	s, err := store.Open(h.CWD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: open store: %v\n", err)
		return
	}
	defer s.Close()

	// Create/clear commits.log
	commitsPath := filepath.Join(h.CWD, ".docket", "commits.log")
	os.WriteFile(commitsPath, []byte{}, 0644)

	features, err := s.ListFeatures("in_progress")
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: list features: %v\n", err)
		return
	}

	out := hookOutput{Continue: true}

	if len(features) == 0 {
		out.SystemMessage = "[docket] No active features. Use docket MCP tools to create one."
	} else {
		var msg strings.Builder
		msg.WriteString("[docket] Active features:\n")
		for i, f := range features {
			msg.WriteString(fmt.Sprintf("- %s (id: %s)", f.Title, f.ID))
			if f.LeftOff != "" {
				msg.WriteString(fmt.Sprintf(" — left off: %s", f.LeftOff))
			}
			msg.WriteString("\n")

			if i == 0 {
				subtasks, err := s.GetSubtasksForFeature(f.ID, false)
				if err == nil {
					for _, st := range subtasks {
						for _, item := range st.Items {
							if !item.Checked {
								msg.WriteString(fmt.Sprintf("Next task: %s\n", item.Title))
								goto done
							}
						}
					}
				}
			}
		done:
		}
		out.SystemMessage = msg.String()
	}

	json.NewEncoder(w).Encode(out)
}

func handlePostToolUse(h *hookInput) {
	if !strings.Contains(h.ToolInput.Command, "git commit") {
		return
	}

	cmd := exec.Command("git", "log", "-1", "--format=%H|||%s")
	cmd.Dir = h.CWD
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: git log: %v\n", err)
		return
	}

	commitsPath := filepath.Join(h.CWD, ".docket", "commits.log")
	f, err := os.OpenFile(commitsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: open commits.log: %v\n", err)
		return
	}
	defer f.Close()

	f.WriteString(strings.TrimSpace(string(output)) + "\n")
}
