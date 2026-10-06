package pages

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

func hasOrderableOffer(offers []SupplierOffer) bool {
	for _, offer := range offers {
		if offer.CanAddToCart {
			return true
		}
	}
	return false
}

func selectedProductOffer(offers []SupplierOffer) SupplierOffer {
	for _, offer := range offers {
		if offer.CanAddToCart {
			return offer
		}
	}
	return offers[0]
}

func hasNegotiableOffer(offers []SupplierOffer) bool {
	for _, offer := range offers {
		if offer.IsNegotiable {
			return true
		}
	}
	return false
}

func localizedProductTitle(product *catalog.Product, lang string) string {
	if product == nil {
		return ""
	}
	if name := product.Name[i18n.Lang(lang)]; name != "" {
		return name
	}
	return productTitle(product)
}

func productTitle(p *catalog.Product) string {
	if p == nil {
		return ""
	}
	if p.Name["ar"] != "" {
		return p.Name["ar"]
	}
	return p.Name["en"]
}
