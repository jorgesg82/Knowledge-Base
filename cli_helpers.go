package main

import (
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

func warnIfCleanCountUnavailable(err error) {
	if err == nil {
		return
	}

	printWarning("Could not read KB store before cleanup: %v", err)
}

func maybePrintOpenAISpend(config *Config) bool {
	usesOpenAI := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != ""
	if config != nil {
		provider, err := ResolveAIProvider(config.AIProvider)
		if err == nil {
			usesOpenAI = usesOpenAI || provider == ProviderChatGPT
		}
	}

	return usesOpenAI
}

func maybePrintAnthropicSpend(config *Config) bool {
	usesAnthropic := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "" ||
		strings.TrimSpace(os.Getenv("ANTHROPIC_CUSTOM_HEADERS")) != "" ||
		strings.TrimSpace(os.Getenv("ANTHROPIC_ADMIN_KEY")) != "" ||
		strings.TrimSpace(os.Getenv("ANTHROPIC_ADMIN_API_KEY")) != ""
	if config != nil {
		provider, err := ResolveAIProvider(config.AIProvider)
		if err == nil {
			usesAnthropic = usesAnthropic || provider == ProviderClaude
		}
	}

	return usesAnthropic
}
