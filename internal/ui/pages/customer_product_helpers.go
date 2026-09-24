package pages

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
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
