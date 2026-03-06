package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StoreStats struct {
	Notes           int
	Captures        int
	Operations      int
	Aliases         int
	Topics          int
	NotesByCategory map[string]int
	LastUpdated     time.Time
}

func loadStoreStats(kbPath string) (*StoreStats, error) {
	notes, err := loadCanonicalNoteManifest(kbPath)
	if err != nil {
		return nil, err
	}

	captureCount, err := countStoreJSONRecords(kbPath, capturesDirName)
	if err != nil {
		return nil, err
	}
	operationCount, err := countStoreJSONRecords(kbPath, opsDirName)
	if err != nil {
		return nil, err
	}

	aliasSet := map[string]struct{}{}
	topicSet := map[string]struct{}{}
	notesByCategory := map[string]int{}
	lastUpdated := time.Time{}

	for _, note := range notes {
		if note == nil {
			continue
		}

		category := deriveNoteCategoryFromTopics(note.Topics)
		notesByCategory[category]++

		for _, alias := range note.Aliases {
			alias = strings.TrimSpace(strings.ToLower(alias))
			if alias != "" {
				aliasSet[alias] = struct{}{}
			}
		}
		for _, topic := range note.Topics {
			topic = strings.TrimSpace(strings.ToLower(topic))
			if topic != "" {
				topicSet[topic] = struct{}{}
			}
		}

		if note.UpdatedAt.After(lastUpdated) {
			lastUpdated = note.UpdatedAt
		}
	}

	if lastUpdated.IsZero() {
		if state, err := loadKBStateIfPresent(kbPath); err == nil && state != nil {
			lastUpdated = state.CreatedAt
		}
	}

	return &StoreStats{
		Notes:           len(notes),
		Captures:        captureCount,
		Operations:      operationCount,
		Aliases:         len(aliasSet),
		Topics:          len(topicSet),
		NotesByCategory: notesByCategory,
		LastUpdated:     lastUpdated,
	}, nil
}

func loadKBStateIfPresent(kbPath string) (*KBState, error) {
	statePath := filepath.Join(kbPath, ".kb", stateFileName)
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read state: %w", err)
	}

	var state KBState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state: %w", err)
	}
	return &state, nil
}

func countStoreJSONRecords(kbPath, dirName string) (int, error) {
	dirPath := filepath.Join(kbPath, ".kb", dirName)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read %s: %w", dirPath, err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		count++
	}

	return count, nil
}

func loadCanonicalNoteCount(kbPath string) (int, error) {
	stats, err := loadStoreStats(kbPath)
	if err != nil {
		return 0, err
	}
	return stats.Notes, nil
}
