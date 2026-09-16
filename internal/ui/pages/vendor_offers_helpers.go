package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/promo"
)

func countOffersByStatus(offers []*promo.SpecialOffer, status string) int {
	c := 0
	for _, o := range offers {
		if o != nil && o.Status == status {
			c++
		}
	}
	return c
}

func countOffersByAdminStatus(offers []*promo.SpecialOffer, status string) int {
	c := 0
	for _, o := range offers {
		if o != nil && o.AdminStatus == status {
			c++
		}
	}
	return c
}
