package main

import "testing"

func TestParseAddOptionsRejectsMultipleExplicitSources(t *testing.T) {
	config := &Config{AIProvider: "auto"}

	_, _, err := parseAddOptions([]string{"--file", "a.txt", "--url", "https://example.com"}, config)
	if err == nil {
		t.Fatal("expected parseAddOptions to reject multiple explicit sources")
	}
}

func TestInferNoteTitleCleansImperativeAndSentenceTail(t *testing.T) {
	title := inferNoteTitle("remember inspect open ports on macos. Use lsof -iTCP -sTCP:LISTEN.")
	if title != "Inspect Open Ports on macOS" {
		t.Fatalf("unexpected inferred title: %s", title)
	}
}

func TestParseAddOptionsRejectsLegacyFlag(t *testing.T) {
	config := &Config{AIProvider: "auto"}

	_, _, err := parseAddOptions([]string{"--legacy", "linux/ssh"}, config)
	if err == nil {
		t.Fatal("expected parseAddOptions to reject --legacy")
	}
}
