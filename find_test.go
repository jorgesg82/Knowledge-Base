package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseFindOptionsWithFlagsAndMultiWordQuery(t *testing.T) {
	config := &Config{AIProvider: "auto"}

	options, err := parseFindOptions([]string{"--json", "--provider", "chatgpt", "open", "ports"}, config)
	if err != nil {
		t.Fatalf("parseFindOptions failed: %v", err)
	}

	if !options.JSON {
		t.Fatal("expected JSON option to be true")
	}
	if options.Provider != ProviderChatGPT {
		t.Fatalf("expected provider chatgpt, got %s", options.Provider)
	}
	if options.Query != "open ports" {
		t.Fatalf("unexpected query: %s", options.Query)
	}
}

func TestParseFindOptionsAllowsBrowseWithoutQuery(t *testing.T) {
	config := &Config{AIProvider: "auto"}

	options, err := parseFindOptions(nil, config)
	if err != nil {
		t.Fatalf("parseFindOptions failed: %v", err)
	}

	if options.Query != "" {
		t.Fatalf("expected empty query for browse mode, got %q", options.Query)
	}
}

func TestSelectBestCanonicalFindCandidatePrefersExactAlias(t *testing.T) {
	candidates := rankCanonicalNotesForFind([]*CanonicalNote{
		{
			ID:      "note_1",
			Title:   "Inspect Open Ports on macOS",
			Aliases: []string{"open ports mac"},
			Summary: "Check listening TCP ports.",
			Body:    "Use lsof -iTCP -sTCP:LISTEN.",
		},
		{
			ID:      "note_2",
			Title:   "SSH Tunnels",
			Summary: "Forward local ports over SSH.",
			Body:    "ssh -L ...",
		},
	}, "open ports mac")

	selected, confident := selectBestCanonicalFindCandidate(candidates)
	if selected == nil {
		t.Fatal("expected selected candidate")
	}
	if !confident {
		t.Fatal("expected alias match to be high confidence")
	}
	if selected.ID != "note_1" {
		t.Fatalf("expected note_1, got %s", selected.ID)
	}
}

func TestSelectBestCanonicalFindCandidatePrefersSlugLikeTitleQuery(t *testing.T) {
	candidates := rankCanonicalNotesForFind([]*CanonicalNote{
		{
			ID:               "note_1",
			Title:            "Viewer Test",
			MaterializedPath: "entries/viewer-test-ab12cd34.md",
			Body:             "# Viewer Test\n\nBody.",
		},
	}, "viewer-test")

	selected, confident := selectBestCanonicalFindCandidate(candidates)
	if selected == nil {
		t.Fatal("expected selected candidate")
	}
	if !confident {
		t.Fatal("expected slug-like query to be high confidence")
	}
	if selected.ID != "note_1" {
		t.Fatalf("expected note_1, got %s", selected.ID)
	}
}

func TestBuildBrowseCandidatesPrefersRecentlyUpdatedNotes(t *testing.T) {
	now := time.Now().UTC()
	candidates, topics := buildBrowseCandidates([]*CanonicalNote{
		{
			ID:        "note_older",
			Title:     "Older Note",
			Topics:    []string{"networking"},
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        "note_newer",
			Title:     "Newer Note",
			Topics:    []string{"networking"},
			CreatedAt: now.Add(-1 * time.Hour),
			UpdatedAt: now,
		},
	})

	if len(candidates) != 2 {
		t.Fatalf("expected 2 browse candidates, got %d", len(candidates))
	}
	if candidates[0].note.ID != "note_newer" {
		t.Fatalf("expected most recently updated note first, got %s", candidates[0].note.ID)
	}
	if len(topics) != 1 || topics[0].Topic != "networking" || topics[0].Count != 2 {
		t.Fatalf("unexpected browse topics: %#v", topics)
	}
}

func TestBrowsePageBoundsSlicesAllCandidates(t *testing.T) {
	start, end := browsePageBounds(25, 0, 12)
	if start != 0 || end != 12 {
		t.Fatalf("unexpected first page bounds: %d %d", start, end)
	}

	start, end = browsePageBounds(25, 1, 12)
	if start != 12 || end != 24 {
		t.Fatalf("unexpected second page bounds: %d %d", start, end)
	}

	start, end = browsePageBounds(25, 2, 12)
	if start != 24 || end != 25 {
		t.Fatalf("unexpected last page bounds: %d %d", start, end)
	}
}

func TestParseBrowseInputSupportsPagingAndSelection(t *testing.T) {
	action, selectedIndex, nextPage, err := parseBrowseInput("n", 0, 25, 12)
	if err != nil {
		t.Fatalf("parseBrowseInput next failed: %v", err)
	}
	if action != browseActionNext || selectedIndex != 0 || nextPage != 1 {
		t.Fatalf("unexpected next action: action=%v index=%d page=%d", action, selectedIndex, nextPage)
	}

	action, selectedIndex, nextPage, err = parseBrowseInput("p", 2, 25, 12)
	if err != nil {
		t.Fatalf("parseBrowseInput prev failed: %v", err)
	}
	if action != browseActionPrev || nextPage != 1 {
		t.Fatalf("unexpected prev action: action=%v index=%d page=%d", action, selectedIndex, nextPage)
	}

	action, selectedIndex, nextPage, err = parseBrowseInput("13", 1, 25, 12)
	if err != nil {
		t.Fatalf("parseBrowseInput selection failed: %v", err)
	}
	if action != browseActionSelect || selectedIndex != 12 || nextPage != 1 {
		t.Fatalf("unexpected select action: action=%v index=%d page=%d", action, selectedIndex, nextPage)
	}
}

func TestParseBrowseInputRejectsInvalidSelection(t *testing.T) {
	_, _, _, err := parseBrowseInput("99", 0, 25, 12)
	if err == nil {
		t.Fatal("expected out-of-range selection error")
	}

	_, _, _, err = parseBrowseInput("wat", 0, 25, 12)
	if err == nil {
		t.Fatal("expected invalid command error")
	}
}

func TestEnsureCanonicalNoteMaterializedReloadsFullNoteRecord(t *testing.T) {
	tmpDir := t.TempDir()
	note := &CanonicalNote{
		ID:               "note_full",
		Title:            "Recovered Note",
		Summary:          "Recovered from note store.",
		Body:             "Recovered body content.",
		Topics:           []string{"testing"},
		SourceCaptureIDs: []string{"cap_1"},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		Revision:         1,
	}

	if err := ensureStoreLayout(tmpDir); err != nil {
		t.Fatalf("failed to create store layout: %v", err)
	}
	if err := materializeCanonicalNote(tmpDir, note); err != nil {
		t.Fatalf("failed to materialize note: %v", err)
	}
	if err := saveCanonicalNoteRecord(tmpDir, note); err != nil {
		t.Fatalf("failed to save note record: %v", err)
	}
	if err := os.Remove(filepath.Join(tmpDir, note.MaterializedPath)); err != nil {
		t.Fatalf("failed to remove materialized note: %v", err)
	}

	stub := &CanonicalNote{
		ID:               note.ID,
		Title:            note.Title,
		MaterializedPath: note.MaterializedPath,
	}
	if err := ensureCanonicalNoteMaterialized(tmpDir, stub); err != nil {
		t.Fatalf("failed to re-materialize note: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, stub.MaterializedPath))
	if err != nil {
		t.Fatalf("failed to read rebuilt materialized note: %v", err)
	}
	if !strings.Contains(string(content), "Recovered body content.") {
		t.Fatalf("expected rebuilt note to use stored body, got: %s", content)
	}
}
