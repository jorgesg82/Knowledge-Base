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

	root, err := findRepoRoot(filepath.Dir(filename))
	if err != nil {
		panic(err.Error())
	}
	return root
}

func findRepoRoot(startDir string) (string, error) {
	for dir := startDir; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	return "", &execError{err: errors.New("failed to locate repo root"), output: startDir}
}

func runCLI(t *testing.T, dir string, env []string, stdin string, args ...string) (string, string, int) {
	t.Helper()

	cmd := exec.Command(buildCLIBinary(t), args...)
	cmd.Dir = dir
	cmd.Env = testCLIEnv()
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

func testCLIEnv() []string {
	blockedPrefixes := []string{
		"OPENAI_API_KEY=",
		"OPENAI_ADMIN_KEY=",
		"OPENAI_PROJECT_ID=",
		"OPENAI_BASE_URL=",
		"OPENAI_MODEL=",
		"OPENAI_ORG_ID=",
		"ANTHROPIC_API_KEY=",
		"ANTHROPIC_ADMIN_KEY=",
		"ANTHROPIC_ADMIN_API_KEY=",
		"ANTHROPIC_ADMIN_BASE_URL=",
		"ANTHROPIC_CUSTOM_HEADERS=",
		"ANTHROPIC_BASE_URL=",
		"ANTHROPIC_MODEL=",
		"ANTHROPIC_DEFAULT_SONNET_MODEL=",
		"KB_AI_PROVIDER=",
		"KB_PRETTY_PROVIDER=",
	}

	filtered := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		blocked := false
		for _, prefix := range blockedPrefixes {
			if strings.HasPrefix(kv, prefix) {
				blocked = true
				break
			}
		}
		if !blocked {
			filtered = append(filtered, kv)
		}
	}

	filtered = append(filtered, "NO_COLOR=1")
	return filtered
}

func TestFindRepoRoot(t *testing.T) {
	root, err := findRepoRoot(filepath.Join(repoRootDir(), "entries", "misc"))
	if err != nil {
		t.Fatalf("findRepoRoot failed: %v", err)
	}
	if root != repoRootDir() {
		t.Fatalf("expected repo root %s, got %s", repoRootDir(), root)
	}
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

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "remember to use lsof -iTCP -sTCP:LISTEN on macos")
	if exitCode != 0 {
		t.Fatalf("add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	notes, err := loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to load canonical notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected one canonical note, got %d", len(notes))
	}
	if _, err := os.Stat(filepath.Join(tmpDir, notes[0].MaterializedPath)); err != nil {
		t.Fatalf("expected materialized note to exist: %v", err)
	}

	index, err := LoadIndex(tmpDir)
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}
	if len(index.Entries) != 0 {
		t.Fatalf("expected index to stay empty when auto_update_index=false, got %d entries", len(index.Entries))
	}
}

func TestCLIStatsUsesCanonicalStoreWhenIndexUpdatesAreDisabled(t *testing.T) {
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

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "remember to use lsof -iTCP -sTCP:LISTEN on macos")
	if exitCode != 0 {
		t.Fatalf("add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	stdout, stderr, exitCode = runCLI(t, tmpDir, nil, "", "stats")
	if exitCode != 0 {
		t.Fatalf("stats failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Total notes: 1") {
		t.Fatalf("expected canonical note count in stats output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Captures: 1") {
		t.Fatalf("expected capture count in stats output, got: %s", stdout)
	}
}

func TestCLIAddWithoutArgsUsesEditorCapture(t *testing.T) {
	tmpDir := t.TempDir()

	editorScript := filepath.Join(tmpDir, "kb-editor.sh")
	if err := os.WriteFile(editorScript, []byte("#!/bin/sh\ncat <<'EOF' > \"$1\"\nHow to inspect open ports on macos\n\nUse lsof -iTCP -sTCP:LISTEN.\nEOF\n"), 0755); err != nil {
		t.Fatalf("failed to write editor script: %v", err)
	}

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=" + editorScript}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add")
	if exitCode != 0 {
		t.Fatalf("add failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Captured.") {
		t.Fatalf("unexpected add output: %s", stdout)
	}

	notes, err := loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to load canonical notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected one canonical note, got %d", len(notes))
	}
	if notes[0].Title == "" {
		t.Fatal("expected note title to be inferred")
	}
	if notes[0].Body == "" {
		t.Fatal("expected note body to be persisted")
	}
}

func TestCLIAddPersistsCaptureBeforeApplyFailure(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	if err := os.RemoveAll(filepath.Join(tmpDir, "entries")); err != nil {
		t.Fatalf("failed to remove entries dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "entries"), []byte("not a directory"), 0644); err != nil {
		t.Fatalf("failed to replace entries dir with file: %v", err)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "remember to use lsof -iTCP -sTCP:LISTEN on macos")
	if exitCode == 0 {
		t.Fatalf("expected add to fail during note materialization: stdout=%s stderr=%s", stdout, stderr)
	}

	captures, err := os.ReadDir(filepath.Join(tmpDir, ".kb", capturesDirName))
	if err != nil {
		t.Fatalf("failed to read captures dir: %v", err)
	}
	count := 0
	for _, entry := range captures {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected failed add to persist one raw capture, got %d", count)
	}
}

func TestCLIAddSlashTextUsesCaptureFlowByDefault(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "linux/ssh-tunneling")
	if exitCode != 0 {
		t.Fatalf("add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	notes, err := loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to load notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected one canonical note, got %d", len(notes))
	}
	if notes[0].Title != "Linux SSH Tunneling" {
		t.Fatalf("expected slash text to be treated as capture content, got title %q", notes[0].Title)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "entries", "linux", "ssh-tunneling.md")); !os.IsNotExist(err) {
		t.Fatalf("did not expect legacy path materialization without --legacy, got err=%v", err)
	}
}

func TestCLIConcurrentAddsPreserveAllCapturesAndState(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	type procResult struct {
		stdout string
		stderr string
		err    error
	}

	binary := buildCLIBinary(t)
	runAdd := func(text string, resultCh chan<- procResult) {
		cmd := exec.Command(binary, "add", text)
		cmd.Dir = tmpDir
		cmd.Env = testCLIEnv()

		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		resultCh <- procResult{
			stdout: stdout.String(),
			stderr: stderr.String(),
			err:    err,
		}
	}

	resultCh := make(chan procResult, 6)
	inputs := []string{
		"alpha walrus transport tip",
		"beta citrus debugging note",
		"gamma lattice deployment reminder",
		"delta beacon networking trick",
		"epsilon kernel profiling memo",
		"zeta postgres tuning snippet",
	}
	for _, input := range inputs {
		go runAdd(input, resultCh)
	}
	for range inputs {
		result := <-resultCh
		if result.err != nil {
			t.Fatalf("concurrent add failed: stdout=%s stderr=%s err=%v", result.stdout, result.stderr, result.err)
		}
	}

	notes, err := loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to load notes: %v", err)
	}
	if len(notes) != len(inputs) {
		t.Fatalf("expected %d notes, got %d", len(inputs), len(notes))
	}

	state, err := loadOrInitKBState(tmpDir)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if state.LastCaptureSeq != len(inputs) {
		t.Fatalf("expected LastCaptureSeq=%d, got %d", len(inputs), state.LastCaptureSeq)
	}
	if state.LastNoteSeq != len(inputs) {
		t.Fatalf("expected LastNoteSeq=%d, got %d", len(inputs), state.LastNoteSeq)
	}
	if state.LastOperationSeq != len(inputs) {
		t.Fatalf("expected LastOperationSeq=%d, got %d", len(inputs), state.LastOperationSeq)
	}
}

func TestCLIAddRejectsLegacyFlag(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "--legacy", "linux/ssh-tunneling")
	if exitCode == 0 {
		t.Fatalf("expected --legacy to fail in current workflow: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "unknown flag: --legacy") {
		t.Fatalf("expected unknown flag error, got stdout=%s stderr=%s", stdout, stderr)
	}
}

func TestCLIFindFindsCanonicalNoteByAlias(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	state, err := loadOrInitKBState(tmpDir)
	if err != nil {
		t.Fatalf("failed to init state: %v", err)
	}
	note := &CanonicalNote{
		ID:               nextNoteID(state, time.Now().UTC()),
		Title:            "Inspect Open Ports on macOS",
		Aliases:          []string{"open ports mac"},
		Summary:          "Check listening TCP ports.",
		Body:             "Use `lsof -iTCP -sTCP:LISTEN`.",
		Topics:           []string{"macos", "networking"},
		SourceCaptureIDs: []string{"cap_test"},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		Revision:         1,
	}
	if err := saveAndIndexCanonicalNote(tmpDir, GetDefaultConfig(tmpDir), note); err != nil {
		t.Fatalf("failed to save canonical note: %v", err)
	}
	if err := saveKBState(tmpDir, state); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, []string{"KB_VIEWER=builtin", "COLUMNS=80"}, "", "find", "open ports mac")
	if exitCode != 0 {
		t.Fatalf("find failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Inspect Open Ports on macOS") {
		t.Fatalf("expected find output to include note title, got: %s", stdout)
	}
}

func TestCLIAddSecondSimilarCaptureUpdatesExistingCanonicalNote(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "Inspect open ports on macos. Use lsof -iTCP -sTCP:LISTEN.")
	if exitCode != 0 {
		t.Fatalf("first add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	stdout, stderr, exitCode = runCLI(t, tmpDir, nil, "", "add", "remember lsof -iTCP -sTCP:LISTEN on macos")
	if exitCode != 0 {
		t.Fatalf("second add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	notes, err := loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to load canonical notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected one canonical note after merge, got %d", len(notes))
	}
	if !strings.Contains(notes[0].Body, "remember lsof -iTCP -sTCP:LISTEN on macos") {
		t.Fatalf("expected second capture to be appended, got body: %s", notes[0].Body)
	}

	stdout, stderr, exitCode = runCLI(t, tmpDir, nil, "", "add", "remember lsof -iTCP -sTCP:LISTEN on macos")
	if exitCode != 0 {
		t.Fatalf("third add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	notes, err = loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to reload canonical notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected one canonical note after duplicate capture, got %d", len(notes))
	}
	if strings.Count(notes[0].Body, "remember lsof -iTCP -sTCP:LISTEN on macos") != 1 {
		t.Fatalf("expected duplicate capture not to be appended twice, got body: %s", notes[0].Body)
	}
}

func TestCLIAddFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	filePath := filepath.Join(tmpDir, "capture.txt")
	if err := os.WriteFile(filePath, []byte("how to inspect open ports on macos\n\nuse lsof -iTCP -sTCP:LISTEN"), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "--file", filePath)
	if exitCode != 0 {
		t.Fatalf("add --file failed: stdout=%s stderr=%s", stdout, stderr)
	}

	notes, err := loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to load notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected one canonical note, got %d", len(notes))
	}
	if !strings.Contains(notes[0].Body, "> Source file: `"+filePath+"`") {
		t.Fatalf("expected note body to include formatted source file metadata, got: %s", notes[0].Body)
	}
	if strings.Contains(notes[0].Body, "how to inspect open ports on macos") {
		t.Fatalf("expected repeated title line to be cleaned from note body, got: %s", notes[0].Body)
	}
	if !strings.Contains(notes[0].Body, "use lsof -iTCP -sTCP:LISTEN") {
		t.Fatalf("expected useful body content to remain, got: %s", notes[0].Body)
	}
}

func TestCLIAddFromClipboard(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "pbpaste"), []byte("#!/bin/sh\nprintf 'clipboard capture about ssh tunnels'"), 0755); err != nil {
		t.Fatalf("failed to write pbpaste stub: %v", err)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, []string{"PATH=" + binDir}, "", "add", "--clipboard")
	if exitCode != 0 {
		t.Fatalf("add --clipboard failed: stdout=%s stderr=%s", stdout, stderr)
	}

	notes, err := loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to load notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected one canonical note, got %d", len(notes))
	}
	if !strings.Contains(notes[0].Body, "clipboard capture about ssh tunnels") {
		t.Fatalf("expected clipboard text in note body, got: %s", notes[0].Body)
	}
}

func TestCLIFindJSONForCanonicalNote(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "remember inspect open ports on macos. Use lsof -iTCP -sTCP:LISTEN.")
	if exitCode != 0 {
		t.Fatalf("add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	stdout, stderr, exitCode = runCLI(t, tmpDir, nil, "", "find", "--json", "open ports")
	if exitCode != 0 {
		t.Fatalf("find --json failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, `"mode": "note"`) {
		t.Fatalf("expected mode note in JSON output, got: %s", stdout)
	}
	if !strings.Contains(stdout, `"title": "Inspect Open Ports on macOS"`) {
		t.Fatalf("expected title in JSON output, got: %s", stdout)
	}
}

func TestCLIFindWithoutQueryPrintsBrowseList(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "remember inspect open ports on macos. Use lsof -iTCP -sTCP:LISTEN.")
	if exitCode != 0 {
		t.Fatalf("add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	stdout, stderr, exitCode = runCLI(t, tmpDir, nil, "", "find")
	if exitCode != 0 {
		t.Fatalf("find browse failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Browse notes (") {
		t.Fatalf("expected browse header in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Inspect Open Ports on macOS") {
		t.Fatalf("expected browse output to include note title, got: %s", stdout)
	}
}

func TestCLIFindRawForCanonicalNote(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "clipboard ssh tunnel tip")
	if exitCode != 0 {
		t.Fatalf("add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	stdout, stderr, exitCode = runCLI(t, tmpDir, nil, "", "find", "--raw", "ssh tunnel")
	if exitCode != 0 {
		t.Fatalf("find --raw failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "---") || !strings.Contains(stdout, "title: Clipboard SSH Tunnel Tip") {
		t.Fatalf("expected raw markdown/frontmatter output, got: %s", stdout)
	}
}

func TestCLIFindSynthesizeUsesOpenAI(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "Inspect open ports on macos. Use lsof -iTCP -sTCP:LISTEN.")
	if exitCode != 0 {
		t.Fatalf("first add failed: stdout=%s stderr=%s", stdout, stderr)
	}
	stdout, stderr, exitCode = runCLI(t, tmpDir, nil, "", "add", "SSH tunnels can be created with ssh -L local:remote.")
	if exitCode != 0 {
		t.Fatalf("second add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_123","object":"response","status":"completed","model":"gpt-test","output":[{"id":"msg_123","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"# Retrieval Summary\n\nUse lsof -iTCP -sTCP:LISTEN to inspect open ports.\n\n## Sources\n- Inspect Open Ports on macOS (note_1)\n","annotations":[]}]}]}`))
	}))
	defer server.Close()

	stdout, stderr, exitCode = runCLI(t, tmpDir, []string{
		"OPENAI_API_KEY=test-key",
		"OPENAI_BASE_URL=" + server.URL,
		"OPENAI_MODEL=gpt-test",
	}, "", "find", "--synthesize", "--provider", "chatgpt", "open ports")
	if exitCode != 0 {
		t.Fatalf("find --synthesize failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Retrieval Summary") {
		t.Fatalf("expected synthesized summary in output, got: %s", stdout)
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

	stdout, stderr, exitCode := runCLI(t, tmpDir, []string{"KB_AI_PROVIDER=chatgpt", "OPENAI_API_KEY="}, "", "doctor")
	if exitCode != 0 {
		t.Fatalf("doctor failed: stdout=%s stderr=%s", stdout, stderr)
	}

	if !strings.Contains(stdout, "AI provider") || !strings.Contains(stdout, "resolved=chatgpt") {
		t.Fatalf("unexpected doctor output: %s", stdout)
	}
	if !strings.Contains(stdout, "OpenAI") || !strings.Contains(stdout, "missing") {
		t.Fatalf("expected doctor output to report missing OpenAI credentials: %s", stdout)
	}
}

func TestCLIRemovedCommandsFailWithMigrationMessage(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	for _, command := range []string{"edit", "rm", "list", "search", "tag", "tags", "pretty", "show"} {
		stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", command)
		if exitCode == 0 {
			t.Fatalf("expected %s to fail in current workflow: stdout=%s stderr=%s", command, stdout, stderr)
		}
		if !strings.Contains(stderr, "removed from the current workflow") {
			t.Fatalf("expected migration message for %s, got stdout=%s stderr=%s", command, stdout, stderr)
		}
	}
}

func TestCLIConfigShowsResolvedProvider(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, []string{"KB_AI_PROVIDER=auto"}, "", "config")
	if exitCode != 0 {
		t.Fatalf("config failed: stdout=%s stderr=%s", stdout, stderr)
	}

	if !strings.Contains(stdout, "AI Provider: auto") {
		t.Fatalf("expected provider auto in config output: %s", stdout)
	}
	if !strings.Contains(stdout, "AI Provider Resolved:") {
		t.Fatalf("expected resolved provider in config output: %s", stdout)
	}
}

func TestCLIStatsDoesNotCrashWithoutAdminKey(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".kb"), 0755); err != nil {
		t.Fatalf("failed to create .kb: %v", err)
	}
	config := GetDefaultConfig(tmpDir)
	config.AIProvider = "chatgpt"
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

func TestCLIStatsDoesNotCrashWithoutAnthropicAdminKey(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".kb"), 0755); err != nil {
		t.Fatalf("failed to create .kb: %v", err)
	}
	config := GetDefaultConfig(tmpDir)
	config.AIProvider = "claude"
	if err := SaveConfig(config, tmpDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
	if err := SaveIndex(&Index{Entries: []IndexEntry{}, LastUpdated: time.Now()}, tmpDir); err != nil {
		t.Fatalf("failed to save index: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "entries"), 0755); err != nil {
		t.Fatalf("failed to create entries dir: %v", err)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, []string{"ANTHROPIC_API_KEY=test-key", "ANTHROPIC_ADMIN_KEY="}, "", "stats")
	if exitCode != 0 {
		t.Fatalf("stats failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Anthropic API spend:") {
		t.Fatalf("expected Anthropic spend section in stats output: %s", stdout)
	}
	if !strings.Contains(stdout, "Unavailable. Set ANTHROPIC_ADMIN_KEY") {
		t.Fatalf("expected missing Anthropic admin key message in stats output: %s", stdout)
	}
}

func TestCLIRebuildRematerializesMissingEntries(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, nil, "", "add", "Inspect open ports on macos. Use lsof -iTCP -sTCP:LISTEN.")
	if exitCode != 0 {
		t.Fatalf("add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	notes, err := loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to load notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected one note, got %d", len(notes))
	}

	if err := os.RemoveAll(filepath.Join(tmpDir, "entries")); err != nil {
		t.Fatalf("failed to remove entries dir: %v", err)
	}

	stdout, stderr, exitCode = runCLI(t, tmpDir, nil, "", "rebuild")
	if exitCode != 0 {
		t.Fatalf("rebuild failed: stdout=%s stderr=%s", stdout, stderr)
	}

	restored, err := loadCanonicalNoteRecord(tmpDir, notes[0].ID)
	if err != nil {
		t.Fatalf("failed to reload note record: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, restored.MaterializedPath)); err != nil {
		t.Fatalf("expected rebuilt materialized note to exist: %v", err)
	}

	stdout, stderr, exitCode = runCLI(t, tmpDir, []string{"KB_VIEWER=builtin"}, "", "find", "open ports")
	if exitCode != 0 {
		t.Fatalf("find failed after rebuild: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Inspect Open Ports on macOS") {
		t.Fatalf("expected rebuilt note to be readable, got: %s", stdout)
	}
}

func TestCLIAddPersistsAppliedOperation(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	stdout, stderr := "", ""
	stdout, stderr, exitCode = runCLI(t, tmpDir, nil, "", "add", "how to inspect open ports on macos with lsof")
	if exitCode != 0 {
		t.Fatalf("add failed: stdout=%s stderr=%s", stdout, stderr)
	}

	opsDir := filepath.Join(tmpDir, ".kb", opsDirName)
	ops, err := os.ReadDir(opsDir)
	if err != nil {
		t.Fatalf("failed to read ops dir: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected one operation, got %d", len(ops))
	}

	opData, err := os.ReadFile(filepath.Join(opsDir, ops[0].Name()))
	if err != nil {
		t.Fatalf("failed to read operation: %v", err)
	}
	var op AppliedOperation
	if err := json.Unmarshal(opData, &op); err != nil {
		t.Fatalf("failed to parse operation: %v", err)
	}
	if op.Status != OperationStatusApplied {
		t.Fatalf("expected applied operation status, got %s", op.Status)
	}
	if strings.TrimSpace(op.CaptureID) == "" {
		t.Fatal("expected operation to reference capture ID")
	}

	captureData, err := os.ReadFile(filepath.Join(tmpDir, ".kb", capturesDirName, op.CaptureID+".json"))
	if err != nil {
		t.Fatalf("failed to read capture: %v", err)
	}
	var capture CaptureRecord
	if err := json.Unmarshal(captureData, &capture); err != nil {
		t.Fatalf("failed to parse capture: %v", err)
	}
	if capture.OperationID != op.ID {
		t.Fatalf("expected capture operation ID %s, got %s", op.ID, capture.OperationID)
	}
}

func TestCLIConcurrentAddsReserveUniqueIDs(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	type result struct {
		stdout string
		stderr string
		code   int
	}

	inputs := []string{
		"alpha unique capture about ssh port forwarding",
		"bravo unique capture about tcp keepalive settings",
		"charlie unique capture about macos launchctl tips",
		"delta unique capture about linux journalctl filters",
		"echo unique capture about docker prune safety",
		"foxtrot unique capture about tmux session restore",
	}

	results := make(chan result, len(inputs))
	var wg sync.WaitGroup
	for _, input := range inputs {
		wg.Add(1)
		go func(content string) {
			defer wg.Done()
			stdout, stderr, code := runCLI(t, tmpDir, nil, "", "add", content)
			results <- result{stdout: stdout, stderr: stderr, code: code}
		}(input)
	}
	wg.Wait()
	close(results)

	for result := range results {
		if result.code != 0 {
			t.Fatalf("concurrent add failed: stdout=%s stderr=%s", result.stdout, result.stderr)
		}
	}

	captureCount, err := countStoreJSONRecords(tmpDir, capturesDirName)
	if err != nil {
		t.Fatalf("failed to count captures: %v", err)
	}
	if captureCount != len(inputs) {
		t.Fatalf("expected %d captures, got %d", len(inputs), captureCount)
	}

	operationCount, err := countStoreJSONRecords(tmpDir, opsDirName)
	if err != nil {
		t.Fatalf("failed to count operations: %v", err)
	}
	if operationCount != len(inputs) {
		t.Fatalf("expected %d operations, got %d", len(inputs), operationCount)
	}

	notes, err := loadCanonicalNotes(tmpDir)
	if err != nil {
		t.Fatalf("failed to load notes: %v", err)
	}
	if len(notes) != len(inputs) {
		t.Fatalf("expected %d notes, got %d", len(inputs), len(notes))
	}

	seen := map[string]struct{}{}
	for _, note := range notes {
		if _, exists := seen[note.ID]; exists {
			t.Fatalf("duplicate note ID detected: %s", note.ID)
		}
		seen[note.ID] = struct{}{}
	}
}

func TestCLIFindFallsBackToBuiltinRendererWhenViewerIsUnavailable(t *testing.T) {
	tmpDir := t.TempDir()

	_, stderr, exitCode := runCLI(t, repoRootDir(), []string{"EDITOR=true"}, "", "init", tmpDir)
	if exitCode != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	state, err := loadOrInitKBState(tmpDir)
	if err != nil {
		t.Fatalf("failed to init state: %v", err)
	}
	note := &CanonicalNote{
		ID:               nextNoteID(state, time.Now().UTC()),
		Title:            "Viewer Test",
		Body:             "# Viewer Test\n\n- first item\n",
		SourceCaptureIDs: []string{"cap_test"},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		Revision:         1,
	}
	if err := saveAndIndexCanonicalNote(tmpDir, GetDefaultConfig(tmpDir), note); err != nil {
		t.Fatalf("failed to save canonical note: %v", err)
	}
	if err := saveKBState(tmpDir, state); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	stdout, stderr, exitCode := runCLI(t, tmpDir, []string{"KB_VIEWER=definitely-not-installed-viewer"}, "", "find", "Viewer Test")
	if exitCode != 0 {
		t.Fatalf("find failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Viewer Test") || !strings.Contains(stdout, "first item") {
		t.Fatalf("expected builtin renderer output, got: %s", stdout)
	}
}
