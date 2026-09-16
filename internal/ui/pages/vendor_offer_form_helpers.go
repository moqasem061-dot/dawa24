package pages

import (
	"encoding/json"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
)

type VendorOfferItemOption struct {
	VariantID      int64   `json:"variant_id"`
	NameAr         string  `json:"name_ar"`
	NameEn         string  `json:"name_en"`
	SKU            string  `json:"sku"`
	BatchNumber    string  `json:"batch_number"`
	ExpiryDate     string  `json:"expiry_date"`
	Price          string  `json:"price"`
	PriceFloat     float64 `json:"price_float"`
	WarehouseName  string  `json:"warehouse_name"`
	AvailableStock int     `json:"available_stock"`
}

type VendorOfferFormData struct {
	Offer       *promo.SpecialOffer
	Branches    []*org.Branch
	Variants    []*catalog.ProductVariant
	ItemOptions []VendorOfferItemOption
	IsEdit      bool
}

func itemsToJSON(items []VendorOfferItemOption) string {
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func offerTitleAr(o *promo.SpecialOffer) string {
	if o == nil {
		return ""
	}
	return o.Title.Get("ar")
}

func offerTitleEn(o *promo.SpecialOffer) string {
	if o == nil {
		return ""
	}
	return o.Title.Get("en")
}

func offerDesc(o *promo.SpecialOffer) string {
	if o == nil {
		return ""
	}
	return o.Description.Get("ar")
}

func offerDiscPct(o *promo.SpecialOffer) string {
	if o == nil || o.DiscountPercentage <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", o.DiscountPercentage)
}

func offerTotalPrice(o *promo.SpecialOffer) string {
	if o == nil || !o.TotalPrice.IsPositive() {
		return ""
	}
	return o.TotalPrice.String()
}

func offerMinOrder(o *promo.SpecialOffer) string {
	if o == nil || !o.MinOrderAmount.IsPositive() {
		return ""
	}
	return o.MinOrderAmount.String()
}

func offerStartDate(o *promo.SpecialOffer) string {
	if o == nil || o.StartDate == nil {
		return ""
	}
	return o.StartDate.Format("2006-01-02")
}

func offerEndDate(o *promo.SpecialOffer) string {
	if o == nil || o.EndDate == nil {
		return ""
	}
	return o.EndDate.Format("2006-01-02")
}

func selectedItemsJSON(offer *promo.SpecialOffer, options []VendorOfferItemOption) string {
	if offer == nil || len(offer.Products) == 0 {
		return "[]"
	}
	optMap := make(map[int64]VendorOfferItemOption)
	for _, opt := range options {
		optMap[opt.VariantID] = opt
	}

	type selectedItem struct {
		VariantID        int64   `json:"variant_id"`
		Name             string  `json:"name"`
		SKU              string  `json:"sku"`
		Batch            string  `json:"batch"`
		Expiry           string  `json:"expiry"`
		Warehouse        string  `json:"warehouse"`
		Stock            int     `json:"stock"`
		OriginalPrice    float64 `json:"original_price"`
		OriginalPriceStr string  `json:"original_price_str"`
		Qty              int     `json:"qty"`
		DiscountPct      float64 `json:"discount_pct"`
		CustomPrice      float64 `json:"custom_price"`
	}

	var list []selectedItem
	for _, p := range offer.Products {
		opt := optMap[p.VariantID]
		name := p.VariantName
		if name == "" {
			name = opt.NameAr
		}
		if name == "" {
			name = opt.NameEn
		}
		if name == "" {
			name = fmt.Sprintf("صنف رقم #%d", p.VariantID)
		}
		origFloat := opt.PriceFloat
		if origFloat == 0 && p.OriginalPrice.IsPositive() {
			origFloat = float64(p.OriginalPrice.Minor()) / 100.0
		}
		origStr := opt.Price
		if origStr == "" && p.OriginalPrice.IsPositive() {
			origStr = p.OriginalPrice.String()
		}
		customFloat := float64(p.CustomPrice.Minor()) / 100.0
		if customFloat == 0 && origFloat > 0 {
			customFloat = origFloat
		}
		qty := p.Quantity
		if qty <= 0 {
			qty = 1
		}
		list = append(list, selectedItem{
			VariantID:        p.VariantID,
			Name:             name,
			SKU:              opt.SKU,
			Batch:            opt.BatchNumber,
			Expiry:           opt.ExpiryDate,
			Warehouse:        opt.WarehouseName,
			Stock:            opt.AvailableStock,
			OriginalPrice:    origFloat,
			OriginalPriceStr: origStr,
			Qty:              qty,
			DiscountPct:      p.DiscountPercentage,
			CustomPrice:      customFloat,
		})
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(b)
}
