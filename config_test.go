package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDefaultConfig(t *testing.T) {
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

	if config.PrettyMode != "moderate" {
		t.Errorf("Expected PrettyMode moderate, got %s", config.PrettyMode)
	}

	if !config.PrettyAutoApply {
		t.Error("Expected PrettyAutoApply to be true")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		KBPath:          tmpDir,
		Editor:          "vim",
		Viewer:          "less",
		DefaultCategory: "test",
		AutoUpdateIndex: false,
		PrettyMode:      "aggressive",
		PrettyAutoApply: false,
	}

	err := SaveConfig(config, tmpDir)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
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

	if loaded.PrettyMode != config.PrettyMode {
		t.Errorf("PrettyMode mismatch: expected %s, got %s", config.PrettyMode, loaded.PrettyMode)
	}

	if loaded.PrettyAutoApply != config.PrettyAutoApply {
		t.Error("PrettyAutoApply mismatch")
	}
}

func TestLoadConfigWithDefaults(t *testing.T) {
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

	if loaded.PrettyMode != "moderate" {
		t.Errorf("Expected default pretty mode moderate, got %s", loaded.PrettyMode)
	}
}

func TestLoadConfigNonExistent(t *testing.T) {
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
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub", "dir")
	os.MkdirAll(subDir, 0755)

	// Create .kb directory in tmpDir
	kbDir := filepath.Join(tmpDir, ".kb")
	os.MkdirAll(kbDir, 0755)

	// Change to subdirectory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
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
