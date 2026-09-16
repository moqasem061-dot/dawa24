package billing

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// TogglePlatformPaymentMethod activates or deactivates a payment method.
func (s *Service) TogglePlatformPaymentMethod(ctx context.Context, id string, active bool) error {
	if id == "" {
		return apperr.Validation("payment_method.invalid_id", "ID is required.", nil)
	}
	return s.repo.TogglePlatformPaymentMethod(ctx, id, active)
}

// TogglePlatformPaymentMethodCheckout enables or disables a platform payment method for checkout.
func (s *Service) TogglePlatformPaymentMethodCheckout(ctx context.Context, id string, enabled bool) error {
	if id == "" {
		return apperr.Validation("payment_method.invalid_id", "ID is required.", nil)
	}
	return s.repo.TogglePlatformPaymentMethodCheckout(ctx, id, enabled)
}

// DeletePlatformPaymentMethod deletes a platform payment channel.
func (s *Service) DeletePlatformPaymentMethod(ctx context.Context, id string) error {
	if id == "" {
		return apperr.Validation("payment_method.invalid_id", "ID is required.", nil)
	}
	return s.repo.DeletePlatformPaymentMethod(ctx, id)
}

// ListPayments returns payments for an organization.
func (s *Service) ListPayments(ctx context.Context, orgID int64, limit, offset int) ([]*Payment, error) {
	return s.repo.ListPaymentsByOrg(ctx, orgID, limit, offset)
}

// ListPaymentsWithTotal returns paginated payments for an organization with total count.
func (s *Service) ListPaymentsWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*Payment, int, error) {
	return s.repo.ListPaymentsByOrgWithTotal(ctx, orgID, limit, offset)
}

// RequestDepositExtended initiates a funds deposit workflow with structured platform and sender channel details.
func (s *Service) RequestDepositExtended(
	ctx context.Context,
	userID int64,
	orgID *int64,
	currency string,
	amount money.Amount,
	method string,
	referenceNumber string,
	attachmentURL string,
	userNotes string,
	platformMethodID string,
	senderAccount string,
	senderPaymentMethodID *int64,
) (*WalletDeposit, error) {
	if err := ValidateCreditAmount(amount); err != nil {
		return nil, err
	}
	if currency == "" {
		currency = "EGP"
	}
	if method == "" && platformMethodID != "" {
		method = platformMethodID
	}
	if method == "" {
		return nil, apperr.Validation("payment_method.required", i18n.T("ar", "billing.val.method_required"), nil)
	}
	if referenceNumber == "" {
		return nil, apperr.Validation("reference_number.required", i18n.T("ar", "billing.val.ref_required"), nil)
	}

	wallet, err := s.repo.GetOrCreateWallet(ctx, userID, currency)
	if err != nil {
		return nil, err
	}

	dep := &WalletDeposit{
		WalletID:              wallet.ID,
		UserID:                userID,
		OrganizationID:        orgID,
		Amount:                amount,
		Currency:              currency,
		PaymentMethod:         method,
		ReferenceNumber:       referenceNumber,
		AttachmentURL:         attachmentURL,
		UserNotes:             userNotes,
		Status:                DepositPending,
		PlatformMethodID:      platformMethodID,
		SenderAccount:         senderAccount,
		SenderPaymentMethodID: senderPaymentMethodID,
	}

	if err := s.repo.CreateDepositRequest(ctx, dep); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "wallet deposit requested", "deposit_id", dep.ID, "user_id", userID, "amount", amount.String(), "method", method, "platform_method", platformMethodID)
	return dep, nil
}

// RequestWithdrawal initiates a withdrawal workflow, creating a request in pending status for admin review.
func (s *Service) RequestWithdrawal(
	ctx context.Context,
	userID int64,
	orgID *int64,
	currency string,
	amount money.Amount,
	payoutMethodType string,
	destinationDetails string,
	userPaymentMethodID *int64,
	userNotes string,
) (*WalletWithdrawal, error) {
	if err := ValidateCreditAmount(amount); err != nil {
		return nil, err
	}
	if currency == "" {
		currency = "EGP"
	}
	if destinationDetails == "" {
		return nil, apperr.Validation("destination.required", "يرجى تحديد حساب أو محفظة الاستلام لسحب الرصيد.", nil)
	}

	wallet, err := s.repo.GetOrCreateWallet(ctx, userID, currency)
	if err != nil {
		return nil, err
	}

	// Verify wallet available balance is sufficient
	if wallet.AvailableBalance.Minor() < amount.Minor() {
		return nil, apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة المتاح غير كافٍ لإتمام طلب السحب.", nil)
	}

	w := &WalletWithdrawal{
		WalletID:            wallet.ID,
		UserID:              userID,
		OrganizationID:      orgID,
		Amount:              amount,
		Currency:            currency,
		PayoutMethodType:    payoutMethodType,
		DestinationDetails:  destinationDetails,
		UserPaymentMethodID: userPaymentMethodID,
		UserNotes:           userNotes,
		Status:              WithdrawalPending,
	}

	if err := s.repo.CreateWithdrawalRequest(ctx, w); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "wallet withdrawal requested", "withdrawal_id", w.ID, "user_id", userID, "amount", amount.String(), "dest", destinationDetails)
	return w, nil
}

// ListUserWithdrawalsWithStatus returns paginated withdrawal requests for a user.
func (s *Service) ListUserWithdrawalsWithStatus(
	ctx context.Context, userID int64, status string, limit, offset int,
) ([]*WalletWithdrawal, error) {
	return s.repo.ListWithdrawalRequestsByUserWithStatus(ctx, userID, status, limit, offset)
}
