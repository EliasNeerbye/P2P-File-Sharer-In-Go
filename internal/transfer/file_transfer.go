package transfer

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FileInfo struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum,omitempty"`
	IsDir    bool   `json:"is_dir"`
	Mode     string `json:"mode"`
}

type Command struct {
	Action  string      `json:"action"`
	Path    string      `json:"path,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
}

func ListFiles(rootDir, subDir string) ([]FileInfo, error) {
	directory := filepath.Join(rootDir, subDir)
	directory = filepath.Clean(directory)

	// Security check - make sure we're not going outside the root directory
	if !IsPathSafe(rootDir, directory) {
		return nil, fmt.Errorf("path traversal detected, access denied")
	}

	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var fileInfos []FileInfo
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			continue
		}

		fileInfo := FileInfo{
			Name:  file.Name(),
			Size:  info.Size(),
			IsDir: file.IsDir(),
			Mode:  info.Mode().String(),
		}
		fileInfos = append(fileInfos, fileInfo)
	}

	return fileInfos, nil
}

func GetFileInfo(rootDir, filePath string) (*FileInfo, error) {
	fullPath := filepath.Join(rootDir, filePath)
	fullPath = filepath.Clean(fullPath)

	// Security check - make sure we're not going outside the root directory
	if !IsPathSafe(rootDir, fullPath) {
		return nil, fmt.Errorf("path traversal detected, access denied")
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	fileInfo := &FileInfo{
		Name:  info.Name(),
		Size:  info.Size(),
		IsDir: info.IsDir(),
		Mode:  info.Mode().String(),
	}

	if !fileInfo.IsDir {
		checksum, err := CalculateChecksum(fullPath)
		if err == nil {
			fileInfo.Checksum = checksum
		}
	}

	return fileInfo, nil
}

func SendFile(rootDir, filePath string, w io.Writer, maxSize int) error {
	fullPath := filepath.Join(rootDir, filePath)
	fullPath = filepath.Clean(fullPath)

	// Security check - make sure we're not going outside the root directory
	if !IsPathSafe(rootDir, fullPath) {
		return fmt.Errorf("path traversal detected, access denied")
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	if maxSize > 0 && fileInfo.Size() > int64(maxSize*1024*1024) {
		return fmt.Errorf("file size exceeds maximum allowed size of %d MB", maxSize)
	}

	bufferedWriter := bufio.NewWriter(w)
	_, err = io.Copy(bufferedWriter, file)
	if err != nil {
		return fmt.Errorf("failed to send file: %w", err)
	}

	err = bufferedWriter.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush buffer: %w", err)
	}

	return nil
}

func ReceiveFile(rootDir, filePath string, r io.Reader, size int64, maxSize int) error {
	fullPath := filepath.Join(rootDir, filePath)
	fullPath = filepath.Clean(fullPath)

	// Security check - make sure we're not going outside the root directory
	if !IsPathSafe(rootDir, fullPath) {
		return fmt.Errorf("path traversal detected, access denied")
	}

	if maxSize > 0 && size > int64(maxSize*1024*1024) {
		return fmt.Errorf("file size exceeds maximum allowed size of %d MB", maxSize)
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = r
	if maxSize > 0 {
		reader = io.LimitReader(r, int64(maxSize*1024*1024))
	}

	bufferedReader := bufio.NewReader(reader)
	_, err = io.Copy(file, bufferedReader)
	if err != nil {
		return fmt.Errorf("failed to receive file: %w", err)
	}

	return nil
}

func VerifyChecksum(rootDir, filePath, expectedChecksum string) (bool, error) {
	fullPath := filepath.Join(rootDir, filePath)
	fullPath = filepath.Clean(fullPath)

	// Security check - make sure we're not going outside the root directory
	if !IsPathSafe(rootDir, fullPath) {
		return false, fmt.Errorf("path traversal detected, access denied")
	}

	actualChecksum, err := CalculateChecksum(fullPath)
	if err != nil {
		return false, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return actualChecksum == expectedChecksum, nil
}

func CalculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate hash: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Helper function to safely verify paths
func IsPathSafe(rootDir, path string) bool {
	// Clean and absolute paths
	rootDir, err := filepath.Abs(filepath.Clean(rootDir))
	if err != nil {
		return false
	}

	path, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}

	// Check if the path starts with the rootDir
	return strings.HasPrefix(path, rootDir)
}
