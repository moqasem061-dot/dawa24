package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/filescan"
	"github.com/muhiya/dawa24-store/internal/platform/queue"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
)

type documentScanWorker struct {
	river.WorkerDefaults[queue.DocumentScanArgs]
	db  *database.DB
	s3  *storage.Client
	log *slog.Logger
}

func newDocumentScanWorker(db *database.DB, s3 *storage.Client, log *slog.Logger) *documentScanWorker {
	return &documentScanWorker{db: db, s3: s3, log: log}
}

func (w *documentScanWorker) Work(ctx context.Context, job *river.Job[queue.DocumentScanArgs]) error {
	docID := job.Args.DocumentID
	if docID <= 0 {
		return fmt.Errorf("invalid document_id for scan")
	}

	sysCtx := database.AsSystem(ctx)

	var fileURL, storageKey, origName, mimeType, currentStatus string
	var metaBytes []byte

	err := w.db.InReadTx(sysCtx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT file_url, COALESCE(storage_key, ''), original_name, mime_type, status, meta
			FROM platform_admin.documents
			WHERE id = $1 AND deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query, docID).Scan(
			&fileURL, &storageKey, &origName, &mimeType, &currentStatus, &metaBytes,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			w.log.WarnContext(ctx, "document not found for scanning", "document_id", docID)
			return nil
		}
		return fmt.Errorf("fetch document %d: %w", docID, err)
	}

	content, readErr := w.readFileContent(sysCtx, fileURL, storageKey)
	if readErr != nil {
		w.log.WarnContext(ctx, "could not read document file for scanning", "document_id", docID, "error", readErr)
		return nil
	}

	scanResult := filescan.Scan(content, origName, mimeType)

	var meta map[string]interface{}
	if len(metaBytes) > 0 {
		_ = json.Unmarshal(metaBytes, &meta)
	}
	if meta == nil {
		meta = make(map[string]interface{})
	}

	scanRecord := map[string]interface{}{
		"scanned_at":    time.Now().UTC().Format(time.RFC3339),
		"passed":        scanResult.Passed,
		"verdict":       string(scanResult.Verdict),
		"reason":        scanResult.Reason,
		"detected_mime": scanResult.DetectedMIME,
	}
	meta["security_scan"] = scanRecord
	newMetaBytes, _ := json.Marshal(meta)

	return w.db.InTx(sysCtx, func(txCtx context.Context, tx pgx.Tx) error {
		if !scanResult.Passed {
			rejectNote := fmt.Sprintf("Rejected by automated security scanner: %s (verdict: %s)", scanResult.Reason, scanResult.Verdict)
			query := `
				UPDATE platform_admin.documents
				SET status = 'rejected', review_notes = $1, meta = $2, updated_at = now()
				WHERE id = $3;
			`
			_, err := tx.Exec(txCtx, query, rejectNote, newMetaBytes, docID)
			if err != nil {
				return err
			}
			w.log.WarnContext(ctx, "document rejected by automated security scan",
				"document_id", docID,
				"verdict", scanResult.Verdict,
				"reason", scanResult.Reason,
			)
			return nil
		}

		query := `
			UPDATE platform_admin.documents
			SET meta = $1, updated_at = now()
			WHERE id = $2;
		`
		_, err := tx.Exec(txCtx, query, newMetaBytes, docID)
		if err != nil {
			return err
		}
		w.log.InfoContext(ctx, "document passed automated security scan",
			"document_id", docID,
			"verdict", scanResult.Verdict,
		)
		return nil
	})
}

func (w *documentScanWorker) readFileContent(ctx context.Context, fileURL, storageKey string) ([]byte, error) {
	baseDir := storage.UploadBaseDir()
	key := strings.TrimPrefix(storageKey, "/")
	if key == "" {
		key = strings.TrimPrefix(fileURL, "/")
	}
	key = strings.TrimPrefix(key, "uploads/")

	candidates := []string{
		filepath.Join(baseDir, key),
		filepath.Join(baseDir, "documents", filepath.Base(key)),
		filepath.Join(baseDir, "licenses", filepath.Base(key)),
		filepath.Join("data", "uploads", key),
		filepath.Join("data", "uploads", "documents", filepath.Base(key)),
		filepath.Join("data", "uploads", "licenses", filepath.Base(key)),
	}

	for _, cand := range candidates {
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return os.ReadFile(cand)
		}
	}

	if w.s3 != nil && storageKey != "" {
		rc, _, err := w.s3.Get(ctx, storageKey)
		if err == nil && rc != nil {
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, 50*1024*1024))
		}
	}

	return nil, fmt.Errorf("file not found on disk or storage")
}
