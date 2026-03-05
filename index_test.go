package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAndSaveIndex(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Entries: []IndexEntry{
			{
				ID:       "test-entry-1",
				Path:     "entries/test/entry-1.md",
				Title:    "Entry 1",
				Category: "test",
				Tags:     []string{"tag1", "tag2"},
				Created:  time.Now().Round(time.Second),
				Updated:  time.Now().Round(time.Second),
			},
			{
				ID:       "test-entry-2",
				Path:     "entries/test/entry-2.md",
				Title:    "Entry 2",
				Category: "test",
				Tags:     []string{"tag3"},
				Created:  time.Now().Round(time.Second),
				Updated:  time.Now().Round(time.Second),
			},
		},
		LastUpdated: time.Now(),
	}

	err := SaveIndex(index, tmpDir)
	if err != nil {
		t.Fatalf("Failed to save index: %v", err)
	}

	loaded, err := LoadIndex(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load index: %v", err)
	}

	if len(loaded.Entries) != len(index.Entries) {
		t.Errorf("Entries count mismatch: expected %d, got %d", len(index.Entries), len(loaded.Entries))
	}

	for i, entry := range index.Entries {
		if loaded.Entries[i].ID != entry.ID {
			t.Errorf("Entry %d ID mismatch: expected %s, got %s", i, entry.ID, loaded.Entries[i].ID)
		}
		if loaded.Entries[i].Title != entry.Title {
			t.Errorf("Entry %d Title mismatch: expected %s, got %s", i, entry.Title, loaded.Entries[i].Title)
		}
	}
}

func TestLoadIndexNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	index, err := LoadIndex(tmpDir)
	if err != nil {
		t.Fatalf("Expected no error for non-existent index, got: %v", err)
	}

	if len(index.Entries) != 0 {
		t.Errorf("Expected empty index, got %d entries", len(index.Entries))
	}
}

func TestAddToIndex(t *testing.T) {
	tmpDir := t.TempDir()
	index := &Index{Entries: []IndexEntry{}}

	entry := &Entry{
		Metadata: EntryMetadata{
			Title:    "Test Entry",
			Category: "test",
			Tags:     []string{"tag1"},
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		FilePath: filepath.Join(tmpDir, "entries", "test", "entry.md"),
		ID:       "test-test-entry",
	}

	AddToIndex(index, entry, tmpDir)

	if len(index.Entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(index.Entries))
	}

	indexEntry := index.Entries[0]
	if indexEntry.ID != entry.ID {
		t.Errorf("ID mismatch: expected %s, got %s", entry.ID, indexEntry.ID)
	}

	if indexEntry.Title != entry.Metadata.Title {
		t.Errorf("Title mismatch: expected %s, got %s", entry.Metadata.Title, indexEntry.Title)
	}

	if indexEntry.Category != entry.Metadata.Category {
		t.Errorf("Category mismatch: expected %s, got %s", entry.Metadata.Category, indexEntry.Category)
	}
}

func TestAddToIndexUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	index := &Index{
		Entries: []IndexEntry{
			{
				ID:       "test-entry",
				Path:     "entries/test/entry.md",
				Title:    "Old Title",
				Category: "test",
				Tags:     []string{"old"},
			},
		},
	}

	updatedEntry := &Entry{
		Metadata: EntryMetadata{
			Title:    "New Title",
			Category: "test",
			Tags:     []string{"new"},
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		FilePath: filepath.Join(tmpDir, "entries", "test", "entry.md"),
		ID:       "test-entry",
	}

	AddToIndex(index, updatedEntry, tmpDir)

	if len(index.Entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(index.Entries))
	}

	if index.Entries[0].Title != "New Title" {
		t.Errorf("Title not updated: expected 'New Title', got %s", index.Entries[0].Title)
	}

	if len(index.Entries[0].Tags) != 1 || index.Entries[0].Tags[0] != "new" {
		t.Errorf("Tags not updated: got %v", index.Entries[0].Tags)
	}
}

func TestRemoveFromIndex(t *testing.T) {
	index := &Index{
		Entries: []IndexEntry{
			{ID: "entry-1", Title: "Entry 1"},
			{ID: "entry-2", Title: "Entry 2"},
			{ID: "entry-3", Title: "Entry 3"},
		},
	}

	removed := RemoveFromIndex(index, "entry-2")
	if !removed {
		t.Error("Expected RemoveFromIndex to return true")
	}

	if len(index.Entries) != 2 {
		t.Errorf("Expected 2 entries after removal, got %d", len(index.Entries))
	}

	for _, entry := range index.Entries {
		if entry.ID == "entry-2" {
			t.Error("Entry-2 should have been removed")
		}
	}

	removed = RemoveFromIndex(index, "non-existent")
	if removed {
		t.Error("Expected RemoveFromIndex to return false for non-existent entry")
	}
}

func TestRebuildIndex(t *testing.T) {
	tmpDir := t.TempDir()
	entriesDir := filepath.Join(tmpDir, "entries", "test")
	os.MkdirAll(entriesDir, 0755)

	// Create test entries
	entry1 := &Entry{
		Metadata: EntryMetadata{
			Title:    "Entry 1",
			Category: "test",
			Tags:     []string{"tag1"},
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		Content:  "# Entry 1\n\nContent",
		FilePath: filepath.Join(entriesDir, "entry-1.md"),
		ID:       "test-entry-1",
	}

	entry2 := &Entry{
		Metadata: EntryMetadata{
			Title:    "Entry 2",
			Category: "test",
			Tags:     []string{"tag2"},
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		Content:  "# Entry 2\n\nContent",
		FilePath: filepath.Join(entriesDir, "entry-2.md"),
		ID:       "test-entry-2",
	}

	WriteEntry(entry1, entry1.FilePath)
	WriteEntry(entry2, entry2.FilePath)

	// Rebuild index
	index, err := RebuildIndex(tmpDir)
	if err != nil {
		t.Fatalf("Failed to rebuild index: %v", err)
	}

	if len(index.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(index.Entries))
	}

	// Verify entries are indexed
	found1 := false
	found2 := false
	for _, entry := range index.Entries {
		if entry.ID == "test-entry-1" {
			found1 = true
		}
		if entry.ID == "test-entry-2" {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("Not all entries were indexed during rebuild")
	}
}

func TestFindEntryByID(t *testing.T) {
	index := &Index{
		Entries: []IndexEntry{
			{ID: "entry-1", Title: "Entry 1"},
			{ID: "entry-2", Title: "Entry 2"},
		},
	}

	found := FindEntryByID(index, "entry-2")
	if found == nil {
		t.Fatal("Expected to find entry-2")
	}

	if found.Title != "Entry 2" {
		t.Errorf("Expected title 'Entry 2', got %s", found.Title)
	}

	notFound := FindEntryByID(index, "non-existent")
	if notFound != nil {
		t.Error("Expected nil for non-existent entry")
	}
}

func TestFindEntryByQuery(t *testing.T) {
	index := &Index{
		Entries: []IndexEntry{
			{ID: "test-ssh-tunneling", Title: "SSH Tunneling"},
			{ID: "test-go-snippets", Title: "Go Snippets"},
		},
	}

	// Find by ID
	found := FindEntryByQuery(index, "test-ssh-tunneling")
	if found == nil {
		t.Fatal("Expected to find by ID")
	}

	// Find by exact title
	found = FindEntryByQuery(index, "SSH Tunneling")
	if found == nil {
		t.Fatal("Expected to find by exact title")
	}

	// Not found
	notFound := FindEntryByQuery(index, "partial")
	if notFound != nil {
		t.Error("Expected nil for partial match (use FindEntriesByPartialQuery)")
	}
}

func TestFindEntriesByPartialQuery(t *testing.T) {
	index := &Index{
		Entries: []IndexEntry{
			{ID: "linux-ssh-tunneling", Title: "SSH Tunneling"},
			{ID: "linux-networking", Title: "Networking Tips"},
			{ID: "programming-go", Title: "Go Programming"},
		},
	}

	// Find by partial ID
	results := FindEntriesByPartialQuery(index, "linux")
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'linux', got %d", len(results))
	}

	// Find by partial title (case insensitive)
	results = FindEntriesByPartialQuery(index, "networking")
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'networking', got %d", len(results))
	}

	// Find by partial title mixed case
	results = FindEntriesByPartialQuery(index, "SSH")
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'SSH', got %d", len(results))
	}

	// No matches
	results = FindEntriesByPartialQuery(index, "nonexistent")
	if len(results) != 0 {
		t.Errorf("Expected 0 results for 'nonexistent', got %d", len(results))
	}
}
