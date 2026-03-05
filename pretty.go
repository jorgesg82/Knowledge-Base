package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PrettyOptions struct {
	Mode       PrettyMode
	Provider   PrettyProvider
	AutoApply  bool
	DryRun     bool
	ShowDiff   bool
	ProcessAll bool
	Query      string
}

func parsePrettyOptions(args []string, config *Config) (*PrettyOptions, error) {
	mode, err := ParsePrettyMode(config.PrettyMode)
	if err != nil {
		return nil, err
	}

	provider, err := ParsePrettyProvider(config.PrettyProvider)
	if err != nil {
		return nil, err
	}

	options := &PrettyOptions{
		Mode:       mode,
		Provider:   provider,
		AutoApply:  config.PrettyAutoApply,
		ProcessAll: false,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--mode":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for --mode")
			}
			mode, err := ParsePrettyMode(args[i+1])
			if err != nil {
				return nil, err
			}
			options.Mode = mode
			i++
		case "--provider":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for --provider")
			}
			provider, err := ParsePrettyProvider(args[i+1])
			if err != nil {
				return nil, err
			}
			options.Provider = provider
			i++
		case "--confirm":
			options.AutoApply = false
		case "--dry-run":
			options.DryRun = true
		case "--diff":
			options.ShowDiff = true
		case "--all":
			options.ProcessAll = true
		default:
			if strings.HasPrefix(arg, "--") {
				return nil, fmt.Errorf("unknown flag: %s", arg)
			}
			if options.Query == "" {
				options.Query = arg
				continue
			}
			return nil, fmt.Errorf("unexpected extra argument: %s", arg)
		}
	}

	return options, nil
}

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

	options, err := parsePrettyOptions(args, config)
	if err != nil {
		printError("%v", err)
		os.Exit(1)
	}

	if options.ProcessAll {
		prettifyAll(config, index, kbPath, options)
		return
	}

	if options.Query == "" {
		fmt.Fprintln(os.Stderr, "Error: missing entry query")
		fmt.Fprintln(os.Stderr, "Usage: kb pretty <query>")
		fmt.Fprintln(os.Stderr, "       kb pretty --all")
		os.Exit(1)
	}

	prettifyOne(config, index, kbPath, options.Query, options)
}

func prettifyOne(config *Config, index *Index, kbPath, query string, options *PrettyOptions) {
	indexEntry := FindEntryWithInference(index, query)
	if indexEntry == nil {
		printError("No entry found matching: %s", query)
		os.Exit(1)
	}

	entryPath := filepath.Join(kbPath, indexEntry.Path)
	provider, err := resolvedPrettyProviderName(string(options.Provider))
	if err != nil {
		printError("%v", err)
		os.Exit(1)
	}

	printInfo("Prettifying: %s (%s mode, %s)", indexEntry.Title, options.Mode, provider)

	content, err := os.ReadFile(entryPath)
	if err != nil {
		printError("Failed to read entry: %v", err)
		os.Exit(1)
	}

	improved, err := PrettifyEntry(string(content), options.Mode, string(options.Provider))
	if err != nil {
		printError("Failed to prettify: %v", err)
		os.Exit(1)
	}

	previewPrettyResult(indexEntry.Title, string(content), improved, options)

	if strings.TrimSpace(improved) == strings.TrimSpace(string(content)) {
		printInfo("No changes suggested for %s", indexEntry.Title)
		return
	}

	if options.DryRun {
		printInfo("Dry run: no changes written")
		return
	}

	if !options.AutoApply {
		fmt.Print("\n" + Highlight("Apply these changes? (y/N): "))
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted")
			return
		}
	}

	if err := os.WriteFile(entryPath, []byte(improved), 0644); err != nil {
		printError("Failed to write entry: %v", err)
		os.Exit(1)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		printWarning("Failed to parse entry after prettifying: %v", err)
	} else {
		if err := refreshIndexFromEntry(config, index, kbPath, entry); err != nil {
			printWarning("Failed to update index: %v", err)
		}
	}
	warnIfIndexSkipped(config)

	printSuccess("Prettified: %s", indexEntry.Title)
}

func prettifyAll(config *Config, index *Index, kbPath string, options *PrettyOptions) {
	entries := SnapshotEntries(index)
	provider, err := resolvedPrettyProviderName(string(options.Provider))
	if err != nil {
		printError("%v", err)
		os.Exit(1)
	}

	if !options.AutoApply {
		fmt.Printf(Warning("This will prettify ALL %d entries. Continue? (y/N): "), len(entries))
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted")
			return
		}
	}

	printInfo("Prettifying all entries (%s mode, %s)...", options.Mode, provider)

	successCount := 0
	failCount := 0

	for i, indexEntry := range entries {
		entryPath := filepath.Join(kbPath, indexEntry.Path)

		fmt.Printf(Dim("[%d/%d] "), i+1, len(entries))
		fmt.Printf("Processing: %s... ", indexEntry.Title)

		content, err := os.ReadFile(entryPath)
		if err != nil {
			fmt.Println(Red("FAILED"))
			printError("  Failed to read: %v", err)
			failCount++
			continue
		}

		improved, err := PrettifyEntry(string(content), options.Mode, string(options.Provider))
		if err != nil {
			fmt.Println(Red("FAILED"))
			printError("  API error: %v", err)
			failCount++
			continue
		}

		previewPrettyResult(indexEntry.Title, string(content), improved, options)

		if strings.TrimSpace(improved) == strings.TrimSpace(string(content)) {
			fmt.Println(Gray("UNCHANGED"))
			successCount++
			continue
		}

		if options.DryRun {
			fmt.Println(Cyan("DRY-RUN"))
			successCount++
			continue
		}

		if err := os.WriteFile(entryPath, []byte(improved), 0644); err != nil {
			fmt.Println(Red("FAILED"))
			printError("  Failed to write: %v", err)
			failCount++
			continue
		}

		entry, err := ParseEntry(entryPath)
		if err == nil && shouldAutoUpdateIndex(config) {
			AddToIndex(index, entry, kbPath)
		}

		fmt.Println(Green("OK"))
		successCount++
	}

	if shouldAutoUpdateIndex(config) {
		if err := SaveIndex(index, kbPath); err != nil {
			printWarning("Failed to save index: %v", err)
		}
	} else {
		warnIfIndexSkipped(config)
	}

	fmt.Println()
	printSuccess("Completed: %d succeeded, %d failed", successCount, failCount)
}

func previewPrettyResult(title, current, improved string, options *PrettyOptions) {
	if !options.DryRun && !options.ShowDiff && options.AutoApply {
		return
	}

	fmt.Printf("\n%s %s\n", Header("Preview:"), Bold(title))
	if options.ShowDiff {
		diff, err := buildUnifiedDiff(current, improved)
		if err != nil {
			printWarning("Failed to render diff: %v", err)
		} else if strings.TrimSpace(diff) == "" {
			fmt.Println(Dim("No textual changes"))
		} else {
			fmt.Print(diff)
		}
		return
	}

	fmt.Println(Dim("────────────────────────────────────────"))
	fmt.Println(improved)
	fmt.Println(Dim("────────────────────────────────────────"))
}
