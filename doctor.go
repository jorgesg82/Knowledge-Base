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

	if stats, err := loadStoreStats(kbPath); err == nil {
		checks = append(checks, DoctorCheck{
			Name:   "Store",
			OK:     true,
			Detail: fmt.Sprintf("%d notes, %d captures, %d ops", stats.Notes, stats.Captures, stats.Operations),
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:   "Store",
			OK:     false,
			Detail: err.Error(),
		})
	}

	if missing, err := countMissingMaterializedNotes(kbPath); err == nil {
		checks = append(checks, DoctorCheck{
			Name:   "Materialized notes",
			OK:     missing == 0,
			Detail: fmt.Sprintf("%d missing", missing),
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:   "Materialized notes",
			OK:     false,
			Detail: err.Error(),
		})
	}

	manifestPath := filepath.Join(kbPath, ".kb", notesManifestFileName)
	if info, err := os.Stat(manifestPath); err == nil && !info.IsDir() {
		checks = append(checks, DoctorCheck{
			Name:   "Notes manifest",
			OK:     true,
			Detail: manifestPath,
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:   "Notes manifest",
			OK:     false,
			Detail: manifestPath + " missing",
		})
	}

	if state, err := loadKBStateIfPresent(kbPath); err == nil && state != nil {
		checks = append(checks, DoctorCheck{
			Name:   "State",
			OK:     true,
			Detail: fmt.Sprintf("version=%d captures=%d notes=%d ops=%d", state.Version, state.LastCaptureSeq, state.LastNoteSeq, state.LastOperationSeq),
		})
	} else if err != nil {
		checks = append(checks, DoctorCheck{
			Name:   "State",
			OK:     false,
			Detail: err.Error(),
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:   "State",
			OK:     false,
			Detail: "state.json missing",
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

		resolvedProvider, err := ResolveAIProvider(config.AIProvider)
		if err != nil {
			checks = append(checks, DoctorCheck{
				Name:   "AI provider",
				OK:     false,
				Detail: err.Error(),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:   "AI provider",
				OK:     true,
				Detail: fmt.Sprintf("configured=%s resolved=%s", config.AIProvider, resolvedProvider),
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

func countMissingMaterializedNotes(kbPath string) (int, error) {
	notes, err := loadCanonicalNoteManifest(kbPath)
	if err != nil {
		return 0, err
	}

	missing := 0
	for _, note := range notes {
		if note == nil || strings.TrimSpace(note.MaterializedPath) == "" {
			missing++
			continue
		}
		if _, err := os.Stat(filepath.Join(kbPath, note.MaterializedPath)); err != nil {
			if os.IsNotExist(err) {
				missing++
				continue
			}
			return 0, err
		}
	}

	return missing, nil
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
