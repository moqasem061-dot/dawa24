package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// storedVariant is a variant as it exists before an edit.
func storedVariant() *catalog.ProductVariant {
	cost := money.FromMajor(70)
	expiry := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	return &catalog.ProductVariant{
		ID:                     7,
		OrganizationID:         100,
		Name:                   i18n.Text{i18n.AR: "أماريل ١ مجم", i18n.EN: "Amaryl 1mg"},
		SKU:                    "SKU-9901",
		Barcode:                "6221234567890",
		Unit:                   "علبة",
		Price:                  money.FromMajor(120),
		Discount:               money.FromMajor(10),
		CostPrice:              &cost,
		CostDiscountPercentage: 12.5,
		MinOrderQty:            3,
		BatchNumber:            "B-77",
		ExpiryDate:             &expiry,
		Status:                 catalog.StatusInactive,
		IsNegotiable:           true,
	}
}

func postForm(values url.Values) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/vendor/variants/7/update",
		strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = r.ParseForm()
	return r
}

// A form that changes one field must not erase the fields it does not mention.
//
// This is the regression test for the bug that emptied the catalogue's codes.
// The edit dialog had no SKU, barcode, English-name or unit input, and the
// handler assigned all four unconditionally from r.FormValue — so every save
// wrote "" over them. The partial unique index on (organization_id, sku)
// excludes the empty string, so nothing ever errored: 915 of 2,107 live
// variants have no SKU and every one of them has no barcode.
func TestVariantEditLeavesUnmentionedFieldsAlone(t *testing.T) {
	v := storedVariant()
	before := *v

	// Exactly what a "change the price" submission carries.
	if err := applyVariantEdit(postForm(url.Values{"price": {"135.00"}}), v, "ar"); err != nil {
		t.Fatalf("applyVariantEdit: %v", err)
	}

	if got := v.Price.String(); got != "135.00" {
		t.Errorf("price = %s, want 135.00", got)
	}
	if v.SKU != before.SKU {
		t.Errorf("SKU was erased: %q -> %q", before.SKU, v.SKU)
	}
	if v.Barcode != before.Barcode {
		t.Errorf("barcode was erased: %q -> %q", before.Barcode, v.Barcode)
	}
	if v.Unit != before.Unit {
		t.Errorf("unit was erased: %q -> %q", before.Unit, v.Unit)
	}
	if v.Name.Get(i18n.EN) != before.Name.Get(i18n.EN) {
		t.Errorf("English name was erased: %q -> %q", before.Name.Get(i18n.EN), v.Name.Get(i18n.EN))
	}
	if v.CostPrice == nil || v.CostPrice.Minor() != before.CostPrice.Minor() {
		t.Error("cost price was cleared by a form that never mentioned it")
	}
	if v.CostDiscountPercentage != before.CostDiscountPercentage {
		t.Errorf("cost discount was cleared: %v -> %v", before.CostDiscountPercentage, v.CostDiscountPercentage)
	}
	if v.Status != before.Status {
		t.Errorf("status changed on its own: %s -> %s", before.Status, v.Status)
	}
	if v.IsNegotiable != before.IsNegotiable {
		t.Error("negotiability was cleared by a form that never mentioned it")
	}
	if v.MinOrderQty != before.MinOrderQty {
		t.Errorf("minimum order quantity changed: %d -> %d", before.MinOrderQty, v.MinOrderQty)
	}
	if v.BatchNumber != before.BatchNumber {
		t.Errorf("batch number was erased: %q -> %q", before.BatchNumber, v.BatchNumber)
	}
	if v.ExpiryDate == nil || !v.ExpiryDate.Equal(*before.ExpiryDate) {
		t.Error("expiry date was cleared by a form that never mentioned it")
	}
}

// A field the form does send is applied, including an empty one, because
// clearing an optional value is a real choice a person can make.
func TestVariantEditAppliesWhatTheFormCarries(t *testing.T) {
	v := storedVariant()
	err := applyVariantEdit(postForm(url.Values{
		"name_ar":                  {"أماريل ٢ مجم"},
		"name_en":                  {"Amaryl 2mg"},
		"sku":                      {"SKU-1000"},
		"barcode":                  {""},
		"cost_price":               {""},
		"cost_discount_percentage": {""},
		"status":                   {"active"},
		"is_negotiable":            {"false"},
		"min_order_qty":            {"5"},
		"expiry_date":              {""},
	}), v, "ar")
	if err != nil {
		t.Fatalf("applyVariantEdit: %v", err)
	}

	if v.Name.Get(i18n.AR) != "أماريل ٢ مجم" || v.Name.Get(i18n.EN) != "Amaryl 2mg" {
		t.Error("the bilingual name did not update")
	}
	if v.SKU != "SKU-1000" {
		t.Errorf("SKU = %q, want SKU-1000", v.SKU)
	}
	if v.Barcode != "" {
		t.Error("an explicitly emptied barcode should clear")
	}
	if v.CostPrice != nil {
		t.Error("an explicitly emptied cost price should clear")
	}
	if v.CostDiscountPercentage != 0 {
		t.Error("an explicitly emptied cost discount should clear")
	}
	if v.Status != catalog.StatusActive {
		t.Errorf("status = %s, want active", v.Status)
	}
	if v.IsNegotiable {
		t.Error("negotiability should be false")
	}
	if v.MinOrderQty != 5 {
		t.Errorf("min order qty = %d, want 5", v.MinOrderQty)
	}
	if v.ExpiryDate != nil {
		t.Error("an explicitly emptied expiry date should clear")
	}
}

// Bad input is refused with a message rather than written.
func TestVariantEditRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		form url.Values
	}{
		{"price is not a number", url.Values{"price": {"abc"}}},
		{"negative price", url.Values{"price": {"-1"}}},
		{"discount over 100", url.Values{"discount": {"250"}}},
		{"cost discount over 100", url.Values{"cost_discount_percentage": {"140"}}},
		{"zero minimum order", url.Values{"min_order_qty": {"0"}}},
		{"unparseable expiry", url.Values{"expiry_date": {"31-12-2027"}}},
		{"unknown status", url.Values{"status": {"deleted"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := storedVariant()
			before := *v
			if err := applyVariantEdit(postForm(tc.form), v, "ar"); err == nil {
				t.Fatal("expected a validation error")
			}
			if v.Price.Minor() != before.Price.Minor() && tc.form.Has("price") {
				// A refused edit may have written earlier fields; what matters
				// is that the caller does not persist. Recorded rather than
				// asserted, because applyVariantEdit is not transactional.
				t.Log("note: partial application before the refusal; the caller discards the variant")
			}
		})
	}
}

// Money never goes through float64 (AGENTS.md rule 1).
//
// The previous handler parsed prices with strconv.ParseFloat and multiplied by
// 100, which turns 29.99 into 2998 minor units on a binary float.
func TestVariantEditParsesMoneyExactly(t *testing.T) {
	v := storedVariant()
	if err := applyVariantEdit(postForm(url.Values{"price": {"29.99"}, "cost_price": {"8.07"}}), v, "ar"); err != nil {
		t.Fatalf("applyVariantEdit: %v", err)
	}
	if v.Price.Minor() != 2999 {
		t.Errorf("29.99 parsed to %d minor units, want 2999", v.Price.Minor())
	}
	if v.CostPrice == nil || v.CostPrice.Minor() != 807 {
		t.Errorf("8.07 parsed to %v, want 807 minor units", v.CostPrice)
	}
}

func TestVariantEditUpdatesStatus(t *testing.T) {
	v := storedVariant()
	if v.Status != catalog.StatusInactive {
		t.Fatalf("expected initial status inactive, got %v", v.Status)
	}

	// Update to active
	if err := applyVariantEdit(postForm(url.Values{"status": {"active"}}), v, "ar"); err != nil {
		t.Fatalf("applyVariantEdit: %v", err)
	}
	if v.Status != catalog.StatusActive {
		t.Errorf("status = %s, want active", v.Status)
	}

	// Update back to inactive
	if err := applyVariantEdit(postForm(url.Values{"status": {"inactive"}}), v, "ar"); err != nil {
		t.Fatalf("applyVariantEdit: %v", err)
	}
	if v.Status != catalog.StatusInactive {
		t.Errorf("status = %s, want inactive", v.Status)
	}
}
