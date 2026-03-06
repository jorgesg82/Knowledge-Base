package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	tempFile, err := os.CreateTemp(dir, ".kb-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tempPath := tempFile.Name()
	cleanup := func() {
		_ = os.Remove(tempPath)
	}

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tempFile.Chmod(perm); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		cleanup()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return fmt.Errorf("failed to rename temp file into place: %w", err)
	}

	return nil
}
