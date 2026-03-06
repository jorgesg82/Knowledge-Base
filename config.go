package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

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

func defaultEditorCandidates() []string {
	return []string{"nvim", "vim", "nano"}
}

func defaultViewerCandidates() []string {
	switch currentGOOS {
	case "linux":
		return []string{"glow", "bat", "batcat", "mdcat", "mdless"}
	default:
		return []string{"glow", "bat", "mdcat", "batcat", "mdless"}
	}
}

func parseEnvBool(name string) (bool, bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false, false, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, true, fmt.Errorf("invalid %s value %q", name, raw)
	}

	return value, true, nil
}

func applyConfigEnvOverrides(config *Config) error {
	if editor := strings.TrimSpace(os.Getenv("KB_EDITOR")); editor != "" {
		config.Editor = editor
	}
	if viewer := strings.TrimSpace(os.Getenv("KB_VIEWER")); viewer != "" {
		config.Viewer = viewer
	}
	if category := strings.TrimSpace(os.Getenv("KB_DEFAULT_CATEGORY")); category != "" {
		config.DefaultCategory = category
	}
	if provider := strings.TrimSpace(os.Getenv("KB_PRETTY_PROVIDER")); provider != "" {
		config.PrettyProvider = provider
	}
	if mode := strings.TrimSpace(os.Getenv("KB_PRETTY_MODE")); mode != "" {
		config.PrettyMode = mode
	}

	if autoUpdateIndex, ok, err := parseEnvBool("KB_AUTO_UPDATE_INDEX"); err != nil {
		return err
	} else if ok {
		config.AutoUpdateIndex = autoUpdateIndex
	}

	if autoApply, ok, err := parseEnvBool("KB_PRETTY_AUTO_APPLY"); err != nil {
		return err
	} else if ok {
		config.PrettyAutoApply = autoApply
	}

	return nil
}

func GetDefaultConfig(kbPath string) *Config {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = findCommand(defaultEditorCandidates()...)
		if editor == "" {
			editor = "vim"
		}
	}

	viewer := findCommand(defaultViewerCandidates()...)
	if viewer == "" {
		viewer = builtinViewerName
	}

	return &Config{
		KBPath:          kbPath,
		Editor:          editor,
		Viewer:          viewer,
		DefaultCategory: "misc",
		AutoUpdateIndex: true,
		PrettyProvider:  defaultPrettyProvider(),
		PrettyMode:      "moderate",
		PrettyAutoApply: true,
	}
}

func LoadConfig(kbPath string) (*Config, error) {
	configPath := filepath.Join(kbPath, ".kb", "config.yml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			config := GetDefaultConfig(kbPath)
			if err := applyConfigEnvOverrides(config); err != nil {
				return nil, err
			}
			config.KBPath = kbPath
			return config, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	config := *GetDefaultConfig(kbPath)
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	config.KBPath = kbPath

	if err := applyConfigEnvOverrides(&config); err != nil {
		return nil, err
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

	if envKBPath := strings.TrimSpace(os.Getenv("KB_PATH")); envKBPath != "" {
		if _, err := os.Stat(filepath.Join(envKBPath, ".kb")); err == nil {
			return envKBPath, nil
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
