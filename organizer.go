package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

type plannerCandidateNote struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Aliases     []string `json:"aliases,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Topics      []string `json:"topics,omitempty"`
	BodyPreview string   `json:"body_preview,omitempty"`
}

type addPlanningInput struct {
	Capture        string                 `json:"capture"`
	CandidateNotes []plannerCandidateNote `json:"candidate_notes,omitempty"`
}

type rankedCandidateNote struct {
	note        *CanonicalNote
	score       int
	overlap     float64
	sharedTerms int
}

const addPlannerMaxOutputTokens int64 = 1800

func planAddWithProvider(content string, candidates []*CanonicalNote, provider string) (*AddPlan, string, error) {
	resolved, err := ResolveAIProvider(provider)
	if err != nil {
		return nil, "", err
	}

	switch resolved {
	case ProviderChatGPT:
		return planAddWithOpenAI(content, candidates)
	case ProviderClaude:
		return planAddWithClaude(content, candidates)
	default:
		return nil, "", fmt.Errorf("unsupported add provider: %s", provider)
	}
}

func planAddWithOpenAI(content string, candidates []*CanonicalNote) (*AddPlan, string, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return nil, "", fmt.Errorf("OPENAI_API_KEY not set in environment")
	}

	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		model = defaultOpenAIModel()
	}

	client := newOpenAIClient(apiKey, strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), openAIHTTPClient)
	resp, err := client.Responses.New(context.Background(), responses.ResponseNewParams{
		Model:           shared.ResponsesModel(model),
		Instructions:    openai.String(addPlannerSystemPrompt()),
		MaxOutputTokens: openai.Int(addPlannerMaxOutputTokens),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(buildAddPlanningInput(content, candidates)),
		},
	}, option.WithRequestTimeout(60*time.Second))
	if err != nil {
		return nil, "", fmt.Errorf("failed to make OpenAI planning request: %w", err)
	}

	plan, err := decodeAddPlanResponse(resp.OutputText())
	if err != nil {
		return nil, "", err
	}

	return plan, resp.Model, nil
}

func planAddWithClaude(content string, candidates []*CanonicalNote) (*AddPlan, string, error) {
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
		MaxTokens: int(addPlannerMaxOutputTokens),
		System:    addPlannerSystemPrompt(),
		Messages: []ClaudeMessage{
			{
				Role:    "user",
				Content: buildAddPlanningInput(content, candidates),
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal Claude planning request: %w", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Claude planning request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")))

	customHeaders := strings.TrimSpace(os.Getenv("ANTHROPIC_CUSTOM_HEADERS"))
	if customHeaders != "" {
		headers, err := parseCustomHeaders(customHeaders)
		if err != nil {
			return nil, "", err
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}

	if req.Header.Get("x-api-key") == "" && customHeaders == "" {
		return nil, "", fmt.Errorf("ANTHROPIC_API_KEY or ANTHROPIC_CUSTOM_HEADERS not set in environment")
	}

	body, statusCode, err := doClaudeRequest(req)
	if err != nil {
		return nil, "", err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, "", fmt.Errorf("Claude planning API error (status %d): %s", statusCode, string(body))
	}

	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, "", fmt.Errorf("failed to parse Claude planning response: %w", err)
	}
	if len(claudeResp.Content) == 0 || strings.TrimSpace(claudeResp.Content[0].Text) == "" {
		return nil, "", fmt.Errorf("empty response from Claude planning API")
	}

	plan, err := decodeAddPlanResponse(claudeResp.Content[0].Text)
	if err != nil {
		return nil, "", err
	}

	return plan, model, nil
}

func addPlannerSystemPrompt() string {
	return `You organize captures for a personal knowledge base.

Return JSON only. Do not wrap it in markdown fences.

Allowed action types:
- create_note
- update_note

Schema:
{
  "summary": "short summary",
  "actions": [
    {
      "type": "create_note",
      "title": "canonical note title",
      "reason": "why this action is appropriate",
      "aliases_add": ["optional aliases"],
      "topics_set": ["optional topics"],
      "summary_set": "optional one-line summary",
      "body_markdown": "canonical markdown body for the new note"
    },
    {
      "type": "update_note",
      "note_id": "existing note id",
      "reason": "why this should update an existing note",
      "aliases_add": ["optional aliases"],
      "topics_set": ["optional topics"],
      "summary_set": "optional summary replacement",
      "body_append_markdown": "markdown to append to the existing note"
    }
  ]
}

Rules:
- Prefer update_note when the capture clearly expands an existing note.
- Prefer create_note when the capture introduces a distinct concept.
- Keep actions minimal and focused.
- Preserve technical facts from the capture.
- If unsure, create a new note instead of forcing a merge.
- Never return unknown fields.
- Never return actions outside the allowed schema.`
}

func buildAddPlanningInput(content string, candidates []*CanonicalNote) string {
	payload := addPlanningInput{
		Capture:        strings.TrimSpace(content),
		CandidateNotes: plannerCandidatesForPrompt(candidates),
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"capture":%q}`, content)
	}

	return string(data)
}

func plannerCandidatesForPrompt(notes []*CanonicalNote) []plannerCandidateNote {
	result := make([]plannerCandidateNote, 0, len(notes))
	for _, note := range notes {
		if note == nil {
			continue
		}
		preview := strings.TrimSpace(note.Body)
		if len(preview) > 600 {
			preview = strings.TrimSpace(preview[:600]) + "..."
		}
		result = append(result, plannerCandidateNote{
			ID:          note.ID,
			Title:       note.Title,
			Aliases:     compactStrings(note.Aliases),
			Summary:     strings.TrimSpace(note.Summary),
			Topics:      compactStrings(note.Topics),
			BodyPreview: preview,
		})
	}
	return result
}

func decodeAddPlanResponse(raw string) (*AddPlan, error) {
	jsonPayload, err := extractJSONObject(raw)
	if err != nil {
		return nil, err
	}

	var plan AddPlan
	if err := json.Unmarshal([]byte(jsonPayload), &plan); err != nil {
		return nil, fmt.Errorf("failed to decode add plan: %w", err)
	}

	return &plan, nil
}

func extractJSONObject(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("empty model response")
	}

	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start == -1 || end == -1 || end < start {
		return "", fmt.Errorf("model response did not contain a JSON object")
	}

	return trimmed[start : end+1], nil
}

func selectRelevantNotes(notes []*CanonicalNote, capture string, limit int) []*CanonicalNote {
	ranked := rankRelevantNotes(notes, capture)
	if len(ranked) == 0 || limit <= 0 {
		return nil
	}

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	result := make([]*CanonicalNote, 0, len(ranked))
	for _, item := range ranked {
		result = append(result, item.note)
	}

	return result
}

func rankRelevantNotes(notes []*CanonicalNote, capture string) []rankedCandidateNote {
	if len(notes) == 0 {
		return nil
	}

	scored := make([]rankedCandidateNote, 0, len(notes))
	for _, note := range notes {
		score := scoreNoteAgainstQuery(note, capture)
		if score <= 0 {
			continue
		}
		overlap, sharedTerms := tokenOverlapStats(note, capture)
		scored = append(scored, rankedCandidateNote{
			note:        note,
			score:       score,
			overlap:     overlap,
			sharedTerms: sharedTerms,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].note.UpdatedAt.After(scored[j].note.UpdatedAt)
		}
		return scored[i].score > scored[j].score
	})

	return scored
}

func scoreNoteAgainstQuery(note *CanonicalNote, query string) int {
	if note == nil {
		return 0
	}

	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return 0
	}

	score := 0
	if strings.Contains(strings.ToLower(note.Title), query) {
		score += 120
	}
	for _, alias := range note.Aliases {
		if strings.Contains(strings.ToLower(alias), query) {
			score += 90
		}
	}
	if strings.Contains(strings.ToLower(note.Summary), query) {
		score += 70
	}
	if strings.Contains(strings.ToLower(note.Body), query) {
		score += 40
	}

	queryTokens := tokenSet(query)
	if len(queryTokens) == 0 {
		return score
	}

	noteTokens := tokenSet(strings.Join([]string{
		note.Title,
		strings.Join(note.Aliases, " "),
		note.Summary,
		strings.Join(note.Topics, " "),
		note.Body,
	}, " "))

	for token := range queryTokens {
		if _, ok := noteTokens[token]; ok {
			score += 8
		}
	}

	return score
}

func tokenSet(raw string) map[string]struct{} {
	fields := strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= '0' && r <= '9':
			return false
		default:
			return true
		}
	})

	result := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if len(field) < 2 {
			continue
		}
		result[field] = struct{}{}
	}

	return result
}

func tokenOverlapStats(note *CanonicalNote, query string) (float64, int) {
	if note == nil {
		return 0, 0
	}

	queryTokens := tokenSet(query)
	if len(queryTokens) == 0 {
		return 0, 0
	}

	noteTokens := tokenSet(strings.Join([]string{
		note.Title,
		strings.Join(note.Aliases, " "),
		note.Summary,
		strings.Join(note.Topics, " "),
		note.Body,
	}, " "))

	shared := 0
	for token := range queryTokens {
		if _, ok := noteTokens[token]; ok {
			shared++
		}
	}

	return float64(shared) / float64(len(queryTokens)), shared
}

func planAddHeuristically(content string, candidates []*CanonicalNote) (*AddPlan, bool) {
	ranked := rankRelevantNotes(candidates, content)
	if len(ranked) == 0 {
		return nil, false
	}

	best := ranked[0]
	secondScore := 0
	if len(ranked) > 1 {
		secondScore = ranked[1].score
	}

	if best.score < 40 {
		return nil, false
	}
	if best.sharedTerms < 4 {
		return nil, false
	}
	if best.overlap < 0.55 {
		return nil, false
	}
	if len(ranked) > 1 && best.score-secondScore < 12 {
		return nil, false
	}

	if noteAlreadyContainsCapture(best.note, content) {
		return &AddPlan{
			Summary: fmt.Sprintf("Capture already covered by %s.", best.note.Title),
			Actions: nil,
		}, true
	}

	return &AddPlan{
		Summary: fmt.Sprintf("Merged capture into %s.", best.note.Title),
		Actions: []AddAction{
			{
				Type:               "update_note",
				NoteID:             best.note.ID,
				Reason:             "High-confidence lexical overlap with an existing canonical note.",
				AliasesAdd:         inferAliasesFromContent(content, best.note),
				TopicsSet:          best.note.Topics,
				BodyAppendMarkdown: renderCanonicalCaptureMarkdown(content, best.note.Title),
			},
		},
	}, true
}

func noteAlreadyContainsCapture(note *CanonicalNote, capture string) bool {
	if note == nil {
		return false
	}

	normalizedCapture := normalizeComparableText(capture)
	if normalizedCapture == "" {
		return true
	}

	return strings.Contains(normalizeComparableText(note.Body), normalizedCapture)
}

func normalizeComparableText(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	fields := strings.Fields(raw)
	return strings.Join(fields, " ")
}

func inferAliasesFromContent(content string, note *CanonicalNote) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	firstLine := cleanPotentialTitleLine(strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")[0])
	if firstLine == "" || len(firstLine) > 80 {
		return nil
	}
	if note != nil {
		if strings.EqualFold(firstLine, note.Title) {
			return nil
		}
		for _, alias := range note.Aliases {
			if strings.EqualFold(firstLine, alias) {
				return nil
			}
		}
	}

	if len(strings.Fields(firstLine)) < 2 {
		return nil
	}

	return []string{firstLine}
}
