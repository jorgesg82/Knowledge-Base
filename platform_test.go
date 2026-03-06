package main

import "testing"

func TestDetectAIProviderForPlatform(t *testing.T) {
	tests := []struct {
		name             string
		goos             string
		openAIConfigured bool
		claudeConfigured bool
		want             AIProvider
	}{
		{
			name:             "darwin prefers openai when both absent",
			goos:             "darwin",
			openAIConfigured: false,
			claudeConfigured: false,
			want:             ProviderChatGPT,
		},
		{
			name:             "linux prefers claude when both absent",
			goos:             "linux",
			openAIConfigured: false,
			claudeConfigured: false,
			want:             ProviderClaude,
		},
		{
			name:             "openai env wins when only openai configured",
			goos:             "linux",
			openAIConfigured: true,
			claudeConfigured: false,
			want:             ProviderChatGPT,
		},
		{
			name:             "claude env wins when only claude configured",
			goos:             "darwin",
			openAIConfigured: false,
			claudeConfigured: true,
			want:             ProviderClaude,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectAIProviderForPlatform(tt.goos, tt.openAIConfigured, tt.claudeConfigured)
			if got != tt.want {
				t.Fatalf("detectAIProviderForPlatform() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestResolveAIProviderAuto(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "linux"
	t.Cleanup(func() {
		currentGOOS = originalGOOS
	})

	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_CUSTOM_HEADERS", "")

	provider, err := ResolveAIProvider("auto")
	if err != nil {
		t.Fatalf("ResolveAIProvider failed: %v", err)
	}

	if provider != ProviderClaude {
		t.Fatalf("expected auto provider to resolve to claude on linux, got %s", provider)
	}
}
