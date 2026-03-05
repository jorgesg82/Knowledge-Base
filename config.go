package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	KBPath          string `yaml:"kb_path"`
	Editor          string `yaml:"editor"`
	Viewer          string `yaml:"viewer"`
	DefaultCategory string `yaml:"default_category"`
	AutoUpdateIndex bool   `yaml:"auto_update_index"`
	PrettyProvider  string `yaml:"pretty_provider"`
	PrettyMode      string `yaml:"pretty_mode"`
	PrettyAutoApply bool   `yaml:"pretty_auto_apply"`
}

func findCommand(names ...string) string {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func GetDefaultConfig(kbPath string) *Config {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = findCommand("nvim", "vim")
		if editor == "" {
			editor = "vim"
		}
	}

	viewer := findCommand("glow", "bat", "mdcat", "mdless")
	if viewer == "" {
		viewer = "less"
	}

	return &Config{
		KBPath:          kbPath,
		Editor:          editor,
		Viewer:          viewer,
		DefaultCategory: "misc",
		AutoUpdateIndex: true,
		PrettyProvider:  "claude",
		PrettyMode:      "moderate",
		PrettyAutoApply: true,
	}
}

func LoadConfig(kbPath string) (*Config, error) {
	configPath := filepath.Join(kbPath, ".kb", "config.yml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return GetDefaultConfig(kbPath), nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	config := *GetDefaultConfig(kbPath)
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if config.KBPath == "" {
		config.KBPath = kbPath
	}

	return &config, nil
}

func SaveConfig(config *Config, kbPath string) error {
	configDir := filepath.Join(kbPath, ".kb")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yml")
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func GetKBPath() (string, error) {
	cwd, err := workingDirectory()
	if err != nil {
		return "", err
	}

	for dir := cwd; ; dir = filepath.Dir(dir) {
		kbDir := filepath.Join(dir, ".kb")
		if _, err := os.Stat(kbDir); err == nil {
			return dir, nil
		}

		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			break
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	defaultKB := filepath.Join(homeDir, "kb")
	if _, err := os.Stat(filepath.Join(defaultKB, ".kb")); err == nil {
		return defaultKB, nil
	}

	return "", fmt.Errorf("not in a KB directory (use 'kb init' to create one)")
}
