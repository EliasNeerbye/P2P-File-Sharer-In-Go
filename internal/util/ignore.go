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
}

func LoadIgnoreFile(baseFolder string) (*IgnoreList, error) {
	ignoreFile := filepath.Join(baseFolder, ".p2pignore")

	// Check if the ignore file exists
	if _, err := os.Stat(ignoreFile); os.IsNotExist(err) {
		// No ignore file, create an empty ignore list
		return &IgnoreList{Patterns: []IgnorePattern{}}, nil
	}

	file, err := os.Open(ignoreFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	ignoreList := &IgnoreList{
		Patterns: []IgnorePattern{},
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

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
	if il == nil || len(il.Patterns) == 0 {
		return false
	}

	// Always ignore the .p2pignore file itself
	if filepath.Base(path) == ".p2pignore" {
		return true
	}

	normalizedPath := filepath.ToSlash(path)
	for _, pattern := range il.Patterns {
		// If the pattern is for a directory and the path is not a directory, skip
		if pattern.IsDir && !isDir {
			continue
		}

		// Direct match
		if pattern.Pattern == normalizedPath {
			return true
		}

		// Pattern with wildcards
		if strings.Contains(pattern.Pattern, "*") {
			matched, err := filepath.Match(pattern.Pattern, normalizedPath)
			if err == nil && matched {
				return true
			}

			// Check if it matches a path component
			pathParts := strings.Split(normalizedPath, "/")
			for _, part := range pathParts {
				matched, err := filepath.Match(pattern.Pattern, part)
				if err == nil && matched {
					return true
				}
			}
		}

		// Check for directory prefix match (e.g. "dir/" should match "dir/file.txt")
		if isDir && strings.HasPrefix(normalizedPath, pattern.Pattern+"/") {
			return true
		}
	}

	return false
}
