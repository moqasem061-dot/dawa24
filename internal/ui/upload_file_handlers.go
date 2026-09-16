package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/filescan"
	"github.com/muhiya/dawa24-store/internal/platform/media"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
)

const (
	UploadBaseDir  = "data/uploads"
	MaxUploadBytes = 50 * 1024 * 1024 // 50 MB

	// uploadMemoryBudget is how much of a multipart request is held in RAM.
	//
	// It is the argument to r.ParseMultipartForm, and that argument is not a
	// size limit — it is how much Go keeps in memory before spilling the rest
	// to a temp file it deletes when the request ends. Passing the ALLOWED
	// UPLOAD SIZE there, which is what every handler on this platform did,
	// reads as "accept files up to 500 MB" and means "hold 500 MB of heap per
	// concurrent request".
	//
	// On the VPS this runs on that is the whole machine. Two vendors uploading
	// price lists at once was enough to push the process into swap, and once it
	// is swapping every OTHER request slows down too — which is why the reports
	// were never limited to the person doing the uploading.
	//
	// Four megabytes. Large enough that ordinary form fields and small images
	// never touch the disk, small enough that a hundred concurrent uploads cost
	// less memory than one used to. The size limit is a separate thing and is
	// enforced with http.MaxBytesReader, which is what actually rejects an
	// oversized file.
	uploadMemoryBudget = 4 << 20

	// maxImportBatchBytes bounds a whole spreadsheet-import request.
	//
	// This is the limit that was missing. The figure the handlers used to pass
	// to ParseMultipartForm looked like one and was not — it governed memory,
	// never size — so the import endpoints accepted a request of ANY length and
	// then held all of it. A cap has to exist somewhere, and the body is where
	// it costs nothing: an oversized request is refused before a byte of it is
	// parsed, rather than after the machine has swallowed it.
	//
	// Two hundred megabytes covers the largest real batch by a wide margin: the
	// compare tool takes at most ten files and a twenty-thousand-row price list
	// is a few megabytes.
	maxImportBatchBytes int64 = 200 << 20
)

// parseImportUpload reads a spreadsheet-import request: size bounded at the
// body, memory bounded at the form.
//
// Both halves matter and they are different limits. MaxBytesReader refuses an
// oversized request outright; uploadMemoryBudget decides how much of an
// acceptable one is held in RAM rather than spilled to a temp file the server
// deletes when the request ends.
func parseImportUpload(w http.ResponseWriter, r *http.Request) error {
	if r.MultipartForm != nil {
		return nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBatchBytes)
	return r.ParseMultipartForm(uploadMemoryBudget)
}

// GetUploadBaseDir returns the configured uploads directory, respecting the
// UPLOAD_DIR and DATA_DIR environment variables.
//
// Whatever this returns is the only place an upload is written. Until now every
// write was mirrored into the relative "data/uploads" as well, which meant
// configuration could redirect reads but never writes: a test run with
// UPLOAD_DIR pointed at a temporary directory still wrote into the working
// tree. Since internal/ui is where the tests run, that filled the repository
// with uploaded files, 166 of which ended up committed and were then deleted
// and rewritten by every subsequent `go test ./...`.
func GetUploadBaseDir() string {
	return storage.UploadBaseDir()
}

var allowedUploadCategories = map[string]bool{
	"products":  true,
	"licenses":  true,
	"avatars":   true,
	"resumes":   true,
	"documents": true,
	"brands":    true,
	"compare":   true,
	"imports":   true,
	// Temp-warehouse uploads named this category and it was not in the list, so
	// sanitizeCategory quietly rewrote it to "products" and the files landed
	// among the product images. resolveStoragePath found them anyway — it tries
	// the storage key's own path first — so nothing broke and nobody noticed.
	"temp_warehouses": true,
	"receipts":        true,
	"ads":             true,
	"offers":          true,
	"cvs":             true,
}

var imageUploadExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

var docUploadExts = map[string]bool{
	".pdf": true, ".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
}

var sheetUploadExts = map[string]bool{
	".csv": true, ".xlsx": true, ".xls": true, ".tsv": true, ".txt": true,
}

func isAllowedUploadExt(category, ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		return false
	}
	switch category {
	case "products", "avatars", "brands", "ads", "offers":
		return imageUploadExts[ext]
	case "licenses", "resumes", "documents", "receipts", "cvs":
		return docUploadExts[ext]
	case "compare", "imports", "temp_warehouses":
		return sheetUploadExts[ext]
	default:
		return imageUploadExts[ext] || docUploadExts[ext]
	}
}

func sanitizeCategory(category string) string {
	clean := strings.ToLower(strings.TrimSpace(category))
	clean = filepath.Base(clean)
	if strings.Contains(clean, "..") || strings.Contains(clean, "/") || strings.Contains(clean, "\\") {
		return "products"
	}
	if allowedUploadCategories[clean] {
		return clean
	}
	return "products"
}

// saveUploadedFile safely processes multipart uploads, validates extensions, and prevents path traversal.
func saveUploadedFile(r *http.Request, fieldName, category string) (string, error) {
	src, header, err := r.FormFile(fieldName)
	if err != nil {
		return "", fmt.Errorf("no file uploaded or invalid form field: %w", err)
	}
	defer src.Close()

	if header.Size > MaxUploadBytes {
		return "", fmt.Errorf("file size exceeds maximum allowed limit (50MB)")
	}

	data, err := io.ReadAll(io.LimitReader(src, MaxUploadBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read uploaded file: %w", err)
	}
	if int64(len(data)) > MaxUploadBytes {
		return "", fmt.Errorf("file size exceeds maximum allowed limit (50MB)")
	}

	return saveUploadedBytes(data, header.Filename, category)
}

// uploadedFileMeta describes a stored upload for document rows.
type uploadedFileMeta struct {
	URL          string
	OriginalName string
	MimeType     string
	SizeBytes    int64
}

// saveUploadedFileFull stores one multipart upload and returns its public URL
// together with the original filename, detected MIME type and byte size, read
// from the single FormFile call. Callers that only need the URL keep using
// saveUploadedFile; callers that persist document rows need the metadata so
// previews (image-vs-PDF sniff, filenames) keep working.
func saveUploadedFileFull(r *http.Request, fieldName, category string) (uploadedFileMeta, error) {
	var meta uploadedFileMeta
	src, header, err := r.FormFile(fieldName)
	if err != nil {
		return meta, fmt.Errorf("no file uploaded or invalid form field: %w", err)
	}
	defer src.Close()

	if header.Size > MaxUploadBytes {
		return meta, fmt.Errorf("file size exceeds maximum allowed limit (50MB)")
	}

	data, err := io.ReadAll(io.LimitReader(src, MaxUploadBytes+1))
	if err != nil {
		return meta, fmt.Errorf("failed to read uploaded file: %w", err)
	}
	if int64(len(data)) > MaxUploadBytes {
		return meta, fmt.Errorf("file size exceeds maximum allowed limit (50MB)")
	}

	category = sanitizeCategory(category)
	baseDir := GetUploadBaseDir()

	destDir := filepath.Join(baseDir, category)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return meta, fmt.Errorf("failed to create upload directory: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !isAllowedUploadExt(category, ext) {
		return meta, fmt.Errorf("unsupported file format %q for %s uploads", ext, category)
	}

	mimeType := header.Header.Get("Content-Type")

	// Pre-scan file content for exploits, malicious scripts, and header integrity
	if scanRes := filescan.Scan(data, header.Filename, mimeType); !scanRes.Passed {
		return meta, fmt.Errorf("upload rejected by security filter: %s", scanRes.Reason)
	}
	// Optimize and compress images before saving to storage
	if compData, newExt, newMime, wasCompressed := media.Compress(data, media.DefaultMaxEdge); wasCompressed {
		data = compData
		if newExt != "" {
			ext = newExt
		}
		if newMime != "" {
			mimeType = newMime
		}
	}

	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	uniqueName := fmt.Sprintf("%s_%s%s", category, hex.EncodeToString(randomBytes), ext)
	targetPath := filepath.Join(destDir, uniqueName)

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return meta, fmt.Errorf("failed to save uploaded file: %w", err)
	}
	_ = deriveAndStore(destDir, uniqueName, data)

	meta.URL = fmt.Sprintf("/uploads/%s/%s", category, uniqueName)
	meta.OriginalName = header.Filename
	meta.SizeBytes = int64(len(data))
	if mimeType != "" && mimeType != "application/octet-stream" {
		meta.MimeType = mimeType
	} else if mt := mime.TypeByExtension(ext); mt != "" {
		meta.MimeType = mt
	}
	return meta, nil
}

// saveUploadedBytes writes byte data directly to disk.
func saveUploadedBytes(data []byte, originalFilename, category string) (string, error) {
	if len(data) > MaxUploadBytes {
		return "", fmt.Errorf("file size exceeds maximum allowed limit (50MB)")
	}

	category = sanitizeCategory(category)
	baseDir := GetUploadBaseDir()

	ext := strings.ToLower(filepath.Ext(originalFilename))
	if ext == "" {
		ext = ".bin"
	}

	// Pre-scan file content for exploits, malicious scripts, and header integrity
	if scanRes := filescan.Scan(data, originalFilename, ""); !scanRes.Passed {
		return "", fmt.Errorf("upload rejected by security filter: %s", scanRes.Reason)
	}

	// Optimize and compress images before saving to storage
	if compData, newExt, _, wasCompressed := media.Compress(data, media.DefaultMaxEdge); wasCompressed {
		data = compData
		if newExt != "" {
			ext = newExt
		}
	}

	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	safeFilename := fmt.Sprintf("%s_%s%s", category, hex.EncodeToString(randomBytes), ext)

	targetDir := filepath.Join(baseDir, category)
	_ = os.MkdirAll(targetDir, 0755)

	targetPath := filepath.Join(targetDir, safeFilename)
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to save file: %w", err)
	}
	_ = deriveAndStore(targetDir, safeFilename, data)

	return fmt.Sprintf("/uploads/%s/%s", category, safeFilename), nil
}

// UploadAPISubmit allows asynchronous HTMX or JavaScript file uploads.
func (h *UIHandler) UploadAPISubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	category := sanitizeCategory(r.URL.Query().Get("category"))

	url, err := saveUploadedFile(r, "file", category)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(fmt.Sprintf(`{"url":"%s"}`, url)))
}

// readAllUpTo reads r into memory, refusing anything past limit.
//
// io.ReadAll on a multipart part will happily allocate whatever the part
// contains. The body cap upstream bounds the whole request, but a single part
// inside an accepted request can still be most of it, and the importers read
// several parts at once — so each one states its own ceiling rather than
// trusting the sum.
func readAllUpTo(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds the maximum allowed size")
	}
	return data, nil
}
