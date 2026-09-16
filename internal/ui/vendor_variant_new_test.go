package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func newTestUIHandler() *UIHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
}

func sampleBranches() []pages.VendorBranchOption {
	return []pages.VendorBranchOption{
		{ID: 101, Name: "الفرع الرئيسي - القاهرة", IsMain: true},
		{ID: 102, Name: "فرع الإسكندرية", IsMain: false},
	}
}

func TestParseAndValidateVariantNew_MissingProductID(t *testing.T) {
	h := newTestUIHandler()
	formVals := url.Values{
		"name_ar":   {"بنادول أزرق"},
		"price":     {"85.50"},
		"branch_id": {"101"},
		"stock_qty": {"20"},
		"discount":  {"10"},
		"sku":       {"PAN-BLU-01"},
		"barcode":   {"6221234567890"},
		"unit":      {"علبة"},
	}
	r := httptest.NewRequest(http.MethodPost, "/vendor/variants/new", strings.NewReader(formVals.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = r.ParseForm()

	form, errors, variant, _ := parseAndValidateVariantNew(context.Background(), h, r, 50, "ar", sampleBranches())

	if variant != nil {
		t.Errorf("expected variant to be nil when product_id is missing, got %+v", variant)
	}
	if !errors.Has("product_id") {
		t.Errorf("expected validation error on product_id, got %v", errors.Fields)
	}
	if errors.General == "" {
		t.Errorf("expected General error message to be set")
	}
	// Verify that user-entered values are preserved
	if form.NameAR != "بنادول أزرق" {
		t.Errorf("NameAR = %q, want 'بنادول أزرق'", form.NameAR)
	}
	if form.Price != "85.50" {
		t.Errorf("Price = %q, want '85.50'", form.Price)
	}
	if form.SKU != "PAN-BLU-01" {
		t.Errorf("SKU = %q, want 'PAN-BLU-01'", form.SKU)
	}
	if form.Barcode != "6221234567890" {
		t.Errorf("Barcode = %q, want '6221234567890'", form.Barcode)
	}
	if form.Unit != "علبة" {
		t.Errorf("Unit = %q, want 'علبة'", form.Unit)
	}
}

func TestParseAndValidateVariantNew_MissingOrInvalidBranch(t *testing.T) {
	h := newTestUIHandler()
	branches := sampleBranches()

	// Missing branch_id
	formVals := url.Values{
		"product_id": {"5001"},
		"name_ar":    {"صنف تجريبي"},
		"price":      {"100.00"},
		"branch_id":  {""},
	}
	r := httptest.NewRequest(http.MethodPost, "/vendor/variants/new", strings.NewReader(formVals.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = r.ParseForm()

	_, errors, variant, _ := parseAndValidateVariantNew(context.Background(), h, r, 50, "ar", branches)
	if variant != nil {
		t.Fatal("expected nil variant on missing branch")
	}
	if !errors.Has("branch_id") {
		t.Errorf("expected error on branch_id, got %v", errors.Fields)
	}

	// Foreign branch_id not belonging to vendor
	formVals.Set("branch_id", "9999")
	r2 := httptest.NewRequest(http.MethodPost, "/vendor/variants/new", strings.NewReader(formVals.Encode()))
	r2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = r2.ParseForm()

	_, errors2, variant2, _ := parseAndValidateVariantNew(context.Background(), h, r2, 50, "ar", branches)
	if variant2 != nil {
		t.Fatal("expected nil variant on foreign branch")
	}
	if !errors2.Has("branch_id") {
		t.Errorf("expected error on branch_id when id not in branchOptions, got %v", errors2.Fields)
	}
}

func TestParseAndValidateVariantNew_PriceAndDiscountValidation(t *testing.T) {
	h := newTestUIHandler()
	branches := sampleBranches()

	testCases := []struct {
		name      string
		form      url.Values
		wantField string
	}{
		{
			name: "missing price",
			form: url.Values{
				"product_id": {"5001"},
				"name_ar":    {"صنف"},
				"branch_id":  {"101"},
				"price":      {""},
			},
			wantField: "price",
		},
		{
			name: "zero price",
			form: url.Values{
				"product_id": {"5001"},
				"name_ar":    {"صنف"},
				"branch_id":  {"101"},
				"price":      {"0.00"},
			},
			wantField: "price",
		},
		{
			name: "negative price",
			form: url.Values{
				"product_id": {"5001"},
				"name_ar":    {"صنف"},
				"branch_id":  {"101"},
				"price":      {"-50"},
			},
			wantField: "price",
		},
		{
			name: "discount over 100",
			form: url.Values{
				"product_id": {"5001"},
				"name_ar":    {"صنف"},
				"branch_id":  {"101"},
				"price":      {"100.00"},
				"discount":   {"120"},
			},
			wantField: "discount",
		},
		{
			name: "negative discount",
			form: url.Values{
				"product_id": {"5001"},
				"name_ar":    {"صنف"},
				"branch_id":  {"101"},
				"price":      {"100.00"},
				"discount":   {"-5"},
			},
			wantField: "discount",
		},
		{
			name: "cost discount percentage over 100",
			form: url.Values{
				"product_id":               {"5001"},
				"name_ar":                  {"صنف"},
				"branch_id":                {"101"},
				"price":                    {"100.00"},
				"cost_discount_percentage": {"150"},
			},
			wantField: "cost_discount_percentage",
		},
		{
			name: "invalid expiry date format",
			form: url.Values{
				"product_id":  {"5001"},
				"name_ar":     {"صنف"},
				"branch_id":   {"101"},
				"price":       {"100.00"},
				"expiry_date": {"12-31-2027"},
			},
			wantField: "expiry_date",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/vendor/variants/new", strings.NewReader(tc.form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			_ = r.ParseForm()

			_, errors, variant, _ := parseAndValidateVariantNew(context.Background(), h, r, 50, "ar", branches)
			if variant != nil {
				t.Errorf("expected variant to be nil for %s", tc.name)
			}
			if !errors.Has(tc.wantField) {
				t.Errorf("expected error field %q for %s, got errors: %v", tc.wantField, tc.name, errors.Fields)
			}
		})
	}
}

func TestParseAndValidateVariantNew_SuccessAllFieldsMapped(t *testing.T) {
	h := newTestUIHandler()
	branches := sampleBranches()

	formVals := url.Values{
		"product_id":               {"2048"},
		"name_ar":                  {"كونكور ٥ مجم ٣٠ قرص"},
		"name_en":                  {"Concor 5mg 30 Tabs"},
		"sku":                      {"CONC-5MG-30"},
		"barcode":                  {"6229988776655"},
		"unit":                     {"علبة"},
		"price":                    {"75.50"},
		"discount":                 {"12.5"},
		"cost_price":               {"55.00"},
		"cost_discount_percentage": {"5.0"},
		"stock_qty":                {"120"},
		"min_order_qty":            {"2"},
		"quota_limit":              {"10"},
		"branch_id":                {"102"},
		"batch_number":             {"BTH-9941"},
		"expiry_date":              {"2028-06-30"},
		"is_negotiable":            {"true"},
	}

	r := httptest.NewRequest(http.MethodPost, "/vendor/variants/new", strings.NewReader(formVals.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = r.ParseForm()

	_, errors, variant, stockQty := parseAndValidateVariantNew(context.Background(), h, r, 77, "ar", branches)

	if len(errors.Fields) > 0 {
		t.Fatalf("unexpected validation errors: %v", errors.Fields)
	}
	if variant == nil {
		t.Fatal("expected non-nil variant on valid input")
	}

	if variant.OrganizationID != 77 {
		t.Errorf("OrganizationID = %d, want 77", variant.OrganizationID)
	}
	if variant.ProductID != 2048 {
		t.Errorf("ProductID = %d, want 2048", variant.ProductID)
	}
	if variant.Name.Get(i18n.AR) != "كونكور ٥ مجم ٣٠ قرص" {
		t.Errorf("Name(AR) = %q, want 'كونكور ٥ مجم ٣٠ قرص'", variant.Name.Get(i18n.AR))
	}
	if variant.Name.Get(i18n.EN) != "Concor 5mg 30 Tabs" {
		t.Errorf("Name(EN) = %q, want 'Concor 5mg 30 Tabs'", variant.Name.Get(i18n.EN))
	}
	if variant.SKU != "CONC-5MG-30" {
		t.Errorf("SKU = %q, want 'CONC-5MG-30'", variant.SKU)
	}
	if variant.Barcode != "6229988776655" {
		t.Errorf("Barcode = %q, want '6229988776655'", variant.Barcode)
	}
	if variant.Unit != "علبة" {
		t.Errorf("Unit = %q, want 'علبة'", variant.Unit)
	}
	if variant.Price.String() != "75.50" {
		t.Errorf("Price = %s, want '75.50'", variant.Price.String())
	}
	if variant.Discount.String() != "12.50" {
		t.Errorf("Discount = %s, want '12.50'", variant.Discount.String())
	}
	if variant.CostPrice == nil || variant.CostPrice.String() != "55.00" {
		t.Errorf("CostPrice = %v, want '55.00'", variant.CostPrice)
	}
	if variant.CostDiscountPercentage != 5.0 {
		t.Errorf("CostDiscountPercentage = %v, want 5.0", variant.CostDiscountPercentage)
	}
	if variant.MinOrderQty != 2 {
		t.Errorf("MinOrderQty = %d, want 2", variant.MinOrderQty)
	}
	if variant.QuotaLimit == nil || *variant.QuotaLimit != 10 {
		t.Errorf("QuotaLimit = %v, want 10", variant.QuotaLimit)
	}
	if variant.BranchID == nil || *variant.BranchID != 102 {
		t.Errorf("BranchID = %v, want 102", variant.BranchID)
	}
	if variant.BatchNumber != "BTH-9941" {
		t.Errorf("BatchNumber = %q, want 'BTH-9941'", variant.BatchNumber)
	}
	if variant.ExpiryDate == nil || variant.ExpiryDate.Format("2006-01-02") != "2028-06-30" {
		t.Errorf("ExpiryDate = %v, want '2028-06-30'", variant.ExpiryDate)
	}
	if !variant.IsNegotiable {
		t.Errorf("IsNegotiable = false, want true")
	}
	if variant.Status != catalog.StatusActive {
		t.Errorf("Status = %v, want StatusActive", variant.Status)
	}
	if stockQty != 120 {
		t.Errorf("stockQty = %d, want 120", stockQty)
	}
}

func TestVendorVariantNewSubmit_HTMX_ValidationFailureReturns422(t *testing.T) {
	h := newTestUIHandler()

	formVals := url.Values{
		"product_id": {""}, // missing
		"name_ar":    {"صنف تجريبي"},
		"price":      {"50.00"},
		"branch_id":  {"101"},
	}
	r := httptest.NewRequest(http.MethodPost, "/vendor/variants/new", strings.NewReader(formVals.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("HX-Request", "true")

	// Set actor context with organization
	actor := authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
	}
	ctx := authctx.WithActor(r.Context(), actor)
	ctx = database.WithTenant(ctx, 10)
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	h.VendorVariantNewSubmit(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (Unprocessable Entity)", w.Code, http.StatusUnprocessableEntity)
	}

	body := w.Body.String()
	if !strings.Contains(body, "add-custom-variant-form-container") {
		t.Errorf("expected body to contain add-custom-variant-form-container, got:\n%s", body)
	}
	if !strings.Contains(body, "value=\"صنف تجريبي\"") {
		t.Errorf("expected form to preserve name_ar value 'صنف تجريبي'")
	}
	if !strings.Contains(body, "value=\"50.00\"") {
		t.Errorf("expected form to preserve price value '50.00'")
	}
}
