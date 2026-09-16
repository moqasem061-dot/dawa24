package org_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestInstitutionalWorkHierarchyAndConnections(t *testing.T) {
	ctx := context.Background()
	repo := newInstMockRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := org.NewService(repo, logger)

	// 1. Create Root Category: Wholesale (جملة جملة)
	wholesale := &org.InstitutionalWork{
		Title:       i18n.New("جملة جملة", "Wholesale - Wholesale"),
		Description: i18n.New("كبار الموردين والمخازن المركزية", "Primary wholesalers"),
		Icon:        "truck",
		PricingType: org.PricingSubscription,
		IsActive:    true,
		ViewType:    1,
	}
	if err := svc.CreateInstitutionalWork(ctx, wholesale); err != nil {
		t.Fatalf("Create wholesale failed: %v", err)
	}

	// 2. Create Sub-entity under Wholesale: Sector (قطاع)
	sector := &org.InstitutionalWork{
		Title:       i18n.New("قطاع", "Sector"),
		Description: i18n.New("قطاعات التوزيع الإقليمية", "Regional sectors"),
		Icon:        "layers",
		PricingType: org.PricingSubscription,
		IsActive:    true,
		ViewType:    1,
		ParentID:    &wholesale.ID,
	}
	if err := svc.CreateInstitutionalWork(ctx, sector); err != nil {
		t.Fatalf("Create sector failed: %v", err)
	}

	// 3. Create Multi-level Child under Sector: Factory (مصنع)
	factory := &org.InstitutionalWork{
		Title:              i18n.New("مصنع", "Factory"),
		Description:        i18n.New("مصانع الأدوية المعتمدة", "Pharmaceutical manufacturing plants"),
		Icon:               "package",
		PricingType:        org.PricingPaid,
		IsActive:           true,
		ViewType:           1,
		ParentID:           &sector.ID,
		AllowedConnections: []int64{wholesale.ID},
	}
	if err := svc.CreateInstitutionalWork(ctx, factory); err != nil {
		t.Fatalf("Create factory failed: %v", err)
	}

	// 4. Verify Connection: Factory can connect to Wholesale
	canConnect, err := svc.CanConnectInstitutionalWorks(ctx, factory.ID, wholesale.ID)
	if err != nil {
		t.Fatalf("CanConnectInstitutionalWorks check failed: %v", err)
	}
	if !canConnect {
		t.Errorf("expected Factory to connect to Wholesale, got false")
	}

	// Verify Factory cannot connect to non-connected ID
	canConnectSector, err := svc.CanConnectInstitutionalWorks(ctx, factory.ID, sector.ID)
	if err != nil {
		t.Fatalf("CanConnectInstitutionalWorks check failed: %v", err)
	}
	if canConnectSector {
		t.Errorf("expected Factory not to connect to Sector, got true")
	}

	// 5. Test CanConnectTo domain method
	if !factory.CanConnectTo(wholesale.ID) {
		t.Errorf("factory.CanConnectTo(wholesale.ID) returned false, want true")
	}

	// 6. Test Status Toggle
	if err := svc.ToggleInstitutionalWorkStatus(ctx, factory.ID); err != nil {
		t.Fatalf("Toggle status failed: %v", err)
	}
	if factory.IsActive {
		t.Errorf("expected factory to be inactive after toggle")
	}
}

// T1: AllowedWorkIDs — Simple vs WithConnections, using the exact example from the Laravel doc:
// user works [5,7,3,8], connections 5->7, 5->9, 7->10 => allowed in WithConnections is [7,9,10]
func TestInstitutionalWorksFilterModes(t *testing.T) {
	ctx := context.Background()
	repo := newInstMockRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := org.NewService(repo, logger)

	const userID int64 = 42
	const orgID int64 = 101

	// Assign user works [5, 7, 3, 8]
	for _, wID := range []int64{5, 7, 3, 8} {
		_ = svc.AssignEmployeeInstitutionalWork(ctx, orgID, userID, wID)
	}

	// Setup connections: 5 -> 7, 5 -> 9, 7 -> 10
	repo.connections[5] = map[int64]bool{7: true, 9: true}
	repo.connections[7] = map[int64]bool{10: true}

	// Mode Simple: returns user's direct works [5, 7, 3, 8]
	simpleWorks, err := svc.AllowedWorkIDs(ctx, userID, org.FilterSimple)
	if err != nil {
		t.Fatalf("AllowedWorkIDs Simple failed: %v", err)
	}
	if len(simpleWorks) != 4 {
		t.Errorf("Simple mode: got %v, want 4 works", simpleWorks)
	}

	// Mode WithConnections: returns connected targets [7, 9, 10]
	connWorks, err := svc.AllowedWorkIDs(ctx, userID, org.FilterWithConnections)
	if err != nil {
		t.Fatalf("AllowedWorkIDs WithConnections failed: %v", err)
	}

	has := func(slice []int64, target int64) bool {
		for _, v := range slice {
			if v == target {
				return true
			}
		}
		return false
	}

	if len(connWorks) != 3 || !has(connWorks, 7) || !has(connWorks, 9) || !has(connWorks, 10) {
		t.Errorf("WithConnections mode: got %v, want [7, 9, 10]", connWorks)
	}

	// T2: Assign, remove, list
	if err := svc.RemoveEmployeeInstitutionalWork(ctx, orgID, userID, 3); err != nil {
		t.Fatalf("Remove work failed: %v", err)
	}
	remainingWorks, err := svc.AllowedWorkIDs(ctx, userID, org.FilterSimple)
	if err != nil {
		t.Fatalf("AllowedWorkIDs after remove failed: %v", err)
	}
	if has(remainingWorks, 3) || len(remainingWorks) != 3 {
		t.Errorf("after removing work 3: got %v, want 3 works excluding 3", remainingWorks)
	}
}
