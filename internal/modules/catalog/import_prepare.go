package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AnalyzeImport reads an uploaded file and opens a mapping session for it.
//
// It reads the sheet and works out its shape — where the titles are, how many
// blocks, which column looks like which field, what the values in each column
// look like — and stores that description on the session. It deliberately does
// not parse the rows into products: that is the expensive half, it depends on a
// mapping the admin has not confirmed yet, and doing it here is what made the
// upload button feel like it hung.
func (s *Service) AnalyzeImport(
	ctx context.Context, content []byte, filename string, actorID int64,
) (*ImportSession, FileStructure, error) {
	if s.imports == nil {
		return nil, FileStructure{}, ErrImportUnavailable
	}

	sheet, err := ReadSpreadsheet(content, filename)
	if err != nil {
		return nil, FileStructure{}, err
	}
	if len(sheet.Rows) == 0 {
		return nil, FileStructure{}, apperr.Validation("catalog.import_file_empty",
			i18n.TDefault("w4_mod.w4str_82_82"), nil)
	}

	layout := AnalyzeLayout(sheet)
	structure := BuildFileStructure(sheet, layout)

	// The session is scoped to the organisation that owns the master catalogue,
	// so the rows it matches against during review are the same rows the commit
	// will write into.
	orgID, err := s.imports.DefaultCatalogOrg(ctx)
	if err != nil {
		return nil, structure, err
	}

	session := &ImportSession{
		OrganizationID: orgID,
		Filename:       filename,
		FileSizeBytes:  int64(len(content)),
		SourceFormat:   sheet.Format,
		SheetName:      sheet.Sheet,
		Delimiter:      sheet.Delimiter,
		Status:         SessionDraft,
		Mode:           ModeUpdateAndAdd,
		Options:        DefaultImportOptions(),
		Structure:      structure,
		TotalRows:      structure.TotalRows,
		BlockCount:     structure.BlockCount,
	}
	if actorID > 0 {
		session.CreatedBy = &actorID
	}

	if err := s.imports.CreateImportSession(ctx, session, content); err != nil {
		return nil, structure, err
	}
	s.sheets.put(session.PublicID, sheet)

	s.log.InfoContext(ctx, "catalogue import session opened",
		"session", session.PublicID, "file", filename,
		"rows", structure.TotalRows, "blocks", structure.BlockCount,
		"mapped_fields", structure.MappedFields(), "positional", structure.Positional)

	return session, structure, nil
}

// ImportPreviewRows is how many parsed products the mapping screen shows. It is
// a sample, not a page: the admin is checking that the columns line up, and
// twenty-five rows answers that on any file.
const ImportPreviewRows = 25

// ImportPreview is what the mapping screen renders: the file re-read under the
// admin's current corrections, with a handful of products to look at.
type ImportPreview struct {
	Structure FileStructure
	// Products is the first ImportPreviewRows products the mapping would yield.
	Products []*Product
	// SourceRows are the spreadsheet row numbers those products came from.
	SourceRows []int
	// Issues are the findings from the previewed rows only.
	Issues []RowIssue
	// TotalProducts is how many products the whole file would yield under this
	// mapping, so the admin sees the real number before committing to a run.
	TotalProducts int
	// RejectedRows is how many rows this mapping would drop entirely.
	RejectedRows int
	// DuplicateRows is how many rows fold into an earlier one.
	DuplicateRows int
}

// PreviewImport re-reads the session's file under a proposed mapping without
// staging anything, and stores the resulting structure on the session.
//
// This is the whole point of the mapping step: the admin sees what their
// correction actually does — to the columns, to the row count, to the first
// twenty-five products — before a single row is staged and long before the
// catalogue is touched.
func (s *Service) PreviewImport(
	ctx context.Context, publicID string, settings ImportSettings,
) (*ImportSession, *ImportPreview, error) {
	if s.imports == nil {
		return nil, nil, ErrImportUnavailable
	}

	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return nil, nil, err
	}
	if session.IsProcessing() {
		return session, nil, apperr.Conflict("catalog.import_already_running",
			i18n.TDefault("w4_mod.w4str_83_83"))
	}
	if !session.IsRetryable() {
		return session, nil, apperr.Validation("catalog.import_not_reviewable",
			i18n.TDefault("w4_mod.w4str_84_84"), nil)
	}

	sheet, err := s.sheetFor(ctx, session)
	if err != nil {
		return session, nil, err
	}

	session.Mode = settings.Mode
	session.Options = settings.Options
	session.Overrides = settings.Overrides

	parsed := ParseSheet(sheet, session.Overrides, session.Options)
	structure := BuildFileStructure(sheet, parsed.Layout)

	session.Structure = structure
	session.TotalRows = parsed.Stats.TotalRowsRead
	session.ParsedRows = len(parsed.Products)
	session.BlockCount = len(parsed.Layout.Blocks)
	// A preview clears the last run's failure: the admin is looking at a fresh
	// reading, and leaving the old error on the row would have the review screen
	// report a failure that no longer describes anything.
	if session.Status == SessionFailed {
		session.Status = SessionDraft
	}
	session.ErrorMessage = ""
	if err := s.imports.UpdateImportSession(ctx, session, retryableStatuses()...); err != nil {
		return session, nil, err
	}

	preview := &ImportPreview{
		Structure:     structure,
		TotalProducts: len(parsed.Products),
		RejectedRows:  parsed.Stats.RejectedRows,
		DuplicateRows: parsed.Stats.DuplicateRows,
	}
	preview.Products = parsed.Products[:min(len(parsed.Products), ImportPreviewRows)]
	preview.SourceRows = parsed.SourceRows[:min(len(parsed.SourceRows), ImportPreviewRows)]

	lastRow := 0
	if n := len(preview.SourceRows); n > 0 {
		lastRow = preview.SourceRows[n-1]
	}
	for _, issue := range parsed.Issues {
		if issue.Row <= lastRow || issue.Row <= 1 {
			preview.Issues = append(preview.Issues, issue)
		}
	}
	return session, preview, nil
}

// sheetFor decodes a session's uploaded file, reusing the warm copy when the
// admin is working through the mapping screen.
func (s *Service) sheetFor(ctx context.Context, session *ImportSession) (*SheetData, error) {
	if cached := s.sheets.get(session.PublicID); cached != nil {
		return cached, nil
	}

	content, err := s.imports.ImportSourceFile(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, apperr.Validation("catalog.import_file_expired",
			i18n.TDefault("w4_mod.w4str_85_85"), nil)
	}

	sheet, err := ReadSpreadsheet(content, session.Filename)
	if err != nil {
		return nil, err
	}
	s.sheets.put(session.PublicID, sheet)
	return sheet, nil
}

// PrepareImport re-reads the session's file under the admin's settings, decides
// what each row would do, enriches what it can, and stages the result.
//
// It is idempotent by construction: staging rows are replaced wholesale, so the
// admin can adjust the column mapping and run it again as many times as they
// need without accumulating anything.
func (s *Service) PrepareImport(ctx context.Context, publicID string, settings ImportSettings) (*ImportSession, error) {
	return s.prepare(ctx, publicID, settings, nil)
}

// PrepareImportAsync starts preparation in the background and returns at once.
//
// A nine-thousand-row file is ten to fifteen seconds of matching, and with AI
// on it is minutes; running it inside the request that asked for it gives the
// admin a browser hanging on something that may outlive its own timeout.
//
// The session is flipped to 'processing' synchronously, before the goroutine
// starts, and that flip is the durable record of the run. Progress used to live
// only in a map in this process: when a run failed, or the process restarted,
// the session was left at 'draft' with no counts and no error message, and the
// review screen showed an import of nine thousand products as zero rows with
// nothing to explain it.
func (s *Service) PrepareImportAsync(ctx context.Context, publicID string, settings ImportSettings) error {
	if s.imports == nil {
		return ErrImportUnavailable
	}

	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return err
	}
	if session.IsProcessing() {
		return apperr.Conflict("catalog.import_already_running",
			i18n.TDefault("w4_mod.w4str_83_83"))
	}
	if !session.IsRetryable() {
		return apperr.Validation("catalog.import_not_reviewable",
			i18n.TDefault("w4_mod.w4str_84_84"), nil)
	}

	report, claimed := s.progress.TryBegin(publicID)
	if !claimed {
		return apperr.Conflict("catalog.import_already_running",
			i18n.TDefault("w4_mod.w4str_83_83"))
	}

	// Taking the session before the goroutine starts means the caller learns
	// immediately that the run could not be claimed, and means a second submit
	// arriving a millisecond later is refused by the database rather than by a
	// map this process happens to hold.
	session.Mode, session.Options, session.Overrides = settings.Mode, settings.Options, settings.Overrides
	session.Status, session.ErrorMessage = SessionProcessing, ""
	session.Progress = ImportProgress{Phase: ImportPhaseReading, Message: ImportPhaseReading.Label()}
	if err := s.imports.UpdateImportSession(ctx, session, retryableStatuses()...); err != nil {
		s.progress.Finish(publicID, err)
		return err
	}

	// Detached from the request: the admin's browser navigating away, or the
	// request timing out, must not abandon a run that is already underway.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), prepareTimeout)

	go func() {
		defer cancel()
		defer func() {
			if p := recover(); p != nil {
				s.log.ErrorContext(runCtx, "background import preparation panicked",
					"session", publicID, "panic", p)
				s.recordPrepareFailure(runCtx, publicID, fmt.Errorf("panic during import: %v", p))
			}
		}()
		_, err := s.prepare(runCtx, publicID, settings, s.persistedProgress(runCtx, publicID, report))
		s.progress.Finish(publicID, err)
		if err != nil {
			s.log.ErrorContext(runCtx, "background import preparation failed",
				"session", publicID, "error", err)
			s.recordPrepareFailure(runCtx, publicID, err)
		}
	}()
	return nil
}

// persistedProgress wraps the in-memory reporter so each phase change is also
// written to the session row.
//
// Five writes per run, one per phase — not per batch. That is what lets a page
// loaded in a second browser tab, or after a deploy, say what the run is doing
// instead of guessing from an empty map.
func (s *Service) persistedProgress(ctx context.Context, publicID string, report ProgressFunc) ProgressFunc {
	var last ImportPhase
	return func(phase ImportPhase, current, total int) {
		report.report(phase, current, total)

		// Announce EVERY tick, persist only phase changes.
		//
		// These are two different jobs and they want two different rates. The
		// row exists so a page opened in another tab can say what the run is
		// doing, which one write per phase answers. A bar being watched wants
		// the batch-by-batch movement — and taking that from the row would mean
		// either writing per batch or a watcher re-reading on a timer, which is
		// the polling this replaced. Publishing costs a map lookup and a
		// channel send.
		if s.notifyProgress != nil {
			s.notifyProgress(publicID, ImportProgress{
				Phase: phase, Current: current, Total: total, Message: phase.Label(),
			})
		}

		if phase == last {
			return
		}
		last = phase
		if err := s.imports.SaveImportProgress(ctx, publicID, ImportProgress{
			Phase: phase, Current: current, Total: total, Message: phase.Label(),
		}); err != nil {
			s.log.DebugContext(ctx, "persist import progress",
				"session", publicID, "phase", phase, "error", err)
		}
	}
}

// recordPrepareFailure writes the reason a background run stopped onto the
// session, so the screen the admin reloads explains itself.
func (s *Service) recordPrepareFailure(ctx context.Context, publicID string, cause error) {
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return
	}
	// A session the admin cancelled, or one a commit has taken, is not this
	// run's to mark: the guarded write below would fail anyway, and the state
	// it already carries is the truthful one.
	if session.Status != SessionProcessing {
		return
	}

	session.Status = SessionFailed
	session.ErrorMessage = failureMessage(cause)
	session.Progress = ImportProgress{Phase: ImportPhaseFailed, Message: session.ErrorMessage}
	if err := s.imports.UpdateImportSession(ctx, session, SessionProcessing); err != nil {
		s.log.WarnContext(ctx, "could not record import failure",
			"session", publicID, "error", err)
	}
}

// failureMessage prefers the domain's own Arabic wording over a raw Go error,
// which an admin cannot act on.
func failureMessage(err error) string {
	var appErr *apperr.Error
	if errors.As(err, &appErr) && appErr.Msg != "" {
		return appErr.Msg
	}
	return i18n.TDefault("w4_mod.w4str_86_86") + err.Error()
}

// prepareTimeout bounds one background run. A file large enough to exceed this
// needs splitting, and an admin should be told so rather than left watching a
// bar that never moves.
const prepareTimeout = 30 * time.Minute

// ImportProgress reports where a background preparation has reached, according
// to this process's own memory. SessionProgress is what a page should ask.
func (s *Service) ImportProgress(publicID string) (ImportProgress, bool) {
	return s.progress.Progress(publicID)
}
