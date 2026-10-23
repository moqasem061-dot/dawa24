package ui

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestVendorPricingInput(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"discount=17.25", "17.25"},
		{"discount_input_type=sale_price&sale_price=80&discount=999", "20.00"},
		{"discount_input_type=sale_price&sale_price=0", "100.00"},
		{"discount_input_type=sale_price&sale_price=101", ""},
		{"discount_input_type=sale_price&sale_price=bad", ""},
		{"discount_input_type=wrong&discount=20", ""},
	} {
		r := httptest.NewRequestWithContext(context.Background(), "POST", "/", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		discount, err := vendorInputDiscount(r.PostForm, money.MustParse("100"))
		if tc.want == "" && err == nil || tc.want != "" && (err != nil || discount.String() != tc.want) {
			t.Errorf("%s: discount=%s err=%v", tc.body, discount, err)
		}
	}
}

func TestVariantEditUsesSalePriceWithUpdatedPublicPrice(t *testing.T) {
	variant := &catalog.ProductVariant{Price: money.MustParse("100"), Discount: money.MustParse("5")}
	form := url.Values{"price": {"200"}, "sale_price": {"160"}, "discount_input_type": {"sale_price"}, "discount": {"5"}}
	if err := applyVariantValues(form, variant, "ar"); err != nil {
		t.Fatal(err)
	}
	if variant.Discount.String() != "20.00" {
		t.Fatalf("discount=%s", variant.Discount)
	}
}

func TestVendorSaleColumnMapsToNetPrice(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), "POST", "/", nil)
	r.PostForm = url.Values{"field_name": {"0"}, "field_public_price": {"1"}, "field_discount_percent": {"2"}, "discount_input_type": {"sale_price"}}
	mapping, err := vendorMappingOverrides(r)
	if err != nil || mapping[2] != productmatch.FieldNetPrice || mapping[1] != productmatch.FieldPublicPrice {
		t.Fatalf("mapping=%v err=%v", mapping, err)
	}
}
