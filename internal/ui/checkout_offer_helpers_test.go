package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// shuffleAds must rotate ads per refresh without losing, duplicating or
// mutating any ad: it is a permutation, and tracking (IDs/links/beacons)
// rides on identity, never on position.
func TestShuffleAdsKeepsEveryAd(t *testing.T) {
	mk := func(ids ...int64) []*promo.Ad {
		out := make([]*promo.Ad, 0, len(ids))
		for _, id := range ids {
			out = append(out, &promo.Ad{ID: id})
		}
		return out
	}
	if out := shuffleAds(nil); len(out) != 0 {
		t.Errorf("nil in = %d out, want 0", len(out))
	}
	if out := shuffleAds(mk(1)); len(out) != 1 || out[0].ID != 1 {
		t.Error("singleton shuffled")
	}

	// Distribution: over many shuffles every ad must reach the front seat,
	// otherwise rotation is theatre and the cap keeps hiding the same ads.
	fronts := map[int64]bool{}
	for i := 0; i < 200; i++ {
		out := shuffleAds(mk(1, 2, 3, 4, 5, 6, 7, 8))
		if len(out) != 8 {
			t.Fatalf("shuffle %d: len = %d, want 8", i, len(out))
		}
		seen := map[int64]int{}
		for _, a := range out {
			seen[a.ID]++
		}
		for id := int64(1); id <= 8; id++ {
			if seen[id] != 1 {
				t.Fatalf("shuffle %d: ad %d appears %d times", i, id, seen[id])
			}
		}
		fronts[out[0].ID] = true
	}
	if len(fronts) < 2 {
		t.Error("200 shuffles never rotated the front ad")
	}
}

// Special offers that must not be sold are refused with a specific reason.
func TestValidateSpecialOfferForCheckout(t *testing.T) {
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(48 * time.Hour)

	active := &promo.SpecialOffer{
		ID: 7, OrganizationID: 3, Status: "active", AdminStatus: "approved",
		StartDate: &past, EndDate: &future,
	}
	if msg := validateSpecialOfferForCheckout(active); msg != "" {
		t.Errorf("active approved offer refused: %q", msg)
	}
	if msg := validateSpecialOfferForCheckout(nil); msg == "" {
		t.Error("nil offer accepted, want refusal")
	}

	cases := []struct {
		name   string
		mutate func(*promo.SpecialOffer)
	}{
		{"inactive", func(o *promo.SpecialOffer) { o.Status = "inactive" }},
		{"draft", func(o *promo.SpecialOffer) { o.Status = "draft" }},
		{"expired_status", func(o *promo.SpecialOffer) { o.Status = "expired" }},
		{"admin_pending", func(o *promo.SpecialOffer) { o.AdminStatus = "pending" }},
		{"admin_rejected", func(o *promo.SpecialOffer) { o.AdminStatus = "rejected" }},
		{"not_started", func(o *promo.SpecialOffer) { o.StartDate = &future }},
		{"past_end", func(o *promo.SpecialOffer) { o.EndDate = &past }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := *active
			tc.mutate(&cp)
			if msg := validateSpecialOfferForCheckout(&cp); msg == "" {
				t.Errorf("%s offer accepted, want specific refusal message", tc.name)
			}
		})
	}
}

// Checkout validation failures map to specific Arabic messages instead of the
// generic envelope, so the pharmacy knows what to fix.
func TestCheckoutValidationMessage(t *testing.T) {
	minErr := apperr.Validation("checkout.min_order_not_met", "below minimum", map[string]string{
		"order_total": "200.00", "min_order_total": "500.00",
	})
	msg, ok := checkoutValidationMessage("ar", minErr)
	if !ok || msg == "" {
		t.Fatalf("min_order_not_met not mapped: %q %v", msg, ok)
	}

	vendorErr := apperr.Validation("item.vendor_required", "vendor missing", nil)
	if msg, ok := checkoutValidationMessage("ar", vendorErr); !ok || msg == "" {
		t.Errorf("vendor_required not mapped: %q %v", msg, ok)
	}

	lineErr := apperr.Validation("checkout.line_unavailable.out_of_stock", "نفد المخزون", nil)
	if msg, ok := checkoutValidationMessage("ar", lineErr); !ok || msg == "" {
		t.Errorf("line_unavailable not mapped: %q %v", msg, ok)
	}

	// A code this function has never heard of still carries a message the
	// domain wrote for a person to read, and that message is what the pharmacy
	// needs. Sending it to the generic path was the bug: every unenumerated
	// refusal — quantity above the offer's maximum, stock exhausted mid-order,
	// a line below the supplier's minimum — arrived as the envelope's "بيانات
	// الطلب غير صالحة", which names neither the line nor the reason.
	unknown := apperr.Validation("checkout.something_new", "الكمية المطلوبة تتجاوز المتاح", nil)
	msg, ok = checkoutValidationMessage("ar", unknown)
	if !ok || msg != "الكمية المطلوبة تتجاوز المتاح" {
		t.Errorf("an unenumerated refusal lost its own message: %q %v", msg, ok)
	}

	// A code with no message has nothing to show, so it keeps the generic page
	// rather than flashing an empty notice.
	silent := apperr.Validation("checkout.something_new", "", nil)
	if _, ok := checkoutValidationMessage("ar", silent); ok {
		t.Error("a refusal with no message produced an empty notice")
	}
	if _, ok := checkoutValidationMessage("ar", apperr.Internal(errors.New("boom"))); ok {
		t.Error("internal error mapped, want generic path")
	}
}
