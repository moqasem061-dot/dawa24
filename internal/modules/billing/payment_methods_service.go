package billing

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// AddPaymentMethod saves a user payment method.
func (s *Service) AddPaymentMethod(ctx context.Context, pm *UserPaymentMethod) error {
	if pm.UserID <= 0 || pm.Provider == "" || pm.AccountIdentifier == "" {
		return apperr.Validation("payment_method.invalid", "User ID, provider, and account identifier are required.", nil)
	}
	return s.repo.AddPaymentMethod(ctx, pm)
}

// GetPaymentMethodByID returns a single payment method for a user.
func (s *Service) GetPaymentMethodByID(ctx context.Context, userID, id int64) (*UserPaymentMethod, error) {
	if userID <= 0 || id <= 0 {
		return nil, apperr.Validation("payment_method.invalid_id", "Valid user ID and payment method ID are required.", nil)
	}
	return s.repo.GetPaymentMethodByID(ctx, userID, id)
}

// ListPaymentMethods returns saved payment methods for a user.
func (s *Service) ListPaymentMethods(ctx context.Context, userID int64) ([]*UserPaymentMethod, error) {
	return s.repo.ListPaymentMethods(ctx, userID)
}

// UpdatePaymentMethod updates a user payment method.
func (s *Service) UpdatePaymentMethod(ctx context.Context, pm *UserPaymentMethod) error {
	if pm.ID <= 0 || pm.UserID <= 0 || pm.Provider == "" || pm.AccountIdentifier == "" {
		return apperr.Validation("payment_method.invalid", "Valid payment method ID, user ID, provider, and account identifier are required.", nil)
	}
	return s.repo.UpdatePaymentMethod(ctx, pm)
}

// SetDefaultPaymentMethod sets one payment method as default for a user.
func (s *Service) SetDefaultPaymentMethod(ctx context.Context, userID, id int64) error {
	if userID <= 0 || id <= 0 {
		return apperr.Validation("payment_method.invalid_id", "Valid user ID and payment method ID are required.", nil)
	}
	return s.repo.SetDefaultPaymentMethod(ctx, userID, id)
}

// DeletePaymentMethod removes a user payment method scoped to the owner.
func (s *Service) DeletePaymentMethod(ctx context.Context, userID, id int64) error {
	if userID <= 0 || id <= 0 {
		return apperr.Validation("payment_method.invalid_id", "Valid user ID and payment method ID are required.", nil)
	}
	return s.repo.DeletePaymentMethod(ctx, userID, id)
}

// ListPlatformPaymentMethods returns all platform configured payment channels.
func (s *Service) ListPlatformPaymentMethods(ctx context.Context, onlyActive bool) ([]*PlatformPaymentMethod, error) {
	return s.repo.ListPlatformPaymentMethods(ctx, onlyActive)
}

// GetPlatformPaymentMethod returns a single platform payment method.
func (s *Service) GetPlatformPaymentMethod(ctx context.Context, id string) (*PlatformPaymentMethod, error) {
	return s.repo.GetPlatformPaymentMethod(ctx, id)
}

// SavePlatformPaymentMethod adds or updates a platform payment channel configuration.
func (s *Service) SavePlatformPaymentMethod(ctx context.Context, pm *PlatformPaymentMethod) error {
	if pm.ID == "" || pm.Name.IsEmpty() {
		return apperr.Validation("payment_method.invalid", "ID and Name are required for platform payment method.", nil)
	}
	return s.repo.SavePlatformPaymentMethod(ctx, pm)
}
