package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildEditorCommandUsesCustomNvimConfigOnlyForNvim(t *testing.T) {
	tmpDir := t.TempDir()
	nvimConfig := filepath.Join(tmpDir, ".kb", "nvim.lua")
	if err := os.MkdirAll(filepath.Dir(nvimConfig), 0755); err != nil {
		t.Fatalf("Failed to create .kb directory: %v", err)
	}
	if err := os.WriteFile(nvimConfig, []byte("set number"), 0644); err != nil {
		t.Fatalf("Failed to write nvim config: %v", err)
	}

	nvimCmd := buildEditorCommand(&Config{Editor: "nvim"}, tmpDir, "/tmp/entry.md")
	if len(nvimCmd.Args) < 4 || nvimCmd.Args[1] != "-u" || nvimCmd.Args[2] != nvimConfig {
		t.Fatalf("expected nvim command to use custom config, got %v", nvimCmd.Args)
	}

	vimCmd := buildEditorCommand(&Config{Editor: "vim"}, tmpDir, "/tmp/entry.md")
	if len(vimCmd.Args) != 2 || vimCmd.Args[0] != "vim" {
		t.Fatalf("expected vim command to use configured editor directly, got %v", vimCmd.Args)
	}
}

func TestUpdateIndexWithEntryRespectsAutoUpdateIndex(t *testing.T) {
	tmpDir := t.TempDir()

	entry := &Entry{
		Metadata: EntryMetadata{
			Title:    "Test Entry",
			Category: "misc",
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		FilePath: filepath.Join(tmpDir, "entries", "misc", "test-entry.md"),
		ID:       "misc-test-entry",
	}

	if err := updateIndexWithEntry(&Config{AutoUpdateIndex: false}, tmpDir, entry); err != nil {
		t.Fatalf("updateIndexWithEntry failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".kb", "index.json")); !os.IsNotExist(err) {
		t.Fatalf("expected index.json to remain absent when auto_update_index=false, got err=%v", err)
	}

	if err := updateIndexWithEntry(&Config{AutoUpdateIndex: true}, tmpDir, entry); err != nil {
		t.Fatalf("updateIndexWithEntry failed with auto update enabled: %v", err)
	}

	index, err := LoadIndex(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load index: %v", err)
	}
	if len(index.Entries) != 1 {
		t.Fatalf("expected 1 entry in index, got %d", len(index.Entries))
	}
}

func TestLoadIndexedEntryCountReturnsErrorForCorruptIndex(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, ".kb", "index.json")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
		t.Fatalf("Failed to create .kb directory: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("{ invalid json "), 0644); err != nil {
		t.Fatalf("Failed to write corrupt index: %v", err)
	}

	if _, err := loadIndexedEntryCount(tmpDir); err == nil {
		t.Fatal("expected corrupt index to return an error")
	}
}
