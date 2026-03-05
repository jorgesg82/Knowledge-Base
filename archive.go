package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ExportKB(kbPath, exportPath string) (int, error) {
	absExportPath, err := filepath.Abs(exportPath)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve export path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(absExportPath), 0755); err != nil {
		return 0, fmt.Errorf("failed to create export directory: %w", err)
	}

	file, err := os.Create(absExportPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create export file: %w", err)
	}
	defer file.Close()

	return writeKBArchive(file, kbPath, absExportPath)
}

func writeKBArchive(writer io.Writer, kbPath, excludePath string) (int, error) {
	gzipWriter := gzip.NewWriter(writer)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	entryCount := 0
	err := filepath.Walk(kbPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if excludePath != "" && sameFilePath(path, excludePath) {
			return nil
		}

		relPath, err := filepath.Rel(kbPath, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}

		if _, err := io.Copy(tarWriter, file); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}

		if strings.HasSuffix(relPath, ".md") {
			entryCount++
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create export: %w", err)
	}

	return entryCount, nil
}

func ImportKB(kbPath, tarballPath string) (int, error) {
	file, err := os.Open(tarballPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open tarball: %w", err)
	}
	defer file.Close()

	return importKBArchive(kbPath, file)
}

func importKBArchive(kbPath string, reader io.Reader) (int, error) {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return 0, fmt.Errorf("failed to read gzip: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	entryCount := 0

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("failed to read tar: %w", err)
		}

		targetPath, err := safeJoinUnderBase(kbPath, header.Name)
		if err != nil {
			return 0, fmt.Errorf("invalid archive path %q: %w", header.Name, err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return 0, fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return 0, fmt.Errorf("failed to create directory: %w", err)
			}

			outFile, err := os.Create(targetPath)
			if err != nil {
				return 0, fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return 0, fmt.Errorf("failed to write file: %w", err)
			}
			if err := outFile.Close(); err != nil {
				return 0, fmt.Errorf("failed to close file: %w", err)
			}

			if strings.HasSuffix(header.Name, ".md") {
				entryCount++
			}
		default:
			continue
		}
	}

	index, err := RebuildIndex(kbPath)
	if err != nil {
		return 0, fmt.Errorf("failed to rebuild index: %w", err)
	}

	if err := SaveIndex(index, kbPath); err != nil {
		return 0, fmt.Errorf("failed to save index: %w", err)
	}

	return entryCount, nil
}
