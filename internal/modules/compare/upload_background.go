package compare

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Staging a supplier list without holding the pharmacy's browser open.
//
// UploadAndProcessCompareFile does two very different jobs in one call: it
// creates the file row, which is a single insert, and then it parses the whole
// workbook and writes every row of it, which on a thirty-thousand-row price
// list is seconds — times ten files in a batch, six at a time.
//
// All of that used to happen inside the POST. Three consequences, and the first
// two are the ones people reported:
//
//   - The browser waited on a blank page for minutes. /compare/upload is
//     exempt from the request deadline precisely so it could, which only moved
//     the failure to the proxy in front of the application.
//   - A pharmacy that navigated away lost the batch. The context belonged to
//     the request, so closing the tab cancelled the parsing — halfway through,
//     with some files staged and some not.
//   - Retrying looked like the fix and made it worse: a second POST re-uploaded
//     and re-parsed everything, against a quota that had already counted the
//     first attempt.
//
// The split here is the same one internal/modules/ingest made for the vendor
// import: register the file synchronously so the caller gets an id it can
// redirect to, then parse in a goroutine that outlives the request.

// stageTimeout bounds one detached staging pass.
//
// Generous because the work is genuinely long — the largest supplier lists on
// this platform are tens of thousands of rows — and short enough that a file
// which cannot be read at all does not hold a goroutine for ever.
const stageTimeout = 20 * time.Minute

// StagedFile is what the caller learns immediately about one uploaded file.
type StagedFile struct {
	File     *CompareFile
	Archived []string
}

// RegisterAndStage creates the file row and returns, leaving the parse to a
// goroutine.
//
// The returned file is in FileProcessing, so the screen that receives the
// redirect knows to wait rather than to offer a column mapping for a file whose
// columns have not been read yet. Its outcome is reported through the row's
// status — FileReady or FileFailed — rather than to this caller, who is by then
// a request that has long since been answered.
//
// The payload arrives already scanned, as a filesecurity.Scanned that only the
// scan can produce.
//
// It used to arrive as a plain []byte and be scanned HERE — after the handler
// had already scanned the identical bytes a few lines earlier. Both calls were
// real: the handler refuses a bad file before writing it to disk, and this one
// existed so the service could not be handed something unchecked. Together they
// cost a full extra parse of every workbook in the batch, in the request, and a
// twenty-thousand-row file measured 2.3 seconds a pass.
//
// Taking the proof rather than the bytes settles it without giving anything up:
// the scan still cannot be skipped, because a Scanned cannot be built without
// it, and it can no longer be run twice, because holding one means it is done.
func (s *Service) RegisterAndStage(
	ctx context.Context, userID int64, orgID *int64,
	supplierName, originalFilename, mimeType string,
	sizeBytes int64, storageKey string, scanned filesecurity.Scanned,
) (*StagedFile, error) {
	fileBytes := scanned.Bytes()

	file, archived, err := s.UploadCompareFile(ctx, userID, orgID,
		supplierName, originalFilename, mimeType, sizeBytes, storageKey)
	if err != nil {
		return nil, err
	}

	// Nothing to parse: an empty upload is finished the moment it is recorded.
	if len(fileBytes) == 0 {
		return &StagedFile{File: file, Archived: archived}, nil
	}

	// Published before the goroutine starts, so a caller redirected faster than
	// the scheduler gets round to the run still finds a file that says it is
	// working rather than one that says it is ready and holds no rows.
	file.Status = FileProcessing
	file.ErrorMessage = ""
	if err := s.repo.UpdateFile(ctx, file); err != nil {
		return nil, err
	}

	// The run outlives the request that asked for it, so it gets a context
	// carrying the request's values — the tenant binding row-level security
	// reads, most of all — but not its cancellation. context.WithoutCancel is
	// exactly this: keep who you are, lose when you must stop.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stageTimeout)

	// The goroutine works on its own copy: the caller still holds the file it
	// was handed and is free to read it while rendering, and two goroutines
	// reading and writing one struct is a data race whatever the fields are.
	staging := *file

	// The bytes travel with the goroutine.
	//
	// Re-reading them from disk would be tidier, and was tried: openStoredUpload
	// searches "data/uploads/compare" while the writer honours UPLOAD_DIR and
	// DATA_DIR, so on any deployment that sets either one the parse would look
	// in the wrong place and report a perfectly good file as unreadable.
	//
	// The cost of carrying them is smaller than it looks. The synchronous
	// version this replaced held exactly the same bytes for exactly the same
	// parse; the only difference is that the request has already been answered.
	// Peak memory is unchanged, and it is bounded by the request body cap
	// (ui.maxImportBatchBytes) rather than by anything decided here.
	bytes := fileBytes

	go func() {
		defer cancel()
		defer func() {
			if p := recover(); p != nil {
				s.log.ErrorContext(runCtx, "compare staging panicked",
					"file", staging.ID, "panic", p)
				s.failStaging(runCtx, &staging, i18n.TDefault("w4_mod.s_376_376"))
			}
		}()

		if err := s.StageUploadedRows(runCtx, &staging, bytes); err != nil {
			s.log.ErrorContext(runCtx, "compare staging failed",
				"file", staging.ID, "error", err)
			// StageUploadedRows records its own failure on the row; this only
			// covers an error it could not write.
			if staging.Status != FileFailed {
				s.failStaging(runCtx, &staging, i18n.TDefault("w4_mod.s_375_375"))
			}
		}
	}()

	return &StagedFile{File: file, Archived: archived}, nil
}

// failStaging records that a staging pass ended badly, so the screen stops
// waiting and says why.
func (s *Service) failStaging(ctx context.Context, file *CompareFile, message string) {
	file.Status = FileFailed
	file.ErrorMessage = message
	if err := s.repo.UpdateFile(ctx, file); err != nil {
		s.log.ErrorContext(ctx, "could not record compare staging failure",
			"file", file.ID, "error", err)
	}
}

// StagingStatus is one file's readiness, for the screen waiting on a batch.
type StagingStatus struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	RowCount int    `json:"row_count"`
	Error    string `json:"error,omitempty"`
	Done     bool   `json:"done"`
}

// StagingProgress reports where a batch of uploads has reached.
//
// Ownership is checked by the caller; this only reads. A file that has vanished
// is reported as done rather than holding the batch open for ever.
func (s *Service) StagingProgress(ctx context.Context, ids []int64) ([]StagingStatus, error) {
	out := make([]StagingStatus, 0, len(ids))
	for _, id := range ids {
		file, err := s.repo.GetFileByID(ctx, id)
		if err != nil || file == nil {
			out = append(out, StagingStatus{ID: id, Done: true})
			continue
		}
		out = append(out, StagingStatus{
			ID:       file.ID,
			Name:     file.SupplierName,
			Status:   string(file.Status),
			RowCount: file.RowCount,
			Error:    file.ErrorMessage,
			Done:     file.Status != FileProcessing,
		})
	}
	return out, nil
}

// openStoredUpload finds the bytes of an uploaded spreadsheet again.
//
// Extracted from ProcessCompareFile so the detached staging pass can use it
// too. That matters for memory rather than for tidiness: staging used to run
// inside the request and could hold the uploaded bytes for the few seconds it
// took, but a goroutine that outlives the request would hold every file of a
// ten-file batch for as long as the parse ran. Re-reading from where the upload
// already wrote them keeps the batch's cost on disk instead of in the heap.
//
// Returns nil when the file cannot be found anywhere, which the callers treat
// as "nothing to parse" rather than as an error, because a run whose rows are
// already in the database does not need it.
func (s *Service) openStoredUpload(ctx context.Context, file *CompareFile) io.ReadCloser {
	// 1. Object storage, where it is configured and the key is not a local path.
	if s.storage != nil && file.StorageKey != "" &&
		!strings.HasPrefix(file.StorageKey, "/") && !strings.HasPrefix(file.StorageKey, "data/") {
		if r, _, err := s.storage.Get(ctx, file.StorageKey); err == nil {
			return r
		}
	}

	uploadBase := storage.UploadBaseDir()
	compareDir := filepath.Join(uploadBase, "compare")

	// 2. The exact storage key on local disk, under configured base dir
	if file.StorageKey != "" {
		cleanKey := strings.TrimPrefix(filepath.FromSlash(file.StorageKey), string(filepath.Separator))
		for _, cand := range []string{
			file.StorageKey,
			filepath.Join(uploadBase, cleanKey),
			filepath.Join(compareDir, filepath.Base(file.StorageKey)),
			filepath.Join(compareDir, filepath.Base(file.OriginalFilename)),
			filepath.Join("data", cleanKey),
			filepath.Join("data", "uploads", "compare", filepath.Base(file.StorageKey)),
			filepath.Join("data", "uploads", "compare", filepath.Base(file.OriginalFilename)),
			"data" + file.StorageKey,
		} {
			if f, err := os.Open(cand); err == nil {
				return f
			}
		}
	}

	// 3. Last resort: scan the upload directory for something that looks like this file.
	for _, dir := range []string{compareDir, filepath.Join("data", "uploads", "compare")} {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.Contains(entry.Name(), file.OriginalFilename) ||
				strings.HasSuffix(entry.Name(), filepath.Ext(file.OriginalFilename)) {
				if f, err := os.Open(filepath.Join(dir, entry.Name())); err == nil {
					return f
				}
			}
		}
	}
	return nil
}
