package commerce

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestPurchaseRequestLifecycleAndValidation(t *testing.T) {
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)
	ctx := context.Background()

	custOrgID := int64(10)
	vendorOrgID := int64(20)
	branchID := int64(101)

	// 1. Validation: empty lines rejected (T1)
	invalidReq := &PurchaseRequest{
		CustomerID:     100,
		OrganizationID: &custOrgID,
		VendorOrgID:    vendorOrgID,
	}
	_, err := svc.CreatePurchaseRequest(ctx, invalidReq, nil)
	if err == nil {
		t.Fatal("expected error creating purchase request without lines, got nil")
	}

	// 2. Validation: invalid quantity rejected
	invalidLine := []*PurchaseRequestLine{
		{ProductName: "Panadol", Quantity: 0},
	}
	_, err = svc.CreatePurchaseRequest(ctx, invalidReq, invalidLine)
	if err == nil {
		t.Fatal("expected error for line with 0 quantity, got nil")
	}

	// 3. Create valid purchase request with lines (T2)
	pID1 := int64(1)
	pID2 := int64(2)
	validLines := []*PurchaseRequestLine{
		{
			ProductID:      &pID1,
			ProductName:    "Panadol Extra 24 Tab",
			ProductSKU:     "SKU-PAN-EXT",
			Quantity:       50,
			TargetPrice:    money.FromMajor(85),
			TargetDiscount: 15.0,
		},
		{
			ProductID:      &pID2,
			ProductName:    "Cataflam 50mg 20 Tab",
			ProductSKU:     "SKU-CAT-50",
			Quantity:       20,
			TargetPrice:    money.FromMajor(40),
			TargetDiscount: 20.0,
		},
	}

	validReq := &PurchaseRequest{
		CustomerID:     100,
		OrganizationID: &custOrgID,
		BranchID:       &branchID,
		VendorOrgID:    vendorOrgID,
		BuyerNotes:     "Please deliver by Thursday.",
	}

	created, err := svc.CreatePurchaseRequest(ctx, validReq, validLines)
	if err != nil {
		t.Fatalf("failed to create purchase request: %v", err)
	}

	if created.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
	if created.RequestNumber == "" {
		t.Errorf("expected generated RequestNumber")
	}
	if created.Status != PurchaseRequestPending {
		t.Errorf("expected pending status, got %s", created.Status)
	}
	if created.TotalItems != 2 {
		t.Errorf("expected 2 items, got %d", created.TotalItems)
	}

	// 4. Retrieve by ID and number
	retrieved, err := svc.GetPurchaseRequest(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPurchaseRequest failed: %v", err)
	}
	if len(retrieved.Lines) != 2 {
		t.Fatalf("expected 2 lines in retrieved request, got %d", len(retrieved.Lines))
	}

	byNum, err := svc.GetPurchaseRequestByNumber(ctx, created.RequestNumber)
	if err != nil || byNum.ID != created.ID {
		t.Errorf("GetPurchaseRequestByNumber failed: %v", err)
	}

	// 5. Vendor view and response (T6)
	vendorReqs, err := svc.ListVendorPurchaseRequests(ctx, vendorOrgID, "pending", 10, 0)
	if err != nil || len(vendorReqs) != 1 {
		t.Fatalf("ListVendorPurchaseRequests failed: %v", err)
	}

	responderID := int64(99)
	err = svc.RespondPurchaseRequest(ctx, created.ID, PurchaseRequestApproved, "Approved with special discount", &responderID)
	if err != nil {
		t.Fatalf("RespondPurchaseRequest failed: %v", err)
	}

	// Update line item offer
	lineID := retrieved.Lines[0].ID
	err = svc.UpdatePurchaseRequestLineOffer(ctx, lineID, money.FromMajor(80), 20.0, "approved")
	if err != nil {
		t.Fatalf("UpdatePurchaseRequestLineOffer failed: %v", err)
	}

	// 6. Customer status counts
	counts, err := svc.CountCustomerPurchaseRequests(ctx, 100, &custOrgID)
	if err != nil {
		t.Fatalf("CountCustomerPurchaseRequests failed: %v", err)
	}
	if counts["approved"] != 1 {
		t.Errorf("expected 1 approved request count, got %d", counts["approved"])
	}

	// 7. Cross-tenant isolation (T3)
	otherVendorReqs, err := svc.ListVendorPurchaseRequests(ctx, 999, "all", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error listing other vendor reqs: %v", err)
	}
	if len(otherVendorReqs) != 0 {
		t.Errorf("expected 0 requests for unassociated vendor, got %d", len(otherVendorReqs))
	}
}
