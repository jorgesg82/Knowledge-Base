package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClaudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []ClaudeMessage `json:"messages"`
}

type ClaudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

type PrettyMode string

const (
	ModeConservative PrettyMode = "conservative"
	ModeModerate     PrettyMode = "moderate"
	ModeAggressive   PrettyMode = "aggressive"
)

func getSystemPrompt(mode PrettyMode) string {
	basePrompt := `You are a technical documentation formatter for a personal knowledge base.
Your task is to improve the formatting and readability of technical notes while preserving all information.

CRITICAL RULES:
1. NEVER change or remove the YAML frontmatter (the --- delimited section at the top)
2. Preserve all technical content and commands exactly as written
3. Return ONLY the improved markdown content (including frontmatter)
4. Do not add conversational text or explanations outside the markdown

Output format:
---
[preserve exact frontmatter]
---

[improved content]
`

	switch mode {
	case ModeConservative:
		return basePrompt + `
Mode: CONSERVATIVE - Minimal changes
- Fix markdown syntax (headers, lists, code blocks)
- Add proper spacing between sections
- Ensure code blocks have language tags
- Fix bullet point formatting
- Do NOT add new content
- Do NOT rewrite or expand explanations`

	case ModeModerate:
		return basePrompt + `
Mode: MODERATE - Format + clarity improvements
- Fix markdown syntax (headers, lists, code blocks)
- Add proper spacing and structure
- Improve unclear sentences for better readability
- Add brief inline clarifications where ambiguous
- Organize content into logical sections if needed
- Keep expansions minimal and focused`

	case ModeAggressive:
		return basePrompt + `
Mode: AGGRESSIVE - Format + expand explanations
- Fix markdown syntax (headers, lists, code blocks)
- Add detailed explanations and context
- Include examples where helpful
- Add related tips and best practices
- Organize into well-structured sections
- Make it comprehensive and tutorial-like`
	}

	return basePrompt
}

func PrettifyEntry(content string, mode PrettyMode) (string, error) {
	apiKey := os.Getenv("ANTHROPIC_CUSTOM_HEADERS")
	if apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_CUSTOM_HEADERS not set in environment")
	}

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.portkey.ai"
	}

	model := os.Getenv("ANTHROPIC_DEFAULT_SONNET_MODEL")
	if model == "" {
		model = "@vertexai-global/anthropic.claude-sonnet-4-5@20250929"
	}

	systemPrompt := getSystemPrompt(mode)

	reqBody := ClaudeRequest{
		Model:     model,
		MaxTokens: 4096,
		Messages: []ClaudeMessage{
			{
				Role:    "user",
				Content: systemPrompt + "\n\nFormat this entry:\n\n" + content,
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Parse custom headers (format: "key: value")
	headerParts := strings.SplitN(apiKey, ":", 2)
	if len(headerParts) == 2 {
		req.Header.Set(strings.TrimSpace(headerParts[0]), strings.TrimSpace(headerParts[1]))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return claudeResp.Content[0].Text, nil
}
