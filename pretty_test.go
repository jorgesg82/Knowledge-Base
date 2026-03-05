package main

import (
	"strings"
	"testing"
)

func TestParsePrettyOptionsWithDryRunAndDiff(t *testing.T) {
	config := &Config{
		PrettyProvider:  "auto",
		PrettyMode:      "moderate",
		PrettyAutoApply: true,
	}

	options, err := parsePrettyOptions([]string{"entry-name", "--dry-run", "--diff"}, config)
	if err != nil {
		t.Fatalf("parsePrettyOptions failed: %v", err)
	}

	if !options.DryRun {
		t.Fatal("expected DryRun to be true")
	}
	if !options.ShowDiff {
		t.Fatal("expected ShowDiff to be true")
	}
	if options.Query != "entry-name" {
		t.Fatalf("expected query entry-name, got %s", options.Query)
	}
}

func TestBuildUnifiedDiff(t *testing.T) {
	diff, err := buildUnifiedDiff("# Old\n", "# New\n")
	if err != nil {
		t.Fatalf("buildUnifiedDiff failed: %v", err)
	}

	if !strings.Contains(diff, "--- current") || !strings.Contains(diff, "+++ proposed") {
		t.Fatalf("unexpected diff header: %s", diff)
	}
	if !strings.Contains(diff, "-# Old") || !strings.Contains(diff, "+# New") {
		t.Fatalf("unexpected diff body: %s", diff)
	}
}
