package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSearchByTags(t *testing.T) {
	index := &Index{
		Entries: []IndexEntry{
			{
				ID:       "entry-1",
				Title:    "Entry 1",
				Category: "test",
				Tags:     []string{"linux", "networking", "ssh"},
			},
			{
				ID:       "entry-2",
				Title:    "Entry 2",
				Category: "test",
				Tags:     []string{"linux", "firewall"},
			},
			{
				ID:       "entry-3",
				Title:    "Entry 3",
				Category: "test",
				Tags:     []string{"programming", "go"},
			},
		},
	}

	// Search single tag
	results := SearchByTags(index, []string{"linux"})
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'linux', got %d", len(results))
	}

	// Search multiple tags (AND operation)
	results = SearchByTags(index, []string{"linux", "networking"})
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'linux' AND 'networking', got %d", len(results))
	}

	if results[0].ID != "entry-1" {
		t.Errorf("Expected entry-1, got %s", results[0].ID)
	}

	// Search non-existent tag
	results = SearchByTags(index, []string{"nonexistent"})
	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-existent tag, got %d", len(results))
	}

	// Case insensitive search
	results = SearchByTags(index, []string{"LINUX"})
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'LINUX' (case insensitive), got %d", len(results))
	}
}

func TestSearchByCategory(t *testing.T) {
	index := &Index{
		Entries: []IndexEntry{
			{ID: "entry-1", Title: "Entry 1", Category: "linux"},
			{ID: "entry-2", Title: "Entry 2", Category: "linux"},
			{ID: "entry-3", Title: "Entry 3", Category: "programming"},
		},
	}

	// Search existing category
	results := SearchByCategory(index, "linux")
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'linux' category, got %d", len(results))
	}

	// Search non-existent category
	results = SearchByCategory(index, "nonexistent")
	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-existent category, got %d", len(results))
	}

	// Case insensitive search
	results = SearchByCategory(index, "LINUX")
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'LINUX' (case insensitive), got %d", len(results))
	}
}

func TestSearchByText(t *testing.T) {
	tmpDir := t.TempDir()
	entriesDir := filepath.Join(tmpDir, "entries", "test")
	os.MkdirAll(entriesDir, 0755)

	// Create test entries with content
	entries := []struct {
		id      string
		title   string
		content string
	}{
		{
			"entry-1",
			"SSH Tunneling",
			"# SSH Tunneling\n\nLocal port forwarding example:\nssh -L 8080:localhost:80 user@host",
		},
		{
			"entry-2",
			"Firewall Rules",
			"# Firewall Rules\n\nAllow SSH:\niptables -A INPUT -p tcp --dport 22 -j ACCEPT",
		},
		{
			"entry-3",
			"Go Programming",
			"# Go Programming\n\nBasic HTTP server:\nhttp.ListenAndServe(\":8080\", nil)",
		},
	}

	index := &Index{Entries: []IndexEntry{}}

	for _, e := range entries {
		entryPath := filepath.Join(entriesDir, e.id+".md")
		entry := &Entry{
			Metadata: EntryMetadata{
				Title:    e.title,
				Category: "test",
				Tags:     []string{},
				Created:  time.Now(),
				Updated:  time.Now(),
			},
			Content:  e.content,
			FilePath: entryPath,
			ID:       "test-" + e.id,
		}
		WriteEntry(entry, entryPath)
		AddToIndex(index, entry, tmpDir)
	}

	// Search for "ssh"
	results, err := SearchByText(index, tmpDir, "ssh")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results for 'ssh', got %d", len(results))
	}

	// Search for "8080"
	results, err = SearchByText(index, tmpDir, "8080")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results for '8080', got %d", len(results))
	}

	// Search for non-existent text
	results, err = SearchByText(index, tmpDir, "nonexistent")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-existent text, got %d", len(results))
	}

	// Case insensitive search
	results, err = SearchByText(index, tmpDir, "SSH")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results for 'SSH' (case insensitive), got %d", len(results))
	}
}

func TestSearchByTextLongLine(t *testing.T) {
	tmpDir := t.TempDir()
	entriesDir := filepath.Join(tmpDir, "entries", "test")
	os.MkdirAll(entriesDir, 0755)

	longLine := strings.Repeat("x", 70*1024) + " needle " + strings.Repeat("y", 70*1024)
	entryPath := filepath.Join(entriesDir, "long-line.md")
	entry := &Entry{
		Metadata: EntryMetadata{
			Title:    "Long Line",
			Category: "test",
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		Content:  "# Long Line\n\n" + longLine,
		FilePath: entryPath,
		ID:       "test-long-line",
	}

	if err := WriteEntry(entry, entryPath); err != nil {
		t.Fatalf("Failed to write long-line entry: %v", err)
	}

	index := &Index{Entries: []IndexEntry{}}
	AddToIndex(index, entry, tmpDir)

	results, err := SearchByText(index, tmpDir, "needle")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result for long line search, got %d", len(results))
	}

	if results[0].LineNumber <= 0 {
		t.Errorf("Expected a positive line number, got %d", results[0].LineNumber)
	}

	if !strings.Contains(results[0].Line, "needle") {
		t.Errorf("Expected result line to contain search term, got %q", results[0].Line)
	}
}

func TestGetAllTags(t *testing.T) {
	index := &Index{
		Entries: []IndexEntry{
			{Tags: []string{"linux", "networking", "ssh"}},
			{Tags: []string{"linux", "firewall"}},
			{Tags: []string{"programming", "go"}},
			{Tags: []string{"linux"}},
		},
	}

	tagCounts := GetAllTags(index)

	if tagCounts["linux"] != 3 {
		t.Errorf("Expected 'linux' count 3, got %d", tagCounts["linux"])
	}

	if tagCounts["networking"] != 1 {
		t.Errorf("Expected 'networking' count 1, got %d", tagCounts["networking"])
	}

	if tagCounts["go"] != 1 {
		t.Errorf("Expected 'go' count 1, got %d", tagCounts["go"])
	}

	if len(tagCounts) != 6 {
		t.Errorf("Expected 6 unique tags, got %d", len(tagCounts))
	}
}

func TestGetAllCategories(t *testing.T) {
	index := &Index{
		Entries: []IndexEntry{
			{Category: "linux"},
			{Category: "linux"},
			{Category: "programming"},
			{Category: "misc"},
			{Category: "linux"},
		},
	}

	categoryCounts := GetAllCategories(index)

	if categoryCounts["linux"] != 3 {
		t.Errorf("Expected 'linux' count 3, got %d", categoryCounts["linux"])
	}

	if categoryCounts["programming"] != 1 {
		t.Errorf("Expected 'programming' count 1, got %d", categoryCounts["programming"])
	}

	if categoryCounts["misc"] != 1 {
		t.Errorf("Expected 'misc' count 1, got %d", categoryCounts["misc"])
	}

	if len(categoryCounts) != 3 {
		t.Errorf("Expected 3 unique categories, got %d", len(categoryCounts))
	}
}
