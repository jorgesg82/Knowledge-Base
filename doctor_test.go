package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectDoctorChecksForOpenAI(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".kb"), 0755); err != nil {
		t.Fatalf("Failed to create .kb directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "entries"), 0755); err != nil {
		t.Fatalf("Failed to create entries directory: %v", err)
	}

	index := &Index{Entries: []IndexEntry{}}
	if err := SaveIndex(index, tmpDir); err != nil {
		t.Fatalf("Failed to save index: %v", err)
	}

	config := &Config{
		Editor:         "sh",
		Viewer:         "cat",
		PrettyProvider: "chatgpt",
	}

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_PROJECT_ID", "proj_test")

	checks := collectDoctorChecks(tmpDir, config)

	var foundProvider, foundOpenAI bool
	for _, check := range checks {
		switch check.Name {
		case "Pretty provider":
			foundProvider = true
			if !strings.Contains(check.Detail, "resolved=chatgpt") {
				t.Fatalf("unexpected provider detail: %s", check.Detail)
			}
		case "OpenAI":
			foundOpenAI = true
			if !check.OK {
				t.Fatalf("expected OpenAI check to pass, got %+v", check)
			}
			if !strings.Contains(check.Detail, "project=proj_test") {
				t.Fatalf("expected project id in detail, got %s", check.Detail)
			}
		}
	}

	if !foundProvider || !foundOpenAI {
		t.Fatalf("expected provider and OpenAI checks, got %+v", checks)
	}
}

func TestCollectDoctorChecksForMissingClaudeConfig(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".kb"), 0755); err != nil {
		t.Fatalf("Failed to create .kb directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "entries"), 0755); err != nil {
		t.Fatalf("Failed to create entries directory: %v", err)
	}

	config := &Config{
		Editor:         "sh",
		Viewer:         "cat",
		PrettyProvider: "claude",
	}

	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_CUSTOM_HEADERS", "")

	checks := collectDoctorChecks(tmpDir, config)
	for _, check := range checks {
		if check.Name == "Claude" {
			if check.OK {
				t.Fatalf("expected Claude check to fail when credentials are missing")
			}
			return
		}
	}

	t.Fatalf("expected Claude check in %+v", checks)
}
