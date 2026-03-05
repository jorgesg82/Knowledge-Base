package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateEntryTemplate(t *testing.T) {
	entry := CreateEntryTemplate("linux", "test-entry")

	if entry.Metadata.Title != "Test Entry" {
		t.Errorf("Expected title 'Test Entry', got %s", entry.Metadata.Title)
	}

	if entry.Metadata.Category != "linux" {
		t.Errorf("Expected category 'linux', got %s", entry.Metadata.Category)
	}

	if entry.ID != "linux-test-entry" {
		t.Errorf("Expected ID 'linux-test-entry', got %s", entry.ID)
	}

	if len(entry.Metadata.Tags) != 0 {
		t.Errorf("Expected empty tags, got %v", entry.Metadata.Tags)
	}

	if !strings.Contains(entry.Content, "# Test Entry") {
		t.Error("Expected content to contain title header")
	}
}

func TestGenerateID(t *testing.T) {
	tests := []struct {
		category string
		title    string
		expected string
	}{
		{"linux", "SSH Tunneling", "linux-ssh-tunneling"},
		{"programming", "Go Snippets", "programming-go-snippets"},
		{"misc", "Quick Notes", "misc-quick-notes"},
		{"test", "Multiple   Spaces", "test-multiple-spaces"},
		{"cat", "Special!@#$%Chars", "cat-special-chars"},
	}

	for _, tt := range tests {
		result := GenerateID(tt.category, tt.title)
		if result != tt.expected {
			t.Errorf("GenerateID(%s, %s) = %s, expected %s",
				tt.category, tt.title, result, tt.expected)
		}
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "Hello World"},
		{"ssh-tunneling", "Ssh Tunneling"},
		{"go_snippets", "Go Snippets"},
		{"UPPERCASE", "Uppercase"},
		{"a", "A"},
		{"", ""},
	}

	for _, tt := range tests {
		result := TitleCase(tt.input)
		if result != tt.expected {
			t.Errorf("TitleCase(%s) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

func TestWriteAndParseEntry(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "test.md")

	entry := &Entry{
		Metadata: EntryMetadata{
			Title:    "Test Entry",
			Category: "test",
			Tags:     []string{"tag1", "tag2"},
			Created:  time.Now().Round(time.Second),
			Updated:  time.Now().Round(time.Second),
		},
		Content:  "# Test Entry\n\nThis is test content.\n\n## Section\n\nMore content here.",
		FilePath: entryPath,
		ID:       "test-test-entry",
	}

	// Write entry
	err := WriteEntry(entry, entryPath)
	if err != nil {
		t.Fatalf("Failed to write entry: %v", err)
	}

	// Parse entry back
	parsed, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("Failed to parse entry: %v", err)
	}

	// Verify metadata
	if parsed.Metadata.Title != entry.Metadata.Title {
		t.Errorf("Title mismatch: expected %s, got %s", entry.Metadata.Title, parsed.Metadata.Title)
	}

	if parsed.Metadata.Category != entry.Metadata.Category {
		t.Errorf("Category mismatch: expected %s, got %s", entry.Metadata.Category, parsed.Metadata.Category)
	}

	if len(parsed.Metadata.Tags) != len(entry.Metadata.Tags) {
		t.Errorf("Tags length mismatch: expected %d, got %d", len(entry.Metadata.Tags), len(parsed.Metadata.Tags))
	}

	// Verify content
	if parsed.Content != entry.Content {
		t.Errorf("Content mismatch:\nExpected:\n%s\n\nGot:\n%s", entry.Content, parsed.Content)
	}

	// Verify ID generation
	expectedID := GenerateID(entry.Metadata.Category, entry.Metadata.Title)
	if parsed.ID != expectedID {
		t.Errorf("ID mismatch: expected %s, got %s", expectedID, parsed.ID)
	}
}

func TestParseEntryWithMissingFields(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "test.md")

	// Create entry with minimal frontmatter
	content := `---
title: Test
---

# Test

Content here.
`
	os.WriteFile(entryPath, []byte(content), 0644)

	parsed, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("Failed to parse entry: %v", err)
	}

	if parsed.Metadata.Title != "Test" {
		t.Errorf("Expected title 'Test', got %s", parsed.Metadata.Title)
	}

	// Should have defaults
	if parsed.Metadata.Category == "" {
		t.Error("Expected category to be set from path")
	}

	if len(parsed.Metadata.Tags) != 0 {
		t.Errorf("Expected empty tags, got %v", parsed.Metadata.Tags)
	}
}

func TestParseEntryInvalidFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "test.md")

	// Create entry with invalid YAML
	content := `---
title: Test
invalid yaml here: [
---

Content
`
	os.WriteFile(entryPath, []byte(content), 0644)

	_, err := ParseEntry(entryPath)
	if err == nil {
		t.Error("Expected error for invalid frontmatter")
	}
}

func TestGetEntryPath(t *testing.T) {
	kbPath := "/test/kb"

	tests := []struct {
		category string
		title    string
		expected string
	}{
		{"linux", "ssh-tunneling", "/test/kb/entries/linux/ssh-tunneling.md"},
		{"programming", "go-snippets", "/test/kb/entries/programming/go-snippets.md"},
		{"misc", "notes", "/test/kb/entries/misc/notes.md"},
	}

	for _, tt := range tests {
		result := GetEntryPath(kbPath, tt.category, tt.title)
		if result != tt.expected {
			t.Errorf("GetEntryPath(%s, %s) = %s, expected %s",
				tt.category, tt.title, result, tt.expected)
		}
	}
}
