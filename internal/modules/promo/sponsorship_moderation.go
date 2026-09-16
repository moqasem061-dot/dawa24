package promo

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Admin moderation of sponsorship requests: see them, approve them, reject
// them, or create one directly.
//
// Split from sponsorship_service.go, which was at the 400-line ceiling.
//
// The credit rule these share: a credit is taken once, when the vendor submits
// the request, and given back if the request is rejected or cancelled.
// Approving does not charge — see ActivateSponsorshipRequest.

// AdminListSponsorshipRequests returns all requests for admin moderation.
func (s *Service) AdminListSponsorshipRequests(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, error) {
	return s.repo.ListAllSponsorshipRequests(database.AsSystem(ctx), limit, offset)
}

// AdminApproveSponsorshipRequest activates a pending request.
func (s *Service) AdminApproveSponsorshipRequest(ctx context.Context, id int64, notes string) (*SponsorshipRequest, error) {
	reviewerID, _ := authctx.UserID(ctx)
	sysCtx := database.AsSystem(ctx)

	sr, err := s.repo.GetSponsorshipRequestByID(sysCtx, id)
	if err != nil {
		return nil, err
	}
	if sr.AdminStatus != "pending" {
		return nil, apperr.Conflict("sponsorship.already_reviewed", i18n.TDefault("w4_mod.w4str_250_250"))
	}

	activated, err := s.repo.ActivateSponsorshipRequest(sysCtx, id, reviewerID)
	if err != nil {
		return nil, err
	}
	if notes != "" {
		_ = s.repo.UpdateSponsorshipRequestAdminStatus(sysCtx, id, AdminApproved, notes, reviewerID)
	}
	s.log.InfoContext(ctx, "sponsorship request approved", "request_id", id, "reviewer_id", reviewerID)
	return activated, nil
}

// AdminRejectSponsorshipRequest rejects a pending request and refunds the credit.
func (s *Service) AdminRejectSponsorshipRequest(ctx context.Context, id int64, notes string) error {
	reviewerID, _ := authctx.UserID(ctx)
	sysCtx := database.AsSystem(ctx)

	sr, err := s.repo.GetSponsorshipRequestByID(sysCtx, id)
	if err != nil {
		return err
	}
	if sr.AdminStatus != "pending" && sr.AdminStatus != "approved" {
		return apperr.Conflict("sponsorship.already_reviewed", i18n.TDefault("w4_mod.w4str_250_250"))
	}

	if err := s.repo.UpdateSponsorshipRequestAdminStatus(sysCtx, id, AdminRejected, notes, reviewerID); err != nil {
		return err
	}

	// Refund the reserved credit if a purchase was linked. A refund that fails
	// is a balance the vendor is owed, so it is logged rather than dropped.
	if sr.PurchaseID != nil && sr.CreditsUsed > 0 {
		if _, err := s.repo.ConsumeSponsorshipCredits(sysCtx, ConsumeCredits{
			OrganizationID: sr.OrganizationID,
			PurchaseID:     *sr.PurchaseID,
			Credits:        sr.CreditsUsed,
			Refund:         true,
			Reason:         CreditSponsorshipRejected,
			EntityType:     string(sr.ItemType),
			EntityID:       &sr.ID,
			ActorUserID:    &reviewerID,
			Note:           i18n.TDefault("promo.credits.note.request_rejected"),
		}); err != nil {
			s.log.ErrorContext(ctx, "sponsorship reject: credit refund failed",
				"error", err, "request_id", id, "purchase_id", *sr.PurchaseID)
		}
	}
	s.log.InfoContext(ctx, "sponsorship request rejected", "request_id", id, "reviewer_id", reviewerID)
	return nil
}

// AdminCreateDirectSponsorship allows an admin to create and immediately activate a product or offer sponsorship.
func (s *Service) AdminCreateDirectSponsorship(ctx context.Context, sr *SponsorshipRequest) error {
	sysCtx := database.AsSystem(ctx)
	sr.AdminStatus = AdminApproved
	sr.Status = SRSActive
	if err := s.repo.CreateSponsorshipRequest(sysCtx, sr); err != nil {
		return err
	}
	reviewerID, _ := authctx.UserID(ctx)
	_, err := s.repo.ActivateSponsorshipRequest(sysCtx, sr.ID, reviewerID)
	return err
}
