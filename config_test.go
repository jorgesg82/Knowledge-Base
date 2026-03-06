package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearConfigOverrideEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"KB_PATH",
		"KB_EDITOR",
		"KB_VIEWER",
		"KB_DEFAULT_CATEGORY",
		"KB_AUTO_UPDATE_INDEX",
		"KB_AI_PROVIDER",
		"KB_PRETTY_PROVIDER",
	} {
		t.Setenv(name, "")
	}
}

func TestGetDefaultConfig(t *testing.T) {
	clearConfigOverrideEnv(t)

	config := GetDefaultConfig("/test/path")

	if config.KBPath != "/test/path" {
		t.Errorf("Expected KBPath /test/path, got %s", config.KBPath)
	}

	if config.DefaultCategory != "misc" {
		t.Errorf("Expected DefaultCategory misc, got %s", config.DefaultCategory)
	}

	if !config.AutoUpdateIndex {
		t.Error("Expected AutoUpdateIndex to be true")
	}

	if config.AIProvider != "auto" {
		t.Errorf("Expected AIProvider auto, got %s", config.AIProvider)
	}
}

func TestGetDefaultConfigFallsBackToBuiltinViewer(t *testing.T) {
	clearConfigOverrideEnv(t)

	t.Setenv("EDITOR", "nvim")
	t.Setenv("PATH", t.TempDir())

	config := GetDefaultConfig("/test/path")
	if config.Viewer != builtinViewerName {
		t.Fatalf("Expected Viewer %s, got %s", builtinViewerName, config.Viewer)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	clearConfigOverrideEnv(t)

	tmpDir := t.TempDir()

	config := &Config{
		KBPath:          tmpDir,
		Editor:          "vim",
		Viewer:          "less",
		DefaultCategory: "test",
		AutoUpdateIndex: false,
		AIProvider:      "chatgpt",
	}

	err := SaveConfig(config, tmpDir)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	rawConfig, err := os.ReadFile(filepath.Join(tmpDir, ".kb", "config.yml"))
	if err != nil {
		t.Fatalf("Failed to read saved config: %v", err)
	}
	if !strings.Contains(string(rawConfig), "ai_provider: chatgpt") {
		t.Fatalf("expected saved config to persist ai_provider, got %s", string(rawConfig))
	}
	if strings.Contains(string(rawConfig), "pretty_provider:") {
		t.Fatalf("did not expect legacy pretty_provider key in saved config: %s", string(rawConfig))
	}

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if loaded.KBPath != config.KBPath {
		t.Errorf("KBPath mismatch: expected %s, got %s", config.KBPath, loaded.KBPath)
	}

	if loaded.Editor != config.Editor {
		t.Errorf("Editor mismatch: expected %s, got %s", config.Editor, loaded.Editor)
	}

	if loaded.DefaultCategory != config.DefaultCategory {
		t.Errorf("DefaultCategory mismatch: expected %s, got %s", config.DefaultCategory, loaded.DefaultCategory)
	}

	if loaded.AutoUpdateIndex != config.AutoUpdateIndex {
		t.Error("AutoUpdateIndex mismatch")
	}

	if loaded.AIProvider != config.AIProvider {
		t.Errorf("AIProvider mismatch: expected %s, got %s", config.AIProvider, loaded.AIProvider)
	}

}

func TestLoadConfigWithDefaults(t *testing.T) {
	clearConfigOverrideEnv(t)

	tmpDir := t.TempDir()

	// Save config with missing fields
	configPath := filepath.Join(tmpDir, ".kb", "config.yml")
	os.MkdirAll(filepath.Dir(configPath), 0755)
	os.WriteFile(configPath, []byte("kb_path: "+tmpDir+"\n"), 0644)

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Should fill in defaults
	if loaded.DefaultCategory != "misc" {
		t.Errorf("Expected default category misc, got %s", loaded.DefaultCategory)
	}

	if !loaded.AutoUpdateIndex {
		t.Error("Expected default auto_update_index to be true")
	}

	if loaded.AIProvider != "auto" {
		t.Errorf("Expected default AI provider auto, got %s", loaded.AIProvider)
	}

}

func TestLoadConfigNonExistent(t *testing.T) {
	clearConfigOverrideEnv(t)

	tmpDir := t.TempDir()

	config, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("Expected no error for non-existent config, got: %v", err)
	}

	// Should return default config
	if config.KBPath != tmpDir {
		t.Errorf("Expected KBPath %s, got %s", tmpDir, config.KBPath)
	}

	if config.DefaultCategory != "misc" {
		t.Errorf("Expected default category misc, got %s", config.DefaultCategory)
	}
}

func TestGetKBPath(t *testing.T) {
	clearConfigOverrideEnv(t)

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub", "dir")
	os.MkdirAll(subDir, 0755)

	// Create .kb directory in tmpDir
	kbDir := filepath.Join(tmpDir, ".kb")
	os.MkdirAll(kbDir, 0755)

	// Change to subdirectory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	t.Setenv("PWD", subDir)
	os.Chdir(subDir)

	// Should find tmpDir as KB path
	kbPath, err := GetKBPath()
	if err != nil {
		t.Fatalf("Failed to get KB path: %v", err)
	}

	if kbPath != tmpDir {
		t.Errorf("Expected KB path %s, got %s", tmpDir, kbPath)
	}
}

func TestLoadConfigAppliesEnvironmentOverrides(t *testing.T) {
	clearConfigOverrideEnv(t)

	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, ".kb", "config.yml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("editor: vim\nviewer: less\npretty_provider: claude\n"), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	t.Setenv("KB_EDITOR", "nvim")
	t.Setenv("KB_VIEWER", "batcat")
	t.Setenv("KB_DEFAULT_CATEGORY", "work")
	t.Setenv("KB_AI_PROVIDER", "chatgpt")
	t.Setenv("KB_AUTO_UPDATE_INDEX", "false")

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config with overrides: %v", err)
	}

	if loaded.Editor != "nvim" {
		t.Errorf("Expected editor nvim, got %s", loaded.Editor)
	}
	if loaded.Viewer != "batcat" {
		t.Errorf("Expected viewer batcat, got %s", loaded.Viewer)
	}
	if loaded.DefaultCategory != "work" {
		t.Errorf("Expected default category work, got %s", loaded.DefaultCategory)
	}
	if loaded.AIProvider != "chatgpt" {
		t.Errorf("Expected provider chatgpt, got %s", loaded.AIProvider)
	}
	if loaded.AutoUpdateIndex {
		t.Error("Expected auto_update_index to be false after override")
	}
	if loaded.KBPath != tmpDir {
		t.Errorf("Expected KB path %s, got %s", tmpDir, loaded.KBPath)
	}
}

func TestLoadConfigSupportsAIProviderAlias(t *testing.T) {
	clearConfigOverrideEnv(t)

	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, ".kb", "config.yml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("ai_provider: claude\n"), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if loaded.AIProvider != "claude" {
		t.Fatalf("expected ai_provider alias to load as claude, got %s", loaded.AIProvider)
	}
}

func TestGetKBPathUsesEnvironmentOverride(t *testing.T) {
	clearConfigOverrideEnv(t)

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".kb"), 0755); err != nil {
		t.Fatalf("Failed to create .kb directory: %v", err)
	}

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)

	otherDir := t.TempDir()
	if err := os.Chdir(otherDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	t.Setenv("PWD", otherDir)
	t.Setenv("KB_PATH", tmpDir)

	kbPath, err := GetKBPath()
	if err != nil {
		t.Fatalf("Failed to get KB path from KB_PATH: %v", err)
	}

	if kbPath != tmpDir {
		t.Errorf("Expected KB path %s, got %s", tmpDir, kbPath)
	}
}
