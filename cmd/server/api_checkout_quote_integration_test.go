package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	catalogpg "github.com/moqasem061-dot/dawa24/internal/modules/catalog/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	orgpg "github.com/moqasem061-dot/dawa24/internal/modules/org/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	promopg "github.com/moqasem061-dot/dawa24/internal/modules/promo/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/workflow"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/timeutil"
)

func TestAPIQuoteResolverCatalogPrices(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{URL: dsn, MaxConns: 5, MinConns: 1, MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute, StatementTimeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var buyerID, vendorID, branchID, vendorBranchID, productID, variantID, offerID, offerLineID, buyerWorkID, vendorWorkID int64
	stamp := time.Now().UnixNano()
	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `INSERT INTO org.organizations (name,type,status) VALUES ('{"ar":"صيدلية","en":"Pharmacy"}','customer','approved') RETURNING id`).Scan(&buyerID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `INSERT INTO org.organizations (name,type,status) VALUES ('{"ar":"مورد","en":"Supplier"}','vendor','approved') RETURNING id`).Scan(&vendorID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `INSERT INTO org.branches (organization_id,name,code,latitude,longitude) VALUES ($1,'{"ar":"فرع","en":"Branch"}',$2,30.04,31.23) RETURNING id`, buyerID, fmt.Sprintf("Q-%d", stamp)).Scan(&branchID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `INSERT INTO org.branches (organization_id,name,code,latitude,longitude) VALUES ($1,'{"ar":"مورد","en":"Vendor"}',$2,30.04,31.23) RETURNING id`, vendorID, fmt.Sprintf("V-%d", stamp)).Scan(&vendorBranchID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `INSERT INTO org.institutional_works (title) VALUES ('{"ar":"صيدلية","en":"Buyer"}') RETURNING id`).Scan(&buyerWorkID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `INSERT INTO org.institutional_works (title) VALUES ('{"ar":"مورد","en":"Vendor"}') RETURNING id`).Scan(&vendorWorkID); err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO org.institutional_work_connections (from_institutional_work_id,to_institutional_work_id) VALUES ($1,$2)`, buyerWorkID, vendorWorkID); err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO org.branch_institutional_works (branch_id,work_category,institutional_work_id) VALUES ($1,'pharmacy',$2),($3,'retail',$4)`, branchID, buyerWorkID, vendorBranchID, vendorWorkID); err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO workflow.weekly_coverages (organization_id,branch_id,day_of_week,latitude,longitude,distance_meters,is_active) VALUES ($1,$2,$3,30.04,31.23,20000,true)`, vendorID, vendorBranchID, int(timeutil.Now().Weekday())); err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `INSERT INTO org.delivery_bands (organization_id,from_meters,to_meters,fee,is_active) VALUES ($1,0,10000,12.00,true)`, vendorID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `INSERT INTO catalog.products (organization_id,name,dosage_form) VALUES ($1,'{"ar":"دواء","en":"Medicine"}','tablet') RETURNING id`, vendorID).Scan(&productID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `INSERT INTO catalog.product_variants (organization_id,product_id,branch_id,name,sku,price,discount,status) VALUES ($1,$2,$3,'{"ar":"عبوة","en":"Pack"}',$4,100.00,10.00,'active') RETURNING id`, vendorID, productID, vendorBranchID, fmt.Sprintf("QUOTE-%d", stamp)).Scan(&variantID); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `INSERT INTO promo.offers (organization_id,branch_id,title,discount_type,discount_value,min_order_amount,admin_status,is_active,starts_at,expires_at) VALUES ($1,$2,'{"ar":"عرض","en":"Offer"}','percentage',20.00,0,'approved',true,now()-interval '1 day',now()+interval '1 day') RETURNING id`, vendorID, vendorBranchID).Scan(&offerID); err != nil {
			return err
		}
		return tx.QueryRow(txCtx, `INSERT INTO promo.offer_products (offer_id,product_id,product_variant_id,variant_id) VALUES ($1,$2,$3,$3) RETURNING id`, offerID, productID, variantID).Scan(&offerLineID)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			for _, query := range []struct {
				sql string
				id  int64
			}{
				{`DELETE FROM promo.offers WHERE id=$1`, offerID},
				{`DELETE FROM workflow.weekly_coverages WHERE organization_id=$1`, vendorID},
				{`DELETE FROM catalog.product_variants WHERE id=$1`, variantID},
				{`DELETE FROM catalog.products WHERE id=$1`, productID},
				{`DELETE FROM org.branches WHERE id=$1`, branchID},
				{`DELETE FROM org.branches WHERE id=$1`, vendorBranchID},
				{`DELETE FROM org.institutional_works WHERE id=$1`, buyerWorkID},
				{`DELETE FROM org.institutional_works WHERE id=$1`, vendorWorkID},
				{`DELETE FROM org.organizations WHERE id=$1`, vendorID},
				{`DELETE FROM org.organizations WHERE id=$1`, buyerID},
			} {
				if _, err := tx.Exec(txCtx, query.sql, query.id); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Errorf("remove quote fixtures: %v", err)
		}
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver := apiQuoteResolver{catalog: catalog.NewService(catalogpg.NewRepository(db), logger), org: org.NewService(orgpg.NewRepository(db), logger), promo: promo.NewService(promopg.NewRepository(db), logger), coverage: workflow.NewCoverageService(db)}
	resolver.paymentMethod, resolver.addresses = checkoutEnabledMethods, buyerAddresses
	input := commerce.CheckoutInput{CustomerOrgID: buyerID, BranchID: &branchID, Items: []commerce.CheckoutLineItem{{ProductVariantID: &variantID, VendorOrgID: vendorID, Quantity: 2, UnitPrice: money.MustParse("1.00"), DiscountAmount: money.MustParse("999.00")}}}
	quote, err := resolver.quote(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if quote.Items[0].UnitPrice != money.MustParse("90.00") || quote.Items[0].DiscountAmount != money.MustParse("20.00") || quote.Items[0].VendorOrgID != vendorID || quote.Items[0].ProductID == nil || *quote.Items[0].ProductID != productID {
		t.Fatalf("unexpected quote: %+v", quote.Items[0])
	}
	if quote.ShippingFee != money.MustParse("12.00") || quote.VendorShippingFees[vendorID] != money.MustParse("12.00") {
		t.Fatalf("incorrect delivery: %s %+v", quote.ShippingFee, quote.VendorShippingFees)
	}
	input.OfferID = offerID
	input.Items[0].OfferProductID = &offerLineID
	quote, err = resolver.quote(ctx, input)
	if err != nil {
		t.Fatalf("offer quote: %v", err)
	}
	if quote.Items[0].UnitPrice != money.MustParse("80.00") || quote.Items[0].DiscountAmount != money.MustParse("40.00") {
		t.Fatalf("unexpected offer quote: %+v", quote.Items[0])
	}
	input.OfferID = 0
	input.Items[0].OfferProductID = nil
	input.Items[0].VendorOrgID = buyerID
	if _, err = resolver.quote(ctx, input); err == nil {
		t.Fatal("accepted mismatched supplier")
	}
}
