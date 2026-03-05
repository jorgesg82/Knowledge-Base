package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test empty or whitespace-only titles
func TestGenerateIDEdgeCases(t *testing.T) {
	tests := []struct {
		category string
		title    string
		desc     string
	}{
		{"test", "", "empty title"},
		{"test", "   ", "whitespace only"},
		{"test", "!!!!", "only special chars"},
		{"test", "a/b/c", "slashes in title"},
		{"test", "../../../etc/passwd", "path traversal attempt"},
		{"test", strings.Repeat("a", 1000), "extremely long title"},
		{"", "title", "empty category"},
		{"cat/subcat", "title", "category with slash"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			// Should not panic
			id := GenerateID(tt.category, tt.title)

			// ID should not contain dangerous chars
			if strings.Contains(id, "/") || strings.Contains(id, "..") {
				t.Errorf("ID contains dangerous chars: %s", id)
			}

			// ID should not be empty (unless both inputs are)
			if tt.category != "" || tt.title != "" {
				if id == "" || id == "-" {
					t.Errorf("Generated empty or invalid ID from (%s, %s): %s",
						tt.category, tt.title, id)
				}
			}
		})
	}
}

// Test parsing entries with malformed frontmatter
func TestParseMalformedEntries(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			"no frontmatter",
			"# Just content\n\nNo frontmatter here",
			true,
		},
		{
			"unterminated frontmatter",
			"---\ntitle: Test\nno closing delimiter",
			true,
		},
		{
			"empty frontmatter",
			"---\n---\n\nContent",
			true, // Should fail because no title
		},
		{
			"invalid yaml",
			"---\ntitle: Test\ntags: [unclosed\n---\n",
			true,
		},
		{
			"duplicate keys",
			"---\ntitle: First\ntitle: Second\n---\n",
			true, // YAML rejects duplicate keys
		},
		{
			"very large frontmatter",
			"---\ntitle: Test\ntags: [" + strings.Repeat("tag,", 10000) + "]\n---\n",
			false, // Should handle large data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(tmpDir, tt.name+".md")
			os.WriteFile(path, []byte(tt.content), 0644)

			_, err := ParseEntry(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEntry() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test concurrent index operations
func TestConcurrentIndexOperations(t *testing.T) {
	tmpDir := t.TempDir()
	index := &Index{Entries: []IndexEntry{}}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			entry := &Entry{
				Metadata: EntryMetadata{
					Title:    "Entry " + strconv.Itoa(n),
					Category: "test",
					Tags:     []string{"tag"},
					Created:  time.Now(),
					Updated:  time.Now(),
				},
				FilePath: filepath.Join(tmpDir, "entry.md"),
				ID:       "test-entry-" + strconv.Itoa(n),
			}
			AddToIndex(index, entry, tmpDir)
		}(i)
	}

	wg.Wait()

	if len(index.Entries) != 10 {
		t.Fatalf("Expected 10 entries, got %d", len(index.Entries))
	}
}

// Test file permission issues
func TestFilePermissionErrors(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Running as root, cannot test permission errors")
	}

	tmpDir := t.TempDir()

	// Create read-only directory
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	os.MkdirAll(readOnlyDir, 0555)
	defer os.Chmod(readOnlyDir, 0755) // Cleanup

	entry := &Entry{
		Metadata: EntryMetadata{
			Title:    "Test",
			Category: "test",
			Tags:     []string{},
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		Content:  "Content",
		FilePath: filepath.Join(readOnlyDir, "test.md"),
		ID:       "test-entry",
	}

	// Should fail due to permissions
	err := WriteEntry(entry, entry.FilePath)
	if err == nil {
		t.Error("Expected error writing to read-only directory")
	}
}

// Test very long content
func TestLargeEntryContent(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "large.md")

	// Create entry with 1MB of content
	largeContent := strings.Repeat("This is a line of content.\n", 40000)

	entry := &Entry{
		Metadata: EntryMetadata{
			Title:    "Large Entry",
			Category: "test",
			Tags:     []string{"large"},
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		Content:  largeContent,
		FilePath: entryPath,
		ID:       "test-large-entry",
	}

	// Write and parse large entry
	err := WriteEntry(entry, entryPath)
	if err != nil {
		t.Fatalf("Failed to write large entry: %v", err)
	}

	parsed, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("Failed to parse large entry: %v", err)
	}

	// Content might differ by trailing whitespace due to TrimSpace in ParseEntry
	if strings.TrimSpace(parsed.Content) != strings.TrimSpace(entry.Content) {
		t.Error("Content mismatch after trim")
	}

	// Ensure large content was handled correctly
	if len(parsed.Content) < 1000000 {
		t.Errorf("Expected large content (>1MB), got %d bytes", len(parsed.Content))
	}
}

// Test special characters in tags
func TestSpecialCharactersInTags(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "test.md")

	specialTags := []string{
		"tag with spaces",
		"tag/with/slashes",
		"tag-with-dashes",
		"tag_with_underscores",
		"tag.with.dots",
		"日本語",
		"émojis😀",
		"UPPERCASE",
	}

	entry := &Entry{
		Metadata: EntryMetadata{
			Title:    "Test",
			Category: "test",
			Tags:     specialTags,
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		Content:  "Content",
		FilePath: entryPath,
		ID:       "test-entry",
	}

	err := WriteEntry(entry, entryPath)
	if err != nil {
		t.Fatalf("Failed to write entry with special tags: %v", err)
	}

	parsed, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("Failed to parse entry with special tags: %v", err)
	}

	if len(parsed.Metadata.Tags) != len(specialTags) {
		t.Errorf("Tag count mismatch: expected %d, got %d",
			len(specialTags), len(parsed.Metadata.Tags))
	}

	// Verify all tags preserved
	for _, tag := range specialTags {
		found := false
		for _, parsedTag := range parsed.Metadata.Tags {
			if parsedTag == tag {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Tag not preserved: %s", tag)
		}
	}
}

// Test empty index operations
func TestEmptyIndexOperations(t *testing.T) {
	index := &Index{Entries: []IndexEntry{}}

	// Operations on empty index should not panic
	results := SearchByTags(index, []string{"tag"})
	if len(results) != 0 {
		t.Error("Expected no results from empty index")
	}

	results = SearchByCategory(index, "category")
	if len(results) != 0 {
		t.Error("Expected no results from empty index")
	}

	entry := FindEntryByID(index, "id")
	if entry != nil {
		t.Error("Expected nil from empty index")
	}

	removed := RemoveFromIndex(index, "id")
	if removed {
		t.Error("Expected false when removing from empty index")
	}
}

// Test path traversal in GetEntryPath
func TestPathTraversalPrevention(t *testing.T) {
	kbPath := "/test/kb"

	// Try various path traversal attempts
	dangerous := []struct {
		category string
		title    string
	}{
		{"../../../etc", "passwd"},
		{"test", "../../../etc/passwd"},
		{".", "../../secrets"},
		{"..", "file"},
	}

	for _, d := range dangerous {
		path := GetEntryPath(kbPath, d.category, d.title)

		// Path should still be under kbPath
		if !strings.HasPrefix(path, kbPath) {
			t.Errorf("Path traversal detected: %s escapes %s", path, kbPath)
		}
	}
}

// Test index corruption recovery
func TestIndexCorruptionRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, ".kb", "index.json")
	os.MkdirAll(filepath.Dir(indexPath), 0755)

	// Write corrupted JSON
	os.WriteFile(indexPath, []byte("{ invalid json "), 0644)

	// Should return error, not panic
	_, err := LoadIndex(tmpDir)
	if err == nil {
		t.Error("Expected error loading corrupted index")
	}

	// Write empty file
	os.WriteFile(indexPath, []byte(""), 0644)
	_, err = LoadIndex(tmpDir)
	if err == nil {
		t.Error("Expected error loading empty index")
	}
}

// Test duplicate IDs in index
func TestDuplicateIDHandling(t *testing.T) {
	tmpDir := t.TempDir()
	index := &Index{Entries: []IndexEntry{}}

	entry1 := &Entry{
		Metadata: EntryMetadata{
			Title:    "First",
			Category: "test",
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		FilePath: filepath.Join(tmpDir, "entry.md"),
		ID:       "test-duplicate",
	}

	entry2 := &Entry{
		Metadata: EntryMetadata{
			Title:    "Second",
			Category: "test",
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		FilePath: filepath.Join(tmpDir, "entry2.md"),
		ID:       "test-duplicate", // Same ID
	}

	AddToIndex(index, entry1, tmpDir)
	AddToIndex(index, entry2, tmpDir)

	// Should have only 1 entry (second one replaces first)
	if len(index.Entries) != 1 {
		t.Errorf("Expected 1 entry after duplicate ID, got %d", len(index.Entries))
	}

	if index.Entries[0].Title != "Second" {
		t.Error("Second entry should have replaced first")
	}
}

// Test search with empty or whitespace queries
func TestSearchWithEmptyQueries(t *testing.T) {
	tmpDir := t.TempDir()
	index := &Index{
		Entries: []IndexEntry{
			{ID: "test", Title: "Test", Category: "test", Tags: []string{"tag"}},
		},
	}

	// Empty tag search
	results := SearchByTags(index, []string{})
	if len(results) != 1 {
		t.Errorf("Expected 1 result for empty tag query, got %d", len(results))
	}

	// Empty category search
	results = SearchByCategory(index, "")
	if len(results) != 0 {
		t.Errorf("Expected empty category search to return 0 results, got %d", len(results))
	}

	// Empty text search
	results2, _ := SearchByText(index, tmpDir, "")
	if len(results2) != 0 {
		t.Errorf("Expected empty text search to return 0 results, got %d", len(results2))
	}

	// Whitespace query
	results = SearchByCategory(index, "   ")
	if len(results) != 0 {
		t.Errorf("Expected whitespace category search to return 0 results, got %d", len(results))
	}
}

// Test time edge cases
func TestTimeEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "test.md")

	// Zero time
	entry := &Entry{
		Metadata: EntryMetadata{
			Title:    "Test",
			Category: "test",
			Created:  time.Time{}, // Zero value
			Updated:  time.Time{},
		},
		Content:  "Content",
		FilePath: entryPath,
		ID:       "test",
	}

	err := WriteEntry(entry, entryPath)
	if err != nil {
		t.Fatalf("Failed to write entry with zero time: %v", err)
	}

	parsed, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("Failed to parse entry with zero time: %v", err)
	}

	// Should handle zero times gracefully
	if parsed.Metadata.Created.IsZero() {
		t.Log("Created time is zero - may want to set to file mtime instead")
	}
}
