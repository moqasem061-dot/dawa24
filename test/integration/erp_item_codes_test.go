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

// TestERPItemCodes_FinishIdempotencyUpserts proves the purchase finish writes
// the buyer's item code for the variant, and that a later purchase with a new
// code for the same variant replaces it (one row per organization and variant).
func TestERPItemCodes_FinishIdempotencyUpserts(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		t.Fatal("database required")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	org := time.Now().UnixNano()/1000 + 7
	var variantID, integrationID int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Item code buyer"}')`, org); err != nil {
			return err
		}
		var product int64
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.products (organization_id, name) VALUES ($1, '{"en":"Item code product"}') RETURNING id`, org).Scan(&product); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, price, status) VALUES ($1, $2, '{"en":"Item code variant"}', $3, 10.00, 'active') RETURNING id`, org, product, fmt.Sprintf("ICU-%d", org)).Scan(&variantID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id, provider) VALUES ($1, 'orgasoft') RETURNING id`, org).Scan(&integrationID)
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id = $1`, org)
			return err
		})
	})
	repo := erppg.New(db)
	read := func() string {
		var code string
		err := db.InReadTx(database.WithTenant(ctx, org), func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT external_code FROM erp.item_codes WHERE organization_id = $1 AND variant_id = $2`, org, variantID).Scan(&code)
		})
		if err != nil {
			t.Fatalf("read item code: %v", err)
		}
		return code
	}
	for _, code := range []string{"100001", "ORGA-7"} {
		if err := repo.FinishIdempotency(ctx, org, integrationID, "REF-"+code, 0, "", []byte(`{}`), erp.AuditRecord{}, []erp.ItemCode{{VariantID: variantID, Code: code}}); err != nil {
			t.Fatalf("finish with code %s: %v", code, err)
		}
		if got := read(); got != code {
			t.Fatalf("item code = %q, want %q", got, code)
		}
	}
}
