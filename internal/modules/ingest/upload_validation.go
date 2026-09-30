package ingest

import (
	"fmt"
	"path"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

func validateUploadFilename(filename string) error {
	if filename == "" || filename == "." || filename == ".." || strings.ContainsAny(filename, "/\\:\x00") {
		return apperr.Validation("upload.filename_invalid", i18n.TDefault("upload.filename_invalid"), nil)
	}
	return nil
}

func validateUploadStorageKey(orgID int64, key string) error {
	prefix := fmt.Sprintf("orgs/%d/uploads/", orgID)
	if !strings.HasPrefix(key, prefix) || path.Clean(key) != key || strings.ContainsAny(key, "\\:\x00") || key == prefix {
		return apperr.Validation("upload.storage_key_invalid", i18n.TDefault("upload.storage_key_invalid"), nil)
	}
	return nil
}
