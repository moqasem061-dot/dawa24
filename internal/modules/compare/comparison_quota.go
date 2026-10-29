package compare

import (
	"context"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

func (s *Service) ValidateComparisonCount(ctx context.Context, userID, orgID int64, count int) error {
	ent, err := s.EntitlementFor(ctx, userID, orgID)
	if err != nil {
		return err
	}
	if count > ent.MaxActiveFiles {
		message := fmt.Sprintf(i18n.T("ar", "compare.run.max_suppliers_exceeded"), ent.MaxActiveFiles) + " / " + fmt.Sprintf(i18n.T("en", "compare.run.max_suppliers_exceeded"), ent.MaxActiveFiles)
		return apperr.Validation("compare.max_suppliers", message, nil)
	}
	return nil
}
