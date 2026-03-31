package transcript

import "github.com/sniffle6/claude-docket/internal/store"

// Delta is the result of parsing a transcript range. It contains
// filtered semantic text (user/assistant only) and mechanical facts.
type Delta struct {
	SemanticText    string              // filtered user+assistant text
	MechanicalFacts store.MechanicalFacts
	EndOffset       int64 // byte offset after last processed line
	HasContent      bool  // true if any non-trivial content found
}

// trivialUserMessages are acknowledgment-only messages that don't count
// as meaningful user input.
var trivialUserMessages = map[string]bool{
	"ok":           true,
	"okay":         true,
	"thanks":       true,
	"thank you":    true,
	"continue":     true,
	"go on":        true,
	"run it":       true,
	"yep":          true,
	"yes":          true,
	"yeah":         true,
	"sure":         true,
	"go":           true,
	"do it":        true,
	"looks good":   true,
	"lgtm":         true,
	"sounds good":  true,
}
