package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAddContentFromURLExtractsReadableHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head><title>Open Ports on macOS</title></head>
  <body>
    <main>
      <h1>Inspect Open Ports on macOS</h1>
      <p>Use <code>lsof -iTCP -sTCP:LISTEN</code> to inspect listening TCP ports.</p>
    </main>
  </body>
</html>`))
	}))
	defer server.Close()

	content, source, err := readAddContentFromURL(server.URL)
	if err != nil {
		t.Fatalf("readAddContentFromURL failed: %v", err)
	}
	if source != CaptureSourceURL {
		t.Fatalf("expected source %s, got %s", CaptureSourceURL, source)
	}
	if !strings.Contains(content, "Source URL: "+server.URL) {
		t.Fatalf("expected source URL in content, got: %s", content)
	}
	if !strings.Contains(content, "Source title: Open Ports on macOS") {
		t.Fatalf("expected extracted title in content, got: %s", content)
	}
	if !strings.Contains(content, "Inspect Open Ports on macOS") {
		t.Fatalf("expected readable text in content, got: %s", content)
	}
}

func TestReadAddContentFromClipboardUsesAvailableCommand(t *testing.T) {
	tmpDir := t.TempDir()
	clipboardScript := filepath.Join(tmpDir, "pbpaste")
	if err := os.WriteFile(clipboardScript, []byte("#!/bin/sh\nprintf 'clipboard note from test'"), 0755); err != nil {
		t.Fatalf("failed to write clipboard script: %v", err)
	}

	t.Setenv("PATH", tmpDir)
	content, source, err := readAddContentFromClipboard()
	if err != nil {
		t.Fatalf("readAddContentFromClipboard failed: %v", err)
	}
	if source != CaptureSourceClipboard {
		t.Fatalf("expected source %s, got %s", CaptureSourceClipboard, source)
	}
	if content != "clipboard note from test" {
		t.Fatalf("unexpected clipboard content: %q", content)
	}
}

func TestReadAddContentFromFileRejectsOversizedInput(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large.txt")
	large := strings.Repeat("a", maxCaptureInputBytes+1)
	if err := os.WriteFile(filePath, []byte(large), 0644); err != nil {
		t.Fatalf("failed to write large input file: %v", err)
	}

	if _, _, err := readAddContentFromFile(filePath); err == nil {
		t.Fatal("expected oversized file input to be rejected")
	}
}
