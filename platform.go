package main

import (
	"os"
	"runtime"
	"strings"
)

var currentGOOS = runtime.GOOS

func hasOpenAIConfig() bool {
	return strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != ""
}

func hasClaudeConfig() bool {
	return strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "" ||
		strings.TrimSpace(os.Getenv("ANTHROPIC_CUSTOM_HEADERS")) != ""
}

func detectPrettyProviderForPlatform(goos string, openAIConfigured, claudeConfigured bool) PrettyProvider {
	switch {
	case openAIConfigured && !claudeConfigured:
		return ProviderChatGPT
	case claudeConfigured && !openAIConfigured:
		return ProviderClaude
	}

	switch goos {
	case "darwin":
		return ProviderChatGPT
	case "linux":
		return ProviderClaude
	default:
		if openAIConfigured {
			return ProviderChatGPT
		}
		return ProviderClaude
	}
}

func detectPrettyProvider() PrettyProvider {
	return detectPrettyProviderForPlatform(currentGOOS, hasOpenAIConfig(), hasClaudeConfig())
}

func defaultPrettyProvider() string {
	return string(ProviderAuto)
}

func defaultOpenAIModel() string {
	return "gpt-5-mini"
}

func defaultClaudeModel() string {
	return "claude-sonnet-4-6"
}
