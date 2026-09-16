package org_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func band(from, to int, fee int64, active bool) *org.DeliveryBand {
	return &org.DeliveryBand{
		FromMeters: from, ToMeters: to,
		Fee: money.FromMajor(fee), IsActive: active,
	}
}

// The bands are ordered by their own numbers, never by the row order.
//
// A vendor whose 0–5 km tier happened to be inserted second had every short
// delivery priced from whichever tier the database listed first — on the live
// shape of this table, the long one.
func TestBandsAreOrderedByDistanceNotByRowOrder(t *testing.T) {
	bands := []*org.DeliveryBand{
		band(20000, 50000, 200, true), // inserted first
		band(0, 5000, 25, true),
	}
	q := org.QuoteDelivery(bands, 300)
	if q.Fee.String() != "25.00" {
		t.Errorf("300 m priced at %s, want the 0–5 km tier's 25.00", q.Fee)
	}
	if q.Basis != org.BasisExact {
		t.Errorf("basis = %q, want exact", q.Basis)
	}
}

// A hole in the vendor's table charges the next tier up, and says so.
//
// It used to fall through to bands[0]: with tiers of 0–5 km and 10–20 km, a
// seven-kilometre delivery was billed as if it were three hundred metres.
func TestGapChargesTheNextTierUp(t *testing.T) {
	bands := []*org.DeliveryBand{
		band(0, 5000, 25, true),
		band(10000, 20000, 90, true),
	}
	q := org.QuoteDelivery(bands, 7000)
	if q.Fee.String() != "90.00" {
		t.Errorf("7 km in a gap priced at %s, want the next tier's 90.00", q.Fee)
	}
	if q.Basis != org.BasisGap {
		t.Errorf("basis = %q, want gap", q.Basis)
	}
}

// An inactive tier prices nothing, including as a fallback.
func TestInactiveBandsAreIgnoredEntirely(t *testing.T) {
	bands := []*org.DeliveryBand{
		band(0, 5000, 25, false),
		band(5000, 20000, 90, true),
	}
	q := org.QuoteDelivery(bands, 300)
	if q.Fee.String() != "90.00" {
		t.Errorf("priced at %s using an inactive tier; want the only active one, 90.00", q.Fee)
	}
	if q.Basis != org.BasisBelowRange {
		t.Errorf("basis = %q, want below_range", q.Basis)
	}
}

// An unmeasurable distance takes the cheapest tier, never a guessed one.
//
// The caller used to invent 5,000 metres, which is a real band on most vendors'
// tables, so "we don't know where this pharmacy is" produced a confident and
// arbitrary mid-range fee.
func TestUnknownDistanceTakesTheCheapestTier(t *testing.T) {
	bands := []*org.DeliveryBand{
		band(0, 5000, 25, true),
		band(5000, 20000, 90, true),
		band(20000, 50000, 200, true),
	}
	q := org.QuoteDelivery(bands, org.UnknownDistance)
	if q.Fee.String() != "25.00" {
		t.Errorf("unknown distance priced at %s, want the cheapest tier 25.00", q.Fee)
	}
	if q.Basis != org.BasisUnknownDistance {
		t.Errorf("basis = %q, want unknown_distance", q.Basis)
	}
	if q.Known() {
		t.Error("Known() is true for an unmeasured distance")
	}
}

// Beyond the furthest tier, the furthest tier's fee applies.
func TestAboveRangeUsesTheFurthestTier(t *testing.T) {
	bands := []*org.DeliveryBand{
		band(0, 5000, 25, true),
		band(5000, 20000, 90, true),
	}
	q := org.QuoteDelivery(bands, 45000)
	if q.Fee.String() != "90.00" {
		t.Errorf("45 km priced at %s, want the furthest tier 90.00", q.Fee)
	}
	if q.Basis != org.BasisAboveRange {
		t.Errorf("basis = %q, want above_range", q.Basis)
	}
}

// A vendor with no tiers charges nothing, which is what having no tiers means.
func TestNoBandsChargesNothing(t *testing.T) {
	q := org.QuoteDelivery(nil, 7000)
	if !q.Fee.IsZero() {
		t.Errorf("fee = %s with no bands declared, want zero", q.Fee)
	}
	if q.Basis != org.BasisNoBands {
		t.Errorf("basis = %q, want no_bands", q.Basis)
	}
}

// Overlapping tiers are refused at save time.
//
// Two tiers both covering 7 km have no correct answer, and resolving it
// silently is how a cart, a checkout and an invoice come to disagree.
func TestOverlappingBandsAreRefused(t *testing.T) {
	err := org.ValidateDeliveryBands([]*org.DeliveryBand{
		band(0, 10000, 25, true),
		band(5000, 20000, 90, true),
	})
	if err == nil {
		t.Fatal("overlapping tiers were accepted")
	}

	// A gap is not an error: a vendor building their table one tier at a time
	// must be able to save it.
	if err := org.ValidateDeliveryBands([]*org.DeliveryBand{
		band(0, 5000, 25, true),
		band(10000, 20000, 90, true),
	}); err != nil {
		t.Errorf("a gap between tiers was refused: %v", err)
	}

	// A tier that ends before it starts is an error.
	if err := org.ValidateDeliveryBands([]*org.DeliveryBand{band(20000, 5000, 25, true)}); err == nil {
		t.Error("an inverted tier was accepted")
	}
}

// The distance is the great-circle one, and it is right.
func TestHaversineIsAccurate(t *testing.T) {
	// Cairo (Tahrir) to Giza (Pyramids): about 12.4 km as the crow flies.
	got := org.HaversineMeters(30.0444, 31.2357, 29.9792, 31.1342)
	if got < 12000 || got > 13000 {
		t.Errorf("Cairo→Giza = %d m, want ~12,400", got)
	}
	// The same point is zero, not a rounding artefact.
	if got := org.HaversineMeters(30.0444, 31.2357, 30.0444, 31.2357); got != 0 {
		t.Errorf("identical points = %d m, want 0", got)
	}
}
