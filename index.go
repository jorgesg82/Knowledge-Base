package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	mu          sync.RWMutex `json:"-"`
	Entries     []IndexEntry `json:"entries"`
	LastUpdated time.Time    `json:"last_updated"`
}

type persistedIndex struct {
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

	var stored persistedIndex
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}

	return &Index{
		Entries:     stored.Entries,
		LastUpdated: stored.LastUpdated,
	}, nil
}

func SaveIndex(index *Index, kbPath string) error {
	indexDir := filepath.Join(kbPath, ".kb")
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}

	index.mu.Lock()
	index.LastUpdated = time.Now()
	snapshot := persistedIndex{
		Entries:     append([]IndexEntry(nil), index.Entries...),
		LastUpdated: index.LastUpdated,
	}
	index.mu.Unlock()

	indexPath := filepath.Join(indexDir, "index.json")
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	if err := writeFileAtomically(indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

func SnapshotEntries(index *Index) []IndexEntry {
	index.mu.RLock()
	defer index.mu.RUnlock()

	return append([]IndexEntry(nil), index.Entries...)
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

	index.mu.Lock()
	defer index.mu.Unlock()

	for i, e := range index.Entries {
		if e.ID == entry.ID {
			index.Entries[i] = indexEntry
			return
		}
	}

	index.Entries = append(index.Entries, indexEntry)
}

func RemoveFromIndex(index *Index, id string) bool {
	index.mu.Lock()
	defer index.mu.Unlock()

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
	index.mu.RLock()
	defer index.mu.RUnlock()

	for i := range index.Entries {
		if index.Entries[i].ID == id {
			entry := index.Entries[i]
			return &entry
		}
	}
	return nil
}
