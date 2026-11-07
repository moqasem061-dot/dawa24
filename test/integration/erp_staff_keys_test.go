package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// TestERPKeysStaffListShowsOrgNameAndProvider proves the real staff query:
// every row must carry the owning organization's actual name and the key's
// integration provider, so a staff member can attribute a key before
// revoking it. The in-memory fakes used by unit tests cannot prove this.
func TestERPKeysStaffListShowsOrgNameAndProvider(t *testing.T) {
	db := connectTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	defer db.Close()
	ctx := context.Background()
	base := time.Now().UnixNano() / 1000
	type orgSpec struct {
		id       int64
		name     string
		provider string
		prefix   string
	}
	orgs := []orgSpec{
		{id: base, name: fmt.Sprintf(`{"ar":"صيدلية %d","en":"Pharmacy %d"}`, base%1000, base%1000), provider: "testprovider", prefix: fmt.Sprintf("sp%06x", uint32(base)&0xffffff)},
		{id: base + 1, name: fmt.Sprintf(`{"ar":"مورد %d","en":"Supplier %d"}`, base%1000, base%1000), provider: "testprovider2", prefix: fmt.Sprintf("sp%06x", (uint32(base)+1)&0xffffff)},
	}
	var keyIDs [2]int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		for i, org := range orgs {
			if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, $2::jsonb)`, org.id, org.name); err != nil {
				return err
			}
			var branch, warehouse, integration int64
			if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id, name) VALUES ($1, '{"en":"Staff branch"}') RETURNING id`, org.id).Scan(&branch); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO inventory.warehouses (organization_id, name) VALUES ($1, 'Staff warehouse') RETURNING id`, org.id).Scan(&warehouse); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id, provider) VALUES ($1, $2) RETURNING id`, org.id, org.provider).Scan(&integration); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO erp.api_keys (organization_id, integration_id, name, key_prefix, key_hash, scopes, warehouse_id, branch_id)
				VALUES ($1, $2, 'staff-list', $3, '0000000000000000000000000000000000000000000000000000000000000000', ARRAY['erp.orders.read'], $4, $5) RETURNING id`,
				org.id, integration, org.prefix, warehouse, branch).Scan(&keyIDs[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id IN ($1, $2)`, orgs[0].id, orgs[1].id)
			return err
		})
	})
	repo := erppg.New(db)
	rows, err := repo.ListKeysAsStaff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[int64]erp.StaffKeyRow{}
	for _, row := range rows {
		byKey[row.Key.ID] = row
	}
	for i, org := range orgs {
		row, ok := byKey[keyIDs[i]]
		if !ok {
			t.Fatalf("staff list is missing key %d for organization %d", keyIDs[i], org.id)
		}
		if row.OrganizationName["en"] == "" || row.OrganizationName["ar"] == "" {
			t.Errorf("key %d: organization name is empty (%+v); staff cannot attribute the key", row.Key.ID, row.OrganizationName)
		}
		if got := row.OrganizationName["en"]; got != fmt.Sprintf("Pharmacy %d", base%1000) && got != fmt.Sprintf("Supplier %d", base%1000) {
			t.Errorf("key %d: unexpected organization name %q", row.Key.ID, got)
		}
		if row.Integration.Provider != org.provider {
			t.Errorf("key %d: provider %q, want %q", row.Key.ID, row.Integration.Provider, org.provider)
		}
		if row.Integration.Status != "active" || row.Integration.OrganizationID != org.id {
			t.Errorf("key %d: integration not joined correctly (%+v)", row.Key.ID, row.Integration)
		}
	}
}
