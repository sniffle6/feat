package store

import "testing"

func TestAddIssue(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Test Feature", "desc")

	issue, err := s.AddIssue("test-feature", "Button is broken", nil)
	if err != nil {
		t.Fatalf("AddIssue: %v", err)
	}
	if issue.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if issue.FeatureID != "test-feature" {
		t.Errorf("FeatureID = %q, want %q", issue.FeatureID, "test-feature")
	}
	if issue.Description != "Button is broken" {
		t.Errorf("Description = %q, want %q", issue.Description, "Button is broken")
	}
	if issue.Status != "open" {
		t.Errorf("Status = %q, want %q", issue.Status, "open")
	}
	if issue.TaskItemID != nil {
		t.Errorf("TaskItemID = %v, want nil", issue.TaskItemID)
	}
}

func TestAddIssueWithTaskItem(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Test Feature", "desc")
	st, _ := s.AddSubtask("test-feature", "Phase 1", 1)
	item, _ := s.AddTaskItem(st.ID, "Do thing", 1)

	taskItemID := item.ID
	issue, err := s.AddIssue("test-feature", "Thing is buggy", &taskItemID)
	if err != nil {
		t.Fatalf("AddIssue: %v", err)
	}
	if issue.TaskItemID == nil || *issue.TaskItemID != taskItemID {
		t.Errorf("TaskItemID = %v, want %d", issue.TaskItemID, taskItemID)
	}
}

func TestResolveIssue(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Test Feature", "desc")

	issue, _ := s.AddIssue("test-feature", "Bug found", nil)

	err := s.ResolveIssue(issue.ID, "abc123")
	if err != nil {
		t.Fatalf("ResolveIssue: %v", err)
	}

	resolved, _ := s.getIssue(issue.ID)
	if resolved.Status != "resolved" {
		t.Errorf("Status = %q, want %q", resolved.Status, "resolved")
	}
	if resolved.ResolvedCommit != "abc123" {
		t.Errorf("ResolvedCommit = %q, want %q", resolved.ResolvedCommit, "abc123")
	}
	if resolved.ResolvedAt == nil {
		t.Error("ResolvedAt should be set")
	}
}

func TestResolveIssueNoCommit(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Test Feature", "desc")

	issue, _ := s.AddIssue("test-feature", "Minor bug", nil)

	err := s.ResolveIssue(issue.ID, "")
	if err != nil {
		t.Fatalf("ResolveIssue: %v", err)
	}

	resolved, _ := s.getIssue(issue.ID)
	if resolved.Status != "resolved" {
		t.Errorf("Status = %q, want %q", resolved.Status, "resolved")
	}
	if resolved.ResolvedCommit != "" {
		t.Errorf("ResolvedCommit = %q, want empty", resolved.ResolvedCommit)
	}
}

func TestGetIssuesForFeature(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Test Feature", "desc")
	s.AddIssue("test-feature", "Bug 1", nil)
	s.AddIssue("test-feature", "Bug 2", nil)
	issue3, _ := s.AddIssue("test-feature", "Bug 3", nil)
	s.ResolveIssue(issue3.ID, "fix123")

	issues, err := s.GetIssuesForFeature("test-feature")
	if err != nil {
		t.Fatalf("GetIssuesForFeature: %v", err)
	}
	if len(issues) != 3 {
		t.Fatalf("len = %d, want 3", len(issues))
	}
	// Open issues first (newest first), then resolved
	if issues[0].Description != "Bug 2" {
		t.Errorf("first issue = %q, want %q", issues[0].Description, "Bug 2")
	}
	if issues[1].Description != "Bug 1" {
		t.Errorf("second issue = %q, want %q", issues[1].Description, "Bug 1")
	}
	if issues[2].Status != "resolved" {
		t.Errorf("third issue status = %q, want %q", issues[2].Status, "resolved")
	}
}

func TestGetOpenIssueCount(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Test Feature", "desc")
	s.AddIssue("test-feature", "Bug 1", nil)
	s.AddIssue("test-feature", "Bug 2", nil)
	issue3, _ := s.AddIssue("test-feature", "Bug 3", nil)
	s.ResolveIssue(issue3.ID, "")

	count, err := s.GetOpenIssueCount("test-feature")
	if err != nil {
		t.Fatalf("GetOpenIssueCount: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestGetAllOpenIssues(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "desc")
	s.AddFeature("Feature B", "desc")
	s.AddIssue("feature-a", "Bug in A", nil)
	s.AddIssue("feature-b", "Bug in B", nil)
	resolved, _ := s.AddIssue("feature-a", "Fixed bug", nil)
	s.ResolveIssue(resolved.ID, "")

	issues, err := s.GetAllOpenIssues()
	if err != nil {
		t.Fatalf("GetAllOpenIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Errorf("len = %d, want 2", len(issues))
	}
}
