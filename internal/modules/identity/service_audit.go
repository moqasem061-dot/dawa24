package identity

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// RecordAudit logs a user lifecycle or security action to platform.audit_log.
func (s *Service) RecordAudit(ctx context.Context, e database.AuditEntry) error {
	if s.repo == nil {
		return nil
	}
	return s.repo.RecordAudit(ctx, e)
}
