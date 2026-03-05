package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SearchByTags(index *Index, tags []string) []IndexEntry {
	var results []IndexEntry

	for _, entry := range SnapshotEntries(index) {
		matchCount := 0
		for _, searchTag := range tags {
			for _, entryTag := range entry.Tags {
				if strings.EqualFold(entryTag, searchTag) {
					matchCount++
					break
				}
			}
		}
		if matchCount == len(tags) {
			results = append(results, entry)
		}
	}

	return results
}

func SearchByCategory(index *Index, category string) []IndexEntry {
	var results []IndexEntry

	for _, entry := range SnapshotEntries(index) {
		if strings.EqualFold(entry.Category, category) {
			results = append(results, entry)
		}
	}

	return results
}

type SearchResult struct {
	Entry      IndexEntry
	LineNumber int
	Line       string
}

func SearchByText(index *Index, kbPath, text string) ([]SearchResult, error) {
	var results []SearchResult
	searchLower := strings.ToLower(strings.TrimSpace(text))
	if searchLower == "" {
		return results, nil
	}

	for _, entry := range SnapshotEntries(index) {
		entryPath := filepath.Join(kbPath, entry.Path)
		file, err := os.Open(entryPath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), searchLower) {
				results = append(results, SearchResult{
					Entry:      entry,
					LineNumber: lineNum,
					Line:       strings.TrimSpace(line),
				})
			}
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to scan %s: %w", entryPath, err)
		}
		file.Close()
	}

	return results, nil
}

func GetAllTags(index *Index) map[string]int {
	tagCounts := make(map[string]int)

	for _, entry := range SnapshotEntries(index) {
		for _, tag := range entry.Tags {
			tagCounts[tag]++
		}
	}

	return tagCounts
}

func GetAllCategories(index *Index) map[string]int {
	categoryCounts := make(map[string]int)

	for _, entry := range SnapshotEntries(index) {
		categoryCounts[entry.Category]++
	}

	return categoryCounts
}

func PrintSearchResults(results []IndexEntry, prefix string) {
	if len(results) == 0 {
		fmt.Println(Dim("No entries found"))
		return
	}

	fmt.Printf(Header("Found %d entries:")+"\n", len(results))
	for i, entry := range results {
		num := Dim(fmt.Sprintf("%d.", i+1))
		category := Cyan(entry.Category)
		id := Gray(entry.ID)
		title := Bold(entry.Title)
		fmt.Printf("  %s %s/%s - %s\n", num, category, id, title)
	}
}

func PrintTextSearchResults(results []SearchResult) {
	if len(results) == 0 {
		fmt.Println(Dim("No matches found"))
		return
	}

	fmt.Printf(Header("Found in %d locations:")+"\n", len(results))
	currentEntry := ""
	for _, result := range results {
		entryID := result.Entry.Category + "/" + result.Entry.ID
		if entryID != currentEntry {
			fmt.Printf("\n"+Cyan("%s")+" "+Dim("(%s)")+":\n", entryID, result.Entry.Title)
			currentEntry = entryID
		}
		lineNum := Yellow(fmt.Sprintf("Line %d", result.LineNumber))
		fmt.Printf("  %s: %s\n", lineNum, result.Line)
	}
}
