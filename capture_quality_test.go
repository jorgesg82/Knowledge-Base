package main

import (
	"strings"
	"testing"
)

func TestParseCaptureEnvelopeSplitsSourceMetadataAndBody(t *testing.T) {
	env := parseCaptureEnvelope("Source URL: https://example.com\nSource title: Open Ports on macOS\n\nHow to inspect open ports on macos\n\nUse lsof.")

	if env.SourceURL != "https://example.com" {
		t.Fatalf("unexpected source URL: %s", env.SourceURL)
	}
	if env.SourceTitle != "Open Ports on macOS" {
		t.Fatalf("unexpected source title: %s", env.SourceTitle)
	}
	if !strings.Contains(env.Body, "How to inspect open ports on macos") {
		t.Fatalf("unexpected body: %s", env.Body)
	}
}

func TestRenderCanonicalCaptureMarkdownFormatsSourceAndStripsRepeatedTitle(t *testing.T) {
	markdown := renderCanonicalCaptureMarkdown("Source file: /tmp/capture.txt\n\nHow to inspect open ports on macos. Use lsof -iTCP -sTCP:LISTEN.", "Inspect Open Ports on macOS")

	if !strings.Contains(markdown, "> Source file: `/tmp/capture.txt`") {
		t.Fatalf("expected formatted source metadata, got: %s", markdown)
	}
	if strings.Contains(markdown, "How to inspect open ports on macos.") {
		t.Fatalf("expected repeated title sentence to be stripped, got: %s", markdown)
	}
	if !strings.Contains(markdown, "Use lsof -iTCP -sTCP:LISTEN.") {
		t.Fatalf("expected note body content, got: %s", markdown)
	}
}

func TestInferSummaryFromContentWithTitleUsesBodyNotSourceHeaders(t *testing.T) {
	summary := inferSummaryFromContentWithTitle("Source URL: https://example.com\nSource title: Open Ports on macOS\n\nHow to inspect open ports on macos. Use lsof -iTCP -sTCP:LISTEN.", "Inspect Open Ports on macOS")
	if strings.Contains(summary, "Source URL") || strings.Contains(summary, "example.com") {
		t.Fatalf("expected summary to exclude source headers, got: %s", summary)
	}
	if !strings.Contains(summary, "Use lsof -iTCP -sTCP:LISTEN.") {
		t.Fatalf("expected summary to focus on body, got: %s", summary)
	}
}
