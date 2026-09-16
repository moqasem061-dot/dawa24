package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func productTitle(p *catalog.Product) string {
	if p == nil {
		return ""
	}
	if p.Name["ar"] != "" {
		return p.Name["ar"]
	}
	return p.Name["en"]
}
