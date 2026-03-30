package main

import (
	"strings"
	"testing"
)

func TestUpdateDocketSectionReplacesExisting(t *testing.T) {
	input := `# My Project

## Feature Tracking (docket)

Old snippet that needs updating.
Old line 2.

## Build

` + "```" + `
go build
` + "```" + `
`
	result := updateDocketSection(input, buildDocketSection(false))

	if !strings.Contains(result, "direct MCP calls") {
		t.Error("expected new snippet content")
	}
	if strings.Contains(result, "Old snippet") {
		t.Error("old snippet should be replaced")
	}
	if !strings.Contains(result, "## Build") {
		t.Error("sections after docket should be preserved")
	}
}

func TestUpdateDocketSectionInsertsAfterFirstHeading(t *testing.T) {
	input := `# My Project

## Build

Build instructions here.

## Test

Test instructions here.
`
	result := updateDocketSection(input, buildDocketSection(false))

	buildIdx := strings.Index(result, "## Build")
	docketIdx := strings.Index(result, "## Feature Tracking (docket)")
	testIdx := strings.Index(result, "## Test")

	if docketIdx < 0 {
		t.Fatal("docket section not inserted")
	}
	if docketIdx < buildIdx {
		t.Error("docket section should come after first heading (## Build)")
	}
	if docketIdx > testIdx {
		t.Error("docket section should come before ## Test")
	}
}

func TestUpdateDocketSectionAppendsWhenNoSecondHeading(t *testing.T) {
	input := `# My Project

## Overview

This is the only section.
`
	result := updateDocketSection(input, buildDocketSection(false))

	if !strings.Contains(result, "## Feature Tracking (docket)") {
		t.Error("docket section not inserted")
	}
	overviewIdx := strings.Index(result, "## Overview")
	docketIdx := strings.Index(result, "## Feature Tracking (docket)")
	if docketIdx < overviewIdx {
		t.Error("docket section should come after Overview")
	}
}

func TestUpdateDocketSectionAlreadyUpToDate(t *testing.T) {
	// Build content that already has the current snippet
	section := buildDocketSection(false)
	input := "# My Project\n\n" + section + "\n## Build\n"
	result := updateDocketSection(input, section)

	if result != input {
		t.Error("should not change content that's already up to date")
	}
}

func TestUpdateDocketSectionReplacesAtEOF(t *testing.T) {
	input := `# My Project

## Build

Build stuff.

## Feature Tracking (docket)

Old content at the end of file.`

	result := updateDocketSection(input, buildDocketSection(false))

	if !strings.Contains(result, "direct MCP calls") {
		t.Error("expected new snippet content")
	}
	if strings.Contains(result, "Old content at the end") {
		t.Error("old content should be replaced")
	}
	if !strings.Contains(result, "## Build") {
		t.Error("Build section should be preserved")
	}
}

func TestBuildDocketSectionWithSuperpowers(t *testing.T) {
	result := buildDocketSection(true)

	if !strings.Contains(result, "Plan execution (superpowers)") {
		t.Error("expected superpowers paragraph")
	}
	if !strings.Contains(result, "direct MCP calls") {
		t.Error("expected tail content")
	}
	if !strings.Contains(result, "## Feature Tracking (docket)") {
		t.Error("expected section heading")
	}
}

func TestBuildDocketSectionWithoutSuperpowers(t *testing.T) {
	result := buildDocketSection(false)

	if strings.Contains(result, "Plan execution (superpowers)") {
		t.Error("should not include superpowers paragraph")
	}
	if !strings.Contains(result, "direct MCP calls") {
		t.Error("expected tail content")
	}
	if !strings.Contains(result, "## Feature Tracking (docket)") {
		t.Error("expected section heading")
	}
}
