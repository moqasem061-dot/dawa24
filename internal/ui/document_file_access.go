package ui

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/attachments"
)

// openDocumentUpload confines even legacy fallback paths and symlinks to the
// upload directories. Document metadata is user supplied, not a filesystem grant.
func openDocumentUpload(candidate string) (*os.File, error) {
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return nil, err
	}
	for _, directory := range documentUploadDirectories() {
		rootPath, err := filepath.Abs(directory)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(rootPath, absolute)
		if err != nil || !filepath.IsLocal(relative) {
			continue
		}
		file, err := os.OpenInRoot(rootPath, relative)
		if err == nil { // nolint:errswallow: legacy file lookup tries each permitted upload root
			return file, nil
		}
	}
	return nil, os.ErrNotExist
}

func sandboxDocumentContent(w http.ResponseWriter, contentType string) {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch mediaType {
	case "text/html", "application/xhtml+xml", "image/svg+xml", "application/xml", "text/xml":
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	case "application/pdf":
		w.Header().Set("Content-Security-Policy", "default-src 'none'; object-src 'self' blob:; frame-ancestors 'self'; style-src 'unsafe-inline'; font-src 'self' data:")
	}
}

func documentStorageKeyAllowed(document *attachments.Document, key string) bool {
	if key == "" || !filepath.IsLocal(key) || path.Clean(key) != key || strings.ContainsAny(key, "\\:\x00") {
		return false
	}
	for strings.HasPrefix(key, "uploads/") {
		key = strings.TrimPrefix(key, "uploads/")
	}
	if strings.HasPrefix(key, "orgs/") {
		return document.OrganizationID != nil && strings.HasPrefix(key, fmt.Sprintf("orgs/%d/", *document.OrganizationID))
	}
	if strings.HasPrefix(key, "users/") {
		return document.UserID != nil && strings.HasPrefix(key, fmt.Sprintf("users/%d/", *document.UserID))
	}
	return true
}

func documentUploadDirectories() []string {
	return []string{GetUploadBaseDir(), "data/uploads", "internal/ui/data/uploads", "cmd/server/data/uploads"}
}

func scopedDocumentPaths(key string) []string {
	paths := make([]string, 0, 4)
	for _, directory := range documentUploadDirectories() {
		paths = append(paths, filepath.Join(directory, filepath.FromSlash(key)))
	}
	return paths
}
