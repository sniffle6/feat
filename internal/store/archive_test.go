package store

import "testing"

func TestArchivedStatus(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Old Feature", "")
	archived := "archived"
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &archived})
	if err != nil {
		t.Fatalf("set archived: %v", err)
	}
	f, _ = s.GetFeature(f.ID)
	if f.Status != "archived" {
		t.Fatalf("expected archived, got %q", f.Status)
	}
}

func TestListFeaturesExcludesArchived(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Active", "")
	s.AddFeature("Old", "")
	archived := "archived"
	s.UpdateFeature("old", FeatureUpdate{Status: &archived})

	features, _ := s.ListFeatures("")
	if len(features) != 1 || features[0].ID != "active" {
		t.Fatalf("expected only active feature, got %v", features)
	}
}

func TestListFeaturesShowArchived(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	s.AddFeature("Old", "")
	archived := "archived"
	s.UpdateFeature("old", FeatureUpdate{Status: &archived})

	features, _ := s.ListFeatures("archived")
	if len(features) != 1 || features[0].ID != "old" {
		t.Fatalf("expected archived feature, got %v", features)
	}
}

func TestCompletionGateBypassForArchived(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()

	f, _ := s.AddFeature("Archivable", "")
	s.ApplyTemplate(f.ID, "bugfix") // creates unchecked items

	archived := "archived"
	err := s.UpdateFeature(f.ID, FeatureUpdate{Status: &archived})
	if err != nil {
		t.Fatalf("archiving with unchecked items should work, got: %v", err)
	}
	f, _ = s.GetFeature(f.ID)
	if f.Status != "archived" {
		t.Fatalf("expected archived, got %q", f.Status)
	}
}
