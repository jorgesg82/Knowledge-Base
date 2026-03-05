package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type EntryMetadata struct {
	Title    string    `yaml:"title"`
	Tags     []string  `yaml:"tags"`
	Category string    `yaml:"category"`
	Created  time.Time `yaml:"created"`
	Updated  time.Time `yaml:"updated"`
}

type Entry struct {
	Metadata EntryMetadata
	Content  string
	FilePath string
	ID       string
}

func GenerateID(category, title string) string {
	combined := category + "-" + title
	combined = strings.ToLower(combined)
	reg := regexp.MustCompile("[^a-z0-9]+")
	combined = reg.ReplaceAllString(combined, "-")
	combined = strings.Trim(combined, "-")
	return combined
}

func TitleCase(s string) string {
	// Replace separators with spaces
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")

	// Convert to lowercase first, then capitalize
	s = strings.ToLower(s)
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

func ParseEntry(filePath string) (*Entry, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read entry: %w", err)
	}

	frontmatter, content, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	if len(bytes.TrimSpace(frontmatter)) == 0 {
		return nil, fmt.Errorf("invalid entry format: empty frontmatter")
	}

	var metadata EntryMetadata
	if err := yaml.Unmarshal(frontmatter, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Validate required fields
	if strings.TrimSpace(metadata.Title) == "" {
		return nil, fmt.Errorf("invalid entry: title is required")
	}

	// Infer category from path if not set
	if metadata.Category == "" {
		// Extract category from path like entries/linux/file.md -> linux
		parts := strings.Split(filepath.ToSlash(filePath), "/")
		for i, part := range parts {
			if part == "entries" && i+1 < len(parts) {
				metadata.Category = parts[i+1]
				break
			}
		}
		if metadata.Category == "" {
			metadata.Category = "misc"
		}
	}

	contentString := strings.TrimSpace(string(content))

	id := GenerateID(metadata.Category, metadata.Title)

	return &Entry{
		Metadata: metadata,
		Content:  contentString,
		FilePath: filePath,
		ID:       id,
	}, nil
}

func splitFrontmatter(data []byte) ([]byte, []byte, error) {
	var offset int
	switch {
	case bytes.HasPrefix(data, []byte("---\n")):
		offset = len("---\n")
	case bytes.HasPrefix(data, []byte("---\r\n")):
		offset = len("---\r\n")
	default:
		return nil, nil, fmt.Errorf("invalid entry format: missing frontmatter start")
	}

	for start := offset; start <= len(data); {
		lineEndOffset := bytes.IndexByte(data[start:], '\n')
		if lineEndOffset == -1 {
			line := bytes.TrimRight(data[start:], "\r")
			if bytes.Equal(line, []byte("---")) {
				return data[offset:start], nil, nil
			}
			break
		}

		lineEnd := start + lineEndOffset
		line := bytes.TrimRight(data[start:lineEnd], "\r")
		if bytes.Equal(line, []byte("---")) {
			return data[offset:start], data[lineEnd+1:], nil
		}

		start = lineEnd + 1
	}

	return nil, nil, fmt.Errorf("invalid entry format: missing frontmatter end")
}

func WriteEntry(entry *Entry, filePath string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	metadata, err := yaml.Marshal(&entry.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(metadata)
	buf.WriteString("---\n\n")
	buf.WriteString(entry.Content)

	// Ensure content ends with newline
	if !strings.HasSuffix(entry.Content, "\n") {
		buf.WriteString("\n")
	}

	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write entry: %w", err)
	}

	return nil
}

func CreateEntryTemplate(category, title string) *Entry {
	now := time.Now()
	titleCased := TitleCase(title)

	return &Entry{
		Metadata: EntryMetadata{
			Title:    titleCased,
			Tags:     []string{},
			Category: category,
			Created:  now,
			Updated:  now,
		},
		Content: fmt.Sprintf("# %s\n\n[Write your content here]\n", titleCased),
		ID:      GenerateID(category, title),
	}
}

func GetEntryPath(kbPath, category, title string) string {
	// Sanitize category to prevent path traversal
	category = filepath.Clean(category)
	category = strings.ReplaceAll(category, "..", "")
	category = strings.Trim(category, "/\\")
	if category == "" || category == "." {
		category = "misc"
	}

	// Sanitize filename
	filename := strings.ToLower(title)
	reg := regexp.MustCompile("[^a-z0-9]+")
	filename = reg.ReplaceAllString(filename, "-")
	filename = strings.Trim(filename, "-")
	if filename == "" {
		filename = "untitled"
	}
	filename = filename + ".md"

	// Build path and ensure it's within kbPath
	entriesRoot := filepath.Join(kbPath, "entries")
	fullPath := filepath.Join(entriesRoot, category, filename)

	// Security check: ensure the resolved path is still under kbPath
	if !isWithinBase(entriesRoot, fullPath) {
		// Path traversal attempt detected, use safe default
		return filepath.Join(kbPath, "entries", "misc", filename)
	}

	return fullPath
}
