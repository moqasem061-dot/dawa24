package promo

import "time"

// MaxAnnouncedCities caps the city names an announcement carries: a supplier
// covering all of Cairo serves sixty districts, which no post should list.
const MaxAnnouncedCities = 12

// PublishedOffer is an offer that became visible to buyers recently, described
// for an announcement: the social-media workflow posts about it.
type PublishedOffer struct {
	ID             int64   `json:"id"`
	TitleAr        string  `json:"title_ar"`
	TitleEn        string  `json:"title_en"`
	DescriptionAr  string  `json:"description_ar"`
	DiscountType   string  `json:"discount_type"`
	DiscountValue  float64 `json:"discount_value"`
	TotalPrice     float64 `json:"total_price"`
	MinOrderAmount float64 `json:"min_order_amount"`
	SupplierName   string  `json:"supplier_name"`
	// Cities names up to MaxAnnouncedCities of the areas served; CitiesTotal
	// counts them all, so a post can say "and 40 more areas".
	Cities      []string   `json:"cities"`
	CitiesTotal int        `json:"cities_total"`
	Products    []string   `json:"products"`
	PublishedAt time.Time  `json:"published_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}
