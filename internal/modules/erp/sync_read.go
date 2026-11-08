package erp

import "context"

// Read-only accessors the HTTP handlers and review pages use. They carry the
// tenant's explicit organization filter only (no AsSystem, no writes — rule 17),
// so the handlers never touch the repository directly.

// BatchStatus reads one batch by its public sync id for the key's own
// organization.
func (s *Service) BatchStatus(ctx context.Context, orgID int64, syncID string) (SyncBatch, error) {
	return s.repo.GetBatchByPublicID(ctx, orgID, syncID)
}

// BatchItems pages one batch's items (the cursor is sync_items.id).
func (s *Service) BatchItems(ctx context.Context, orgID, batchID, afterID int64, state string, limit int) ([]SyncItemRow, error) {
	return s.repo.ListSyncItems(ctx, orgID, batchID, afterID, state, limit)
}

// OpenReview counts nothing and lists nothing: it exists so the loaders stay
// symmetric; the page flows use ListReview.
func (s *Service) ReviewPage(ctx context.Context, orgID int64, page Page) ([]ReviewItem, error) {
	return s.repo.ListReview(ctx, orgID, page)
}
