package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func TestTenantIsolation_ERPIntegrations(t *testing.T) { testERPIsolation(t, "integrations") }
func TestTenantIsolation_ERPApiKeys(t *testing.T)      { testERPIsolation(t, "api_keys") }
func TestTenantIsolation_ERPCalls(t *testing.T)        { testERPIsolation(t, "api_calls") }

func testERPIsolation(t *testing.T, table string) {
	t.Helper()
	db := getTestDB(t)
	if db == nil {
		t.Fatal("isolation test requires a database")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	orgA := time.Now().UnixNano() / 1000
	orgB := orgA + 1
	var id int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"ERP A"}'), ($2, '{"en":"ERP B"}')`, orgA, orgB); err != nil {
			return err
		}
		var integrationID int64
		if err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id, provider) VALUES ($1, 'orgasoft') RETURNING id`, orgA).Scan(&integrationID); err != nil {
			return err
		}
		if table == "integrations" {
			id = integrationID
			return nil
		}
		if table == "api_calls" {
			return tx.QueryRow(ctx, `INSERT INTO erp.api_calls (organization_id, request_id, method, route, status, duration_ms) VALUES ($1, 'test', 'GET', '/erp/v1/health', 200, 1) RETURNING id`, orgA).Scan(&id)
		}
		var branchID, warehouseID int64
		if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id, name) VALUES ($1, '{"en":"ERP branch"}') RETURNING id`, orgA).Scan(&branchID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO inventory.warehouses (organization_id, name) VALUES ($1, 'ERP warehouse') RETURNING id`, orgA).Scan(&warehouseID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO erp.api_keys (organization_id, integration_id, name, key_prefix, key_hash, scopes, warehouse_id, branch_id) VALUES ($1, $2, 'test', $3, $4, ARRAY['erp.orders.read'], $5, $6) RETURNING id`, orgA, integrationID, fmt.Sprintf("%08x", uint32(orgA)), "0000000000000000000000000000000000000000000000000000000000000000", warehouseID, branchID).Scan(&id)
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
	for _, tc := range []struct {
		org  int64
		want int
	}{{orgA, 1}, {orgB, 0}} {
		err := db.InReadTx(database.WithTenant(ctx, tc.org), func(ctx context.Context, tx pgx.Tx) error {
			var count int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM erp."+table+" WHERE id = $1", id).Scan(&count); err != nil {
				return err
			}
			if count != tc.want {
				return fmt.Errorf("organization %d sees %d rows, want %d", tc.org, count, tc.want)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
