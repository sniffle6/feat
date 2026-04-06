package handoff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sniffle6/claude-docket/internal/store"
)

// CheckpointData holds checkpoint observations and mechanical facts for
// inclusion in a handoff file's "Last Session" section.
type CheckpointData struct {
	Observations    []store.CheckpointObservation
	MechanicalFacts *store.MechanicalFacts
}

// Render produces the markdown text for a handoff file.
func Render(data *store.HandoffData, cpData *CheckpointData) string {
	var b strings.Builder
	f := data.Feature

	fmt.Fprintf(&b, "# Handoff: %s\n\n", f.Title)

	fmt.Fprintf(&b, "## Status\n")
	fmt.Fprintf(&b, "%s | Progress: %d/%d | Updated: %s\n\n",
		f.Status, data.Done, data.Total, f.UpdatedAt.Format("2006-01-02 15:04"))

	if f.SpecPath != "" {
		fmt.Fprintf(&b, "Spec: %s\n", f.SpecPath)
	}
	if f.PlanPath != "" {
		fmt.Fprintf(&b, "Plan: %s\n", f.PlanPath)
	}
	if f.SpecPath != "" || f.PlanPath != "" {
		b.WriteString("\n")
	}

	if f.LeftOff != "" {
		fmt.Fprintf(&b, "## Left Off\n%s\n\n", f.LeftOff)
	}

	if f.Synthesis != "" {
		fmt.Fprintf(&b, "## Synthesis\n%s\n\n", f.Synthesis)
	}

	// Last Session section from checkpoint observations
	if cpData != nil && (len(cpData.Observations) > 0 || cpData.MechanicalFacts != nil) {
		b.WriteString("## Last Session\n")

		for _, obs := range cpData.Observations {
			switch obs.Kind {
			case "summary":
				fmt.Fprintf(&b, "%s\n\n", obs.SummaryText)
			case "blocker":
				fmt.Fprintf(&b, "- **Blocker:** %s\n", obs.SummaryText)
			case "dead_end":
				fmt.Fprintf(&b, "- **Dead end:** %s\n", obs.SummaryText)
			case "next_step":
				fmt.Fprintf(&b, "- **Next:** %s\n", obs.SummaryText)
			case "decision_candidate":
				fmt.Fprintf(&b, "- **Decision:** %s\n", obs.SummaryText)
			case "gotcha":
				fmt.Fprintf(&b, "- **Gotcha:** %s\n", obs.SummaryText)
			}
		}

		if cpData.MechanicalFacts != nil {
			mf := cpData.MechanicalFacts
			if len(mf.FilesEdited) > 0 {
				var parts []string
				for _, fe := range mf.FilesEdited {
					if fe.Count > 1 {
						parts = append(parts, fmt.Sprintf("%s (%d×)", fe.Path, fe.Count))
					} else {
						parts = append(parts, fe.Path)
					}
				}
				fmt.Fprintf(&b, "\nFiles: %s\n", strings.Join(parts, ", "))
			}
			if len(mf.TestRuns) > 0 {
				passed := 0
				for _, tr := range mf.TestRuns {
					if tr.Passed {
						passed++
					}
				}
				fmt.Fprintf(&b, "Tests: %d runs (%d passed, %d failed)\n",
					len(mf.TestRuns), passed, len(mf.TestRuns)-passed)
			}
			if len(mf.Commits) > 0 {
				var msgs []string
				for _, c := range mf.Commits {
					msgs = append(msgs, fmt.Sprintf("%q", c.Message))
				}
				fmt.Fprintf(&b, "Commits: %s\n", strings.Join(msgs, ", "))
			}
		}
		b.WriteString("\n")
	}

	if len(data.NextTasks) > 0 {
		b.WriteString("## Next Tasks\n")
		for _, task := range data.NextTasks {
			fmt.Fprintf(&b, "- [ ] %s\n", task)
		}
		b.WriteString("\n")
	}

	if len(f.KeyFiles) > 0 {
		b.WriteString("## Key Files\n")
		for _, kf := range f.KeyFiles {
			fmt.Fprintf(&b, "- %s\n", kf)
		}
		b.WriteString("\n")
	}

	if len(data.RecentSessions) > 0 {
		b.WriteString("## Recent Activity\n")
		for _, sess := range data.RecentSessions {
			line := fmt.Sprintf("- %s: %s", sess.CreatedAt.Format("2006-01-02"), sess.Summary)
			if len(sess.Commits) > 0 {
				line += fmt.Sprintf(" [%s]", strings.Join(sess.Commits, ", "))
			}
			fmt.Fprintf(&b, "%s\n", line)
		}
		b.WriteString("\n")
	}

	if len(data.SubtaskSummary) > 0 {
		b.WriteString("## Active Subtasks\n")
		for _, st := range data.SubtaskSummary {
			fmt.Fprintf(&b, "- %s [%d/%d]\n", st.Title, st.Done, st.Total)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// enrichmentHeadings are sections written by board-manager that should be
// preserved when the Stop hook rewrites the mechanical baseline.
var enrichmentHeadings = []string{
	"## Decisions & Context",
	"## Gotchas",
	"## Recommended Approach",
}

// WriteFile writes a handoff markdown file to .docket/handoff/<featureID>.md,
// preserving any enrichment sections from a previous version of the file.
func WriteFile(dir string, data *store.HandoffData, cpData *CheckpointData) error {
	handoffDir := filepath.Join(dir, ".docket", "handoff")
	if err := os.MkdirAll(handoffDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(handoffDir, data.Feature.ID+".md")

	baseline := Render(data, cpData)

	// Preserve enrichment sections from board-manager if they exist
	if existing, err := os.ReadFile(path); err == nil {
		enrichment := ExtractEnrichmentSections(string(existing))
		if enrichment != "" {
			baseline = strings.TrimRight(baseline, "\n") + "\n" + enrichment
		}
	}

	return os.WriteFile(path, []byte(baseline), 0644)
}

// ExtractEnrichmentSections pulls board-manager-written sections from an
// existing handoff file. Returns the combined text or "" if none found.
func ExtractEnrichmentSections(content string) string {
	var sections []string
	for _, heading := range enrichmentHeadings {
		idx := strings.Index(content, heading)
		if idx < 0 {
			continue
		}
		section := content[idx:]
		// Find end: next ## heading that isn't one of our enrichment headings,
		// or EOF
		rest := section[len(heading):]
		endIdx := strings.Index(rest, "\n## ")
		if endIdx >= 0 {
			// Check if the next heading is another enrichment heading
			nextSection := rest[endIdx+1:]
			isEnrichment := false
			for _, eh := range enrichmentHeadings {
				if strings.HasPrefix(nextSection, eh) {
					isEnrichment = true
					break
				}
			}
			if !isEnrichment {
				section = section[:len(heading)+endIdx+1]
			}
		}
		sections = append(sections, strings.TrimRight(section, "\n"))
	}
	if len(sections) == 0 {
		return ""
	}
	return "\n" + strings.Join(sections, "\n\n") + "\n"
}

// CleanStale removes handoff files for features not in the activeIDs set.
func CleanStale(dir string, activeIDs map[string]bool) {
	handoffDir := filepath.Join(dir, ".docket", "handoff")
	entries, err := os.ReadDir(handoffDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		if !activeIDs[name] {
			os.Remove(filepath.Join(handoffDir, e.Name()))
		}
	}
}
