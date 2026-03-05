package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func printUsage() {
	fmt.Println(`kb - Personal Knowledge Base

Usage:
  kb init [path]              Initialize new KB (default: current directory)
  kb add [category/]title     Create new entry
  kb edit <query>             Edit entry (by ID or title)
  kb show <query>             Show entry content (with markdown viewer)
  kb rm <query>               Remove entry
  kb list [category]          List all entries or by category
  kb search <text>            Full-text search
  kb tag <tag> [<tag2>...]    Filter by tags
  kb tags                     List all tags with counts
  kb pretty <query>           Prettify entry with AI formatting
  kb pretty --all             Prettify all entries
  kb export [path]            Export KB to tarball
  kb import <tarball>         Import KB from tarball
  kb stats                    Show KB statistics
  kb config                   Show configuration
  kb rebuild                  Rebuild index from entries
  kb clean                    Remove KB from current directory

Pretty options:
  --mode <mode>               Set mode: conservative, moderate, aggressive
  --provider <name>           Set AI provider: claude or chatgpt
  --confirm                   Ask for confirmation before applying

Examples:
  kb init ~/my-kb
  kb add linux/ssh-tunneling
  kb show ssh-tunneling
  kb tag linux networking
  kb search "port forwarding"
  kb pretty ssh --mode conservative
  kb pretty ssh --provider chatgpt
  kb pretty --all
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
		printInfo("This will reset the index. Use 'kb rebuild' to rebuild from existing entries.")
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

	printSuccess("Initialized KB at %s", absPath)
	printInfo("Use 'kb add <category/title>' to create your first entry")
}

func handleAdd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: missing entry name")
		fmt.Fprintln(os.Stderr, "Usage: kb add [category/]title")
		os.Exit(1)
	}

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

	parts := strings.SplitN(args[0], "/", 2)
	var category, title string
	if len(parts) == 2 {
		category = parts[0]
		title = parts[1]
	} else {
		category = config.DefaultCategory
		title = parts[0]
	}

	entryPath := GetEntryPath(kbPath, category, title)

	if _, err := os.Stat(entryPath); err == nil {
		fmt.Fprintf(os.Stderr, "Error: entry already exists at %s\n", entryPath)
		os.Exit(1)
	}

	entry := CreateEntryTemplate(category, title)
	if err := WriteEntry(entry, entryPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating entry: %v\n", err)
		os.Exit(1)
	}

	// Use nvim with custom kb config
	nvimConfig := filepath.Join(kbPath, ".kb", "nvim.lua")
	var cmd *exec.Cmd

	// Check if custom nvim config exists
	if _, err := os.Stat(nvimConfig); err == nil {
		// Use nvim with custom config
		cmd = exec.Command("nvim", "-u", nvimConfig, entryPath)
	} else {
		// Fallback to configured editor
		cmd = exec.Command(config.Editor, entryPath)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening editor: %v\n", err)
		os.Exit(1)
	}

	entry, err = ParseEntry(entryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse entry after editing: %v\n", err)
		return
	}

	index, err := LoadIndex(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading index: %v\n", err)
		os.Exit(1)
	}

	AddToIndex(index, entry, kbPath)
	if err := SaveIndex(index, kbPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving index: %v\n", err)
		os.Exit(1)
	}

	printSuccess("Created entry: %s/%s", category, entry.ID)
}

func handleEdit(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: missing query")
		fmt.Fprintln(os.Stderr, "Usage: kb edit <query>")
		os.Exit(1)
	}

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

	query := args[0]
	indexEntry := FindEntryWithInference(index, query)
	if indexEntry == nil {
		printError("No entry found matching: %s", query)
		os.Exit(1)
	}

	entryPath := filepath.Join(kbPath, indexEntry.Path)

	// Use nvim with custom kb config
	nvimConfig := filepath.Join(kbPath, ".kb", "nvim.lua")
	var cmd *exec.Cmd

	// Check if custom nvim config exists
	if _, err := os.Stat(nvimConfig); err == nil {
		// Use nvim with custom config
		cmd = exec.Command("nvim", "-u", nvimConfig, entryPath)
	} else {
		// Fallback to configured editor
		cmd = exec.Command(config.Editor, entryPath)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening editor: %v\n", err)
		os.Exit(1)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse entry after editing: %v\n", err)
		return
	}

	AddToIndex(index, entry, kbPath)
	if err := SaveIndex(index, kbPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving index: %v\n", err)
		os.Exit(1)
	}

	printSuccess("Entry updated")
}

func handleShow(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: missing query")
		fmt.Fprintln(os.Stderr, "Usage: kb show <query>")
		os.Exit(1)
	}

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

	query := args[0]
	indexEntry := FindEntryWithInference(index, query)
	if indexEntry == nil {
		printError("No entry found matching: %s", query)
		os.Exit(1)
	}

	entryPath := filepath.Join(kbPath, indexEntry.Path)

	// Configure viewer with appropriate pager flags
	var cmd *exec.Cmd
	viewerBase := filepath.Base(config.Viewer)

	switch viewerBase {
	case "glow":
		cmd = exec.Command(config.Viewer, "-p", entryPath)
	case "bat":
		cmd = exec.Command(config.Viewer, "--paging=always", entryPath)
	case "mdcat":
		// mdcat doesn't have a built-in pager, pipe to less
		mdcatCmd := exec.Command(config.Viewer, entryPath)
		lessCmd := exec.Command("less", "-R")

		pipe, err := mdcatCmd.StdoutPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating pipe: %v\n", err)
			os.Exit(1)
		}

		lessCmd.Stdin = pipe
		lessCmd.Stdout = os.Stdout
		lessCmd.Stderr = os.Stderr
		mdcatCmd.Stderr = os.Stderr

		if err := lessCmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting less: %v\n", err)
			os.Exit(1)
		}
		if err := mdcatCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running mdcat: %v\n", err)
			os.Exit(1)
		}
		pipe.Close()
		if err := lessCmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "Error waiting for less: %v\n", err)
			os.Exit(1)
		}
		return
	default:
		// For less, mdless, or any other viewer, just run it normally
		cmd = exec.Command(config.Viewer, entryPath)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening viewer: %v\n", err)
		os.Exit(1)
	}
}

func handleRm(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: missing query")
		fmt.Fprintln(os.Stderr, "Usage: kb rm <query>")
		os.Exit(1)
	}

	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	index, err := LoadIndex(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading index: %v\n", err)
		os.Exit(1)
	}

	query := args[0]
	indexEntry := FindEntryWithInference(index, query)
	if indexEntry == nil {
		printError("No entry found matching: %s", query)
		os.Exit(1)
	}

	entryPath := filepath.Join(kbPath, indexEntry.Path)
	if err := os.Remove(entryPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing entry: %v\n", err)
		os.Exit(1)
	}

	RemoveFromIndex(index, indexEntry.ID)
	if err := SaveIndex(index, kbPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving index: %v\n", err)
		os.Exit(1)
	}

	printSuccess("Removed entry: %s", indexEntry.ID)
}

func handleList(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	index, err := LoadIndex(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading index: %v\n", err)
		os.Exit(1)
	}

	var results []IndexEntry
	if len(args) > 0 {
		category := args[0]
		results = SearchByCategory(index, category)
	} else {
		results = index.Entries
	}

	PrintSearchResults(results, "")
}

func handleSearch(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: missing search text")
		fmt.Fprintln(os.Stderr, "Usage: kb search <text>")
		os.Exit(1)
	}

	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	index, err := LoadIndex(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading index: %v\n", err)
		os.Exit(1)
	}

	text := strings.Join(args, " ")
	results, err := SearchByText(index, kbPath, text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error searching: %v\n", err)
		os.Exit(1)
	}

	PrintTextSearchResults(results)
}

func handleTag(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: missing tag")
		fmt.Fprintln(os.Stderr, "Usage: kb tag <tag> [<tag2>...]")
		os.Exit(1)
	}

	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	index, err := LoadIndex(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading index: %v\n", err)
		os.Exit(1)
	}

	results := SearchByTags(index, args)
	PrintSearchResults(results, "")
}

func handleTags(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	index, err := LoadIndex(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading index: %v\n", err)
		os.Exit(1)
	}

	tagCounts := GetAllTags(index)
	if len(tagCounts) == 0 {
		fmt.Println(Dim("No tags found"))
		return
	}

	type tagCount struct {
		tag   string
		count int
	}
	var tags []tagCount
	for tag, count := range tagCounts {
		tags = append(tags, tagCount{tag, count})
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].count > tags[j].count
	})

	fmt.Printf(Header("Tags (%d total):")+"\n", len(tags))
	for _, tc := range tags {
		fmt.Printf("  %s %s\n", Cyan(tc.tag), Dim(fmt.Sprintf("(%d)", tc.count)))
	}
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

	index, err := LoadIndex(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading index: %v\n", err)
		os.Exit(1)
	}

	config, _ := LoadConfig(kbPath)
	tagCounts := GetAllTags(index)
	categoryCounts := GetAllCategories(index)

	fmt.Println(Header("KB Statistics:"))
	fmt.Printf("  Total entries: %s\n", Cyan(fmt.Sprintf("%d", len(index.Entries))))
	fmt.Printf("  Categories: %s\n", Cyan(fmt.Sprintf("%d", len(categoryCounts))))
	fmt.Printf("  Tags: %s\n", Cyan(fmt.Sprintf("%d", len(tagCounts))))
	fmt.Printf("  Last updated: %s\n", Dim(index.LastUpdated.Format("2006-01-02 15:04:05")))

	if len(categoryCounts) > 0 {
		fmt.Printf("\n" + Bold("Entries by category:") + "\n")
		type catCount struct {
			cat   string
			count int
		}
		var cats []catCount
		for cat, count := range categoryCounts {
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
		usesOpenAI := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != ""
		if config != nil {
			usesOpenAI = usesOpenAI || config.PrettyProvider == string(ProviderChatGPT)
		}
		if usesOpenAI {
			fmt.Printf("\n" + Bold("OpenAI API spend:") + "\n")
			fmt.Printf("  %s\n", Dim("Unavailable. Set OPENAI_ADMIN_KEY to query organization costs."))
		}
		return
	}

	fmt.Printf("\n" + Bold("OpenAI API spend:") + "\n")
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
	fmt.Printf("  Pretty Provider: %s\n", Cyan(config.PrettyProvider))
	fmt.Printf("  Pretty Mode: %s\n", Cyan(config.PrettyMode))
	fmt.Printf("  Pretty Auto Apply: %s\n", Cyan(fmt.Sprintf("%t", config.PrettyAutoApply)))
}

func handleRebuild(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Rebuilding index...")
	index, err := RebuildIndex(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error rebuilding index: %v\n", err)
		os.Exit(1)
	}

	if err := SaveIndex(index, kbPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving index: %v\n", err)
		os.Exit(1)
	}

	printSuccess("Rebuilt index with %d entries", len(index.Entries))
}

func handleClean(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	index, _ := LoadIndex(kbPath)
	entryCount := len(index.Entries)

	printWarning("This will delete the KB at %s", kbPath)
	if entryCount > 0 {
		printWarning("This KB contains %d entries that will be lost!", entryCount)
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
		handleEdit(args)
	case "show":
		handleShow(args)
	case "rm":
		handleRm(args)
	case "list":
		handleList(args)
	case "search":
		handleSearch(args)
	case "tag":
		handleTag(args)
	case "tags":
		handleTags(args)
	case "export":
		handleExport(args)
	case "import":
		handleImport(args)
	case "stats":
		handleStats(args)
	case "config":
		handleConfig(args)
	case "rebuild":
		handleRebuild(args)
	case "clean":
		handleClean(args)
	case "pretty":
		handlePretty(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}
