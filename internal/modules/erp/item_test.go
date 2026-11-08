package erp

import (
	"encoding/json"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func rawItem(overrides map[string]func(*provider.RawItem)) provider.RawItem {
	price := json.Number("62.50")
	stock := json.Number("10")
	item := provider.RawItem{
		Code:        "4001",
		ArName:      "دواء",
		PublicPrice: &price,
		Stock:       &stock,
	}
	for _, override := range overrides {
		override(&item)
	}
	return item
}

func numPtr(v string) *json.Number {
	n := json.Number(v)
	return &n
}

func TestNormalizeItemRules(t *testing.T) {
	cases := []struct {
		name     string
		raw      provider.RawItem
		wantCode string
	}{
		{"code required", rawItem(map[string]func(*provider.RawItem){"code": func(r *provider.RawItem) { r.Code = "" }}), "ERP_ITEM_CODE_REQUIRED"},
		{"name required", rawItem(map[string]func(*provider.RawItem){"name": func(r *provider.RawItem) { r.ArName = "" }}), "ERP_ITEM_NAME_REQUIRED"},
		{"price missing", rawItem(map[string]func(*provider.RawItem){"price": func(r *provider.RawItem) { r.PublicPrice = nil }}), "ERP_ITEM_PRICE_INVALID"},
		{"price negative", rawItem(map[string]func(*provider.RawItem){"price": func(r *provider.RawItem) { r.PublicPrice = numPtr("-1") }}), "ERP_ITEM_PRICE_INVALID"},
		{"price three decimals", rawItem(map[string]func(*provider.RawItem){"price": func(r *provider.RawItem) { r.PublicPrice = numPtr("12.345") }}), "ERP_ITEM_PRICE_PRECISION"},
		{"price garbage", rawItem(map[string]func(*provider.RawItem){"price": func(r *provider.RawItem) { r.PublicPrice = numPtr("abc") }}), "ERP_ITEM_PRICE_INVALID"},
		{"stock missing", rawItem(map[string]func(*provider.RawItem){"stock": func(r *provider.RawItem) { r.Stock = nil }}), "ERP_ITEM_STOCK_INVALID"},
		{"stock negative", rawItem(map[string]func(*provider.RawItem){"stock": func(r *provider.RawItem) { r.Stock = numPtr("-1") }}), "ERP_ITEM_STOCK_NEGATIVE"},
		{"stock fractional", rawItem(map[string]func(*provider.RawItem){"stock": func(r *provider.RawItem) { r.Stock = numPtr("1.5") }}), "ERP_ITEM_STOCK_INVALID"},
		{"stock garbage", rawItem(map[string]func(*provider.RawItem){"stock": func(r *provider.RawItem) { r.Stock = numPtr("ten") }}), "ERP_ITEM_STOCK_INVALID"},
		{"discount over hundred", rawItem(map[string]func(*provider.RawItem){"discount": func(r *provider.RawItem) { r.Discount = numPtr("100.01") }}), "ERP_ITEM_DISCOUNT_INVALID"},
		{"discount negative", rawItem(map[string]func(*provider.RawItem){"discount": func(r *provider.RawItem) { r.Discount = numPtr("-1") }}), "ERP_ITEM_DISCOUNT_INVALID"},
		{"tax below zero", rawItem(map[string]func(*provider.RawItem){"tax": func(r *provider.RawItem) { r.Tax = numPtr("-1") }}), "ERP_ITEM_TAX_INVALID"},
		{"tax over hundred", rawItem(map[string]func(*provider.RawItem){"tax": func(r *provider.RawItem) { r.Tax = numPtr("100.5") }}), "ERP_ITEM_TAX_INVALID"},
		{"quota flag without quantity", rawItem(map[string]func(*provider.RawItem){"quota": func(r *provider.RawItem) {
			yes := true
			r.IsQuota = &yes
			r.QuotaQty = numPtr("0")
		}}), "ERP_ITEM_QUOTA_INVALID"},
		{"quota negative quantity", rawItem(map[string]func(*provider.RawItem){"quota": func(r *provider.RawItem) {
			yes := true
			r.IsQuota = &yes
			r.QuotaQty = numPtr("-2")
		}}), "ERP_ITEM_QUOTA_INVALID"},
		{"expiry bad format", rawItem(map[string]func(*provider.RawItem){"expiry": func(r *provider.RawItem) { r.Expiry = "31/12/2027" }}), "ERP_ITEM_EXPIRY_INVALID"},
		{"expiry valid format", rawItem(map[string]func(*provider.RawItem){"expiry": func(r *provider.RawItem) { r.Expiry = "2028-01-10" }}), ""},
		{"barcode letters", rawItem(map[string]func(*provider.RawItem){"barcode": func(r *provider.RawItem) { r.Barcode = "ABC" }}), "ERP_ITEM_BARCODE_INVALID"},
		{"barcode too long", rawItem(map[string]func(*provider.RawItem){"barcode": func(r *provider.RawItem) {
			r.Barcode = "123456789012345678901234567890123" // 33 digits
		}}), "ERP_ITEM_BARCODE_INVALID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item, itemErr := NormalizeItem(tc.raw)
			if tc.wantCode == "" {
				if itemErr != nil {
					t.Fatalf("unexpected error %s", itemErr.Code)
				}
				return
			}
			if itemErr == nil || itemErr.Code != tc.wantCode {
				t.Fatalf("error = %+v, want %s", itemErr, tc.wantCode)
			}
			_ = item
		})
	}
}

func TestNormalizeItemValidRows(t *testing.T) {
	t.Run("camelCase sample", func(t *testing.T) {
		price := json.Number("1305.46")
		discount := json.Number("2")
		tax := json.Number("14")
		stock := json.Number("1976")
		noQuota := false
		quotaQty := json.Number("0")
		raw := provider.RawItem{Code: "4001", ArName: "دواء", Barcode: "6221790408214", Expiry: "2028-01-10", PublicPrice: &price, Discount: &discount, Tax: &tax, Stock: &stock, IsQuota: &noQuota, QuotaQty: &quotaQty}
		item, itemErr := NormalizeItem(raw)
		if itemErr != nil {
			t.Fatal(itemErr.Code)
		}
		if item.PublicPrice.Minor() != money.MustParse("1305.46").Minor() {
			t.Errorf("price = %s", item.PublicPrice.String())
		}
		if item.Discount == nil || item.Discount.Minor() != money.MustParse("2").Minor() {
			t.Errorf("discount = %v", item.Discount)
		}
		if item.Tax == nil || item.Tax.Minor() != money.MustParse("14").Minor() {
			t.Errorf("tax = %v", item.Tax)
		}
		if item.Stock != 1976 {
			t.Errorf("stock = %d", item.Stock)
		}
		if item.IsQuota != nil && *item.IsQuota {
			t.Error("IsQuota should read false")
		}
		// quotaQuantity 0 with isQuota false is valid: no quota at all.
		if item.QuotaQty != nil {
			t.Errorf("QuotaQty = %v, want nil", item.QuotaQty)
		}
		if item.Expiry == nil || item.Expiry.Format("2006-01-02") != "2028-01-10" {
			t.Errorf("expiry = %v", item.Expiry)
		}
	})
	t.Run("timestamp expiry keeps the date part", func(t *testing.T) {
		item, itemErr := NormalizeItem(provider.RawItem{Code: "1", ArName: "د", PublicPrice: numPtr("10"), Stock: numPtr("1"), Expiry: "2027-09-30T00:00:00"})
		if itemErr != nil {
			t.Fatal(itemErr.Code)
		}
		if item.Expiry == nil || item.Expiry.Format("2006-01-02") != "2027-09-30" {
			t.Errorf("expiry = %v", item.Expiry)
		}
	})
	t.Run("numeric text code", func(t *testing.T) {
		item, itemErr := NormalizeItem(provider.RawItem{Code: "900001", ArName: "د", PublicPrice: numPtr("10.50"), Stock: numPtr("5")})
		if itemErr != nil {
			t.Fatal(itemErr.Code)
		}
		if item.Code != "900001" {
			t.Errorf("code = %q", item.Code)
		}
	})
	t.Run("quota flag with quantity", func(t *testing.T) {
		yes := true
		item, itemErr := NormalizeItem(provider.RawItem{Code: "1", ArName: "د", PublicPrice: numPtr("10"), Stock: numPtr("5"), IsQuota: &yes, QuotaQty: numPtr("3")})
		if itemErr != nil {
			t.Fatal(itemErr.Code)
		}
		if item.IsQuota == nil || !*item.IsQuota || item.QuotaQty == nil || *item.QuotaQty != 3 {
			t.Errorf("quota = %+v %v", item.IsQuota, item.QuotaQty)
		}
	})
}

func TestNormalizeItemDiscountAndTaxNilAreAccepted(t *testing.T) {
	item, itemErr := NormalizeItem(provider.RawItem{Code: "1", ArName: "د", PublicPrice: numPtr("10"), Stock: numPtr("0")})
	if itemErr != nil {
		t.Fatal(itemErr.Code)
	}
	if item.Discount != nil || item.Tax != nil {
		t.Errorf("absent discount/tax = %v/%v, want nil", item.Discount, item.Tax)
	}
	if item.Stock != 0 {
		t.Errorf("stock 0 is a real quantity, got %d", item.Stock)
	}
}

func TestHashChangesWithInputs(t *testing.T) {
	base := provider.RawItem{Code: "1", ArName: "د", PublicPrice: numPtr("10"), Stock: numPtr("5")}
	item, itemErr := NormalizeItem(base)
	if itemErr != nil {
		t.Fatal(itemErr.Code)
	}
	same, _ := NormalizeItem(base)
	if item.Hash(10, 11) != same.Hash(10, 11) {
		t.Fatal("identical item produced a different hash")
	}
	if item.Hash(10, 11) == item.Hash(10, 12) {
		t.Error("branch change did not change the hash")
	}
	if item.Hash(10, 11) == item.Hash(12, 11) {
		t.Error("warehouse change did not change the hash")
	}
	dearer, _ := NormalizeItem(provider.RawItem{Code: "1", ArName: "د", PublicPrice: numPtr("11"), Stock: numPtr("5")})
	if item.Hash(10, 11) == dearer.Hash(10, 11) {
		t.Error("price change did not change the hash")
	}
	more, _ := NormalizeItem(provider.RawItem{Code: "1", ArName: "د", PublicPrice: numPtr("10"), Stock: numPtr("6")})
	if item.Hash(10, 11) == more.Hash(10, 11) {
		t.Error("stock change did not change the hash")
	}
}
