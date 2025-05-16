// internal/util/ignore.go
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
		return ignoreList, nil
	} else if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

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
	// Always add system files to ignore list
	defaultIgnore := &IgnoreList{
		Patterns: []IgnorePattern{
			{Pattern: ".fshignore", IsNegated: false},
			{Pattern: ".gitignore", IsNegated: false},
		},
	}

	// Try to load .fshignore first (our own format)
	ignoreList, err := LoadIgnoreFile(filepath.Join(baseDir, ".fshignore"))
	if err == nil && len(ignoreList.Patterns) > 0 {
		defaultIgnore.Patterns = append(defaultIgnore.Patterns, ignoreList.Patterns...)
		return defaultIgnore
	}

	// Try to load .gitignore as fallback
	ignoreList, err = LoadIgnoreFile(filepath.Join(baseDir, ".gitignore"))
	if err == nil {
		defaultIgnore.Patterns = append(defaultIgnore.Patterns, ignoreList.Patterns...)
		return defaultIgnore
	}

	return defaultIgnore
}

func (il *IgnoreList) ShouldIgnore(path string) bool {
	if len(il.Patterns) == 0 {
		return false
	}

	// Always ignore system files
	baseName := filepath.Base(path)
	if baseName == ".fshignore" || baseName == ".gitignore" {
		return true
	}

	// Clean path for checking
	path = filepath.ToSlash(path)

	// Default to not ignored
	ignored := false

	for _, pattern := range il.Patterns {
		matched, err := filepath.Match(pattern.Pattern, path)
		if err != nil {
			continue
		}

		// Also check basename match
		if !matched {
			matched, _ = filepath.Match(pattern.Pattern, baseName)
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

func IsSystemFile(path string) bool {
	basename := filepath.Base(path)
	return basename == ".fshignore" || basename == ".gitignore"
}
