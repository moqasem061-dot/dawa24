package catalog

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
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

	s.resolveTaxonomies(ctx, session, parsed, progress)

	progress.report(ImportPhaseMatching, 0, len(parsed.Products))
	matches, err := s.imports.MatchExistingProducts(ctx, parsed.Products)
	if err != nil {
		return nil, err
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

	progress.report(ImportPhaseStaging, 0, len(parsed.Products))
	rows := buildStagingRows(parsed, matches, session.Mode)
	applyRowStats(session, rows)
	session.NewBrands = collectNewBrands(parsed.Products, matches)
	// NewCategories was already computed by resolveTaxonomies; both proposals
	// are persisted here so the review screen can show them.
	// The rows land before the status does, and that order matters.
	//
	// Flipping the session to 'ready' first opened a window — a second on a
	// nine-thousand-row file — in which the review screen, the progress poll
	// and a commit all saw a finished import with an empty staging table. That
	// window is the "it says it imported nine thousand products and shows me
	// nothing" report. The session stays 'processing' until the rows it
	// promises are actually there, and 'processing' is not reviewable, so
	// nothing can cancel or commit underneath the write.
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
