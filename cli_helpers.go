package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func buildEditorCommand(config *Config, kbPath, entryPath string) *exec.Cmd {
	editor := strings.TrimSpace(config.Editor)
	if editor == "" {
		editor = "vi"
	}

	nvimConfig := filepath.Join(kbPath, ".kb", "nvim.lua")
	editorBase := filepath.Base(editor)
	if editorBase == "nvim" {
		if _, err := os.Stat(nvimConfig); err == nil {
			return exec.Command(editor, "-u", nvimConfig, entryPath)
		}
	}

	return exec.Command(editor, entryPath)
}

func shouldAutoUpdateIndex(config *Config) bool {
	return config == nil || config.AutoUpdateIndex
}

func updateIndexWithEntry(config *Config, kbPath string, entry *Entry) error {
	if !shouldAutoUpdateIndex(config) {
		return nil
	}

	index, err := LoadIndex(kbPath)
	if err != nil {
		return err
	}

	AddToIndex(index, entry, kbPath)
	return SaveIndex(index, kbPath)
}

func removeEntryFromIndex(config *Config, kbPath, entryID string) error {
	if !shouldAutoUpdateIndex(config) {
		return nil
	}

	index, err := LoadIndex(kbPath)
	if err != nil {
		return err
	}

	RemoveFromIndex(index, entryID)
	return SaveIndex(index, kbPath)
}

func refreshIndexFromEntry(config *Config, index *Index, kbPath string, entry *Entry) error {
	if !shouldAutoUpdateIndex(config) {
		return nil
	}

	AddToIndex(index, entry, kbPath)
	return SaveIndex(index, kbPath)
}

func loadIndexedEntryCount(kbPath string) (int, error) {
	index, err := LoadIndex(kbPath)
	if err != nil {
		return 0, err
	}

	return len(SnapshotEntries(index)), nil
}

func resolvedPrettyProviderName(rawProvider string) (string, error) {
	provider, err := ResolvePrettyProvider(rawProvider)
	if err != nil {
		return "", err
	}

	return string(provider), nil
}

func warnIfIndexSkipped(config *Config) {
	if shouldAutoUpdateIndex(config) {
		return
	}

	printWarning("Index update skipped because auto_update_index is disabled")
}

func warnIfCleanCountUnavailable(err error) {
	if err == nil {
		return
	}

	printWarning("Could not read index before cleanup: %v", err)
}

func validatePrettyProviderOrExit(rawProvider string) PrettyProvider {
	provider, err := ResolvePrettyProvider(rawProvider)
	if err != nil {
		printError("%v", err)
		os.Exit(1)
	}

	return provider
}

func maybePrintOpenAISpend(config *Config) bool {
	usesOpenAI := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != ""
	if config != nil {
		provider, err := ResolvePrettyProvider(config.PrettyProvider)
		if err == nil {
			usesOpenAI = usesOpenAI || provider == ProviderChatGPT
		}
	}

	return usesOpenAI
}

func formatEntryCount(count int) string {
	return fmt.Sprintf("%d", count)
}
