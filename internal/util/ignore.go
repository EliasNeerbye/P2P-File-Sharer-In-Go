package util

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type IgnorePattern struct {
	Pattern   string
	IsNegated bool
}

type IgnoreList struct {
	Patterns []IgnorePattern
}

func LoadIgnoreFile(path string) (*IgnoreList, error) {
	ignoreList := &IgnoreList{
		Patterns: make([]IgnorePattern, 0),
	}

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		// Ignore file not found is not an error
		return ignoreList, nil
	} else if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		isNegated := false
		if strings.HasPrefix(line, "!") {
			isNegated = true
			line = line[1:]
		}

		ignoreList.Patterns = append(ignoreList.Patterns, IgnorePattern{
			Pattern:   line,
			IsNegated: isNegated,
		})
	}

	return ignoreList, scanner.Err()
}

func LoadDefaultIgnoreFile(baseDir string) *IgnoreList {
	// Try to load .fshignore first (our own format)
	ignoreList, err := LoadIgnoreFile(filepath.Join(baseDir, ".fshignore"))
	if err == nil && len(ignoreList.Patterns) > 0 {
		return ignoreList
	}

	// Try to load .gitignore as fallback
	ignoreList, err = LoadIgnoreFile(filepath.Join(baseDir, ".gitignore"))
	if err == nil {
		return ignoreList
	}

	// Return empty ignore list if no valid ignore file found
	return &IgnoreList{Patterns: make([]IgnorePattern, 0)}
}

func (il *IgnoreList) ShouldIgnore(path string) bool {
	if len(il.Patterns) == 0 {
		return false
	}

	// Clean path for checking
	path = filepath.ToSlash(path)

	// Default to not ignored
	ignored := false

	for _, pattern := range il.Patterns {
		matched, err := filepath.Match(pattern.Pattern, path)
		if err != nil {
			// If pattern is invalid, just skip it
			continue
		}

		// Also try to match with directories
		if !matched && strings.Contains(path, "/") {
			matched, _ = filepath.Match(pattern.Pattern+"/*", path)
		}

		if matched {
			ignored = !pattern.IsNegated
		}
	}

	return ignored
}
