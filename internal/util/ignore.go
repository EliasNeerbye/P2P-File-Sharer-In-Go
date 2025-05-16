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
	RawLines []string 
}

func LoadIgnoreFile(baseFolder string) (*IgnoreList, error) {
	ignoreFile := filepath.Join(baseFolder, ".p2pignore")

	
	if _, err := os.Stat(ignoreFile); os.IsNotExist(err) {
		
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

		
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		
		ignoreList.RawLines = append(ignoreList.RawLines, line)

		
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

	
	filename := filepath.Base(path)
	if filename == ".p2pignore" {
		return true
	}

	if len(il.Patterns) == 0 {
		return false
	}

	
	for _, pattern := range il.Patterns {
		
		if pattern.Pattern == filename {
			return true
		}
	}

	normalizedPath := filepath.ToSlash(path)
	for _, pattern := range il.Patterns {
		
		if pattern.IsDir && !isDir {
			continue
		}

		
		if pattern.Pattern == normalizedPath {
			return true
		}

		
		if strings.Contains(pattern.Pattern, "*") || strings.Contains(pattern.Pattern, "?") {
			
			matched, err := filepath.Match(pattern.Pattern, normalizedPath)
			if err == nil && matched {
				return true
			}

			
			matched, err = filepath.Match(pattern.Pattern, filename)
			if err == nil && matched {
				return true
			}

			
			pathParts := strings.Split(normalizedPath, "/")
			for _, part := range pathParts {
				matched, err := filepath.Match(pattern.Pattern, part)
				if err == nil && matched {
					return true
				}
			}
		}

		
		if pattern.IsDir && strings.HasPrefix(normalizedPath, pattern.Pattern+"/") {
			return true
		}

		
		if strings.HasPrefix(pattern.Pattern, "*.") {
			ext := pattern.Pattern[1:] 
			if strings.HasSuffix(filename, ext) {
				return true
			}
		}
	}

	return false
}
