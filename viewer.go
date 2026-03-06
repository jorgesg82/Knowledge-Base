package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour"
)

const builtinViewerName = "builtin"

func isBuiltinViewer(viewer string) bool {
	viewer = strings.TrimSpace(viewer)
	if viewer == "" {
		return true
	}

	switch strings.ToLower(filepath.Base(viewer)) {
	case "", builtinViewerName, "auto", "internal", "less":
		return true
	default:
		return false
	}
}

func showEntryWithViewer(viewer, entryPath string) error {
	if isBuiltinViewer(viewer) {
		return showEntryWithBuiltinRenderer(entryPath)
	}

	if _, err := resolveCommandPath(viewer); err != nil {
		return showEntryWithBuiltinRenderer(entryPath)
	}

	viewerBase := filepath.Base(viewer)

	switch viewerBase {
	case "glow":
		return runViewerCommand(exec.Command(viewer, "-p", entryPath))
	case "bat", "batcat":
		return runViewerCommand(exec.Command(viewer, "--paging=always", entryPath))
	case "mdcat":
		return pipeMarkdownViewerToLess(exec.Command(viewer, entryPath))
	default:
		return runViewerCommand(exec.Command(viewer, entryPath))
	}
}

func showEntryWithBuiltinRenderer(entryPath string) error {
	content, err := os.ReadFile(entryPath)
	if err != nil {
		return fmt.Errorf("failed to read entry: %w", err)
	}

	return showMarkdownWithBuiltinRenderer(builtinViewerMarkdown(entryPath, string(content)))
}

func builtinViewerMarkdown(entryPath, rawContent string) string {
	entry, err := ParseEntry(entryPath)
	if err != nil {
		return rawContent
	}

	var builder strings.Builder
	writeBuiltinViewerMetadata(&builder, entry)

	body := strings.TrimSpace(entry.Content)
	if body == "" {
		return builder.String()
	}

	if builder.Len() > 0 {
		builder.WriteString("\n")
	}

	if !strings.HasPrefix(body, "#") && entry.Metadata.Title != "" {
		builder.WriteString("# ")
		builder.WriteString(entry.Metadata.Title)
		builder.WriteString("\n\n")
	}

	builder.WriteString(body)
	builder.WriteString("\n")

	return builder.String()
}

func writeBuiltinViewerMetadata(builder *strings.Builder, entry *Entry) {
	lines := []string{}
	if entry.Metadata.Category != "" {
		lines = append(lines, fmt.Sprintf("Category: `%s`", entry.Metadata.Category))
	}
	if len(entry.Metadata.Tags) > 0 {
		lines = append(lines, fmt.Sprintf("Tags: `%s`", strings.Join(entry.Metadata.Tags, "`, `")))
	}
	if !entry.Metadata.Updated.IsZero() {
		lines = append(lines, "Updated: "+entry.Metadata.Updated.Format("2006-01-02 15:04"))
	}

	for _, line := range lines {
		builder.WriteString("> ")
		builder.WriteString(line)
		builder.WriteString("\n")
	}
}

func renderMarkdownForTerminal(content string) (string, error) {
	options := []glamour.TermRendererOption{
		glamour.WithEnvironmentConfig(),
		glamour.WithAutoStyle(),
	}

	if width := markdownRenderWidth(); width > 0 {
		options = append(options, glamour.WithWordWrap(width))
	}

	renderer, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return "", err
	}

	return renderer.Render(content)
}

func showMarkdownWithBuiltinRenderer(markdown string) error {
	rendered, err := renderMarkdownForTerminal(markdown)
	if err != nil {
		return fmt.Errorf("failed to render markdown: %w", err)
	}

	if shouldUsePager() {
		return pageRenderedMarkdown(rendered)
	}

	if _, err := fmt.Fprint(os.Stdout, rendered); err != nil {
		return err
	}
	if !strings.HasSuffix(rendered, "\n") {
		_, err = fmt.Fprintln(os.Stdout)
	}
	return err
}

func markdownRenderWidth() int {
	if raw := strings.TrimSpace(os.Getenv("COLUMNS")); raw != "" {
		if width, err := strconv.Atoi(raw); err == nil && width > 20 {
			return width
		}
	}

	return 100
}

func shouldUsePager() bool {
	lessPath, err := exec.LookPath("less")
	if err != nil || lessPath == "" {
		return false
	}

	return isTerminal(os.Stdout) && isTerminal(os.Stdin)
}

func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func pageRenderedMarkdown(rendered string) error {
	cmd := exec.Command("less", "-R")
	cmd.Stdin = strings.NewReader(rendered)
	return runViewerCommand(cmd)
}

func pipeMarkdownViewerToLess(viewerCmd *exec.Cmd) error {
	lessCmd := exec.Command("less", "-R")

	pipe, err := viewerCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}

	lessCmd.Stdin = pipe
	lessCmd.Stdout = os.Stdout
	lessCmd.Stderr = os.Stderr
	viewerCmd.Stderr = os.Stderr

	if err := lessCmd.Start(); err != nil {
		return fmt.Errorf("starting less: %w", err)
	}
	if err := viewerCmd.Run(); err != nil {
		return fmt.Errorf("running %s: %w", filepath.Base(viewerCmd.Path), err)
	}
	_ = pipe.Close()
	if err := lessCmd.Wait(); err != nil {
		return fmt.Errorf("waiting for less: %w", err)
	}

	return nil
}

func runViewerCommand(cmd *exec.Cmd) error {
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}
