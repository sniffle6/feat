package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/sniffle6/claude-docket/internal/store"
)

func runExport() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: docket export <feature_id> [--file <path>]")
		os.Exit(1)
	}

	featureID := os.Args[2]
	var outFile string
	for i := 3; i < len(os.Args); i++ {
		if os.Args[i] == "--file" && i+1 < len(os.Args) {
			outFile = os.Args[i+1]
			i++
		}
	}

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	s, err := store.Open(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening store: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	md, err := renderExport(s, featureID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if outFile != "" {
		if err := os.WriteFile(outFile, []byte(md), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Exported to %s\n", outFile)
	} else {
		fmt.Print(md)
	}
}

func renderExport(s *store.Store, featureID string) (string, error) {
	f, err := s.GetFeature(featureID)
	if err != nil {
		return "", fmt.Errorf("feature %q not found", featureID)
	}

	done, total, _ := s.GetFeatureProgress(featureID)
	subtasks, _ := s.GetSubtasksForFeature(featureID, false)
	decisions, _ := s.GetDecisionsForFeature(featureID)
	sessions, _ := s.GetSessionsForFeature(featureID)

	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "# %s\n\n", f.Title)
	fmt.Fprintf(&b, "**Status:** %s | **Progress:** %d/%d | **Updated:** %s\n\n",
		f.Status, done, total, f.UpdatedAt.Format("2006-01-02 15:04"))

	if f.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", f.Description)
	}

	if f.LeftOff != "" {
		fmt.Fprintf(&b, "## Left Off\n\n%s\n\n", f.LeftOff)
	}

	// Key files
	if len(f.KeyFiles) > 0 {
		b.WriteString("## Key Files\n\n")
		for _, kf := range f.KeyFiles {
			fmt.Fprintf(&b, "- `%s`\n", kf)
		}
		b.WriteString("\n")
	}

	// Subtasks with task items
	if len(subtasks) > 0 {
		b.WriteString("## Subtasks\n\n")
		for _, st := range subtasks {
			fmt.Fprintf(&b, "### %s\n\n", st.Title)
			for _, item := range st.Items {
				check := " "
				if item.Checked {
					check = "x"
				}
				fmt.Fprintf(&b, "- [%s] %s", check, item.Title)
				if item.CommitHash != "" {
					fmt.Fprintf(&b, " (`%s`)", item.CommitHash)
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	// Decisions
	if len(decisions) > 0 {
		b.WriteString("## Decisions\n\n")
		for _, d := range decisions {
			fmt.Fprintf(&b, "- **%s** → %s", d.Approach, d.Outcome)
			if d.Reason != "" {
				fmt.Fprintf(&b, " _%s_", d.Reason)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Sessions
	if len(sessions) > 0 {
		b.WriteString("## Sessions\n\n")
		for _, sess := range sessions {
			line := fmt.Sprintf("- **%s**: %s", sess.CreatedAt.Format("2006-01-02 15:04"), sess.Summary)
			if len(sess.Commits) > 0 {
				line += fmt.Sprintf(" [%s]", strings.Join(sess.Commits, ", "))
			}
			fmt.Fprintf(&b, "%s\n", line)
		}
		b.WriteString("\n")
	}

	if f.Notes != "" {
		fmt.Fprintf(&b, "## Notes\n\n%s\n", f.Notes)
	}

	return b.String(), nil
}
