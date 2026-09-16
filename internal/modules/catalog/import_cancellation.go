package catalog

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// releaseClaim hands a claimed session back to review. Used when commit stops
// before writing anything — nothing selected, an oversized batch — where the
// right outcome is the admin back on the review screen, not a failed session.
func (s *Service) releaseClaim(ctx context.Context, session *ImportSession) {
	session.Status = SessionReady
	if err := s.imports.UpdateImportSession(ctx, session, SessionCommitting); err != nil {
		s.log.ErrorContext(ctx, "could not release import commit claim",
			"session", session.PublicID, "error", err)
	}
}

// CancelImport discards a session without touching the catalogue.
//
// The transition is guarded: cancelling an already-committed session is refused
// rather than reported as success, which is what the unguarded overwrite used
// to do.
func (s *Service) CancelImport(ctx context.Context, publicID string) error {
	if s.imports == nil {
		return ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return err
	}
	// The session row is the authority on whether a run is live. The in-memory
	// tracker lags it — it is updated after the transaction commits — so
	// consulting it here refused a cancel for the few milliseconds after a run
	// had already finished and durably said so.
	if session.IsProcessing() {
		return apperr.Conflict("catalog.import_still_processing",
			i18n.TDefault("w4_mod.w4str_103_103"))
	}
	if !session.IsReviewable() && session.Status != SessionFailed {
		return apperr.Conflict("catalog.import_not_cancellable",
			i18n.TDefault("w4_mod.w4str_104_104"))
	}
	from := session.Status
	session.Status = SessionCancelled
	if err := s.imports.UpdateImportSession(ctx, session, from); err != nil {
		return err
	}
	s.releaseSessionWorkspace(ctx, session)
	return nil
}

// releaseSessionWorkspace frees what a finished session no longer needs.
//
// Best-effort by design: an import that has already been written must not be
// reported as failed because its scratch space could not be tidied. The reaper
// that runs when the next session opens collects whatever is left.
func (s *Service) releaseSessionWorkspace(ctx context.Context, session *ImportSession) {
	s.sheets.drop(session.PublicID)
	if err := s.imports.ReleaseImportSourceFile(ctx, session.ID); err != nil {
		s.log.WarnContext(ctx, "could not release import source file",
			"session", session.PublicID, "error", err)
	}
	if err := s.imports.ClearStagingRows(ctx, session.ID); err != nil {
		s.log.WarnContext(ctx, "could not clear staged rows",
			"session", session.PublicID, "error", err)
	}
}

// GetImportSession loads a session for the review screen.
func (s *Service) GetImportSession(ctx context.Context, publicID string) (*ImportSession, error) {
	if s.imports == nil {
		return nil, ErrImportUnavailable
	}
	return s.imports.GetImportSession(ctx, publicID)
}

// ListStagingRows reads a page of the review table.
func (s *Service) ListStagingRows(
	ctx context.Context, publicID string, filter StagingFilter,
) (*ImportSession, []*StagingRow, int, error) {
	if s.imports == nil {
		return nil, nil, 0, ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return nil, nil, 0, err
	}
	rows, total, err := s.ListStagingRowsFor(ctx, session.ID, filter)
	if err != nil {
		return nil, nil, 0, err
	}
	return session, rows, total, nil
}

// ListStagingRowsFor reads a page of the review table for a session already in
// hand, so a caller that has just loaded the session does not load it again.
//
// The extra read is not free and it is not harmless: it is a second answer to
// "what state is this import in", and a run finishing between the two is enough
// for a screen to disagree with itself.
func (s *Service) ListStagingRowsFor(
	ctx context.Context, sessionID int64, filter StagingFilter,
) ([]*StagingRow, int, error) {
	if s.imports == nil {
		return nil, 0, ErrImportUnavailable
	}
	return s.imports.ListStagingRows(ctx, sessionID, filter)
}

// GetStagingRow returns one staged row for review.
func (s *Service) GetStagingRow(ctx context.Context, publicID string, rowID int64) (*StagingRow, error) {
	if s.imports == nil {
		return nil, ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return nil, err
	}
	return s.imports.GetStagingRow(ctx, session.ID, rowID)
}

// SetRowIncluded flips one row's inclusion switch in the review table.
func (s *Service) SetRowIncluded(ctx context.Context, publicID string, rowID int64, included bool) error {
	if s.imports == nil {
		return ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return err
	}
	return s.imports.SetRowIncluded(ctx, session.ID, rowID, included)
}

// SetRowsIncludedByAction flips every row sharing an action.
func (s *Service) SetRowsIncludedByAction(
	ctx context.Context, publicID string, action RowAction, included bool,
) (int64, error) {
	if s.imports == nil {
		return 0, ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return 0, err
	}
	return s.imports.SetRowsIncludedByAction(ctx, session.ID, action, included)
}

// StagingCounts tallies what the admin has left selected.
func (s *Service) StagingCounts(ctx context.Context, sessionID int64) (StagingCounts, error) {
	if s.imports == nil {
		return StagingCounts{}, ErrImportUnavailable
	}
	return s.imports.CountStagingActions(ctx, sessionID)
}

// ImportVocabulary is the taxonomy offered in the wizard's category chooser.
//
// An organisation of zero means "the master catalogue", which the wizard uses
// before a session exists and therefore before it knows which organisation owns
// it.
func (s *Service) ImportVocabulary(ctx context.Context, orgID int64) (EnrichVocabulary, error) {
	if s.imports == nil {
		return EnrichVocabulary{}, ErrImportUnavailable
	}
	if orgID <= 0 {
		resolved, err := s.imports.DefaultCatalogOrg(ctx)
		if err != nil {
			return EnrichVocabulary{}, err
		}
		orgID = resolved
	}
	return s.imports.ImportVocabulary(ctx, orgID)
}

// RecentImportSessions backs the history panel on the upload screen.
func (s *Service) RecentImportSessions(ctx context.Context, orgID int64, limit int) ([]*ImportSession, error) {
	if s.imports == nil {
		return nil, nil
	}
	if orgID <= 0 {
		resolved, err := s.imports.DefaultCatalogOrg(ctx)
		if err != nil {
			return nil, err
		}
		orgID = resolved
	}
	return s.imports.ListRecentImportSessions(ctx, orgID, limit)
}

// SummarizeProduct renders a staged product as one readable line for the review
// table's detail column.
func SummarizeProduct(p *Product) string {
	if p == nil {
		return ""
	}
	var parts []string
	if p.SKU != "" {
		parts = append(parts, i18n.TDefault("w4_mod.w4str_105_105")+p.SKU)
	}
	if p.Price.IsPositive() {
		parts = append(parts, i18n.TDefault("w4_mod.w4str_106_106")+p.Price.String())
	}
	if p.DosageForm != "" {
		parts = append(parts, p.DosageForm)
	}
	if p.ManufacturingCompanies != "" {
		parts = append(parts, p.ManufacturingCompanies)
	}
	if en := p.Name.Get(i18n.EN); en != "" && en != p.Name.Get(i18n.AR) {
		parts = append(parts, en)
	}
	return strings.Join(parts, " · ")
}

// ImportStructure re-reads a session's file and reports how its columns map.
//
// The review screen needs this to draw the mapping panel. It re-parses rather
// than storing the plan because the admin's overrides change it, and a stored
// copy would drift from what the next run actually does.
func (s *Service) ImportStructure(ctx context.Context, publicID string) (FileStructure, error) {
	if s.imports == nil {
		return FileStructure{}, ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return FileStructure{}, err
	}
	if !session.Structure.IsEmpty() {
		return session.Structure, nil
	}

	// A session opened before the structure was stored, or one whose analysis
	// predates this build. Re-read it once and keep the result, so the next
	// render is free.
	sheet, err := s.sheetFor(ctx, session)
	if err != nil {
		return FileStructure{}, err
	}
	session.Structure = BuildFileStructure(sheet, AnalyzeLayout(sheet).Apply(sheet, session.Overrides))
	if err := s.imports.UpdateImportSession(ctx, session, session.Status); err != nil {
		s.log.DebugContext(ctx, "could not backfill import structure",
			"session", publicID, "error", err)
	}
	return session.Structure, nil
}

// EnricherRunning reports whether a background preparation is in flight. Tests
// use it to catch a run mid-flight; the UI uses ImportProgress.
func (s *Service) EnricherRunning(publicID string) bool {
	return s.progress.Running(publicID)
}
