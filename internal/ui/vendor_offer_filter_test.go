package ui

import (
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func offerAt(id int64, status, adminStatus string, branch *int64, pct float64, start, end string) *promo.SpecialOffer {
	o := &promo.SpecialOffer{
		ID: id, Status: status, AdminStatus: adminStatus, BranchID: branch,
		DiscountPercentage: pct,
		Title:              i18n.New("عرض", "Offer"),
	}
	if start != "" {
		t, _ := time.Parse("2006-01-02", start)
		o.StartDate = &t
	}
	if end != "" {
		t, _ := time.Parse("2006-01-02", end)
		o.EndDate = &t
	}
	return o
}

// The status control spans two columns -- admin_status for moderation and
// status for the vendor's own draft/active/expired -- and conflating them is
// how an offer could be approved and invisible under "active" at once.
func TestFilterVendorOffersSeparatesModerationFromLifecycle(t *testing.T) {
	branch := int64(7)
	offers := []*promo.SpecialOffer{
		offerAt(1, "active", "approved", &branch, 10, "", ""),
		offerAt(2, "draft", "pending", nil, 0, "", ""),
		offerAt(3, "active", "rejected", &branch, 0, "", ""),
	}

	approved := filterVendorOffers(offers, pages.VendorSpecialOffersData{FilterStatus: "approved"}, nil, nil)
	if len(approved) != 1 || approved[0].ID != 1 {
		t.Fatalf("moderation filter: got %d rows, want offer 1", len(approved))
	}

	active := filterVendorOffers(offers, pages.VendorSpecialOffersData{FilterStatus: "active"}, nil, nil)
	if len(active) != 2 {
		t.Fatalf("lifecycle filter: got %d rows, want 2", len(active))
	}
}

func TestFilterVendorOffersByBranchAndDiscount(t *testing.T) {
	branch := int64(7)
	other := int64(8)
	offers := []*promo.SpecialOffer{
		offerAt(1, "active", "approved", &branch, 10, "", ""),
		offerAt(2, "active", "approved", &other, 0, "", ""),
		offerAt(3, "active", "approved", nil, 0, "", ""),
	}
	offers[1].DiscountAmount = money.FromMinor(500)

	byBranch := filterVendorOffers(offers, pages.VendorSpecialOffersData{BranchID: branch}, nil, nil)
	if len(byBranch) != 1 || byBranch[0].ID != 1 {
		t.Errorf("branch filter: got %d rows, want offer 1", len(byBranch))
	}

	byPercent := filterVendorOffers(offers, pages.VendorSpecialOffersData{DiscountType: "percentage"}, nil, nil)
	if len(byPercent) != 1 || byPercent[0].ID != 1 {
		t.Errorf("percentage filter: got %d rows, want offer 1", len(byPercent))
	}
	byAmount := filterVendorOffers(offers, pages.VendorSpecialOffersData{DiscountType: "amount"}, nil, nil)
	if len(byAmount) != 1 || byAmount[0].ID != 2 {
		t.Errorf("amount filter: got %d rows, want offer 2", len(byAmount))
	}
}

// The date filter is on the campaign's own window, so an offer that overlaps
// the range at all is in it. Comparing against the created date instead would
// hide a campaign running right now because it was set up last month.
func TestFilterVendorOffersMatchesOverlappingWindows(t *testing.T) {
	offers := []*promo.SpecialOffer{
		offerAt(1, "active", "approved", nil, 0, "2026-09-01", "2026-09-30"),
		offerAt(2, "active", "approved", nil, 0, "2026-11-01", "2026-11-30"),
	}
	from, _ := time.Parse("2006-01-02", "2026-09-15")
	to, _ := time.Parse("2006-01-02", "2026-10-16")

	got := filterVendorOffers(offers, pages.VendorSpecialOffersData{}, &from, &to)
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("overlap filter: got %d rows, want the September campaign", len(got))
	}
}

// A page number past the end clamps to the last page. Deleting the last row of
// page three must not strand the reader on an empty page three.
func TestPageSliceClampsPastTheEnd(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	if got := pageSlice(items, 99, 2); len(got) != 1 || got[0] != 5 {
		t.Errorf("clamp: got %v, want the last page [5]", got)
	}
	if got := pageSlice(items, 2, 2); len(got) != 2 || got[0] != 3 {
		t.Errorf("page 2: got %v, want [3 4]", got)
	}
	if got := pageSlice([]int{}, 3, 2); got != nil {
		t.Errorf("empty list: got %v, want nil", got)
	}
}
