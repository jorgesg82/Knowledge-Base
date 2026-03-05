package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParsePrettyOptions(t *testing.T) {
	config := &Config{
		PrettyProvider:  "auto",
		PrettyMode:      "moderate",
		PrettyAutoApply: true,
	}

	options, err := parsePrettyOptions([]string{"entry-name", "--provider", "openai", "--mode", "aggressive", "--confirm"}, config)
	if err != nil {
		t.Fatalf("parsePrettyOptions failed: %v", err)
	}

	if options.Query != "entry-name" {
		t.Errorf("Expected query entry-name, got %s", options.Query)
	}

	if options.Provider != ProviderChatGPT {
		t.Errorf("Expected provider chatgpt, got %s", options.Provider)
	}

	if options.Mode != ModeAggressive {
		t.Errorf("Expected aggressive mode, got %s", options.Mode)
	}

	if options.AutoApply {
		t.Error("Expected --confirm to disable auto-apply")
	}
}

func TestParsePrettyProviderAuto(t *testing.T) {
	provider, err := ParsePrettyProvider("auto")
	if err != nil {
		t.Fatalf("ParsePrettyProvider failed: %v", err)
	}
	if provider != ProviderAuto {
		t.Fatalf("expected auto provider, got %s", provider)
	}
}

func TestPrettifyEntryClaude(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("Unexpected path: %s", r.URL.Path)
		}

		if got := r.Header.Get("x-portkey-api-key"); got != "test-portkey-key" {
			t.Fatalf("Expected custom header x-portkey-api-key, got %q", got)
		}

		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Fatalf("Expected anthropic-version header, got %q", got)
		}

		var reqBody ClaudeRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode Claude request: %v", err)
		}

		if reqBody.System == "" {
			t.Fatal("Expected Claude request to include system prompt")
		}

		if len(reqBody.Messages) != 1 || !strings.Contains(reqBody.Messages[0].Content, "Format this entry:") {
			t.Fatal("Expected Claude request to include formatting input")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"content":[{"text":"---\ntitle: Test\n---\n\n# Improved"}]}`))
	}))
	defer server.Close()

	t.Setenv("ANTHROPIC_BASE_URL", server.URL)
	t.Setenv("ANTHROPIC_MODEL", "claude-test")
	t.Setenv("ANTHROPIC_CUSTOM_HEADERS", "x-portkey-api-key: test-portkey-key")
	t.Setenv("ANTHROPIC_API_KEY", "")

	result, err := PrettifyEntry("---\ntitle: Test\n---\n\n# Test", ModeModerate, "claude")
	if err != nil {
		t.Fatalf("PrettifyEntry failed: %v", err)
	}

	if !strings.Contains(result, "# Improved") {
		t.Errorf("Expected Claude prettify result to contain improved content, got %s", result)
	}
}

func TestPrettifyEntryChatGPT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("Unexpected path: %s", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer test-openai-key" {
			t.Fatalf("Expected Bearer token, got %q", got)
		}

		if got := r.Header.Get("OpenAI-Project"); got != "proj_test" {
			t.Fatalf("Expected OpenAI-Project header proj_test, got %q", got)
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode OpenAI request: %v", err)
		}

		if reqBody["model"] != "gpt-test" {
			t.Fatalf("Expected model gpt-test, got %v", reqBody["model"])
		}

		if reqBody["instructions"] == "" {
			t.Fatal("Expected OpenAI request to include instructions")
		}

		input, ok := reqBody["input"].(string)
		if !ok || !strings.Contains(input, "Format this entry:") {
			t.Fatal("Expected OpenAI request to include formatting input")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"resp_123","object":"response","status":"completed","model":"gpt-test","output":[{"id":"msg_123","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"---\ntitle: Test\n---\n\n# Improved via ChatGPT","annotations":[]}]}]}`))
	}))
	defer server.Close()

	t.Setenv("OPENAI_BASE_URL", server.URL)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("OPENAI_MODEL", "gpt-test")
	t.Setenv("OPENAI_PROJECT_ID", "proj_test")

	normalizedBaseURL := normalizeOpenAIBaseURL(server.URL)
	parsedBaseURL, err := url.Parse(normalizedBaseURL)
	if err != nil {
		t.Fatalf("Failed to parse normalized base URL: %v", err)
	}
	if parsedBaseURL.Path != "/v1/" {
		t.Fatalf("Expected normalized base URL to end with /v1/, got %s", parsedBaseURL.String())
	}

	result, err := PrettifyEntry("---\ntitle: Test\n---\n\n# Test", ModeConservative, "chatgpt")
	if err != nil {
		t.Fatalf("PrettifyEntry failed: %v", err)
	}

	if !strings.Contains(result, "# Improved via ChatGPT") {
		t.Errorf("Expected ChatGPT prettify result to contain improved content, got %s", result)
	}
}

func TestPrettifyEntryChatGPTUsesDefaultModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode OpenAI request: %v", err)
		}
		if reqBody["model"] != defaultOpenAIModel() {
			t.Fatalf("expected default model %s, got %v", defaultOpenAIModel(), reqBody["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"resp_123","object":"response","status":"completed","model":"gpt-test","output":[{"id":"msg_123","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}]}`))
	}))
	defer server.Close()

	t.Setenv("OPENAI_BASE_URL", server.URL)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_PROJECT_ID", "")

	if _, err := PrettifyEntry("---\ntitle: Test\n---\n\n# Test", ModeConservative, "chatgpt"); err != nil {
		t.Fatalf("PrettifyEntry failed: %v", err)
	}
}
