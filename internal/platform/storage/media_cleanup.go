package storage

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// UploadBaseDir returns the configured uploads directory on disk,
// respecting the UPLOAD_DIR and DATA_DIR environment variables.
func UploadBaseDir() string {
	if dir := os.Getenv("UPLOAD_DIR"); strings.TrimSpace(dir) != "" {
		return strings.TrimSpace(dir)
	}
	if dataDir := os.Getenv("DATA_DIR"); strings.TrimSpace(dataDir) != "" {
		return filepath.Join(strings.TrimSpace(dataDir), "uploads")
	}
	return "data/uploads"
}

// DeleteUploadedMedia removes an uploaded file and all its derived image renditions
// (e.g. _thumb, _card, _full) from local disk and, if configured, from object storage.
func DeleteUploadedMedia(ctx context.Context, rawURL string, s3Client *Client) error {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return nil
	}

	// 1. Delete from local disk
	_ = deleteLocalUpload(raw)

	// 2. Delete from object storage if configured
	if s3Client != nil {
		_ = deleteS3Object(ctx, raw, s3Client)
	}

	return nil
}

func deleteLocalUpload(raw string) error {
	// Parse URL path if it's a full URL
	cleanPath := raw
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		cleanPath = u.Path
	}

	// Remove leading slashes and any /uploads/ prefix
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	cleanPath = strings.TrimPrefix(cleanPath, "uploads/")
	cleanPath = filepath.Clean(cleanPath)

	// Guard against path traversal
	if strings.Contains(cleanPath, "..") {
		return nil
	}

	baseDir := UploadBaseDir()
	targetPath := filepath.Join(baseDir, cleanPath)

	// Double check targetPath is within baseDir
	rel, err := filepath.Rel(baseDir, targetPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil
	}

	// Remove main file
	_ = os.Remove(targetPath)

	// Remove all derivative renditions (e.g. ads_123_thumb.jpg, ads_123_card.png, etc.)
	ext := filepath.Ext(targetPath)
	if ext != "" {
		stem := strings.TrimSuffix(targetPath, ext)
		// Match any stem_* file in the same directory
		matches, err := filepath.Glob(stem + "_*")
		if err == nil {
			for _, m := range matches {
				_ = os.Remove(m)
			}
		}
	}

	return nil
}

func deleteS3Object(ctx context.Context, raw string, s3Client *Client) error {
	if s3Client == nil {
		return nil
	}

	key := raw
	if s3Client.publicBaseURL != "" && strings.HasPrefix(raw, s3Client.publicBaseURL) {
		key = strings.TrimPrefix(raw, s3Client.publicBaseURL)
		key = strings.TrimPrefix(key, "/")
	}

	if strings.HasPrefix(key, "orgs/") || strings.HasPrefix(key, "ads/") || strings.HasPrefix(key, "offers/") {
		return s3Client.Delete(ctx, key)
	}

	return nil
}
