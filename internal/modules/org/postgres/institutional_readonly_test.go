package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// TestInstitutionalReadPaths pins the institutional reads that decide whether a
// pharmacy can see a supplier's products at all.
//
// They used to open with CREATE TABLE IF NOT EXISTS, which is fine in a write
// transaction and fatal in a read one: every call failed with "cannot execute
// CREATE TABLE in a read-only transaction". The error travelled up through
// availabilityProbe.VendorInstitutionalConnection into
// commerce.CheckAvailability, which fails closed — so every supplier offer was
// hidden from every pharmacy, whatever the institutional works said. A test
// that only asserted the returned ids would have passed on a write path; this
// one exists to prove the reads survive their own transaction mode.
func TestInstitutionalReadPaths(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	repo := postgres.NewRepository(db)
	ctx := context.Background()
	sysCtx := database.AsSystem(ctx)

	warehouse := &org.InstitutionalWork{
		Title:       i18n.Text{"ar": "مخزن اختبار", "en": "Test Warehouse"},
		Description: i18n.Text{"ar": "", "en": ""},
		Slug:        "test-inst-warehouse",
	}
	unrelated := &org.InstitutionalWork{
		Title:       i18n.Text{"ar": "نشاط غير متصل", "en": "Test Unconnected"},
		Description: i18n.Text{"ar": "", "en": ""},
		Slug:        "test-inst-unconnected",
	}
	for _, w := range []*org.InstitutionalWork{warehouse, unrelated} {
		if err := repo.CreateInstitutionalWork(ctx, w); err != nil {
			t.Fatalf("create institutional work %q: %v", w.Slug, err)
		}
	}
	// The pharmacy may connect to the warehouse and to nothing else — the exact
	// shape of the live data this rule is meant to enforce.
	pharmacy := &org.InstitutionalWork{
		Title:              i18n.Text{"ar": "صيدلية اختبار", "en": "Test Pharmacy"},
		Description:        i18n.Text{"ar": "", "en": ""},
		Slug:               "test-inst-pharmacy",
		AllowedConnections: []int64{warehouse.ID},
	}
	if err := repo.CreateInstitutionalWork(ctx, pharmacy); err != nil {
		t.Fatalf("create institutional work %q: %v", pharmacy.Slug, err)
	}
	// defer, not t.Cleanup: cleanups run after the test function returns, by
	// which time the deferred db.Close above has already shut the pool and the
	// deletes would silently do nothing.
	defer func() {
		_ = db.InTx(sysCtx, func(txCtx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(txCtx,
				`DELETE FROM org.institutional_works WHERE slug LIKE 'test-inst-%';`)
			return err
		})
	}()

	// 1. Reading one work back must not need a write transaction.
	got, err := repo.GetInstitutionalWorkByID(ctx, pharmacy.ID)
	if err != nil {
		t.Fatalf("GetInstitutionalWorkByID: %v", err)
	}
	if got == nil || got.ID != pharmacy.ID {
		t.Fatalf("GetInstitutionalWorkByID returned %+v, want id %d", got, pharmacy.ID)
	}

	// 2. The connection lookup behind product visibility.
	allowed, err := repo.GetConnectedInstitutionalWorkIDs(ctx, []int64{pharmacy.ID})
	if err != nil {
		t.Fatalf("GetConnectedInstitutionalWorkIDs: %v", err)
	}
	if !containsID(allowed, warehouse.ID) {
		t.Fatalf("allowed ids %v do not include the connected work %d", allowed, warehouse.ID)
	}
	if containsID(allowed, unrelated.ID) {
		t.Fatalf("allowed ids %v must not include the unconnected work %d", allowed, unrelated.ID)
	}

	if ok, err := repo.CanConnectInstitutionalWorks(ctx, pharmacy.ID, warehouse.ID); err != nil || !ok {
		t.Fatalf("CanConnectInstitutionalWorks(connected) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := repo.CanConnectInstitutionalWorks(ctx, pharmacy.ID, unrelated.ID); err != nil || ok {
		t.Fatalf("CanConnectInstitutionalWorks(unconnected) = %v, %v; want false, nil", ok, err)
	}

	// 3. The branch lookup both sides of the availability rule depend on.
	resetFixtures(t, db)
	o := &org.Organization{
		LegalName:          "Test Org Institutional",
		CommercialRegister: "CR-INST-01",
		Type:               org.TypeVendor,
		Status:             org.StatusApproved,
	}
	if err := repo.CreateOrganization(ctx, o); err != nil {
		t.Fatalf("create org: %v", err)
	}
	b := &org.Branch{
		OrganizationID: o.ID,
		Name:           i18n.Text{"ar": "فرع اختبار", "en": "Test Branch"},
		Status:         "active",
	}
	if err := repo.CreateBranch(database.WithTenant(ctx, o.ID), b); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if err := repo.AssignBranchInstitutionalWorks(ctx, b.ID, []int64{warehouse.ID}); err != nil {
		t.Fatalf("assign branch works: %v", err)
	}

	works, err := repo.GetBranchInstitutionalWorks(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetBranchInstitutionalWorks: %v", err)
	}
	if len(works) != 1 || works[0].ID != warehouse.ID {
		t.Fatalf("branch works = %+v, want exactly work %d", works, warehouse.ID)
	}
}

func containsID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
