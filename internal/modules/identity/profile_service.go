package identity

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// GetMe retrieves the authenticated user's profile and active permissions.
func (s *Service) GetMe(ctx context.Context, userID int64, activeOrgID *int64) (*MeResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var orgID int64
	if activeOrgID != nil {
		orgID = *activeOrgID
	}

	permissions, err := s.repo.GetPermissionsForUser(ctx, userID, orgID)
	if err != nil {
		permissions = []string{}
	}

	return &MeResponse{
		User:        user,
		ActiveOrgID: activeOrgID,
		Role:        user.Role,
		Permissions: permissions,
	}, nil
}

// UpdateProfile updates user profile settings.
func (s *Service) UpdateProfile(ctx context.Context, userID int64, nameAr, nameEn, phone, timezone, lang string) (*User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if nameAr != "" || nameEn != "" {
		if nameAr != "" {
			user.Name["ar"] = nameAr
		}
		if nameEn != "" {
			user.Name["en"] = nameEn
		}
	}
	if phone != "" {
		user.Phone = phone
	}
	if timezone != "" {
		user.Timezone = timezone
	}
	if lang != "" {
		user.Language = i18n.ParseLang(lang)
	}

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateAvatar updates user's profile avatar URL.
func (s *Service) UpdateAvatar(ctx context.Context, userID int64, avatarURL string) (*User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.AvatarURL = avatarURL
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// CreateAddress saves an address for a user.
func (s *Service) CreateAddress(ctx context.Context, addr *UserAddress) (*UserAddress, error) {
	if addr.Recipient == "" || addr.Address == "" || addr.CityID <= 0 {
		return nil, apperr.Validation("address.invalid", "Recipient, address, and city are required.", nil)
	}
	if err := s.repo.CreateAddress(ctx, addr); err != nil {
		return nil, err
	}
	return addr, nil
}

// ListAddresses returns all addresses for a user.
func (s *Service) ListAddresses(ctx context.Context, userID int64) ([]*UserAddress, error) {
	return s.repo.ListAddresses(ctx, userID)
}

// UpdateAddress modifies a user's address.
func (s *Service) UpdateAddress(ctx context.Context, addr *UserAddress) error {
	return s.repo.UpdateAddress(ctx, addr)
}

// DeleteAddress deletes an address for a user.
func (s *Service) DeleteAddress(ctx context.Context, id, userID int64) error {
	return s.repo.DeleteAddress(ctx, id, userID)
}

// ListAddressHistory returns the append-only change trail for a user's addresses.
func (s *Service) ListAddressHistory(ctx context.Context, userID int64, limit int) ([]*UserAddressHistory, error) {
	return s.repo.ListAddressHistory(ctx, userID, limit)
}

// GetPreferences returns the user's display and notification preferences.
func (s *Service) GetPreferences(ctx context.Context, userID int64) (*UserPreferences, error) {
	return s.repo.GetPreferences(ctx, userID)
}

// UpdatePreferences saves the user's preferences.
func (s *Service) UpdatePreferences(ctx context.Context, p *UserPreferences) error {
	return s.repo.UpdatePreferences(ctx, p)
}

// ListSessionPlans returns available concurrent sign-in plans.
func (s *Service) ListSessionPlans(ctx context.Context) ([]*SessionPlan, error) {
	return s.repo.ListSessionPlans(ctx)
}

// PurchaseSessionPlan applies a plan's concurrency limit to the user. Payment
// integration is deferred; the limit is applied directly for now.
func (s *Service) PurchaseSessionPlan(ctx context.Context, userID, planID int64) error {
	plan, err := s.repo.GetSessionPlanByID(ctx, planID)
	if err != nil {
		return err
	}
	if err := s.repo.SetMaxLoginSessions(ctx, userID, plan.MaxLoginSessions); err != nil {
		return err
	}
	return nil
}

// AddFavorite adds a product to user favorites.
func (s *Service) AddFavorite(ctx context.Context, userID, productID int64) error {
	if productID <= 0 {
		return apperr.Validation("favorite.invalid", "Invalid product ID", nil)
	}
	return s.repo.AddFavorite(ctx, userID, productID)
}

// RemoveFavorite removes a product from user favorites.
func (s *Service) RemoveFavorite(ctx context.Context, userID, productID int64) error {
	return s.repo.RemoveFavorite(ctx, userID, productID)
}

// ListFavorites returns all favorited product IDs for a user.
func (s *Service) ListFavorites(ctx context.Context, userID int64) ([]int64, error) {
	return s.repo.ListFavorites(ctx, userID)
}
