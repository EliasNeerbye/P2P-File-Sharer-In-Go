package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func ListFiles(dirPath, baseFolder string, recursive bool) ([]string, error) {
	var absPath string
	var err error

	if filepath.IsAbs(dirPath) {
		absPath = dirPath
	} else {
		absPath, err = filepath.Abs(filepath.Join(baseFolder, dirPath))
		if err != nil {
			return nil, err
		}
	}

	absBaseFolder, err := filepath.Abs(baseFolder)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(absPath, absBaseFolder) {
		return nil, fmt.Errorf("access denied: path is outside the shared folder")
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		_, filename := filepath.Split(absPath)
		return []string{filename}, nil
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}

	var files []string

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			files = append(files, name+"/")
		} else {
			info, err := entry.Info()
			if err == nil {
				size := formatFileSize(info.Size())
				files = append(files, fmt.Sprintf("%-40s %10s", name, size))
			} else {
				files = append(files, name)
			}
		}

		if recursive && entry.IsDir() {
			subDir := filepath.Join(absPath, name)

			subFiles, err := ListFiles(subDir, baseFolder, recursive)
			if err != nil {
				continue
			}

			for i, subFile := range subFiles {
				subFiles[i] = filepath.Join(name, subFile)
			}

			files = append(files, subFiles...)
		}
	}

	return files, nil
}

func formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	} else if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
}

func ListFilesRecursive(dirPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

func FindMatchingFiles(baseFolder, pattern string) ([]string, error) {
	var matches []string

	dir := filepath.Dir(pattern)
	if dir == "." {
		dir = ""
	}

	filePattern := filepath.Base(pattern)

	fullDirPath := filepath.Join(baseFolder, dir)

	if _, err := os.Stat(fullDirPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory not found: %s", dir)
	}

	err := filepath.Walk(fullDirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && !strings.Contains(filePattern, "*/") {
			return nil
		}

		relPath, err := filepath.Rel(baseFolder, path)
		if err != nil {
			return err
		}

		match, err := filepath.Match(filePattern, filepath.Base(path))
		if err != nil {
			return err
		}

		if match || filePattern == "*" {
			matches = append(matches, relPath)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return matches, nil
}

func ParseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func ParseFloat64(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func ResolvePath(path, baseDir string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %v", err)
	}

	var absPath string
	if filepath.IsAbs(path) {
		absPath = path
	} else {
		absPath = filepath.Join(absBase, path)
	}

	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %v", err)
	}

	if !strings.HasPrefix(absPath, absBase) {
		return "", fmt.Errorf("access denied: path is outside the shared folder")
	}

	return absPath, nil
}

func IsValidRelativePath(path string) bool {
	if path == "" {
		return false
	}

	if strings.Contains(path, "..") {
		return false
	}

	if strings.Contains(path, "\\") {
		return false
	}

	if strings.HasPrefix(path, "/") {
		return false
	}

	if strings.Contains(path, ":") {
		return false
	}

	return true
}
