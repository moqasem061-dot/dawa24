package identity

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// The moderator hierarchy, as the rest of the platform sees it.
//
// The one method that matters is TempWarehouseOwnerScope. It answers "whose
// uploads may this user act on?" and every screen and action that touches a
// team's temporary warehouses asks it — rather than each deciding for itself,
// which is how a permission check and a listing query come to disagree.

// ListModerators returns every moderator with their parent and team size.
func (s *Service) ListModerators(ctx context.Context) ([]*Moderator, error) {
	return s.repo.ListModerators(ctx)
}

// SetModeratorParent assigns a moderator under a main moderator, or promotes
// them to top-level when parentID is nil.
func (s *Service) SetModeratorParent(ctx context.Context, userID int64, parentID *int64, actorID int64) error {
	if userID <= 0 {
		return apperr.Validation("identity.moderator.invalid_user",
			"لم يتم تحديد المشرف.", nil)
	}
	return s.repo.SetModeratorParent(ctx, userID, parentID, actorID)
}

// ModeratorSubordinateIDs lists the moderators reporting to one moderator.
func (s *Service) ModeratorSubordinateIDs(ctx context.Context, parentID int64) ([]int64, error) {
	return s.repo.ModeratorSubordinateIDs(ctx, parentID)
}

// TeamOwnerIDs is the set of uploader ids a main moderator may see on the
// "مستودعات المشرفين تحت إدارتي" screen.
//
// Their subordinates and nobody else — deliberately NOT including themselves.
// Their own uploads have their own screen (مستودعاتي المرفوعة), and mixing the
// two would make "the warehouses of the people who report to me" quietly mean
// something else.
//
// An empty result is not an error: a moderator with nobody under them has an
// empty team, and the screen says so. What must never happen is an empty scope
// being read as "no filter", so callers get a distinct ok=false for "this user
// leads no team" rather than an empty slice they might treat as unrestricted.
func (s *Service) TeamOwnerIDs(ctx context.Context, moderatorID int64) ([]int64, bool, error) {
	if moderatorID <= 0 {
		return nil, false, nil
	}
	ids, err := s.repo.ModeratorSubordinateIDs(ctx, moderatorID)
	if err != nil {
		return nil, false, err
	}
	return ids, len(ids) > 0, nil
}

// IsMainModerator reports whether a user is a top-level moderator.
func (s *Service) IsMainModerator(ctx context.Context, userID int64) (bool, error) {
	parent, err := s.repo.ModeratorParentID(ctx, userID)
	if err != nil {
		return false, err
	}
	return parent == nil, nil
}

// ModeratorParentID returns the direct parent supervisor ID for a subordinate moderator, or nil if top-level.
func (s *Service) ModeratorParentID(ctx context.Context, userID int64) (*int64, error) {
	if userID <= 0 {
		return nil, nil
	}
	return s.repo.ModeratorParentID(ctx, userID)
}

