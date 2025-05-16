package util

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type IgnorePattern struct {
	Pattern string
	IsDir   bool
}

type IgnoreList struct {
	Patterns []IgnorePattern
	RawLines []string // Keep original pattern strings for debugging
}

func LoadIgnoreFile(baseFolder string) (*IgnoreList, error) {
	ignoreFile := filepath.Join(baseFolder, ".p2pignore")

	// Check if the ignore file exists
	if _, err := os.Stat(ignoreFile); os.IsNotExist(err) {
		// No ignore file, create an empty ignore list
		return &IgnoreList{
			Patterns: []IgnorePattern{},
			RawLines: []string{},
		}, nil
	}

	file, err := os.Open(ignoreFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	ignoreList := &IgnoreList{
		Patterns: []IgnorePattern{},
		RawLines: []string{},
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Store original pattern for reference
		ignoreList.RawLines = append(ignoreList.RawLines, line)

		// Check if it's a directory pattern (ends with /)
		isDir := strings.HasSuffix(line, "/")
		if isDir {
			line = line[:len(line)-1]
		}

		ignoreList.Patterns = append(ignoreList.Patterns, IgnorePattern{
			Pattern: line,
			IsDir:   isDir,
		})
	}

	return ignoreList, scanner.Err()
}

func (il *IgnoreList) ShouldIgnore(path string, isDir bool) bool {
	if il == nil {
		return false
	}

	// Always ignore the .p2pignore file itself
	filename := filepath.Base(path)
	if filename == ".p2pignore" {
		return true
	}

	if len(il.Patterns) == 0 {
		return false
	}

	// For exact filename matching (handles the case in the example)
	for _, pattern := range il.Patterns {
		// Check if the pattern exactly matches the filename
		if pattern.Pattern == filename {
			return true
		}
	}

	normalizedPath := filepath.ToSlash(path)
	for _, pattern := range il.Patterns {
		// If the pattern is for a directory and the path is not a directory, skip
		if pattern.IsDir && !isDir {
			continue
		}

		// Direct match with full path
		if pattern.Pattern == normalizedPath {
			return true
		}

		// Check if the pattern matches the entire base filename
		if strings.Contains(pattern.Pattern, "*") || strings.Contains(pattern.Pattern, "?") {
			// Try to match against the full path first
			matched, err := filepath.Match(pattern.Pattern, normalizedPath)
			if err == nil && matched {
				return true
			}

			// Try to match against the base filename
			matched, err = filepath.Match(pattern.Pattern, filename)
			if err == nil && matched {
				return true
			}

			// Check each component in the path
			pathParts := strings.Split(normalizedPath, "/")
			for _, part := range pathParts {
				matched, err := filepath.Match(pattern.Pattern, part)
				if err == nil && matched {
					return true
				}
			}
		}

		// Check for directory prefix match (e.g. "dir/" should match "dir/file.txt")
		if pattern.IsDir && strings.HasPrefix(normalizedPath, pattern.Pattern+"/") {
			return true
		}

		// Check extensions (*.ext format)
		if strings.HasPrefix(pattern.Pattern, "*.") {
			ext := pattern.Pattern[1:] // Get the ".ext" part
			if strings.HasSuffix(filename, ext) {
				return true
			}
		}
	}

	return false
}
