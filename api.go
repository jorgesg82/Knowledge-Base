package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type AIProvider string

const (
	ProviderAuto    AIProvider = "auto"
	ProviderClaude  AIProvider = "claude"
	ProviderChatGPT AIProvider = "chatgpt"
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

var claudeHTTPClient = &http.Client{Timeout: 60 * time.Second}
var openAIHTTPClient = &http.Client{Timeout: 60 * time.Second}

func ParseAIProvider(provider string) (AIProvider, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", string(ProviderAuto):
		return ProviderAuto, nil
	case string(ProviderClaude):
		return ProviderClaude, nil
	case string(ProviderChatGPT), "openai":
		return ProviderChatGPT, nil
	default:
		return "", fmt.Errorf("invalid provider: %s. Use: auto, claude, or chatgpt", provider)
	}
}

func ResolveAIProvider(provider string) (AIProvider, error) {
	resolvedProvider, err := ParseAIProvider(provider)
	if err != nil {
		return "", err
	}

	if resolvedProvider == ProviderAuto {
		return detectAIProvider(), nil
	}

	return resolvedProvider, nil
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
