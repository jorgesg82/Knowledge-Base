package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type IndexEntry struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Title    string    `json:"title"`
	Category string    `json:"category"`
	Tags     []string  `json:"tags"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
}

type Index struct {
	Entries     []IndexEntry `json:"entries"`
	LastUpdated time.Time    `json:"last_updated"`
}

func LoadIndex(kbPath string) (*Index, error) {
	indexPath := filepath.Join(kbPath, ".kb", "index.json")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Index{
				Entries:     []IndexEntry{},
				LastUpdated: time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("failed to read index: %w", err)
	}

	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}

	return &index, nil
}

func SaveIndex(index *Index, kbPath string) error {
	indexDir := filepath.Join(kbPath, ".kb")
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}

	index.LastUpdated = time.Now()

	indexPath := filepath.Join(indexDir, "index.json")
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

func AddToIndex(index *Index, entry *Entry, kbPath string) {
	relPath, _ := filepath.Rel(kbPath, entry.FilePath)

	indexEntry := IndexEntry{
		ID:       entry.ID,
		Path:     relPath,
		Title:    entry.Metadata.Title,
		Category: entry.Metadata.Category,
		Tags:     entry.Metadata.Tags,
		Created:  entry.Metadata.Created,
		Updated:  entry.Metadata.Updated,
	}

	for i, e := range index.Entries {
		if e.ID == entry.ID {
			index.Entries[i] = indexEntry
			return
		}
	}

	index.Entries = append(index.Entries, indexEntry)
}

func RemoveFromIndex(index *Index, id string) bool {
	for i, e := range index.Entries {
		if e.ID == id {
			index.Entries = append(index.Entries[:i], index.Entries[i+1:]...)
			return true
		}
	}
	return false
}

func RebuildIndex(kbPath string) (*Index, error) {
	index := &Index{
		Entries:     []IndexEntry{},
		LastUpdated: time.Now(),
	}

	entriesDir := filepath.Join(kbPath, "entries")
	err := filepath.Walk(entriesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".md" {
			entry, err := ParseEntry(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse %s: %v\n", path, err)
				return nil
			}

			relPath, _ := filepath.Rel(kbPath, path)
			indexEntry := IndexEntry{
				ID:       entry.ID,
				Path:     relPath,
				Title:    entry.Metadata.Title,
				Category: entry.Metadata.Category,
				Tags:     entry.Metadata.Tags,
				Created:  entry.Metadata.Created,
				Updated:  entry.Metadata.Updated,
			}
			index.Entries = append(index.Entries, indexEntry)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk entries directory: %w", err)
	}

	return index, nil
}

func FindEntryByID(index *Index, id string) *IndexEntry {
	for i := range index.Entries {
		if index.Entries[i].ID == id {
			return &index.Entries[i]
		}
	}
	return nil
}

func FindEntryByQuery(index *Index, query string) *IndexEntry {
	for i := range index.Entries {
		if index.Entries[i].ID == query || index.Entries[i].Title == query {
			return &index.Entries[i]
		}
	}
	return nil
}

func FindEntriesByPartialQuery(index *Index, query string) []IndexEntry {
	var matches []IndexEntry
	queryLower := strings.ToLower(query)

	for _, entry := range index.Entries {
		idLower := strings.ToLower(entry.ID)
		titleLower := strings.ToLower(entry.Title)

		if strings.Contains(idLower, queryLower) || strings.Contains(titleLower, queryLower) {
			matches = append(matches, entry)
		}
	}

	return matches
}

func FindEntryWithInference(index *Index, query string) *IndexEntry {
	// Try exact match first
	exactMatch := FindEntryByQuery(index, query)
	if exactMatch != nil {
		return exactMatch
	}

	// Try partial match
	partialMatches := FindEntriesByPartialQuery(index, query)

	if len(partialMatches) == 0 {
		return nil
	}

	if len(partialMatches) == 1 {
		// Single match - use it directly
		return &partialMatches[0]
	}

	// Multiple matches - show selection menu
	fmt.Printf(Header("Multiple matches found for '%s':") + "\n", query)
	for i, entry := range partialMatches {
		num := Dim(fmt.Sprintf("%d.", i+1))
		category := Cyan(entry.Category)
		id := Gray(entry.ID)
		title := Bold(entry.Title)
		fmt.Printf("  %s %s/%s - %s\n", num, category, id, title)
	}

	fmt.Print("\n" + Highlight("Select entry number (or 0 to cancel): "))
	var selection int
	fmt.Scanln(&selection)

	if selection < 1 || selection > len(partialMatches) {
		return nil
	}

	return &partialMatches[selection-1]
}
