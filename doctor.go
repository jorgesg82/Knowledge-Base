package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DoctorCheck struct {
	Name   string
	OK     bool
	Detail string
}

func resolveCommandPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("not configured")
	}
	if isBuiltinViewer(name) {
		return builtinViewerName + " renderer", nil
	}

	if strings.ContainsRune(name, os.PathSeparator) {
		if _, err := os.Stat(name); err != nil {
			return "", err
		}
		return name, nil
	}

	return exec.LookPath(name)
}

func collectDoctorChecks(kbPath string, config *Config) []DoctorCheck {
	checks := []DoctorCheck{
		{
			Name:   "KB root",
			OK:     kbPath != "",
			Detail: kbPath,
		},
	}

	entriesPath := filepath.Join(kbPath, "entries")
	if info, err := os.Stat(entriesPath); err == nil && info.IsDir() {
		checks = append(checks, DoctorCheck{
			Name:   "Entries directory",
			OK:     true,
			Detail: entriesPath,
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:   "Entries directory",
			OK:     false,
			Detail: entriesPath + " missing",
		})
	}

	if index, err := LoadIndex(kbPath); err == nil {
		checks = append(checks, DoctorCheck{
			Name:   "Index",
			OK:     true,
			Detail: fmt.Sprintf("%d entries", len(SnapshotEntries(index))),
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:   "Index",
			OK:     false,
			Detail: err.Error(),
		})
	}

	if config != nil {
		if path, err := resolveCommandPath(config.Editor); err == nil {
			checks = append(checks, DoctorCheck{
				Name:   "Editor",
				OK:     true,
				Detail: path,
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:   "Editor",
				OK:     false,
				Detail: fmt.Sprintf("%s (%v)", config.Editor, err),
			})
		}

		if path, err := resolveCommandPath(config.Viewer); err == nil {
			checks = append(checks, DoctorCheck{
				Name:   "Viewer",
				OK:     true,
				Detail: path,
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:   "Viewer",
				OK:     true,
				Detail: fmt.Sprintf("%s unavailable, using %s renderer fallback", config.Viewer, builtinViewerName),
			})
		}

		resolvedProvider, err := ResolvePrettyProvider(config.PrettyProvider)
		if err != nil {
			checks = append(checks, DoctorCheck{
				Name:   "Pretty provider",
				OK:     false,
				Detail: err.Error(),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:   "Pretty provider",
				OK:     true,
				Detail: fmt.Sprintf("configured=%s resolved=%s", config.PrettyProvider, resolvedProvider),
			})

			switch resolvedProvider {
			case ProviderChatGPT:
				apiKeySet := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != ""
				detail := "OPENAI_API_KEY missing"
				if apiKeySet {
					detail = "OPENAI_API_KEY set"
				}
				if projectID := strings.TrimSpace(os.Getenv("OPENAI_PROJECT_ID")); projectID != "" {
					detail += ", project=" + projectID
				}
				checks = append(checks, DoctorCheck{
					Name:   "OpenAI",
					OK:     apiKeySet,
					Detail: detail,
				})
			case ProviderClaude:
				ready := hasClaudeConfig()
				detail := "ANTHROPIC_API_KEY/ANTHROPIC_CUSTOM_HEADERS missing"
				if strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "" {
					detail = "ANTHROPIC_API_KEY set"
				} else if strings.TrimSpace(os.Getenv("ANTHROPIC_CUSTOM_HEADERS")) != "" {
					detail = "ANTHROPIC_CUSTOM_HEADERS set"
				}
				checks = append(checks, DoctorCheck{
					Name:   "Claude",
					OK:     ready,
					Detail: detail,
				})
			}
		}
	}

	return checks
}

func printDoctorChecks(checks []DoctorCheck) {
	fmt.Println(Header("KB Doctor:"))
	for _, check := range checks {
		status := Red("FAIL")
		if check.OK {
			status = Green("OK")
		}
		fmt.Printf("  %s %-16s %s\n", status, check.Name, check.Detail)
	}
}

func handleDoctor(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	config, err := LoadConfig(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	printDoctorChecks(collectDoctorChecks(kbPath, config))
}
