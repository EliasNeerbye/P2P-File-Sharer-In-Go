package util

import (
	"fmt"
	"path/filepath"
	"strings"
)

func NormalizePath(path string) string {

	normalized := strings.ReplaceAll(path, "\\", "/")

	normalized = strings.TrimPrefix(normalized, "/")

	return normalized
}

func IsValidRelativePath(path string) bool {
	if path == "" {
		return false
	}

	normalized := NormalizePath(path)

	if strings.Contains(normalized, "..") {
		return false
	}

	if filepath.IsAbs(normalized) {
		return false
	}

	if strings.Contains(normalized, ":") {
		return false
	}

	return true
}

func SafeJoin(base, relPath string) (string, error) {

	normalized := NormalizePath(relPath)

	fullPath := filepath.Join(base, normalized)

	if !isPathSafe(fullPath, base) {
		return "", fmt.Errorf("access denied: path is outside the shared folder")
	}

	return fullPath, nil
}

func isPathSafe(requestedPath, baseFolder string) bool {
	absBase, err := filepath.Abs(baseFolder)
	if err != nil {
		return false
	}

	absTarget, err := filepath.Abs(requestedPath)
	if err != nil {
		return false
	}

	return strings.HasPrefix(absTarget, absBase)
}
