package main

import (
	"fmt"
	"strings"
)

type captureEnvelope struct {
	SourceFile  string
	SourceURL   string
	SourceTitle string
	Body        string
}

func parseCaptureEnvelope(raw string) captureEnvelope {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	env := captureEnvelope{}
	bodyStart := 0

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			if env.SourceFile != "" || env.SourceURL != "" || env.SourceTitle != "" {
				bodyStart = i + 1
				break
			}
			continue
		}

		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "source file:"):
			env.SourceFile = strings.TrimSpace(line[len("Source file:"):])
			bodyStart = i + 1
		case strings.HasPrefix(lower, "source url:"):
			env.SourceURL = strings.TrimSpace(line[len("Source URL:"):])
			bodyStart = i + 1
		case strings.HasPrefix(lower, "source title:"):
			env.SourceTitle = strings.TrimSpace(line[len("Source title:"):])
			bodyStart = i + 1
		default:
			bodyStart = i
			env.Body = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
			return env
		}
	}

	if env.Body == "" && bodyStart < len(lines) {
		env.Body = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	}

	return env
}

func renderCanonicalCaptureMarkdown(content, title string) string {
	env := parseCaptureEnvelope(content)

	parts := []string{}
	if sourceBlock := renderCaptureSourceBlock(env); sourceBlock != "" {
		parts = append(parts, sourceBlock)
	}

	body := normalizeCaptureMarkdownBody(env.Body, title)
	if body != "" {
		parts = append(parts, body)
	}

	if len(parts) == 0 {
		return strings.TrimSpace(content)
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func renderCaptureSourceBlock(env captureEnvelope) string {
	lines := []string{}
	if env.SourceFile != "" {
		lines = append(lines, fmt.Sprintf("> Source file: `%s`", env.SourceFile))
	}
	if env.SourceURL != "" {
		lines = append(lines, fmt.Sprintf("> Source URL: <%s>", env.SourceURL))
	}
	if env.SourceTitle != "" {
		lines = append(lines, fmt.Sprintf("> Source title: %s", env.SourceTitle))
	}

	return strings.Join(lines, "\n")
}

func normalizeCaptureMarkdownBody(body, title string) string {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	if body == "" {
		return ""
	}

	lines := strings.Split(body, "\n")
	firstIdx := firstMeaningfulLineIndex(lines)
	if firstIdx >= 0 {
		trimmedTitle := normalizeComparableText(title)
		firstLine := strings.TrimSpace(lines[firstIdx])
		if trimmedTitle != "" && normalizeComparableText(cleanPotentialTitleLine(firstLine)) == trimmedTitle {
			remainder := remainderAfterTitleLine(firstLine, title)
			hasMoreContent := hasMeaningfulLineAfter(lines, firstIdx)
			switch {
			case remainder != "":
				lines[firstIdx] = remainder
			case hasMoreContent:
				lines = append(lines[:firstIdx], lines[firstIdx+1:]...)
			}
		}
	}

	return trimAndCollapseBlankLines(strings.Join(lines, "\n"))
}

func remainderAfterTitleLine(line, title string) string {
	normalizedTitle := normalizeComparableText(title)
	for _, sep := range []string{". ", ": ", " - ", " -- "} {
		idx := strings.Index(line, sep)
		if idx == -1 {
			continue
		}

		prefix := strings.TrimSpace(line[:idx])
		if normalizeComparableText(cleanPotentialTitleLine(prefix)) == normalizedTitle {
			return strings.TrimSpace(line[idx+len(sep):])
		}
	}

	return ""
}

func firstMeaningfulLineIndex(lines []string) int {
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "```" {
			continue
		}
		return i
	}
	return -1
}

func hasMeaningfulLineAfter(lines []string, idx int) bool {
	for i := idx + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || line == "```" {
			continue
		}
		return true
	}
	return false
}

func trimAndCollapseBlankLines(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	blank := false

	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if blank {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}

	return strings.TrimSpace(strings.Join(out, "\n"))
}

func inferSummaryFromContentWithTitle(content, title string) string {
	env := parseCaptureEnvelope(content)
	body := normalizeCaptureMarkdownBody(env.Body, title)
	summary := markdownishPlainText(body)
	if summary == "" {
		summary = markdownishPlainText(env.SourceTitle)
	}
	if len(summary) > 140 {
		summary = strings.TrimSpace(summary[:140]) + "..."
	}
	return summary
}

func markdownishPlainText(raw string) string {
	replacer := strings.NewReplacer(
		"\r\n", "\n",
		"`", "",
		"#", "",
		">", "",
		"*", "",
	)
	raw = replacer.Replace(raw)
	return strings.Join(strings.Fields(raw), " ")
}
