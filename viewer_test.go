package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestIsBuiltinViewer(t *testing.T) {
	tests := []struct {
		viewer string
		want   bool
	}{
		{viewer: "", want: true},
		{viewer: "builtin", want: true},
		{viewer: "internal", want: true},
		{viewer: "auto", want: true},
		{viewer: "less", want: true},
		{viewer: "/usr/bin/less", want: true},
		{viewer: "glow", want: false},
		{viewer: "bat", want: false},
	}

	for _, tt := range tests {
		if got := isBuiltinViewer(tt.viewer); got != tt.want {
			t.Fatalf("isBuiltinViewer(%q) = %t, want %t", tt.viewer, got, tt.want)
		}
	}
}

func TestResolveCommandPathBuiltinViewer(t *testing.T) {
	path, err := resolveCommandPath("builtin")
	if err != nil {
		t.Fatalf("resolveCommandPath failed: %v", err)
	}
	if path != "builtin renderer" {
		t.Fatalf("expected builtin renderer, got %q", path)
	}
}

func TestRenderMarkdownForTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	rendered, err := renderMarkdownForTerminal("# Title\n\n- item\n")
	if err != nil {
		t.Fatalf("renderMarkdownForTerminal failed: %v", err)
	}

	if !strings.Contains(rendered, "Title") {
		t.Fatalf("expected rendered output to contain title, got %q", rendered)
	}
	if !strings.Contains(rendered, "item") {
		t.Fatalf("expected rendered output to contain list item, got %q", rendered)
	}
}

func TestBuiltinViewerMarkdownStripsFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := tmpDir + "/entry.md"
	content := "---\ntitle: Test Entry\ntags: [alpha, beta]\ncategory: misc\ncreated: 2026-03-06T00:00:00Z\nupdated: 2026-03-06T01:02:03Z\n---\n\n# Test Entry\n\nBody text.\n"

	if err := os.WriteFile(entryPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write entry: %v", err)
	}

	rendered := builtinViewerMarkdown(entryPath, content)
	if strings.Contains(rendered, "title: Test Entry") {
		t.Fatalf("expected frontmatter to be removed, got %q", rendered)
	}
	if !strings.Contains(rendered, "Category: `misc`") {
		t.Fatalf("expected category metadata, got %q", rendered)
	}
	if !strings.Contains(rendered, "Tags: `alpha`, `beta`") {
		t.Fatalf("expected tags metadata, got %q", rendered)
	}
	if !strings.Contains(rendered, "# Test Entry") || !strings.Contains(rendered, "Body text.") {
		t.Fatalf("expected entry body, got %q", rendered)
	}
}

func TestRunViewerCommandPreservesConfiguredStdio(t *testing.T) {
	cmd := exec.Command("cat")
	cmd.Stdin = strings.NewReader("hello from stdin")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := runViewerCommand(cmd); err != nil {
		t.Fatalf("runViewerCommand failed: %v", err)
	}

	if got := stdout.String(); got != "hello from stdin" {
		t.Fatalf("expected preserved stdin/stdout, got %q", got)
	}
}
