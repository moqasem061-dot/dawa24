package org

import (
	"context"
	"math"
)

// Resolving the two ends of a delivery, so the distance is a real one.
//
// The distance used to be worked out in the UI layer, and it took the vendor
// branch as an argument rather than looking it up. The checkout resolved ONE
// vendor branch — the first vendor's — and then called the fee resolver once
// per vendor in the cart with that same branch. A pharmacy buying from three
// suppliers was charged all three deliveries as if every one of them shipped
// from the first supplier's warehouse.
//
// The second bug was the fallback. With no vendor branch coordinates it used
// the first coverage row's latitude and longitude — which is the centre of a
// city the vendor DELIVERS TO, not where they ship from. A supplier in Cairo
// covering Alexandria was measured as being in Alexandria.
//
// Both are gone: the origin is always the vendor's own branch, and when there
// isn't one with coordinates the distance is reported as unknown rather than
// invented. See QuoteDelivery for what an unknown distance is charged.

// QuoteDeliveryFor prices a delivery from one vendor to one pharmacy branch.
func (s *Service) QuoteDeliveryFor(
	ctx context.Context, vendorOrgID int64, pharmacyBranchID *int64,
) (DeliveryQuote, error) {
	if vendorOrgID <= 0 {
		return DeliveryQuote{DistanceMeters: UnknownDistance, Basis: BasisNoBands}, nil
	}

	bands, err := s.repo.GetDeliveryBands(ctx, vendorOrgID)
	if err != nil {
		return DeliveryQuote{}, err
	}

	origin := s.originFor(ctx, vendorOrgID)
	destination := s.branchPoint(ctx, pharmacyBranchID)

	distance := UnknownDistance
	if origin != nil && destination != nil {
		distance = HaversineMeters(origin.Lat, origin.Lon, destination.Lat, destination.Lon)
	}
	return QuoteDelivery(bands, distance), nil
}

// Point is a resolved pair of coordinates.
type Point struct {
	Lat float64
	Lon float64
}

// originFor is where a vendor ships from: their main branch, or any branch of
// theirs that carries coordinates.
//
// Never a coverage row. A coverage row says where the vendor delivers TO.
func (s *Service) originFor(ctx context.Context, vendorOrgID int64) *Point {
	branches, err := s.repo.ListBranchesByOrg(ctx, vendorOrgID)
	if err != nil || len(branches) == 0 {
		return nil
	}
	var fallback *Point
	for _, b := range branches {
		p := pointOf(b)
		if p == nil {
			continue
		}
		if b.IsMain {
			return p
		}
		if fallback == nil {
			fallback = p
		}
	}
	return fallback
}

// branchPoint resolves one branch's coordinates.
func (s *Service) branchPoint(ctx context.Context, branchID *int64) *Point {
	if branchID == nil || *branchID <= 0 {
		return nil
	}
	b, err := s.repo.GetBranchByID(ctx, *branchID)
	if err != nil {
		return nil
	}
	return pointOf(b)
}

// pointOf reads a branch's coordinates, treating a stored (0, 0) as absent.
//
// Null Island is in the Gulf of Guinea. A branch that landed there is a branch
// whose coordinates were never filled in, and measuring a delivery to it
// produces about five thousand kilometres — which lands in the vendor's
// furthest band and bills a pharmacy accordingly.
func pointOf(b *Branch) *Point {
	if b == nil || b.Latitude == nil || b.Longitude == nil {
		return nil
	}
	if *b.Latitude == 0 && *b.Longitude == 0 {
		return nil
	}
	return &Point{Lat: *b.Latitude, Lon: *b.Longitude}
}

// earthRadiusMeters is the mean radius used for great-circle distances.
const earthRadiusMeters = 6371000

// HaversineMeters is the great-circle distance between two points, in metres.
//
// It is the straight-line distance, not the driving distance, and the delivery
// bands are set by vendors who know that: an Egyptian city's road distance runs
// roughly 1.2–1.4× the crow-flight one, and a vendor pricing "0–5 km" is
// pricing what this function measures because it is what the coverage screen
// has always shown them.
func HaversineMeters(lat1, lon1, lat2, lon2 float64) int {
	const toRad = math.Pi / 180
	dLat := (lat2 - lat1) * toRad
	dLon := (lon2 - lon1) * toRad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*toRad)*math.Cos(lat2*toRad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return int(math.Round(earthRadiusMeters * 2 * math.Asin(math.Min(1, math.Sqrt(a)))))
}
