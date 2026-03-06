package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	cliBinaryOnce sync.Once
	cliBinaryPath string
	cliBinaryErr  error
)

func buildCLIBinary(t *testing.T) string {
	t.Helper()

	cliBinaryOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "kb-cli-bin")
		if err != nil {
			cliBinaryErr = err
			return
		}

		cliBinaryPath = filepath.Join(tmpDir, "kb-test-bin")
		cmd := exec.Command("go", "build", "-o", cliBinaryPath, ".")
		cmd.Dir = repoRootDir()
		if output, err := cmd.CombinedOutput(); err != nil {
			cliBinaryErr = &execError{err: err, output: string(output)}
			return
		}
	})

	if cliBinaryErr != nil {
		t.Fatalf("failed to build CLI binary: %v", cliBinaryErr)
	}

	return cliBinaryPath
}

type execError struct {
	err    error
	output string
}

func (e *execError) Error() string {
	return e.err.Error() + ": " + e.output
}

func repoRootDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to locate test file")
	}
	wd := filepath.Dir(filename)
	if filepath.Base(wd) != "kb" {
		panic("unexpected repo root: " + wd)
	}
	return wd
}

func runCLI(t *testing.T, dir string, env []string, stdin string, args ...string) (string, string, int) {
	t.Helper()

	cmd := exec.Command(buildCLIBinary(t), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI %v: %v", args, err)
		}
	}

	return stdout.String(), stderr.String(), exitCode
}

func TestCLIAddRespectsAutoUpdateIndexDisabled(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	configPath := filepath.Join(tmpDir, ".kb", "config.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	updated := strings.Replace(string(data), "auto_update_index: true", "auto_update_index: false", 1)
	if err := os.WriteFile(configPath, []byte(updated), 0644); err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "test-entry")
	if exitCode != 0 {
		t.Fatalf("add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "entries", "misc", "test-entry.md")); err != nil {
		t.Fatalf("expected entry file to exist: %v", err)
	}

	index, err := LoadIndex(tmpDir)
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}
	if len(index.Entries) != 0 {
		t.Fatalf("expected index to stay empty when auto_update_index=false, got %d entries", len(index.Entries))
	}
}

func TestCLICleanHandlesCorruptIndex(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".kb"), 0755); err != nil {
		t.Fatalf("failed to create .kb: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "entries"), 0755); err != nil {
		t.Fatalf("failed to create entries dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".kb", "index.json"), []byte("{ invalid json "), 0644); err != nil {
		t.Fatalf("failed to write corrupt index: %v", err)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "yes\n", "clean")
	if exitCode != 0 {
		t.Fatalf("clean failed: stdout=%s stderr=%s", stdout, stderr)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".kb")); !os.IsNotExist(err) {
		t.Fatalf("expected .kb directory to be removed, got err=%v", err)
	}
	if strings.Contains(stdout+stderr, "panic") {
		t.Fatalf("unexpected panic output: %s %s", stdout, stderr)
	}
}

func TestCLIDoctorReportsMissingOpenAICredentials(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, []string{"KB_PRETTY_PROVIDER=chatgpt", "OPENAI_API_KEY="}, "", "doctor")
	if exitCode != 0 {
		t.Fatalf("doctor failed: stdout=%s stderr=%s", stdout, stderr)
	}

	if !strings.Contains(stdout, "Pretty provider") || !strings.Contains(stdout, "resolved=chatgpt") {
		t.Fatalf("unexpected doctor output: %s", stdout)
	}
	if !strings.Contains(stdout, "OpenAI") || !strings.Contains(stdout, "missing") {
		t.Fatalf("expected doctor output to report missing OpenAI credentials: %s", stdout)
	}
}

func TestCLIPrettyDryRunDiffDoesNotWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".kb"), 0755); err != nil {
		t.Fatalf("failed to create .kb: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "entries", "misc"), 0755); err != nil {
		t.Fatalf("failed to create entries dir: %v", err)
	}

	config := GetDefaultConfig(tmpDir)
	config.PrettyProvider = "chatgpt"
	config.Editor = "true"
	if err := SaveConfig(config, tmpDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	entryPath := filepath.Join(tmpDir, "entries", "misc", "test-entry.md")
	original := "---\ntitle: Test Entry\ncategory: misc\ncreated: 2026-03-06T00:00:00Z\nupdated: 2026-03-06T00:00:00Z\n---\n\n# Test Entry\n\nOriginal content.\n"
	if err := os.WriteFile(entryPath, []byte(original), 0644); err != nil {
		t.Fatalf("failed to write entry: %v", err)
	}

	entry, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("failed to parse entry: %v", err)
	}
	index := &Index{Entries: []IndexEntry{}}
	AddToIndex(index, entry, tmpDir)
	if err := SaveIndex(index, tmpDir); err != nil {
		t.Fatalf("failed to save index: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_123","object":"response","status":"completed","model":"gpt-test","output":[{"id":"msg_123","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"---\ntitle: Test Entry\ncategory: misc\ncreated: 2026-03-06T00:00:00Z\nupdated: 2026-03-06T00:00:00Z\n---\n\n# Test Entry\n\nImproved content.\n","annotations":[]}]}]}`))
	}))
	defer server.Close()

	stdout, stderr, exitCode := runCLI(
		t,
		tmpDir,
		[]string{
			"OPENAI_API_KEY=test-key",
			"OPENAI_BASE_URL=" + server.URL,
			"OPENAI_MODEL=gpt-test",
			"KB_PRETTY_PROVIDER=chatgpt",
		},
		"",
		"pretty", "misc-test-entry", "--dry-run", "--diff",
	)
	if exitCode != 0 {
		t.Fatalf("pretty failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "--- current") || !strings.Contains(stdout, "+++ proposed") {
		t.Fatalf("expected diff output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Dry run: no changes written") {
		t.Fatalf("expected dry-run message, got: %s", stdout)
	}

	after, err := os.ReadFile(entryPath)
	if err != nil {
		t.Fatalf("failed to read entry after pretty: %v", err)
	}
	if string(after) != original {
		t.Fatalf("expected dry-run to keep file unchanged")
	}
}

func TestCLIConfigShowsResolvedProvider(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, []string{"KB_PRETTY_PROVIDER=auto"}, "", "config")
	if exitCode != 0 {
		t.Fatalf("config failed: stdout=%s stderr=%s", stdout, stderr)
	}

	if !strings.Contains(stdout, "Pretty Provider: auto") {
		t.Fatalf("expected provider auto in config output: %s", stdout)
	}
	if !strings.Contains(stdout, "Pretty Provider Resolved:") {
		t.Fatalf("expected resolved provider in config output: %s", stdout)
	}
}

func TestCLIStatsDoesNotCrashWithoutAdminKey(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".kb"), 0755); err != nil {
		t.Fatalf("failed to create .kb: %v", err)
	}
	config := GetDefaultConfig(tmpDir)
	config.PrettyProvider = "chatgpt"
	if err := SaveConfig(config, tmpDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
	if err := SaveIndex(&Index{Entries: []IndexEntry{}, LastUpdated: time.Now()}, tmpDir); err != nil {
		t.Fatalf("failed to save index: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "entries"), 0755); err != nil {
		t.Fatalf("failed to create entries dir: %v", err)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, []string{"OPENAI_API_KEY=test-key", "OPENAI_ADMIN_KEY="}, "", "stats")
	if exitCode != 0 {
		t.Fatalf("stats failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "OpenAI API spend:") {
		t.Fatalf("expected spend section in stats output: %s", stdout)
	}
	if !strings.Contains(stdout, "Unavailable. Set OPENAI_ADMIN_KEY") {
		t.Fatalf("expected missing admin key message in stats output: %s", stdout)
	}
}

func TestCLIShowFallsBackToBuiltinRendererWhenViewerIsUnavailable(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	entryPath := filepath.Join(tmpDir, "entries", "misc", "viewer-test.md")
	if err := os.MkdirAll(filepath.Dir(entryPath), 0755); err != nil {
		t.Fatalf("failed to create entry dir: %v", err)
	}
	entry := "---\ntitle: Viewer Test\ncategory: misc\ncreated: 2026-03-06T00:00:00Z\nupdated: 2026-03-06T00:00:00Z\n---\n\n# Viewer Test\n\n- first item\n"
	if err := os.WriteFile(entryPath, []byte(entry), 0644); err != nil {
		t.Fatalf("failed to write entry: %v", err)
	}

	parsed, err := ParseEntry(entryPath)
	if err != nil {
		t.Fatalf("failed to parse entry: %v", err)
	}
	index := &Index{Entries: []IndexEntry{}}
	AddToIndex(index, parsed, tmpDir)
	if err := SaveIndex(index, tmpDir); err != nil {
		t.Fatalf("failed to save index: %v", err)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, []string{"KB_VIEWER=definitely-not-installed-viewer"}, "", "show", "viewer-test")
	if exitCode != 0 {
		t.Fatalf("show failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Viewer Test") || !strings.Contains(stdout, "first item") {
		t.Fatalf("expected builtin renderer output, got: %s", stdout)
	}
}
