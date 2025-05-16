package util

import (
	"fmt"
	"path/filepath"
	"strings"
)

// NormalizePath standardizes path separators and removes leading slashes
func NormalizePath(path string) string {
	// Convert backslashes to forward slashes
	normalized := strings.ReplaceAll(path, "\\", "/")

	// Remove leading slash if present (cleaner way)
	normalized = strings.TrimPrefix(normalized, "/")

	return normalized
}

// IsValidRelativePath checks if a path is valid and relative
func IsValidRelativePath(path string) bool {
	if path == "" {
		return false
	}

	// Normalize path first
	normalized := NormalizePath(path)

	// Check for parent directory references
	if strings.Contains(normalized, "..") {
		return false
	}

	// Check for absolute paths
	if filepath.IsAbs(normalized) {
		return false
	}

	// Check for drive letters (Windows)
	if strings.Contains(normalized, ":") {
		return false
	}

	return true
}

// SafeJoin joins paths safely ensuring the result stays within base directory
func SafeJoin(base, relPath string) (string, error) {
	// Normalize the relative path
	normalized := NormalizePath(relPath)

	// Join paths
	fullPath := filepath.Join(base, normalized)

	// Verify the path is within the base directory
	if !isPathSafe(fullPath, base) {
		return "", fmt.Errorf("access denied: path is outside the shared folder")
	}

	return fullPath, nil
}

// isPathSafe checks if a path is within the base directory
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
