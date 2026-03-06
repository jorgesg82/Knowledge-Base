package main

import "testing"

func TestParseAIProviderAcceptsAliases(t *testing.T) {
	tests := []struct {
		input string
		want  AIProvider
	}{
		{input: "", want: ProviderAuto},
		{input: "auto", want: ProviderAuto},
		{input: "claude", want: ProviderClaude},
		{input: "chatgpt", want: ProviderChatGPT},
		{input: "openai", want: ProviderChatGPT},
	}

	for _, tt := range tests {
		got, err := ParseAIProvider(tt.input)
		if err != nil {
			t.Fatalf("ParseAIProvider(%q) returned error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("ParseAIProvider(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseAIProviderRejectsUnknownValues(t *testing.T) {
	if _, err := ParseAIProvider("gibberish"); err == nil {
		t.Fatal("expected ParseAIProvider to reject an unknown provider")
	}
}

func TestNormalizeOpenAIBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "root", raw: "https://api.openai.com", want: "https://api.openai.com/v1/"},
		{name: "root trailing slash", raw: "https://api.openai.com/", want: "https://api.openai.com/v1/"},
		{name: "already v1", raw: "https://example.com/v1", want: "https://example.com/v1/"},
		{name: "already v1 trailing slash", raw: "https://example.com/v1/", want: "https://example.com/v1/"},
	}

	for _, tt := range tests {
		if got := normalizeOpenAIBaseURL(tt.raw); got != tt.want {
			t.Fatalf("%s: normalizeOpenAIBaseURL(%q) = %q, want %q", tt.name, tt.raw, got, tt.want)
		}
	}
}

func TestParseCustomHeaders(t *testing.T) {
	headers, err := parseCustomHeaders("x-portkey-api-key: test-key\nx-extra: value")
	if err != nil {
		t.Fatalf("parseCustomHeaders failed: %v", err)
	}

	if headers["x-portkey-api-key"] != "test-key" {
		t.Fatalf("expected x-portkey-api-key header, got %#v", headers)
	}
	if headers["x-extra"] != "value" {
		t.Fatalf("expected x-extra header, got %#v", headers)
	}
}

func TestParseCustomHeadersRejectsInvalidLines(t *testing.T) {
	if _, err := parseCustomHeaders("not-a-header"); err == nil {
		t.Fatal("expected parseCustomHeaders to reject an invalid header line")
	}
}
