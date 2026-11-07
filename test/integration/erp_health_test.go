package integration_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func TestERPHealthDataTenantIsolation(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	base := time.Now().UnixNano() / 1000
	orgs := []int64{base, base + 1}
	var ids [2]int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		for i, org := range orgs {
			name := fmt.Sprintf(`{"ar":"منشأة %d","en":"Organization %d"}`, i, i)
			if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id,name) VALUES ($1,$2::jsonb)`, org, name); err != nil {
				return err
			}
			var branch, warehouse, integration int64
			if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id,name) VALUES ($1,'{"en":"Branch"}') RETURNING id`, org).Scan(&branch); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO inventory.warehouses (organization_id,name) VALUES ($1,$2) RETURNING id`, org, fmt.Sprintf("Warehouse %d", i)).Scan(&warehouse); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id,provider) VALUES ($1,$2) RETURNING id`, org, provider.DefaultProvider).Scan(&integration); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO erp.api_keys (organization_id,integration_id,name,key_prefix,key_hash,scopes,warehouse_id,branch_id) VALUES ($1,$2,'health',$3,$4,ARRAY['erp.orders.read'],$5,$6) RETURNING id`, org, integration, fmt.Sprintf("%08x", uint32(base)+uint32(i)), "0000000000000000000000000000000000000000000000000000000000000000", warehouse, branch).Scan(&ids[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id IN ($1,$2)`, orgs[0], orgs[1])
			return err
		}); err != nil {
			t.Error(err)
		}
	})
	repo := erppg.New(db)
	for i, org := range orgs {
		data, err := repo.HealthData(ctx, erp.Principal{OrganizationID: org, KeyID: ids[i]})
		if err != nil {
			t.Fatal(err)
		}
		if data.OrganizationName["en"] != fmt.Sprintf("Organization %d", i) || data.OrganizationName["ar"] != fmt.Sprintf("منشأة %d", i) || data.WarehouseName != fmt.Sprintf("Warehouse %d", i) || data.IntegrationStatus != "active" {
			t.Errorf("tenant %d: %+v", i, data)
		}
	}
	_, err = repo.HealthData(ctx, erp.Principal{OrganizationID: orgs[1], KeyID: ids[0]})
	if !errors.Is(err, erp.ErrNotFound) {
		t.Errorf("foreign key health: %v", err)
	}
}
