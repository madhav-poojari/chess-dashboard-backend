package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FileStorage handles saving and deleting files on local disk.
// Swap this implementation for S3/Cloudflare later.
type FileStorage struct {
	BaseDir string // e.g. "./uploads"
}

// NewFileStorage creates a FileStorage rooted at baseDir.
func NewFileStorage(baseDir string) *FileStorage {
	return &FileStorage{BaseDir: baseDir}
}

// SaveFile writes the contents of reader to <BaseDir>/<subDir>/<uniqueFilename>.
// It returns the url_suffix (relative path from BaseDir) that can be stored in DB.
// subDir examples: "profile-pictures", "gallery/USR001"
func (fs *FileStorage) SaveFile(subDir, originalFilename string, reader io.Reader) (string, error) {
	dir := filepath.Join(fs.BaseDir, subDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	ext := filepath.Ext(originalFilename)
	uniqueName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	fullPath := filepath.Join(dir, uniqueName)

	out, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, reader); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", fullPath, err)
	}

	// url_suffix is the path relative to BaseDir, using forward slashes
	urlSuffix := filepath.ToSlash(filepath.Join(subDir, uniqueName))
	return urlSuffix, nil
}

// DeleteFile removes the file at <BaseDir>/<urlSuffix>.
// It is safe to call if the file does not exist.
func (fs *FileStorage) DeleteFile(urlSuffix string) error {
	fullPath := filepath.Join(fs.BaseDir, filepath.FromSlash(urlSuffix))
	err := os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file %s: %w", fullPath, err)
	}
	return nil
}
