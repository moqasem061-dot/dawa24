package ui

import (
	"os"
	"strconv"
	"strings"
)

// maxImportUploadBytes caps an uploaded catalogue file (default 200 MB, configurable via MAX_IMPORT_UPLOAD_MB).
var (
	maxImportUploadBytes  int64 = resolveMaxImportUploadBytes()
	maxImportRequestBytes int64 = maxImportUploadBytes + (1 << 20)
)

func resolveMaxImportUploadBytes() int64 {
	if raw := os.Getenv("MAX_IMPORT_UPLOAD_MB"); raw != "" {
		if mb, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && mb > 0 {
			return mb << 20
		}
	}
	return 200 << 20
}

