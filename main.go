package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func printUsage() {
	fmt.Println(`kb - Personal Knowledge Base

Usage:
  kb init [path]              Initialize new KB (default: current directory)

Core workflow:
  kb add [text]               Capture knowledge quickly
  kb find [query]             Browse or retrieve notes

Maintenance:
  kb doctor                   Check local KB/runtime health
  kb stats                    Show KB statistics
  kb export [path]            Export KB to tarball
  kb import <tarball>         Import KB from tarball
  kb rebuild                  Rebuild derived metadata
  kb config                   Show configuration
  kb clean                    Remove KB from current directory

Removed commands:
  kb edit, kb rm, kb list, kb search, kb tag, kb tags, kb pretty, kb show

Add options:
  --file <path>               Capture content from a file
  --url <url>                 Fetch and capture content from a URL
  --clipboard                 Capture current clipboard contents
  --provider <name>           Set organizer provider: auto, claude, or chatgpt
  --dry-run                   Preview add plan without writing changes
  --json                      Print add output as JSON

Find options:
  --json                      Print selected note/candidates as JSON
  --raw                       Print raw markdown instead of rendering
  --synthesize                Answer from the top matching notes
  --provider <name>           Set synthesis provider: auto, claude, or chatgpt

Examples:
  kb init ~/my-kb
  kb add "how to inspect open ports on macos"
  kb add
  kb add --clipboard
  kb add --file ~/Downloads/snippet.txt
  kb add --url https://example.com/article
  kb find "open ports"
  kb find
  kb find --json "open ports"
  kb find --synthesize "ssh tunnel"
  kb export ~/backup/kb.tar.gz
  kb rebuild
  kb clean`)
}

func handleInit(args []string) {
	var kbPath string
	if len(args) > 0 {
		kbPath = args[0]
	} else {
		var err error
		kbPath, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	absPath, err := filepath.Abs(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	kbDir := filepath.Join(absPath, ".kb")
	if _, err := os.Stat(kbDir); err == nil {
		printWarning("KB already exists at %s", absPath)
		printInfo("This will reset derived metadata. Use 'kb rebuild' to rebuild it from the stored notes and entries.")
		fmt.Print("Continue? [y/N]: ")

		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))

		if response != "y" && response != "yes" {
			fmt.Println("Aborted")
			os.Exit(0)
		}
	}

	if err := os.MkdirAll(filepath.Join(absPath, "entries"), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directories: %v\n", err)
		os.Exit(1)
	}

	config := GetDefaultConfig(absPath)
	if err := SaveConfig(config, absPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	index := &Index{Entries: []IndexEntry{}}
	if err := SaveIndex(index, absPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving index: %v\n", err)
		os.Exit(1)
	}

	if err := ensureStoreLayout(absPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing KB storage: %v\n", err)
		os.Exit(1)
	}
	if _, err := loadOrInitKBState(absPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing KB state: %v\n", err)
		os.Exit(1)
	}

	printSuccess("Initialized KB at %s", absPath)
	printInfo("Use 'kb add' to capture your first note")
}

func handleAdd(args []string) {
	handleAddCommand(args)
}

func handleFind(args []string) {
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

	options, err := parseFindOptions(args, config)
	if err != nil {
		printError("%v", err)
		fmt.Fprintln(os.Stderr, "Usage: kb find [query]")
		os.Exit(1)
	}

	if options.Query == "" {
		note, candidates, topics, err := resolveBrowseFind(kbPath, options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading notes: %v\n", err)
			os.Exit(1)
		}

		if note != nil {
			if err := renderResolvedFindNote(kbPath, config, options, note, candidates); err != nil {
				printError("Failed to open note: %v", err)
				os.Exit(1)
			}
			return
		}

		if options.JSON {
			if err := printFindJSON(buildBrowseFindResult(options, nil, candidates, topics)); err != nil {
				printError("Failed to render find JSON: %v", err)
				os.Exit(1)
			}
			return
		}

		if isTerminal(os.Stdin) && isTerminal(os.Stdout) {
			if err := browseNotesInteractively(kbPath, config, options, candidates, topics); err != nil {
				printError("Failed to browse notes: %v", err)
				os.Exit(1)
			}
			return
		}

		printBrowseNotes(candidates, topics)
		return
	}

	note, candidates, _, err := resolveCanonicalFind(kbPath, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading notes: %v\n", err)
		os.Exit(1)
	}

	if options.Synthesize && len(candidates) > 0 {
		summary, providerName, model, err := synthesizeFindAnswer(options.Query, candidates, string(options.Provider))
		if err != nil {
			printWarning("Find synthesis unavailable: %v", err)
		} else {
			result := buildFindResult(options, note, candidates, "synthesized", providerName, model, summary)
			if options.JSON {
				if err := printFindJSON(result); err != nil {
					printError("Failed to render find JSON: %v", err)
					os.Exit(1)
				}
			} else {
				if err := showMarkdownWithBuiltinRenderer(summary); err != nil {
					printError("Failed to render synthesized answer: %v", err)
					os.Exit(1)
				}
			}
			return
		}
	}

	if note != nil {
		if err := renderResolvedFindNote(kbPath, config, options, note, candidates); err != nil {
			printError("Failed to open note: %v", err)
			os.Exit(1)
		}
		return
	}

	if len(candidates) > 0 {
		if options.JSON {
			result := buildFindResult(options, nil, candidates, "candidates", "", "", "")
			if err := printFindJSON(result); err != nil {
				printError("Failed to render find JSON: %v", err)
				os.Exit(1)
			}
			return
		}

		printFindCandidates(options.Query, candidates)
		os.Exit(1)
	}
	printError("No note found matching: %s", options.Query)
	os.Exit(1)
}

func handleRemovedCommand(command string) {
	printError("`kb %s` was removed from the current workflow. Use `kb add` / `kb find`, or use the v1 release if you need the old workflow.", command)
	os.Exit(1)
}

func handleExport(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var exportPath string
	if len(args) > 0 {
		exportPath = args[0]
	} else {
		exportPath = "kb-export.tar.gz"
	}

	entryCount, err := ExportKB(kbPath, exportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating export: %v\n", err)
		os.Exit(1)
	}

	printSuccess("Exported %d entries to %s", entryCount, exportPath)
}

func handleImport(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: missing tarball path")
		fmt.Fprintln(os.Stderr, "Usage: kb import <tarball>")
		os.Exit(1)
	}

	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	count, err := ImportKB(kbPath, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error importing tarball: %v\n", err)
		os.Exit(1)
	}

	printSuccess("Imported %d entries successfully", count)
}

func handleStats(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	stats, err := loadStoreStats(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading store stats: %v\n", err)
		os.Exit(1)
	}

	config, _ := LoadConfig(kbPath)

	fmt.Println(Header("KB Statistics:"))
	fmt.Printf("  Total notes: %s\n", Cyan(fmt.Sprintf("%d", stats.Notes)))
	fmt.Printf("  Captures: %s\n", Cyan(fmt.Sprintf("%d", stats.Captures)))
	fmt.Printf("  Operations: %s\n", Cyan(fmt.Sprintf("%d", stats.Operations)))
	fmt.Printf("  Topics: %s\n", Cyan(fmt.Sprintf("%d", stats.Topics)))
	fmt.Printf("  Aliases: %s\n", Cyan(fmt.Sprintf("%d", stats.Aliases)))
	if stats.LastUpdated.IsZero() {
		fmt.Printf("  Last updated: %s\n", Dim("never"))
	} else {
		fmt.Printf("  Last updated: %s\n", Dim(stats.LastUpdated.Format("2006-01-02 15:04:05")))
	}

	if len(stats.NotesByCategory) > 0 {
		fmt.Printf("\n" + Bold("Notes by category:") + "\n")
		type catCount struct {
			cat   string
			count int
		}
		var cats []catCount
		for cat, count := range stats.NotesByCategory {
			cats = append(cats, catCount{cat, count})
		}
		sort.Slice(cats, func(i, j int) bool {
			return cats[i].count > cats[j].count
		})
		for _, cc := range cats {
			fmt.Printf("  %s: %s\n", Cyan(cc.cat), Yellow(fmt.Sprintf("%d", cc.count)))
		}
	}

	printOpenAISpendStats(config)
	printAnthropicSpendStats(config)
}

func printOpenAISpendStats(config *Config) {
	summary, err := FetchOpenAISpendSummary(time.Now())
	if err == nil {
		fmt.Printf("\n" + Bold("OpenAI API spend:") + "\n")
		fmt.Printf("  Total: %s\n", Cyan(formatCurrencyAmount(summary.Total, summary.Currency)))
		fmt.Printf("  Last 30 days: %s\n", Cyan(formatCurrencyAmount(summary.Last30Days, summary.Currency)))
		fmt.Printf("  Today: %s\n", Cyan(formatCurrencyAmount(summary.Today, summary.Currency)))
		return
	}

	if errors.Is(err, ErrOpenAIAdminKeyNotSet) {
		if maybePrintOpenAISpend(config) {
			fmt.Printf("\n" + Bold("OpenAI API spend:") + "\n")
			fmt.Printf("  %s\n", Dim("Unavailable. Set OPENAI_ADMIN_KEY to query organization costs."))
		}
		return
	}

	fmt.Printf("\n" + Bold("OpenAI API spend:") + "\n")
	fmt.Printf("  %s\n", Yellow(fmt.Sprintf("Unavailable (%v)", err)))
}

func printAnthropicSpendStats(config *Config) {
	summary, err := FetchAnthropicSpendSummary(time.Now())
	if err == nil {
		fmt.Printf("\n" + Bold("Anthropic API spend:") + "\n")
		fmt.Printf("  Total: %s\n", Cyan(formatCurrencyAmount(summary.Total, summary.Currency)))
		fmt.Printf("  Last 30 days: %s\n", Cyan(formatCurrencyAmount(summary.Last30Days, summary.Currency)))
		fmt.Printf("  Today: %s\n", Cyan(formatCurrencyAmount(summary.Today, summary.Currency)))
		return
	}

	if errors.Is(err, ErrAnthropicAdminKeyNotSet) {
		if maybePrintAnthropicSpend(config) {
			fmt.Printf("\n" + Bold("Anthropic API spend:") + "\n")
			fmt.Printf("  %s\n", Dim("Unavailable. Set ANTHROPIC_ADMIN_KEY to query organization costs."))
		}
		return
	}

	fmt.Printf("\n" + Bold("Anthropic API spend:") + "\n")
	fmt.Printf("  %s\n", Yellow(fmt.Sprintf("Unavailable (%v)", err)))
}

func handleConfig(args []string) {
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

	fmt.Println(Header("Configuration:"))
	fmt.Printf("  KB Path: %s\n", Cyan(config.KBPath))
	fmt.Printf("  Editor: %s\n", Cyan(config.Editor))
	fmt.Printf("  Viewer: %s\n", Cyan(config.Viewer))
	fmt.Printf("  Default Category: %s\n", Cyan(config.DefaultCategory))
	fmt.Printf("  Auto Update Index: %s\n", Cyan(fmt.Sprintf("%t", config.AutoUpdateIndex)))
	fmt.Printf("  AI Provider: %s\n", Cyan(config.AIProvider))
	if resolvedProvider, err := ResolveAIProvider(config.AIProvider); err == nil && resolvedProvider != AIProvider(config.AIProvider) {
		fmt.Printf("  AI Provider Resolved: %s\n", Cyan(string(resolvedProvider)))
	}
}

func handleRebuild(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Rebuilding derived metadata...")
	var noteCount, entryCount int
	err = withKBLock(kbPath, func() error {
		notes, err := loadCanonicalNotesFromDisk(kbPath)
		if err != nil {
			return fmt.Errorf("error loading canonical notes: %w", err)
		}

		if err := os.MkdirAll(filepath.Join(kbPath, "entries"), 0755); err != nil {
			return fmt.Errorf("error creating entries directory: %w", err)
		}

		for _, note := range notes {
			if note == nil {
				continue
			}
			if err := materializeCanonicalNote(kbPath, note); err != nil {
				return fmt.Errorf("error materializing note %s: %w", note.ID, err)
			}
			if err := saveCanonicalNoteRecord(kbPath, note); err != nil {
				return fmt.Errorf("error saving canonical note %s: %w", note.ID, err)
			}
		}

		manifest := canonicalNoteManifestEntriesFromNotes(notes)
		if err := saveCanonicalNotesManifest(kbPath, manifest); err != nil {
			return fmt.Errorf("error saving notes manifest: %w", err)
		}

		index, err := RebuildIndex(kbPath)
		if err != nil {
			return fmt.Errorf("error rebuilding index: %w", err)
		}
		if err := SaveIndex(index, kbPath); err != nil {
			return fmt.Errorf("error saving index: %w", err)
		}

		noteCount = len(notes)
		entryCount = len(index.Entries)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	printSuccess("Rebuilt derived metadata for %d notes and %d entries", noteCount, entryCount)
}

func handleClean(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	entryCount, err := loadCanonicalNoteCount(kbPath)
	if err != nil {
		warnIfCleanCountUnavailable(err)
		entryCount = 0
	}

	printWarning("This will delete the KB at %s", kbPath)
	if entryCount > 0 {
		printWarning("This KB contains %d notes that will be lost!", entryCount)
	}
	fmt.Print("Are you sure? Type 'yes' to confirm: ")

	var response string
	fmt.Scanln(&response)
	response = strings.TrimSpace(response)

	if response != "yes" {
		fmt.Println("Aborted")
		os.Exit(0)
	}

	kbDir := filepath.Join(kbPath, ".kb")
	if err := os.RemoveAll(kbDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing .kb directory: %v\n", err)
		os.Exit(1)
	}

	entriesDir := filepath.Join(kbPath, "entries")
	if _, err := os.Stat(entriesDir); err == nil {
		if err := os.RemoveAll(entriesDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing entries directory: %v\n", err)
			os.Exit(1)
		}
	}

	printSuccess("Removed KB from %s", kbPath)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "init":
		handleInit(args)
	case "add":
		handleAdd(args)
	case "edit":
		handleRemovedCommand(command)
	case "find":
		handleFind(args)
	case "show":
		handleRemovedCommand(command)
	case "rm":
		handleRemovedCommand(command)
	case "list":
		handleRemovedCommand(command)
	case "search":
		handleRemovedCommand(command)
	case "tag":
		handleRemovedCommand(command)
	case "tags":
		handleRemovedCommand(command)
	case "export":
		handleExport(args)
	case "import":
		handleImport(args)
	case "stats":
		handleStats(args)
	case "config":
		handleConfig(args)
	case "doctor":
		handleDoctor(args)
	case "rebuild":
		handleRebuild(args)
	case "clean":
		handleClean(args)
	case "pretty":
		handleRemovedCommand(command)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}
