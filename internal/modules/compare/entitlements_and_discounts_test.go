package compare_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CountMarketDiscounts counts what ListMarketDiscounts would return.
//
// Deliberately implemented THROUGH the listing rather than beside it: in
// production these are two statements built from one clause builder, and a mock
// that counted by its own rules would let a divergence between them pass.
func (m *mockCompareRepo) CountMarketDiscounts(ctx context.Context, filter compare.MarketDiscountsFilter) (int64, error) {
	all := filter
	all.Page = 1
	all.Limit = 96
	res, err := m.ListMarketDiscounts(ctx, all)
	if err != nil {
		return 0, err
	}
	return res.TotalCount, nil
}

func (m *mockCompareRepo) ListMarketDiscounts(ctx context.Context, filter compare.MarketDiscountsFilter) (*compare.MarketDiscountsResult, error) {
	var allItems []*compare.MarketDiscountRow
	cleanQ := strings.ToLower(strings.TrimSpace(filter.Query))
	supplierFilter := strings.ToLower(strings.TrimSpace(filter.Supplier))

	for fileID, rows := range m.fileRows {
		file, ok := m.files[fileID]
		if !ok || file.DeletedAt != nil || file.Status != compare.FileReady {
			continue
		}
		// NOTE: this mock no longer mirrors production. /market-discounts is
		// served from catalog.product_variants joined to inventory.stocks, not
		// from compare files at all — see postgres/market_discounts.go, whose
		// invariants are asserted in market_discounts_query_test.go. What is
		// kept here is only enough shape for the service-level tests below.
		if !file.IsTempWarehouse {
			continue
		}
		if supplierFilter != "" && strings.ToLower(file.SupplierName) != supplierFilter {
			continue
		}
		for _, r := range rows {
			if cleanQ != "" && !strings.Contains(strings.ToLower(r.RawName), cleanQ) && !strings.Contains(strings.ToLower(r.NormalizedName), cleanQ) && !strings.Contains(strings.ToLower(r.SKU), cleanQ) && !strings.Contains(strings.ToLower(file.SupplierName), cleanQ) {
				continue
			}

			netPrice := r.PriceAfterDiscount
			if netPrice.IsZero() && r.Price.IsPositive() {
				netPrice = compare.CalculatePriceAfterDiscount(r.Price, r.Discount)
			}

			if filter.MinPrice != nil && float64(netPrice.Minor())/100.0 < *filter.MinPrice {
				continue
			}
			if filter.MaxPrice != nil && float64(netPrice.Minor())/100.0 > *filter.MaxPrice {
				continue
			}
			if filter.MinDiscount != nil && r.Discount < *filter.MinDiscount {
				continue
			}
			if filter.MaxDiscount != nil && r.Discount > *filter.MaxDiscount {
				continue
			}

			var discVal money.Amount
			if r.Price.Minor() > netPrice.Minor() {
				discVal = money.FromMinor(r.Price.Minor() - netPrice.Minor())
			}

			allItems = append(allItems, &compare.MarketDiscountRow{
				ID:                 r.ID,
				FileID:             file.ID,
				SupplierName:       file.SupplierName,
				ProductName:        r.RawName,
				OriginalPrice:      r.Price,
				DiscountPercent:    r.Discount,
				DiscountValue:      discVal,
				PriceAfterDiscount: netPrice,
				MatchedProductID:   r.MatchedProductID,
				InCatalog:          r.MatchedProductID != nil && *r.MatchedProductID > 0,
				UploadedAt:         file.CreatedAt,
			})
		}
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 24
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}

	total := int64(len(allItems))
	totalPages := 1
	if total > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}

	start := (page - 1) * limit
	end := start + limit
	if start > len(allItems) {
		start = len(allItems)
	}
	if end > len(allItems) {
		end = len(allItems)
	}

	pagedItems := allItems[start:end]
	suppliers, _ := m.ListDistinctSuppliers(ctx)

	return &compare.MarketDiscountsResult{
		Items:              pagedItems,
		TotalCount:         total,
		AvailableSuppliers: suppliers,
		Page:               page,
		Limit:              limit,
		TotalPages:         totalPages,
		HasPrev:            page > 1,
		HasNext:            page < totalPages,
	}, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestEntitlementResolution(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	// Free unlimited platform feature access
	ent, err := svc.EntitlementFor(ctx, 101, 201)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ent.Active {
		t.Errorf("expected active entitlement for all users")
	}
	if ent.MaxActiveFiles < 10 {
		t.Errorf("expected high active file limit, got %d", ent.MaxActiveFiles)
	}
	if !ent.AIMatchingEnabled {
		t.Errorf("expected AIMatchingEnabled = true")
	}
}

func TestSessionCapEvictionParity(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	userID := int64(301)
	maxSessions := 2

	// Log in first 2 sessions
	_ = svc.EnforceSessionCap(ctx, userID, nil, maxSessions, compare.ClientInfo{SessionID: "sess-1", DeviceName: "Chrome on Windows"})
	_ = svc.EnforceSessionCap(ctx, userID, nil, maxSessions, compare.ClientInfo{SessionID: "sess-2", DeviceName: "Safari on iPhone"})

	count, _ := repo.CountActiveUserSessions(ctx, userID)
	if count != 2 {
		t.Fatalf("expected 2 active sessions, got %d", count)
	}

	// Log in 3rd session -> should evict the oldest session (sess-1)
	_ = svc.EnforceSessionCap(ctx, userID, nil, maxSessions, compare.ClientInfo{SessionID: "sess-3", DeviceName: "Firefox on Mac"})

	count, _ = repo.CountActiveUserSessions(ctx, userID)
	if count != 2 {
		t.Errorf("expected session cap of 2 maintained after eviction, got %d", count)
	}

	activeList, _ := repo.ListActiveUserSessions(ctx, userID)
	if len(activeList) != 2 {
		t.Fatalf("expected 2 active sessions in list")
	}
	if activeList[0].SessionID == "sess-1" {
		t.Errorf("expected sess-1 to be evicted, but it was still active")
	}
}

func TestPlanRequestReviewFlow(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	req, err := svc.RequestPlan(ctx, 2, 401, 501, "Please approve our vendor pro comparison plan")
	if err != nil {
		t.Fatalf("failed to request plan: %v", err)
	}
	if req.Status != compare.RequestPending {
		t.Errorf("expected pending status, got %s", req.Status)
	}

	// Admin approves
	err = svc.ReviewPlanRequest(ctx, req.ID, true, 999, "Approved by admin")
	if err != nil {
		t.Fatalf("failed to approve plan request: %v", err)
	}

	// User should now have active entitlement for compare
	ent, err := svc.EntitlementFor(ctx, 501, 401)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ent.Active {
		t.Errorf("expected active entitlement after plan approval")
	}
}

func TestPlanValidation(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	// Missing slug
	_, err := svc.CreatePlan(ctx, &compare.Plan{
		Name: i18n.Text{"ar": "خطة جديدة"},
	})
	if err == nil {
		t.Errorf("expected error creating plan with missing slug")
	}

	// Missing name
	_, err = svc.CreatePlan(ctx, &compare.Plan{
		Slug: "valid-slug",
	})
	if err == nil {
		t.Errorf("expected error creating plan with missing name")
	}

	// Valid creation
	created, err := svc.CreatePlan(ctx, &compare.Plan{
		Slug:         "custom-plan",
		Name:         i18n.Text{"ar": "خطة مخصصة", "en": "Custom Plan"},
		PriceMonthly: money.FromMajor(500),
	})
	if err != nil {
		t.Fatalf("failed to create valid plan: %v", err)
	}
	if created.ID <= 0 {
		t.Errorf("expected valid ID assigned to created plan")
	}
}

func TestArchiveRetentionPolicyAndQuota(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	userID := int64(701)
	orgID := int64(801)

	// Direct upload succeeds with free platform entitlement
	f1, _, err := svc.UploadCompareFile(ctx, userID, &orgID, "Sup-1", "file1.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", 1024, "key1")
	if err != nil {
		t.Fatalf("expected upload to succeed: %v", err)
	}
	if f1 == nil || f1.ID <= 0 {
		t.Errorf("expected valid created file")
	}

	// Rename file
	err = svc.RenameFile(ctx, f1.ID, "Supplier I (Cairo Branch)")
	if err != nil {
		t.Fatalf("failed to rename file: %v", err)
	}
	f, _ := svc.GetFile(ctx, f1.ID)
	if f.SupplierName != "Supplier I (Cairo Branch)" {
		t.Errorf("expected renamed supplier label, got %s", f.SupplierName)
	}

	// Manual archive and unarchive
	err = svc.ArchiveFile(ctx, f1.ID, "Seasonal pause")
	if err != nil {
		t.Fatalf("failed to manually archive file: %v", err)
	}
	f, _ = svc.GetFile(ctx, f1.ID)
	if f.Status != compare.FileArchived {
		t.Errorf("expected status archived")
	}

	err = svc.UnarchiveFile(ctx, f1.ID)
	if err != nil {
		t.Fatalf("failed to unarchive file: %v", err)
	}
	f, _ = svc.GetFile(ctx, f1.ID)
	if f.Status != compare.FileReady && f.Status != compare.FileUploaded {
		t.Errorf("expected status active after unarchive")
	}
}

func TestUploadAndProcessCompareFile_CSV(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	userID := int64(701)
	orgID := int64(801)

	csvContent := `كود الصنف,اسم الصنف الدوائي,السعر الرسمي,نسبة الخصم,ملاحظات
1001,بانادول اكسترا 24 قرص,45.00,18.5,متوفر
1002,اوجمنتين 1 جم 14 قرص,135.00,12.0,تسليم فوري
1003,كونجستال 20 قرص,31.00,20.0,عرض خاص`

	file, _, err := svc.UploadAndProcessCompareFile(
		ctx, userID, &orgID, "مورد النور", "test_prices.csv", "text/csv",
		int64(len(csvContent)), "/uploads/compare/test.csv", []byte(csvContent),
	)
	if err != nil {
		t.Fatalf("UploadAndProcessCompareFile failed: %v", err)
	}
	if file.RowCount != 3 {
		t.Errorf("expected 3 rows extracted, got %d", file.RowCount)
	}
	if file.Status != compare.FileReady {
		t.Errorf("expected status ready, got %s", file.Status)
	}

	// Verify rows stored in repository
	rows, err := repo.ListFileRows(ctx, file.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListFileRows failed: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows in repo, got %d", len(rows))
	}
	if rows[0].RawName != "بانادول اكسترا 24 قرص" {
		t.Errorf("unexpected first row product name: %s", rows[0].RawName)
	}
	if rows[0].Discount != 18.5 {
		t.Errorf("expected discount 18.5, got %f", rows[0].Discount)
	}
}
