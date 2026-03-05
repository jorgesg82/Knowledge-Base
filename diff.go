package main

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

func buildUnifiedDiff(current, proposed string) (string, error) {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(ensureTrailingNewline(current)),
		B:        difflib.SplitLines(ensureTrailingNewline(proposed)),
		FromFile: "current",
		ToFile:   "proposed",
		Context:  3,
	}

	return difflib.GetUnifiedDiffString(diff)
}

func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
