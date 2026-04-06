package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/sniffle6/claude-docket/internal/store"
)

// Worker drains checkpoint_jobs and calls the summarizer for each.
type Worker struct {
	store      *store.Store
	summarizer SummarizerBackend
	Wake       chan struct{} // signal to wake immediately when a job is enqueued
}

func NewWorker(s *store.Store, summarizer SummarizerBackend) *Worker {
	return &Worker{store: s, summarizer: summarizer, Wake: make(chan struct{}, 1)}
}

// Notify wakes the worker to check for new jobs immediately.
func (w *Worker) Notify() {
	select {
	case w.Wake <- struct{}{}:
	default: // already pending
	}
}

// Run polls for queued jobs and processes them until ctx is cancelled.
func (w *Worker) Run(ctx context.Context, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for w.ProcessOne() {
			}
		case <-w.Wake:
			for w.ProcessOne() {
			}
		}
	}
}

// ProcessOne dequeues and processes a single checkpoint job.
// Returns true if a job was processed, false if the queue was empty.
func (w *Worker) ProcessOne() bool {
	job, err := w.store.DequeueCheckpointJob()
	if err != nil || job == nil {
		return false
	}

	// Get feature title for context
	featureTitle := job.FeatureID
	if f, err := w.store.GetFeature(job.FeatureID); err == nil {
		featureTitle = f.Title
	}

	// If semantic text is empty, skip LLM call but still mark done
	if job.SemanticText == "" {
		w.store.CompleteCheckpointJob(job.ID, nil)
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := w.summarizer.Summarize(ctx, SummarizeInput{
		SemanticText: job.SemanticText,
		FeatureTitle: featureTitle,
		Reason:       job.Reason,
	})
	if err != nil {
		errMsg := err.Error()
		w.store.FailCheckpointJob(job.ID, errMsg)
		fmt.Fprintf(os.Stderr, "docket checkpoint worker: summarize job %d: %v\n", job.ID, err)
		return true
	}

	// Write observations
	w.writeObservations(job, output)
	w.store.CompleteCheckpointJob(job.ID, nil)

	// Auto-merge key_files from mechanical facts
	w.mergeKeyFiles(job)

	// Run feature synthesis after session_end checkpoint
	if job.Reason == "session_end" {
		w.synthesizeFeature(job.FeatureID)
	}

	return true
}

func (w *Worker) writeObservations(job *store.CheckpointJob, output *SummarizeOutput) {
	if output.Summary != "" {
		payload, _ := json.Marshal(output)
		w.store.AddCheckpointObservation(store.CheckpointObservationInput{
			CheckpointJobID: job.ID,
			WorkSessionID:   job.WorkSessionID,
			FeatureID:       job.FeatureID,
			Kind:            "summary",
			PayloadJSON:     string(payload),
			SummaryText:     output.Summary,
		})
	}
	for _, b := range output.Blockers {
		w.store.AddCheckpointObservation(store.CheckpointObservationInput{
			CheckpointJobID: job.ID, WorkSessionID: job.WorkSessionID,
			FeatureID: job.FeatureID, Kind: "blocker", SummaryText: b,
		})
	}
	for _, d := range output.DeadEnds {
		w.store.AddCheckpointObservation(store.CheckpointObservationInput{
			CheckpointJobID: job.ID, WorkSessionID: job.WorkSessionID,
			FeatureID: job.FeatureID, Kind: "dead_end", SummaryText: d,
		})
	}
	for _, n := range output.NextSteps {
		w.store.AddCheckpointObservation(store.CheckpointObservationInput{
			CheckpointJobID: job.ID, WorkSessionID: job.WorkSessionID,
			FeatureID: job.FeatureID, Kind: "next_step", SummaryText: n,
		})
	}
	for _, d := range output.Decisions {
		w.store.AddCheckpointObservation(store.CheckpointObservationInput{
			CheckpointJobID: job.ID, WorkSessionID: job.WorkSessionID,
			FeatureID: job.FeatureID, Kind: "decision_candidate", SummaryText: d,
		})
	}
	for _, g := range output.Gotchas {
		w.store.AddCheckpointObservation(store.CheckpointObservationInput{
			CheckpointJobID: job.ID, WorkSessionID: job.WorkSessionID,
			FeatureID: job.FeatureID, Kind: "gotcha", SummaryText: g,
		})
	}
}

func (w *Worker) mergeKeyFiles(job *store.CheckpointJob) {
	var facts store.MechanicalFacts
	if err := json.Unmarshal([]byte(job.MechanicalJSON), &facts); err != nil || len(facts.FilesEdited) == 0 {
		return
	}

	feature, err := w.store.GetFeature(job.FeatureID)
	if err != nil {
		return
	}

	existing := make(map[string]bool, len(feature.KeyFiles))
	for _, f := range feature.KeyFiles {
		existing[f] = true
	}

	merged := append([]string{}, feature.KeyFiles...)
	changed := false
	for _, fe := range facts.FilesEdited {
		if !existing[fe.Path] {
			merged = append(merged, fe.Path)
			existing[fe.Path] = true
			changed = true
		}
	}

	if changed {
		w.store.UpdateFeature(job.FeatureID, store.FeatureUpdate{KeyFiles: &merged})
	}
}

func (w *Worker) synthesizeFeature(featureID string) {
	feature, err := w.store.GetFeature(featureID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket synthesis: get feature %q: %v\n", featureID, err)
		return
	}

	obs, err := w.store.GetObservationsForFeature(featureID, 50)
	if err != nil || len(obs) == 0 {
		return
	}

	// Check staleness: if no new observations since last synthesis, skip
	if obs[0].ID <= feature.SynthesisObsID {
		return
	}

	var entries []ObservationEntry
	for _, o := range obs {
		entries = append(entries, ObservationEntry{Kind: o.Kind, SummaryText: o.SummaryText})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := w.summarizer.Synthesize(ctx, SynthesizeInput{
		FeatureTitle: feature.Title,
		Observations: entries,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket synthesis: synthesize %q: %v\n", featureID, err)
		return
	}

	if err := w.store.UpdateFeatureSynthesis(featureID, output.Text, obs[0].ID); err != nil {
		fmt.Fprintf(os.Stderr, "docket synthesis: update %q: %v\n", featureID, err)
		return
	}
	fmt.Fprintf(os.Stderr, "docket synthesis: updated %q (%d observations)\n", featureID, len(obs))
}
