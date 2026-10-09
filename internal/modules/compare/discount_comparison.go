package compare

import (
	"math"
	"strings"
)

// discountBucket keeps badges and their detail lists on the same exact rule.
func discountBucket(offer, yours float64) BenchmarkBucket {
	a, b := math.Round(offer*100), math.Round(yours*100)
	if a > b {
		return BucketBetter
	}
	if a < b {
		return BucketWorse
	}
	return BucketEqual
}

// benchmarkCompetitors gives each supplier one vote using its highest discount.
// Repeated uploads and case/space variants of a supplier label cannot inflate
// counts or select a worse discount just because it appeared first in SQL.
func benchmarkCompetitors(p *MarketProduct, own string) []MarketOffer {
	offers := make([]MarketOffer, 0, len(p.Offers))
	positions := make(map[string]int)
	own = strings.ToLower(strings.TrimSpace(own))
	for _, offer := range p.Offers {
		key := strings.ToLower(strings.TrimSpace(offer.SupplierName))
		if own != "" && key == own {
			continue
		}
		if key != "" {
			if pos, ok := positions[key]; ok {
				current := offers[pos]
				if discountBucket(offer.Discount, current.Discount) == BucketBetter ||
					(discountBucket(offer.Discount, current.Discount) == BucketEqual && offer.NetPrice.Minor() < current.NetPrice.Minor()) {
					offers[pos] = offer
				}
				continue
			}
			positions[key] = len(offers)
		}
		offers = append(offers, offer)
	}
	return offers
}

func validDiscount(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 100
}
