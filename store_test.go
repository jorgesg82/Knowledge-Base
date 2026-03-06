package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseEntryPrefersFrontmatterID(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "entries", "knowledge", "note.md")
	if err := os.MkdirAll(filepath.Dir(entryPath), 0755); err != nil {
		t.Fatalf("failed to create entry dir: %v", err)
	}

	content := `---
id: note_123
title: Stable Note
category: knowledge
created: 2026-03-06T00:00:00Z
updated: 2026-03-06T00:00:00Z
---

# Stable Note

Body.
`
	if err := os.WriteFile(entryPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write entry: %v", err)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("failed to parse entry: %v", err)
	}

	if entry.ID != "note_123" {
		t.Fatalf("expected stable note ID note_123, got %s", entry.ID)
	}
}

func TestMaterializeCanonicalNoteAndSyncFromEntry(t *testing.T) {
	tmpDir := t.TempDir()
	note := &CanonicalNote{
		ID:               "note_123",
		Title:            "Inspect Open Ports on macOS",
		Aliases:          []string{"open ports mac"},
		Summary:          "Check listening TCP ports.",
		Body:             "Use `lsof -iTCP -sTCP:LISTEN`.",
		Topics:           []string{"macos", "networking"},
		SourceCaptureIDs: []string{"cap_123"},
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

	absPath := filepath.Join(tmpDir, note.MaterializedPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("failed to read materialized note: %v", err)
	}
	if !strings.Contains(string(data), "id: note_123") {
		t.Fatalf("expected materialized note to contain stable ID, got: %s", string(data))
	}

	entry, err := ParseEntry(absPath)
	if err != nil {
		t.Fatalf("failed to parse materialized note: %v", err)
	}
	entry.Metadata.Title = "Inspect Listening Ports on macOS"
	entry.Metadata.Summary = "Updated summary."
	entry.Content = "# Inspect Listening Ports on macOS\n\nUpdated body.\n"
	if err := WriteEntry(entry, absPath); err != nil {
		t.Fatalf("failed to rewrite materialized note: %v", err)
	}

	updatedEntry, err := ParseEntry(absPath)
	if err != nil {
		t.Fatalf("failed to parse rewritten note: %v", err)
	}
	if err := syncCanonicalNoteFromEntry(tmpDir, updatedEntry); err != nil {
		t.Fatalf("failed to sync canonical note: %v", err)
	}

	notes, err := loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to reload notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected one note, got %d", len(notes))
	}
	if notes[0].Title != "Inspect Listening Ports on macOS" {
		t.Fatalf("expected synced title, got %s", notes[0].Title)
	}
	if !strings.Contains(notes[0].Body, "Updated body.") {
		t.Fatalf("expected synced body, got %s", notes[0].Body)
	}
}

func TestSaveCanonicalNoteRecordWritesLightweightManifest(t *testing.T) {
	tmpDir := t.TempDir()
	note := &CanonicalNote{
		ID:               "note_456",
		Title:            "Lightweight Manifest",
		Summary:          "Manifest should not duplicate body content.",
		Body:             "Very large body that should stay out of the manifest.",
		Topics:           []string{"testing"},
		SourceCaptureIDs: []string{"cap_456"},
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

	manifestData, err := os.ReadFile(filepath.Join(tmpDir, ".kb", notesManifestFileName))
	if err != nil {
		t.Fatalf("failed to read notes manifest: %v", err)
	}

	if strings.Contains(string(manifestData), "\"body\"") {
		t.Fatalf("expected notes manifest to omit note body, got: %s", manifestData)
	}

	var manifest []CanonicalNoteManifestEntry
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("failed to parse notes manifest: %v", err)
	}
	if len(manifest) != 1 {
		t.Fatalf("expected one manifest entry, got %d", len(manifest))
	}
	if manifest[0].Title != note.Title {
		t.Fatalf("expected manifest title %q, got %q", note.Title, manifest[0].Title)
	}
}
