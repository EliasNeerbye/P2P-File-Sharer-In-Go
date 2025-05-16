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

	// Handle default case of current directory
	if dirPath == "" || dirPath == "." {
		absPath = baseFolder
	} else {
		// Normalize the path
		normalizedPath := NormalizePath(dirPath)

		if filepath.IsAbs(normalizedPath) {
			absPath = normalizedPath
		} else {
			// Don't use filepath.Join which could cause path doubling
			if strings.HasSuffix(baseFolder, "/") || strings.HasSuffix(baseFolder, "\\") {
				absPath = baseFolder + normalizedPath
			} else {
				absPath = baseFolder + "/" + normalizedPath
			}

			// Now convert to absolute
			absPath, err = filepath.Abs(absPath)
			if err != nil {
				return nil, err
			}
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
	return FormatFileSize(size)
}

// FormatFileSize formats a file size in bytes to a human-readable string
func FormatFileSize(size int64) string {
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

	// Normalize the pattern
	normalizedPattern := NormalizePath(pattern)

	dir := filepath.Dir(normalizedPattern)
	if dir == "." {
		dir = ""
	}

	filePattern := filepath.Base(normalizedPattern)

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

		// Normalize the relative path
		relPath = NormalizePath(relPath)

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
	// Handle "." specially as it represents the current directory
	if path == "." || path == "" {
		return baseDir, nil
	}

	// Normalize the input path
	normalizedPath := NormalizePath(path)

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %v", err)
	}

	// Handle paths more reliably
	var absPath string
	if filepath.IsAbs(normalizedPath) {
		absPath = normalizedPath
	} else {
		// Direct join instead of filepath.Join to prevent path issues
		if strings.HasSuffix(absBase, "/") || strings.HasSuffix(absBase, "\\") {
			absPath = absBase + normalizedPath
		} else {
			absPath = absBase + string(filepath.Separator) + normalizedPath
		}
	}

	absPath = filepath.Clean(absPath)

	// Check if the resolved path is within the base directory
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %v", err)
	}

	if !strings.HasPrefix(absPath, absBase) {
		return "", fmt.Errorf("access denied: path is outside the shared folder")
	}

	return absPath, nil
}

// FilterIgnoredFiles filters out files that match patterns in the ignore list
func FilterIgnoredFiles(files []string, baseDir string, ignoreList *IgnoreList) []string {
	if ignoreList == nil || len(ignoreList.Patterns) == 0 {
		return files
	}

	var filtered []string
	for _, file := range files {
		// Normalize the path
		normalizedPath := NormalizePath(file)
		isDir := strings.HasSuffix(normalizedPath, "/")

		relPath, err := filepath.Rel(baseDir, file)
		if err != nil {
			// If we can't get a relative path, include the file
			filtered = append(filtered, file)
			continue
		}

		// Normalize the relative path
		relPath = NormalizePath(relPath)

		if !ignoreList.ShouldIgnore(relPath, isDir) {
			filtered = append(filtered, file)
		}
	}

	return filtered
}

// EnsureUniqueFilename ensures the filename is unique by appending a suffix
func EnsureUniqueFilename(filePath string) string {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return filePath
	}

	dir := filepath.Dir(filePath)
	baseName := filepath.Base(filePath)
	ext := filepath.Ext(baseName)
	nameOnly := strings.TrimSuffix(baseName, ext)

	counter := 1
	for {
		newName := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", nameOnly, counter, ext))
		if _, err := os.Stat(newName); os.IsNotExist(err) {
			return newName
		}
		counter++
	}
}
