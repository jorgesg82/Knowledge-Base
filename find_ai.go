package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

type findSynthesisInput struct {
	Query string                 `json:"query"`
	Notes []plannerCandidateNote `json:"notes"`
}

const findSynthesisMaxOutputTokens int64 = 900

func synthesizeFindAnswer(query string, candidates []scoredCanonicalNote, provider string) (string, string, string, error) {
	resolved, err := ResolveAIProvider(provider)
	if err != nil {
		return "", "", "", err
	}

	switch resolved {
	case ProviderChatGPT:
		content, model, err := synthesizeFindWithOpenAI(query, candidates)
		return content, string(resolved), model, err
	case ProviderClaude:
		content, model, err := synthesizeFindWithClaude(query, candidates)
		return content, string(resolved), model, err
	default:
		return "", "", "", fmt.Errorf("unsupported find provider: %s", provider)
	}
}

func synthesizeFindWithOpenAI(query string, candidates []scoredCanonicalNote) (string, string, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return "", "", fmt.Errorf("OPENAI_API_KEY not set in environment")
	}

	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		model = defaultOpenAIModel()
	}

	client := newOpenAIClient(apiKey, strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), openAIHTTPClient)
	resp, err := client.Responses.New(context.Background(), responses.ResponseNewParams{
		Model:        shared.ResponsesModel(model),
		Instructions: openai.String(findSynthesisSystemPrompt()),
		MaxOutputTokens: openai.Int(findSynthesisMaxOutputTokens),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(buildFindSynthesisInput(query, candidates)),
		},
	}, option.WithRequestTimeout(60*time.Second))
	if err != nil {
		return "", "", fmt.Errorf("failed to make OpenAI find request: %w", err)
	}

	text := strings.TrimSpace(resp.OutputText())
	if text == "" {
		return "", "", fmt.Errorf("empty response from OpenAI find request")
	}

	return text, resp.Model, nil
}

func synthesizeFindWithClaude(query string, candidates []scoredCanonicalNote) (string, string, error) {
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
		model = defaultClaudeModel()
	}

	reqBody := ClaudeRequest{
		Model:     model,
		MaxTokens: int(findSynthesisMaxOutputTokens),
		System:    findSynthesisSystemPrompt(),
		Messages: []ClaudeMessage{
			{
				Role:    "user",
				Content: buildFindSynthesisInput(query, candidates),
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal Claude find request: %w", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", fmt.Errorf("failed to create Claude find request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")))

	customHeaders := strings.TrimSpace(os.Getenv("ANTHROPIC_CUSTOM_HEADERS"))
	if customHeaders != "" {
		headers, err := parseCustomHeaders(customHeaders)
		if err != nil {
			return "", "", err
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}

	if req.Header.Get("x-api-key") == "" && customHeaders == "" {
		return "", "", fmt.Errorf("ANTHROPIC_API_KEY or ANTHROPIC_CUSTOM_HEADERS not set in environment")
	}

	body, statusCode, err := doClaudeRequest(req)
	if err != nil {
		return "", "", err
	}
	if statusCode < 200 || statusCode >= 300 {
		return "", "", fmt.Errorf("Claude find API error (status %d): %s", statusCode, string(body))
	}

	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", "", fmt.Errorf("failed to parse Claude find response: %w", err)
	}
	if len(claudeResp.Content) == 0 || strings.TrimSpace(claudeResp.Content[0].Text) == "" {
		return "", "", fmt.Errorf("empty response from Claude find API")
	}

	return strings.TrimSpace(claudeResp.Content[0].Text), model, nil
}

func findSynthesisSystemPrompt() string {
	return `You answer retrieval queries over a personal knowledge base.

Rules:
- Use only the provided notes.
- Do not invent facts not present in the notes.
- Return markdown only.
- Start with a short answer or summary.
- If there are multiple relevant notes, synthesize them clearly.
- If the notes are ambiguous or incomplete, say so.
- End with a "Sources" section listing the note titles and IDs used.`
}

func buildFindSynthesisInput(query string, candidates []scoredCanonicalNote) string {
	limit := minInt(len(candidates), 4)
	notes := make([]plannerCandidateNote, 0, limit)
	for i := 0; i < limit; i++ {
		note := candidates[i].note
		preview := strings.TrimSpace(note.Body)
		if len(preview) > 900 {
			preview = strings.TrimSpace(preview[:900]) + "..."
		}
		notes = append(notes, plannerCandidateNote{
			ID:          note.ID,
			Title:       note.Title,
			Aliases:     compactStrings(note.Aliases),
			Summary:     strings.TrimSpace(note.Summary),
			Topics:      compactStrings(note.Topics),
			BodyPreview: preview,
		})
	}

	payload := findSynthesisInput{
		Query: strings.TrimSpace(query),
		Notes: notes,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"query":%q}`, query)
	}

	return string(data)
}
