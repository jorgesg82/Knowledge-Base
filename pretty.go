package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func handlePretty(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	config, err := LoadConfig(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	index, err := LoadIndex(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading index: %v\n", err)
		os.Exit(1)
	}

	// Parse flags
	mode := config.PrettyMode
	autoApply := config.PrettyAutoApply
	processAll := false
	var query string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--mode":
			if i+1 < len(args) {
				mode = args[i+1]
				i++
			}
		case "--confirm":
			autoApply = false
		case "--all":
			processAll = true
		default:
			if query == "" {
				query = arg
			}
		}
	}

	// Validate mode
	prettyMode := PrettyMode(mode)
	if prettyMode != ModeConservative && prettyMode != ModeModerate && prettyMode != ModeAggressive {
		printError("Invalid mode: %s. Use: conservative, moderate, or aggressive", mode)
		os.Exit(1)
	}

	if processAll {
		prettifyAll(index, kbPath, prettyMode, autoApply)
	} else {
		if query == "" {
			fmt.Fprintln(os.Stderr, "Error: missing entry query")
			fmt.Fprintln(os.Stderr, "Usage: kb pretty <query>")
			fmt.Fprintln(os.Stderr, "       kb pretty --all")
			os.Exit(1)
		}
		prettifyOne(index, kbPath, query, prettyMode, autoApply)
	}
}

func prettifyOne(index *Index, kbPath, query string, mode PrettyMode, autoApply bool) {
	indexEntry := FindEntryWithInference(index, query)
	if indexEntry == nil {
		printError("No entry found matching: %s", query)
		os.Exit(1)
	}

	entryPath := filepath.Join(kbPath, indexEntry.Path)

	printInfo("Prettifying: %s (%s mode)", indexEntry.Title, mode)

	// Read current content
	content, err := os.ReadFile(entryPath)
	if err != nil {
		printError("Failed to read entry: %v", err)
		os.Exit(1)
	}

	// Call Claude API
	improved, err := PrettifyEntry(string(content), mode)
	if err != nil {
		printError("Failed to prettify: %v", err)
		os.Exit(1)
	}

	// Apply or confirm
	if !autoApply {
		fmt.Println("\n" + Header("Preview of changes:"))
		fmt.Println(Dim("────────────────────────────────────────"))
		fmt.Println(improved)
		fmt.Println(Dim("────────────────────────────────────────"))
		fmt.Print("\n" + Highlight("Apply these changes? (y/N): "))
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted")
			return
		}
	}

	// Write improved content
	if err := os.WriteFile(entryPath, []byte(improved), 0644); err != nil {
		printError("Failed to write entry: %v", err)
		os.Exit(1)
	}

	// Update index
	entry, err := ParseEntry(entryPath)
	if err != nil {
		printWarning("Failed to parse entry after prettifying: %v", err)
	} else {
		AddToIndex(index, entry, kbPath)
		if err := SaveIndex(index, kbPath); err != nil {
			printWarning("Failed to update index: %v", err)
		}
	}

	printSuccess("Prettified: %s", indexEntry.Title)
}

func prettifyAll(index *Index, kbPath string, mode PrettyMode, autoApply bool) {
	if !autoApply {
		fmt.Printf(Warning("This will prettify ALL %d entries. Continue? (y/N): "), len(index.Entries))
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted")
			return
		}
	}

	printInfo("Prettifying all entries (%s mode)...", mode)

	successCount := 0
	failCount := 0

	for i, indexEntry := range index.Entries {
		entryPath := filepath.Join(kbPath, indexEntry.Path)

		fmt.Printf(Dim("[%d/%d] "), i+1, len(index.Entries))
		fmt.Printf("Processing: %s... ", indexEntry.Title)

		// Read current content
		content, err := os.ReadFile(entryPath)
		if err != nil {
			fmt.Println(Red("FAILED"))
			printError("  Failed to read: %v", err)
			failCount++
			continue
		}

		// Call Claude API
		improved, err := PrettifyEntry(string(content), mode)
		if err != nil {
			fmt.Println(Red("FAILED"))
			printError("  API error: %v", err)
			failCount++
			continue
		}

		// Write improved content
		if err := os.WriteFile(entryPath, []byte(improved), 0644); err != nil {
			fmt.Println(Red("FAILED"))
			printError("  Failed to write: %v", err)
			failCount++
			continue
		}

		// Update index entry
		entry, err := ParseEntry(entryPath)
		if err == nil {
			AddToIndex(index, entry, kbPath)
		}

		fmt.Println(Green("OK"))
		successCount++
	}

	// Save updated index
	if err := SaveIndex(index, kbPath); err != nil {
		printWarning("Failed to save index: %v", err)
	}

	fmt.Println()
	printSuccess("Completed: %d succeeded, %d failed", successCount, failCount)
}
