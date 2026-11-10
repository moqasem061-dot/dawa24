package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestTenantIsolation_ERPActivityReads proves the activity screen's reads return
// nothing for another tenant, and that migration 261's columns are written by
// the call recorder and read back for the owner.
func TestTenantIsolation_ERPActivityReads(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("isolation test requires a database")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	orgA := time.Now().UnixNano() / 1000
	orgB := orgA + 1
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Activity A"}'), ($2, '{"en":"Activity B"}')`, orgA, orgB)
		return err
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id IN ($1, $2)`, orgA, orgB)
			return err
		})
	})
	repo := erppg.New(db)
	now := time.Now()
	call := erp.CallRecord{OrganizationID: &orgA, KeyPrefix: "actvty01", RequestID: "0123456789abcdef", Method: "POST", Route: "/products/sync",
		Status: 422, ErrorCode: "ERP_VALIDATION", DurationMS: 3, BytesOut: 12, Query: "state=new", UserAgent: "test-agent",
		Ref: "4821", AuthReason: "", ErrorDetail: []byte(`{"fields":["name"]}`), CreatedAt: now}
	if err := repo.InsertCallsAsSystem(ctx, []erp.CallRecord{call}); err != nil {
		t.Fatalf("InsertCallsAsSystem: %v", err)
	}
	rows, err := repo.ListCalls(ctx, orgA, erp.CallFilter{Limit: erp.CallsDefaultLimit, Result: "error"})
	if err != nil {
		t.Fatalf("ListCalls owner: %v", err)
	}
	if len(rows) != 1 || rows[0].Ref != "4821" || rows[0].BytesOut != 12 || rows[0].ErrorDetail == "" {
		t.Fatalf("owner rows = %+v, want the recorded call with its ref, bytes and detail", rows)
	}
	other, err := repo.ListCalls(ctx, orgB, erp.CallFilter{Limit: erp.CallsDefaultLimit, Result: "all"})
	if err != nil {
		t.Fatalf("ListCalls other: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("other organization sees %d calls, want 0", len(other))
	}
	batches, err := repo.ListBatches(ctx, orgB, 0, erp.BatchesDefaultLimit)
	if err != nil {
		t.Fatalf("ListBatches other: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("other organization sees %d batches, want 0", len(batches))
	}
	counts, err := repo.ActivityCounts(ctx, orgB)
	if err != nil {
		t.Fatalf("ActivityCounts other: %v", err)
	}
	if counts.Calls24h != 0 || counts.IntegrationID != 0 {
		t.Fatalf("other organization counts = %+v, want nothing", counts)
	}
}
