package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func ListFiles(dirPath, baseFolder string, recursive bool) ([]string, error) {
	// Make path absolute
	absPath, err := filepath.Abs(filepath.Join(baseFolder, dirPath))
	if err != nil {
		return nil, err
	}

	// Check if path is within base folder
	absBaseFolder, err := filepath.Abs(baseFolder)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(absPath, absBaseFolder) {
		return nil, fmt.Errorf("access denied: path is outside the shared folder")
	}

	// Get file info
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	// If it's a regular file, just return its name
	if !info.IsDir() {
		return []string{info.Name()}, nil
	}

	// Read directory contents
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}

	var files []string

	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dirPath, name)

		// Format output
		if entry.IsDir() {
			name += "/"
		}

		// Add file/directory to list
		files = append(files, name)

		// If recursive and it's a directory, add its contents
		if recursive && entry.IsDir() {
			subFiles, err := ListFiles(path, baseFolder, recursive)
			if err != nil {
				continue
			}

			// Add path prefix to each file
			for i, subFile := range subFiles {
				subFiles[i] = filepath.Join(name, subFile)
			}

			files = append(files, subFiles...)
		}
	}

	return files, nil
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

	// Split pattern into directory and file parts
	dir := filepath.Dir(pattern)
	if dir == "." {
		dir = ""
	}

	filePattern := filepath.Base(pattern)

	// Get full directory path
	fullDirPath := filepath.Join(baseFolder, dir)

	// Check if directory exists
	if _, err := os.Stat(fullDirPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory not found: %s", dir)
	}

	// Walk through directory and match files
	err := filepath.Walk(fullDirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(baseFolder, path)
		if err != nil {
			return err
		}

		// Check if file matches pattern
		match, err := filepath.Match(filePattern, filepath.Base(path))
		if err != nil {
			return err
		}

		if match {
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
