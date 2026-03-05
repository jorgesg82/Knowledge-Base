package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

type PrettyProvider string

const (
	ProviderClaude  PrettyProvider = "claude"
	ProviderChatGPT PrettyProvider = "chatgpt"
)

type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClaudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
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

var claudeHTTPClient = &http.Client{Timeout: 60 * time.Second}
var openAIHTTPClient = &http.Client{Timeout: 60 * time.Second}

func ParsePrettyMode(mode string) (PrettyMode, error) {
	switch PrettyMode(strings.ToLower(strings.TrimSpace(mode))) {
	case ModeConservative:
		return ModeConservative, nil
	case ModeModerate:
		return ModeModerate, nil
	case ModeAggressive:
		return ModeAggressive, nil
	default:
		return "", fmt.Errorf("invalid mode: %s. Use: conservative, moderate, or aggressive", mode)
	}
}

func ParsePrettyProvider(provider string) (PrettyProvider, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", string(ProviderClaude):
		return ProviderClaude, nil
	case string(ProviderChatGPT), "openai":
		return ProviderChatGPT, nil
	default:
		return "", fmt.Errorf("invalid provider: %s. Use: claude or chatgpt", provider)
	}
}

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

func PrettifyEntry(content string, mode PrettyMode, provider string) (string, error) {
	prettyProvider, err := ParsePrettyProvider(provider)
	if err != nil {
		return "", err
	}

	switch prettyProvider {
	case ProviderClaude:
		return prettifyWithClaude(content, mode)
	case ProviderChatGPT:
		return prettifyWithOpenAI(content, mode)
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

func prettifyWithClaude(content string, mode PrettyMode) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")), "/")
	if baseURL == "" {
		if strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "" {
			baseURL = "https://api.anthropic.com"
		} else {
			baseURL = "https://api.portkey.ai"
		}
	}

	model := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
	if model == "" {
		model = strings.TrimSpace(os.Getenv("ANTHROPIC_DEFAULT_SONNET_MODEL"))
	}
	if model == "" {
		model = "@vertexai-global/anthropic.claude-sonnet-4-5@20250929"
	}

	reqBody := ClaudeRequest{
		Model:     model,
		MaxTokens: 4096,
		System:    getSystemPrompt(mode),
		Messages: []ClaudeMessage{
			{
				Role:    "user",
				Content: "Format this entry:\n\n" + content,
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

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")))

	customHeaders := strings.TrimSpace(os.Getenv("ANTHROPIC_CUSTOM_HEADERS"))
	if customHeaders != "" {
		headers, err := parseCustomHeaders(customHeaders)
		if err != nil {
			return "", err
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}

	if req.Header.Get("x-api-key") == "" && customHeaders == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY or ANTHROPIC_CUSTOM_HEADERS not set in environment")
	}

	body, statusCode, err := doClaudeRequest(req)
	if err != nil {
		return "", err
	}

	if statusCode < 200 || statusCode >= 300 {
		return "", fmt.Errorf("API error (status %d): %s", statusCode, string(body))
	}

	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(claudeResp.Content) == 0 || strings.TrimSpace(claudeResp.Content[0].Text) == "" {
		return "", fmt.Errorf("empty response from API")
	}

	return claudeResp.Content[0].Text, nil
}

func prettifyWithOpenAI(content string, mode PrettyMode) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY not set in environment")
	}

	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		model = "gpt-5"
	}

	client := newOpenAIClient(apiKey, strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), openAIHTTPClient)
	resp, err := client.Responses.New(context.Background(), responses.ResponseNewParams{
		Model:        shared.ResponsesModel(model),
		Instructions: openai.String(getSystemPrompt(mode)),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String("Format this entry:\n\n" + content),
		},
	}, option.WithRequestTimeout(60*time.Second))
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}

	text := strings.TrimSpace(resp.OutputText())
	if text == "" {
		return "", fmt.Errorf("empty response from API")
	}

	return text, nil
}

func newOpenAIClient(apiKey, baseURL string, httpClient *http.Client) openai.Client {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}

	normalizedBaseURL := normalizeOpenAIBaseURL(baseURL)
	if normalizedBaseURL != "" {
		opts = append(opts, option.WithBaseURL(normalizedBaseURL))
	}

	if httpClient != nil {
		opts = append(opts, option.WithHTTPClient(httpClient))
	}

	if orgID := strings.TrimSpace(os.Getenv("OPENAI_ORG_ID")); orgID != "" {
		opts = append(opts, option.WithOrganization(orgID))
	}

	if projectID := strings.TrimSpace(os.Getenv("OPENAI_PROJECT_ID")); projectID != "" {
		opts = append(opts, option.WithProject(projectID))
	}

	return openai.NewClient(opts...)
}

func normalizeOpenAIBaseURL(rawBaseURL string) string {
	trimmed := strings.TrimSpace(rawBaseURL)
	if trimmed == "" {
		return ""
	}

	trimmed = strings.TrimRight(trimmed, "/")
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/"
	}

	return trimmed + "/v1/"
}

func parseCustomHeaders(rawHeaders string) (map[string]string, error) {
	headers := make(map[string]string)

	for _, line := range strings.Split(rawHeaders, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid ANTHROPIC_CUSTOM_HEADERS format: %s", line)
		}

		headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	return headers, nil
}

func doClaudeRequest(req *http.Request) ([]byte, int, error) {
	resp, err := claudeHTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return body, resp.StatusCode, nil
}
