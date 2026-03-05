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
		prettifyAll(index, kbPath, options)
		return
	}

	if options.Query == "" {
		fmt.Fprintln(os.Stderr, "Error: missing entry query")
		fmt.Fprintln(os.Stderr, "Usage: kb pretty <query>")
		fmt.Fprintln(os.Stderr, "       kb pretty --all")
		os.Exit(1)
	}

	prettifyOne(index, kbPath, options.Query, options)
}

func prettifyOne(index *Index, kbPath, query string, options *PrettyOptions) {
	indexEntry := FindEntryWithInference(index, query)
	if indexEntry == nil {
		printError("No entry found matching: %s", query)
		os.Exit(1)
	}

	entryPath := filepath.Join(kbPath, indexEntry.Path)

	printInfo("Prettifying: %s (%s mode, %s)", indexEntry.Title, options.Mode, options.Provider)

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

	if !options.AutoApply {
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

	if err := os.WriteFile(entryPath, []byte(improved), 0644); err != nil {
		printError("Failed to write entry: %v", err)
		os.Exit(1)
	}

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

func prettifyAll(index *Index, kbPath string, options *PrettyOptions) {
	entries := SnapshotEntries(index)

	if !options.AutoApply {
		fmt.Printf(Warning("This will prettify ALL %d entries. Continue? (y/N): "), len(entries))
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted")
			return
		}
	}

	printInfo("Prettifying all entries (%s mode, %s)...", options.Mode, options.Provider)

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

		if err := os.WriteFile(entryPath, []byte(improved), 0644); err != nil {
			fmt.Println(Red("FAILED"))
			printError("  Failed to write: %v", err)
			failCount++
			continue
		}

		entry, err := ParseEntry(entryPath)
		if err == nil {
			AddToIndex(index, entry, kbPath)
		}

		fmt.Println(Green("OK"))
		successCount++
	}

	if err := SaveIndex(index, kbPath); err != nil {
		printWarning("Failed to save index: %v", err)
	}

	fmt.Println()
	printSuccess("Completed: %d succeeded, %d failed", successCount, failCount)
}
