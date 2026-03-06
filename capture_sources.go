package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var addURLHTTPClient = &http.Client{Timeout: 20 * time.Second}

type clipboardCommand struct {
	name string
	args []string
}

func readAddContentFromFile(path string) (string, CaptureSource, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", "", fmt.Errorf("empty file path")
	}

	file, err := os.Open(trimmed)
	if err != nil {
		return "", "", fmt.Errorf("failed to read file: %w", err)
	}
	defer file.Close()

	data, err := readLimitedInput(file, maxCaptureInputBytes, "file input")
	if err != nil {
		return "", "", err
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", CaptureSourceFile, nil
	}

	return fmt.Sprintf("Source file: %s\n\n%s", filepath.Clean(trimmed), content), CaptureSourceFile, nil
}

func readAddContentFromURL(rawURL string) (string, CaptureSource, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", "", fmt.Errorf("empty URL")
	}

	req, err := http.NewRequest("GET", trimmed, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create URL request: %w", err)
	}
	req.Header.Set("User-Agent", "kb/2 add-fetch")

	resp, err := addURLHTTPClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("failed to fetch URL: status %d", resp.StatusCode)
	}

	body, err := readLimitedInput(resp.Body, maxCaptureInputBytes, "URL response")
	if err != nil {
		return "", "", err
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") {
		title, text := extractReadableTextFromHTML(body)
		var builder strings.Builder
		builder.WriteString("Source URL: ")
		builder.WriteString(trimmed)
		builder.WriteString("\n")
		if title != "" {
			builder.WriteString("Source title: ")
			builder.WriteString(title)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
		builder.WriteString(strings.TrimSpace(text))
		return strings.TrimSpace(builder.String()), CaptureSourceURL, nil
	}

	content := strings.TrimSpace(string(body))
	if content == "" {
		return fmt.Sprintf("Source URL: %s", trimmed), CaptureSourceURL, nil
	}

	return fmt.Sprintf("Source URL: %s\n\n%s", trimmed, content), CaptureSourceURL, nil
}

func extractReadableTextFromHTML(document []byte) (string, string) {
	root, err := html.Parse(strings.NewReader(string(document)))
	if err != nil {
		return "", strings.TrimSpace(string(document))
	}

	title := strings.TrimSpace(extractHTMLTitle(root))
	text := collapseWhitespace(extractHTMLVisibleText(root))
	text = clampString(text, 12000)

	return title, text
}

func extractHTMLTitle(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, "title") && node.FirstChild != nil {
		return node.FirstChild.Data
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if title := extractHTMLTitle(child); title != "" {
			return title
		}
	}
	return ""
}

func extractHTMLVisibleText(node *html.Node) string {
	if node == nil {
		return ""
	}

	switch node.Type {
	case html.TextNode:
		return node.Data
	case html.ElementNode:
		switch strings.ToLower(node.Data) {
		case "script", "style", "noscript", "svg":
			return ""
		}
	}

	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text := extractHTMLVisibleText(child)
		if text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(text)
	}

	return builder.String()
}

func collapseWhitespace(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

func readAddContentFromClipboard() (string, CaptureSource, error) {
	for _, candidate := range clipboardCommands() {
		path, err := exec.LookPath(candidate.name)
		if err != nil {
			continue
		}

		cmd := exec.Command(path, candidate.args...)
		output, err := cmd.Output()
		if err != nil {
			continue
		}
		if len(output) > maxCaptureInputBytes {
			return "", "", fmt.Errorf("clipboard exceeds the %d KiB capture limit", maxCaptureInputBytes/1024)
		}

		return strings.TrimSpace(string(output)), CaptureSourceClipboard, nil
	}

	return "", "", fmt.Errorf("no clipboard command available (tried pbpaste, wl-paste, xclip, xsel)")
}

func clipboardCommands() []clipboardCommand {
	switch currentGOOS {
	case "darwin":
		return []clipboardCommand{
			{name: "pbpaste"},
			{name: "wl-paste", args: []string{"--no-newline"}},
			{name: "xclip", args: []string{"-selection", "clipboard", "-o"}},
			{name: "xsel", args: []string{"--clipboard", "--output"}},
		}
	default:
		return []clipboardCommand{
			{name: "wl-paste", args: []string{"--no-newline"}},
			{name: "xclip", args: []string{"-selection", "clipboard", "-o"}},
			{name: "xsel", args: []string{"--clipboard", "--output"}},
			{name: "pbpaste"},
		}
	}
}
