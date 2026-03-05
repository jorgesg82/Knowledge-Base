package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func canonicalPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		return filepath.Clean(resolvedPath), nil
	}

	return filepath.Clean(absPath), nil
}

func sameFilePath(pathA, pathB string) bool {
	infoA, err := os.Stat(pathA)
	if err != nil {
		return false
	}

	infoB, err := os.Stat(pathB)
	if err != nil {
		return false
	}

	return os.SameFile(infoA, infoB)
}

func workingDirectory() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Prefer the logical shell path when it points at the same directory.
	pwd := strings.TrimSpace(os.Getenv("PWD"))
	if pwd != "" && sameFilePath(pwd, cwd) {
		return filepath.Clean(pwd), nil
	}

	return canonicalPath(cwd)
}

func isWithinBase(basePath, targetPath string) bool {
	cleanBase := filepath.Clean(basePath)
	cleanTarget := filepath.Clean(targetPath)

	relPath, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return false
	}

	if relPath == ".." {
		return false
	}

	return !strings.HasPrefix(relPath, ".."+string(os.PathSeparator))
}

func safeJoinUnderBase(basePath, relativePath string) (string, error) {
	normalizedPath := strings.ReplaceAll(relativePath, "\\", "/")
	for _, part := range strings.Split(normalizedPath, "/") {
		if part == ".." {
			return "", fmt.Errorf("parent directory references are not allowed: %s", relativePath)
		}
	}

	cleanRelativePath := filepath.Clean(relativePath)
	if cleanRelativePath == "." {
		return filepath.Clean(basePath), nil
	}

	if filepath.IsAbs(cleanRelativePath) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", relativePath)
	}

	targetPath := filepath.Join(basePath, cleanRelativePath)
	if !isWithinBase(basePath, targetPath) {
		return "", fmt.Errorf("path escapes KB root: %s", relativePath)
	}

	return targetPath, nil
}
