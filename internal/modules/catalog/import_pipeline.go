package catalog

import (
	"context"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// SessionProgress answers "what is this import doing" from whichever source
// knows: the live run in this process, or the session row.
//
// The row is the authority on whether a run exists at all. A poll answered from
// process memory alone reported "failed" for a perfectly healthy run started by
// another replica, and reported nothing at all after a restart — which is how a
// finished import of nine thousand products came to render as an empty screen.
func (s *Service) SessionProgress(ctx context.Context, publicID string) (ImportProgress, *ImportSession, error) {
	session, err := s.GetImportSession(ctx, publicID)
	if err != nil {
		return ImportProgress{}, nil, err
	}

	// Only a live, non-terminal snapshot beats the row. ProgressTracker.Progress
	// reports whether an entry exists, not whether it is still running, and a
	// finished run keeps its entry for ten minutes — so trusting that flag alone
	// published "failed" in the gap between the goroutine marking itself done
	// and the transaction that records it, and a poller acting on that answer
	// raced the very write it was waiting for.
	if live, ok := s.progress.Progress(publicID); ok && session.IsProcessing() && !live.Phase.Terminal() {
		return live, session, nil
	}

	// An orphan: the row says processing, no run in this process is behind it,
	// and nothing has written to it for long enough that none ever will. That
	// is a server restart, a crash, or a deploy mid-import, and the admin needs
	// to be told so they can retry rather than watch a bar that will never move
	// again.
	//
	// It is deliberately NOT the first thing checked. A healthy long run holds
	// its entry in the tracker and is answered above; a failing one writes its
	// own reason before the tracker releases it. Both of those orderings exist
	// because this reaper used to win the race and replace a specific, useful
	// failure with a guess about the server.
	if idle, orphaned := s.orphanedFor(session, publicID); orphaned {
		s.log.WarnContext(ctx, "reaping stale processing session",
			"session", publicID, "idle", idle)
		session.Status = SessionFailed
		session.ErrorMessage = "توقفت معالجة الملف بسبب مقاطعة العملية أو إعادة تشغيل الخادم. يمكنك تصحيح ربط الأعمدة وإعادة المحاولة الآن."
		session.Progress = ImportProgress{
			Phase:   ImportPhaseFailed,
			Message: session.ErrorMessage,
		}
		_ = s.imports.UpdateImportSession(ctx, session, SessionProcessing)
		return session.Progress, session, nil
	}

	progress := session.Progress
	switch {
	case session.IsProcessing():
		// Claimed but nothing reported yet, or reported by another process.
		if progress.Phase == "" {
			progress.Phase = ImportPhaseReading
		}
	case session.Status == SessionFailed:
		progress.Phase = ImportPhaseFailed
		progress.Message = session.ErrorMessage
	case session.Status == SessionDraft:
		// Analysed and waiting on the mapping step: no run has been asked for.
		progress.Phase = PhaseQueued
	default:
		progress.Phase = ImportPhaseDone
	}
	if progress.Message == "" {
		progress.Message = progress.Phase.Label()
	}
	return progress, session, nil
}

// orphanRecoveryDelay is how long a processing session may go without a write
// before it is treated as abandoned.
//
// It has to clear the longest gap a HEALTHY run leaves between writes, because
// anything shorter force-fails a working import. That used to be a whole phase
// — minutes, on a file large enough to matter — against a window of thirty
// seconds, so a live run was one in-memory map miss away from being declared
// dead. A run now writes to its row every progressHeartbeat whatever phase it
// is in, which is what makes a short window safe rather than optimistic.
//
// Ninety seconds is six missed heartbeats: long enough to ride out a slow
// statement or a paused container, short enough that an administrator whose
// deploy interrupted an import is told so rather than left watching.
const orphanRecoveryDelay = 90 * time.Second

// orphanedFor reports whether a processing session has been abandoned, and how
// long it has been silent.
//
// Silence is measured from the freshest thing the row knows: its own updated_at
// and the progress timestamp the run writes at each phase change. Taking only
// the first of those would call a run abandoned while it was demonstrably
// writing progress.
func (s *Service) orphanedFor(session *ImportSession, publicID string) (time.Duration, bool) {
	if !session.IsProcessing() || s.progress.Running(publicID) {
		return 0, false
	}
	last := session.UpdatedAt
	if session.Progress.UpdatedAt.After(last) {
		last = session.Progress.UpdatedAt
	}
	if last.IsZero() {
		return 0, false
	}
	idle := time.Since(last)
	return idle, idle > orphanRecoveryDelay
}

func (s *Service) prepare(
	ctx context.Context, publicID string, settings ImportSettings, progress ProgressFunc,
) (*ImportSession, error) {
	if s.imports == nil {
		return nil, ErrImportUnavailable
	}
	progress.report(ImportPhaseReading, 0, 0)

	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return nil, err
	}
	// A run reaches here having already claimed the session, so 'processing' is
	// the expected state; the synchronous PrepareImport used by tests and the
	// CLI arrives on a session that is merely retryable.
	if !session.IsRetryable() && !session.IsProcessing() {
		return nil, apperr.Validation("catalog.import_not_reviewable",
			i18n.TDefault("w4_mod.w4str_84_84"), nil)
	}

	sheet, err := s.sheetFor(ctx, session)
	if err != nil {
		return nil, err
	}
	from := session.Status

	session.Mode = settings.Mode
	session.Options = settings.Options

	// Request one: which column is which field. It runs before parsing because
	// its whole purpose is to change how the sheet is read.
	session.AICalls, session.AIApplied, session.AIFallback, session.AINote = 0, 0, false, ""
	session.Overrides = s.resolveColumnMapping(ctx, session, sheet, settings.Overrides)

	progress.report(ImportPhaseParsing, 0, 0)
	parsed := ParseSheet(sheet, session.Overrides, settings.Options)
	applyParseStats(session, parsed)
	session.Structure = BuildFileStructure(sheet, parsed.Layout)
	session.SheetName = sheet.Sheet
	session.SourceFormat = sheet.Format
	session.Delimiter = sheet.Delimiter

	// A mapping that yields nothing is a mapping problem, not an empty file,
	// and the admin needs to be sent back to fix it rather than handed a review
	// screen listing zero products with no explanation.
	if len(parsed.Products) == 0 {
		return nil, apperr.Validation("catalog.import_no_products",
			emptyParseMessage(parsed), nil)
	}

	progress.report(ImportPhaseParsing, len(parsed.Products), len(parsed.Products))

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	s.resolveTaxonomies(ctx, session, parsed, progress)

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// No count while the catalogue is read and the identifiers resolved: it is
	// one query and one pass, and nothing inside either can report. Reported
	// WITH a count — which is what this did — the bar showed "0 of 19,996" and
	// was arithmetically unable to move until the call returned, which on a
	// large catalogue is the whole of the wait somebody is watching.
	progress.report(ImportPhaseCatalogue, 0, 0)
	matches, err := s.imports.MatchExistingProducts(ctx, parsed.Products)
	if err != nil {
		return nil, err
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// The exact identifiers have had their turn. Everything they missed is
	// scored against the catalogue in memory, and — where the buyer left the AI
	// switch on — what similarity still cannot settle is adjudicated in batches.
	// Without this the importer matched under a tenth of a real supplier file
	// and staged the rest as new products, quietly duplicating the catalogue.
	matchStats := s.resolveSimilarMatches(ctx, session, parsed.Products, matches, progress)
	session.AIMatched = matchStats.Similar + matchStats.AI
	if matchStats.CeilingHit {
		session.AIFallback = true
	}
	progress.report(ImportPhaseMatching, len(parsed.Products), len(parsed.Products))

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	progress.report(ImportPhaseStaging, 0, len(parsed.Products))
	rows := buildStagingRows(parsed, matches, session.Mode)
	applyRowStats(session, rows)
	session.NewBrands = collectNewBrands(parsed.Products, matches)
	if err := s.imports.ReplaceStagingRows(ctx, session.ID, rows); err != nil {
		return nil, err
	}

	session.Status = SessionReady
	session.Progress = ImportProgress{Phase: ImportPhaseDone, Message: ImportPhaseDone.Label()}
	// Guarded: if the admin cancelled or a commit landed while this prepare
	// ran, the transition fails and the staging rows are left for the reaper
	// rather than resurrecting a dead session with a ready status.
	if err := s.imports.UpdateImportSession(ctx, session, from); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "catalogue import prepared",
		"session", session.PublicID, "mode", session.Mode,
		"insert", session.InsertRows, "update", session.UpdateRows,
		"skip", session.SkipRows, "errors", session.ErrorRows,
		"match_exact", matchStats.Exact, "match_similar", matchStats.Similar,
		"match_ai", matchStats.AI, "match_unmatched", matchStats.Unmatched,
		"match_rate_pct", matchStats.RatePercent(),
		"ai_calls", session.AICalls, "ai_applied", session.AIApplied)
	return session, nil
}

// emptyParseMessage explains a run that produced no products, naming the reason
// the parse itself recorded rather than leaving the admin to guess.
func emptyParseMessage(parsed *ParseResult) string {
	switch {
	case parsed.Stats.TotalRowsRead == 0:
		return i18n.TDefault("w4_mod.w4str_87_87")
	case parsed.Stats.RejectedRows > 0:
		return fmt.Sprintf(
			i18n.TDefault("w4_mod.d_88")+
				i18n.TDefault("w4_mod.w4str_89_89"),
			parsed.Stats.RejectedRows)
	default:
		return i18n.TDefault("w4_mod.w4str_90_90") +
			i18n.TDefault("w4_mod.w4str_91_91")
	}
}
