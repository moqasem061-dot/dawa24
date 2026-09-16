package ingest

// The review stage's own vocabulary.
//
// Everything here answers one question the wizard could not answer before: what
// does a vendor DO with nine thousand staged rows? The per-row edits already
// existed and are right for correcting a price; they are the wrong shape for
// the decision that actually blocks an import, which is repeated identically
// over a page of suggestions.
//
// The rule the whole stage now enforces: a row is imported when the engine
// settled it or when a person said so. Nothing else. A suggestion the engine
// scored at 31% is a question, and a question is not an instruction.

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ReviewBulk is what one bulk action did, for the notice the vendor is shown.
//
// Skipped is not an error and is reported separately: a vendor who selected
// forty rows and confirmed thirty-one needs to know the other nine had no
// product to confirm, or they will assume the whole page is done.
type ReviewBulk struct {
	Requested int
	Applied   int
	Skipped   int
}

// ConfirmRowMatches promotes the engine's suggestion on each row into the
// vendor's own decision, which is what makes those rows importable.
func (s *Service) ConfirmRowMatches(
	ctx context.Context, publicID string, rowIDs []int64,
) (ReviewBulk, error) {
	session, err := s.reviewSession(ctx, publicID)
	if err != nil {
		return ReviewBulk{}, err
	}
	n, err := s.imports.ConfirmRowMatches(ctx, session.ID, rowIDs)
	if err != nil {
		return ReviewBulk{}, err
	}
	return ReviewBulk{Requested: len(rowIDs), Applied: n, Skipped: len(rowIDs) - n}, nil
}

// ClearRowMatches unlinks a list of rows.
func (s *Service) ClearRowMatches(
	ctx context.Context, publicID string, rowIDs []int64,
) (ReviewBulk, error) {
	session, err := s.reviewSession(ctx, publicID)
	if err != nil {
		return ReviewBulk{}, err
	}
	n, err := s.imports.ClearRowMatches(ctx, session.ID, rowIDs)
	if err != nil {
		return ReviewBulk{}, err
	}
	return ReviewBulk{Requested: len(rowIDs), Applied: n}, nil
}

// SetRowsExcluded includes or excludes a list of rows.
func (s *Service) SetRowsExcluded(
	ctx context.Context, publicID string, rowIDs []int64, excluded bool,
) (ReviewBulk, error) {
	session, err := s.reviewSession(ctx, publicID)
	if err != nil {
		return ReviewBulk{}, err
	}
	n, err := s.imports.SetRowsExcluded(ctx, session.ID, rowIDs, excluded)
	if err != nil {
		return ReviewBulk{}, err
	}
	return ReviewBulk{Requested: len(rowIDs), Applied: n}, nil
}

// PendingDecisions is how many included rows a commit would refuse, and the
// first few thousand of their ids.
//
// The count is what the confirmation dialog needs. The ids are what the "select
// everything still awaiting a decision" control needs, and returning both from
// one call keeps the two from disagreeing.
func (s *Service) PendingDecisions(
	ctx context.Context, publicID string,
) (ids []int64, total int, err error) {
	session, err := s.reviewSession(ctx, publicID)
	if err != nil {
		return nil, 0, err
	}
	return s.imports.PendingRowIDs(ctx, session.ID, 5000)
}

// RowIDsForFilter resolves the review screen's current filter to a list of row
// ids, for a bulk action applied beyond the visible page.
func (s *Service) RowIDsForFilter(
	ctx context.Context, publicID string, filter RowFilter,
) ([]int64, error) {
	session, err := s.reviewSession(ctx, publicID)
	if err != nil {
		return nil, err
	}
	return s.imports.RowIDsForFilter(ctx, session.ID, filter, 20000)
}

// reviewSession loads an import and refuses anything not open for review.
func (s *Service) reviewSession(ctx context.Context, publicID string) (*Session, error) {
	if s.imports == nil {
		return nil, ErrImportStoreUnavailable
	}
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if session.Phase != PhaseReview {
		return nil, apperr.Conflict("import.not_in_review",
			i18n.TDefault("w4_mod.w4str_193_193"))
	}
	return session, nil
}
