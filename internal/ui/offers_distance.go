package ui

import (
	"fmt"
	"math"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// calculateHaversineKM is a presentation-only distance calculation used solely
// for the "distance from you" storefront badge. It must never gate purchase
// availability or order placement; geographical coverage is determined by SQL
// and workflow.CoverageService.
func calculateHaversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKM = 6371.0
	dLat := (lat2 - lat1) * (math.Pi / 180.0)
	dLon := (lon2 - lon1) * (math.Pi / 180.0)
	rLat1 := lat1 * (math.Pi / 180.0)
	rLat2 := lat2 * (math.Pi / 180.0)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(rLat1)*math.Cos(rLat2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKM * c
}

func formatDistanceKMText(km float64, lang string) string {
	if km <= 0 {
		return ""
	}
	if km < 1.0 {
		return i18n.T(lang, "offers.distance_less_1km")
	}
	if km < 100.0 {
		return fmt.Sprintf(i18n.T(lang, "offers.distance_km_format"), km)
	}
	return fmt.Sprintf(i18n.T(lang, "offers.distance_km_int_format"), int(km))
}
