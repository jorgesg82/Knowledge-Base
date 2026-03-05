package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExportKBExcludesArchiveTarget(t *testing.T) {
	tmpDir := t.TempDir()
	entryPath := filepath.Join(tmpDir, "entries", "test", "entry.md")
	entry := &Entry{
		Metadata: EntryMetadata{
			Title:    "Entry",
			Category: "test",
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		Content:  "# Entry\n\nContent",
		FilePath: entryPath,
		ID:       "test-entry",
	}

	if err := WriteEntry(entry, entryPath); err != nil {
		t.Fatalf("Failed to write entry: %v", err)
	}

	exportPath := filepath.Join(tmpDir, "kb-export.tar.gz")
	count, err := ExportKB(tmpDir, exportPath)
	if err != nil {
		t.Fatalf("ExportKB failed: %v", err)
	}

	if count != 1 {
		t.Fatalf("Expected 1 exported markdown entry, got %d", count)
	}

	file, err := os.Open(exportPath)
	if err != nil {
		t.Fatalf("Failed to open export: %v", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("Failed to open gzip reader: %v", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read tar: %v", err)
		}

		if header.Name == "kb-export.tar.gz" {
			t.Fatal("Export archive should not contain itself")
		}
	}
}

func TestImportKBRejectsPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	var archive bytes.Buffer

	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)

	body := []byte("stolen")
	header := &tar.Header{
		Name: "entries/test/../../outside.txt",
		Mode: 0644,
		Size: int64(len(body)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}
	if _, err := tarWriter.Write(body); err != nil {
		t.Fatalf("Failed to write body: %v", err)
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	errCount, err := importKBArchive(tmpDir, bytes.NewReader(archive.Bytes()))
	if err == nil {
		t.Fatalf("Expected path traversal import to fail, imported %d files", errCount)
	}

	outsidePath := filepath.Join(filepath.Dir(tmpDir), "outside.txt")
	if _, statErr := os.Stat(outsidePath); !os.IsNotExist(statErr) {
		t.Fatalf("Path traversal created file outside KB root: %s", outsidePath)
	}
}

func TestImportKBRebuildsIndex(t *testing.T) {
	tmpDir := t.TempDir()
	var archive bytes.Buffer

	entryContent := `---
title: Imported Entry
category: test
created: 2026-03-05T10:00:00Z
updated: 2026-03-05T10:00:00Z
---

# Imported Entry

Imported content.
`

	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)

	files := map[string]string{
		"entries/test/imported-entry.md": entryContent,
	}

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write header for %s: %v", name, err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("Failed to write content for %s: %v", name, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	count, err := importKBArchive(tmpDir, bytes.NewReader(archive.Bytes()))
	if err != nil {
		t.Fatalf("ImportKB failed: %v", err)
	}

	if count != 1 {
		t.Fatalf("Expected 1 imported markdown entry, got %d", count)
	}

	index, err := LoadIndex(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load rebuilt index: %v", err)
	}

	if len(index.Entries) != 1 {
		t.Fatalf("Expected rebuilt index to contain 1 entry, got %d", len(index.Entries))
	}

	if index.Entries[0].ID != "test-imported-entry" {
		t.Errorf("Expected rebuilt entry ID test-imported-entry, got %s", index.Entries[0].ID)
	}
}
