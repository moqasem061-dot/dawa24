package promo

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// creditLedgerRepo records every credit movement so the whole request lifecycle
// can be asserted as a balance, which is what a vendor actually experiences.
type creditLedgerRepo struct {
	Repository
	purchase   *SponsorshipPurchase
	pkg        *OfferPackage
	requests   map[int64]*SponsorshipRequest
	nextID     int64
	movements  []int
	reasons    []CreditReason
	cancelled  map[int64]bool
	activated  map[int64]bool
	refundFail bool
}

func newCreditLedgerRepo() *creditLedgerRepo {
	return &creditLedgerRepo{
		purchase: &SponsorshipPurchase{
			ID: 1, PackageID: 10, OrganizationID: 5,
			CreditsTotal: 3, CreditsUsed: 0, Status: "active",
			ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		},
		pkg:       &OfferPackage{ID: 10, DurationDays: 30},
		requests:  map[int64]*SponsorshipRequest{},
		nextID:    100,
		cancelled: map[int64]bool{},
		activated: map[int64]bool{},
	}
}

func (m *creditLedgerRepo) GetPackageByID(_ context.Context, _ int64) (*OfferPackage, error) {
	return m.pkg, nil
}
func (m *creditLedgerRepo) ListActiveSponsorshipPurchasesByOrg(_ context.Context, _ int64) ([]*SponsorshipPurchase, error) {
	return []*SponsorshipPurchase{m.purchase}, nil
}

// The ledger mock records the same signed movement the old counter API took,
// so the assertions below still read as "one charge, one refund" — but it now
// also proves each movement arrives with a reason attached.
func (m *creditLedgerRepo) ConsumeSponsorshipCredits(
	_ context.Context, in ConsumeCredits,
) (*CreditEntry, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if in.Refund && m.refundFail {
		return nil, apperr.New(apperr.KindUnavailable, "promo.credit_refund_failed", "refund unavailable")
	}
	used := in.Credits
	if in.Refund {
		used = -in.Credits
	}
	m.movements = append(m.movements, used)
	m.reasons = append(m.reasons, in.Reason)
	m.purchase.CreditsUsed += used
	return &CreditEntry{
		PurchaseID:   in.PurchaseID,
		Delta:        in.Delta(),
		BalanceAfter: m.purchase.CreditsTotal - m.purchase.CreditsUsed,
		Reason:       in.Reason,
	}, nil
}

func (m *creditLedgerRepo) ListCreditEntries(
	_ context.Context, _ int64, _, _ int,
) ([]*CreditEntry, int, error) {
	return nil, 0, nil
}

func (m *creditLedgerRepo) ListOrgCreditEntries(
	_ context.Context, _ int64, _, _ int,
) ([]*CreditEntry, int, error) {
	return nil, 0, nil
}
func (m *creditLedgerRepo) CreateSponsorshipRequest(_ context.Context, sr *SponsorshipRequest) error {
	m.nextID++
	sr.ID = m.nextID
	cp := *sr
	m.requests[sr.ID] = &cp
	return nil
}
func (m *creditLedgerRepo) GetSponsorshipRequestByID(_ context.Context, id int64) (*SponsorshipRequest, error) {
	if r, ok := m.requests[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, nil
}
func (m *creditLedgerRepo) CancelSponsorshipRequest(_ context.Context, id, _ int64) error {
	m.cancelled[id] = true
	return nil
}
func (m *creditLedgerRepo) ActivateSponsorshipRequest(_ context.Context, id, _ int64) (*SponsorshipRequest, error) {
	m.activated[id] = true
	r := m.requests[id]
	r.AdminStatus = AdminApproved
	r.Status = SRSActive
	cp := *r
	return &cp, nil
}
func (m *creditLedgerRepo) UpdateSponsorshipRequestAdminStatus(_ context.Context, id int64, st AdminStatus, _ string, _ int64) error {
	if r, ok := m.requests[id]; ok {
		r.AdminStatus = st
	}
	return nil
}

func ledgerService(repo *creditLedgerRepo) *Service {
	return &Service{repo: repo, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func vendorCtx() context.Context {
	return database.WithTenant(context.Background(), 5)
}

// A credit is taken once, when the request is submitted. Approving it is a
// decision, not a second purchase.
//
// Charging again on approval meant every sponsorship cost two credits, and a
// vendor who had spent their balance exactly could not be approved at all: the
// admin pressed approve and got "insufficient credits" on a request the vendor
// had already paid for.
func TestApprovalDoesNotChargeCreditsTwice(t *testing.T) {
	repo := newCreditLedgerRepo()
	svc := ledgerService(repo)

	created, err := svc.SubmitBatchSponsorshipRequests(vendorCtx(), SponsorItemProduct, []int64{77}, 10)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("created %d requests, want 1", len(created))
	}
	if repo.purchase.CreditsUsed != 1 {
		t.Fatalf("after submit, credits used = %d, want 1", repo.purchase.CreditsUsed)
	}

	if _, err := svc.AdminApproveSponsorshipRequest(context.Background(), created[0].ID, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}

	if repo.purchase.CreditsUsed != 1 {
		t.Fatalf("after approval, credits used = %d, want 1 — approval charged again", repo.purchase.CreditsUsed)
	}
	if !repo.activated[created[0].ID] {
		t.Fatal("approval did not activate the request")
	}
}

// A vendor who spends their balance exactly must still be approvable.
func TestVendorSpendingTheirWholeBalanceCanStillBeApproved(t *testing.T) {
	repo := newCreditLedgerRepo()
	svc := ledgerService(repo)

	created, err := svc.SubmitBatchSponsorshipRequests(vendorCtx(), SponsorItemProduct, []int64{1, 2, 3}, 10)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if repo.purchase.CreditsUsed != 3 || repo.purchase.CreditsRemainingInt() != 0 {
		t.Fatalf("after submitting 3 of 3 credits: used=%d remaining=%d",
			repo.purchase.CreditsUsed, repo.purchase.CreditsRemainingInt())
	}

	for _, sr := range created {
		if _, err := svc.AdminApproveSponsorshipRequest(context.Background(), sr.ID, ""); err != nil {
			t.Fatalf("approving request %d with an exhausted balance: %v", sr.ID, err)
		}
	}
	if repo.purchase.CreditsUsed != 3 {
		t.Fatalf("credits used = %d after approving 3 requests, want 3", repo.purchase.CreditsUsed)
	}
}

// Withdrawing a pending request gives the credit back, exactly as rejection
// does. Without this a vendor who changed their mind simply lost it.
func TestCancellingAPendingRequestRefundsTheCredit(t *testing.T) {
	repo := newCreditLedgerRepo()
	svc := ledgerService(repo)

	created, err := svc.SubmitBatchSponsorshipRequests(vendorCtx(), SponsorItemProduct, []int64{9}, 10)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if repo.purchase.CreditsRemainingInt() != 2 {
		t.Fatalf("remaining after submit = %d, want 2", repo.purchase.CreditsRemainingInt())
	}

	if err := svc.CancelSponsorshipRequest(vendorCtx(), created[0].ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !repo.cancelled[created[0].ID] {
		t.Fatal("the request was not cancelled")
	}
	if repo.purchase.CreditsRemainingInt() != 3 {
		t.Fatalf("remaining after cancel = %d, want 3 — the credit was not refunded",
			repo.purchase.CreditsRemainingInt())
	}
}

// Cancelling somebody else's request must neither cancel it nor move credits.
func TestCancellingAnotherTenantsRequestIsRefused(t *testing.T) {
	repo := newCreditLedgerRepo()
	svc := ledgerService(repo)

	created, err := svc.SubmitBatchSponsorshipRequests(vendorCtx(), SponsorItemProduct, []int64{9}, 10)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	before := repo.purchase.CreditsUsed

	other := database.WithTenant(context.Background(), 999)
	if err := svc.CancelSponsorshipRequest(other, created[0].ID); err == nil {
		t.Fatal("another tenant cancelled this vendor's request")
	}
	if repo.cancelled[created[0].ID] {
		t.Fatal("another tenant's cancel reached the repository")
	}
	if repo.purchase.CreditsUsed != before {
		t.Fatalf("credits moved on a refused cancel: %d -> %d", before, repo.purchase.CreditsUsed)
	}
}

// A refund that fails must not take the cancellation with it, and must not
// vanish either. The request is already withdrawn by the time the refund runs;
// returning the error would tell the vendor their cancellation failed when it
// did not, and discarding it would lose a credit they are owed. It is logged,
// and the balance is left visibly short so the discrepancy is findable.
func TestCancelSurvivesAFailedRefund(t *testing.T) {
	repo := newCreditLedgerRepo()
	svc := ledgerService(repo)

	created, err := svc.SubmitBatchSponsorshipRequests(vendorCtx(), SponsorItemProduct, []int64{77}, 10)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	purchaseID := repo.purchase.ID
	repo.requests[created[0].ID].PurchaseID = &purchaseID
	repo.requests[created[0].ID].CreditsUsed = 1
	repo.refundFail = true

	if err := svc.CancelSponsorshipRequest(vendorCtx(), created[0].ID); err != nil {
		t.Fatalf("cancel reported an error the vendor cannot act on: %v", err)
	}
	if !repo.cancelled[created[0].ID] {
		t.Error("the request was not cancelled")
	}
	if repo.purchase.CreditsUsed != 1 {
		t.Errorf("the failed refund changed the balance to %d", repo.purchase.CreditsUsed)
	}
}

// Every movement carries a reason. A ledger row that says only "-1" is the
// counter this table replaced.
func TestEveryCreditMovementHasAReason(t *testing.T) {
	repo := newCreditLedgerRepo()
	svc := ledgerService(repo)

	if _, err := svc.SubmitBatchSponsorshipRequests(vendorCtx(), SponsorItemProduct, []int64{77}, 10); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if len(repo.reasons) == 0 {
		t.Fatal("no credit movement was recorded")
	}
	for i, reason := range repo.reasons {
		if !reason.Valid() {
			t.Errorf("movement %d carries reason %q, which the ledger will refuse", i, reason)
		}
	}
}

func (m *creditLedgerRepo) CreditTotals(context.Context, int64) (int, int, error) { return 0, 0, nil }
func (m *creditLedgerRepo) ListCreditAccounts(context.Context, string, int, int) ([]*CreditAccount, int, error) {
	return nil, 0, nil
}
func (m *creditLedgerRepo) ListPurchasesForOrg(context.Context, int64, int, int) ([]*SponsorshipPurchase, int, error) {
	return nil, 0, nil
}
