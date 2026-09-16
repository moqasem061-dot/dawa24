package ui_test

import (
	"net/http"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The template an admin downloads must be one the importer reads correctly.
// Before this, the CSV and Excel templates carried different columns and
// neither carried a SKU, so a filled-in template had no identifier to match on
// and every re-upload duplicated the whole catalogue.
func TestImportTemplatesRoundTripThroughTheImporter(t *testing.T) {
	db := testDB(t)
	h := newRealUIHandler(t, db)

	for _, tc := range []struct{ name, path string }{
		{"csv", "/admin/products/sample.csv"},
		{"xlsx", "/admin/products/sample.xlsx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doGET(t, h, tc.path, superAdmin())
			if rec.Code != http.StatusOK {
				t.Fatalf("download status = %d, want 200", rec.Code)
			}

			sheet, err := catalog.ReadSpreadsheet(rec.Body.Bytes(), "sample."+tc.name)
			if err != nil {
				t.Fatalf("the importer cannot read its own template: %v", err)
			}

			res := catalog.ParseProducts(sheet)
			if len(res.Products) != 5 {
				t.Fatalf("parsed %d products from the template, want 5", len(res.Products))
			}
			if res.Stats.RejectedRows != 0 {
				t.Errorf("%d template rows were rejected", res.Stats.RejectedRows)
			}
			if len(res.MissingFields) != 0 {
				t.Errorf("the template is missing columns the importer wants: %v", res.MissingFields)
			}

			// Every binding at full confidence is the point: the template is the
			// vocabulary the mapper scores highest.
			for _, b := range res.Plan.Bindings {
				if b.Score < 100 {
					t.Errorf("template column %q bound to %s with only score %d", b.Header, b.Field, b.Score)
				}
			}

			first := res.Products[0]
			if first.SKU != "CONG-TAB-650" {
				t.Errorf("sku = %q, want CONG-TAB-650", first.SKU)
			}
			if first.Barcode != "6221234567890" {
				t.Errorf("barcode = %q, want 6221234567890", first.Barcode)
			}
			if first.Price.String() != "30.00" {
				t.Errorf("price = %s, want 30.00", first.Price.String())
			}
			if first.OldPrice.String() != "30.00" {
				t.Errorf("public price = %s, want 30.00", first.OldPrice.String())
			}
			if !first.Discount.IsZero() {
				t.Errorf("discount = %s, want 0", first.Discount.String())
			}
			if got := first.Name.Get(i18n.EN); got != "Congestal Tablets" {
				t.Errorf("english name = %q, want Congestal Tablets", got)
			}
		})
	}
}
