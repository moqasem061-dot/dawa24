package ui

import (
	"net/http"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorOffersPage renders the Laravel-parity Special Offers management view.
func (h *UIHandler) VendorOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	var offers []*promo.SpecialOffer
	if h.promoSvc != nil && actor.OrganizationID > 0 {
		offers, _ = h.promoSvc.ListSpecialOffersByOrg(ctx, actor.OrganizationID)
	}

	q := r.URL.Query()
	data := pages.VendorSpecialOffersData{
		AllOffers:    offers,
		FilterStatus: strings.TrimSpace(q.Get("status")),
		SearchQuery:  strings.TrimSpace(q.Get("q")),
		BranchID:     parseIDParam(q.Get("branch_id")),
		DiscountType: strings.TrimSpace(q.Get("discount")),
		DateFrom:     strings.TrimSpace(q.Get("date_from")),
		DateTo:       strings.TrimSpace(q.Get("date_to")),
		Page:         pagination.PageNumber(r),
		PerPage:      pagination.RowsPerPage(r),
	}
	if data.DiscountType != "percentage" && data.DiscountType != "amount" {
		data.DiscountType = ""
	}

	// The filtering runs in Go because ListSpecialOffersByOrg already returns
	// one vendor's offers -- tens of rows, not thousands. Paging in SQL here
	// would mean a second query for a set the page already holds.
	filtered := filterVendorOffers(offers, data,
		parseFilterDate(data.DateFrom), exclusiveEndOfDay(data.DateTo))

	data.TotalCount = len(filtered)
	data.Offers = pageSlice(filtered, data.Page, data.PerPage)

	if h.orgSvc != nil && actor.OrganizationID > 0 {
		data.Branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	}

	h.renderPage(ctx, w, "render vendor offers page", pages.VendorSpecialOffersPage(data, lang, dir))
}

// filterVendorOffers applies the filter bar to one vendor's offers.
//
// The status filter spans two columns: admin_status carries the moderation
// state and status carries the vendor's own draft/active/expired. They were
// conflated in one control before, which is why an offer could be "approved"
// and invisible under the "active" filter at the same time.
func filterVendorOffers(
	offers []*promo.SpecialOffer, d pages.VendorSpecialOffersData,
	from, to *time.Time,
) []*promo.SpecialOffer {
	moderation := map[string]bool{
		"pending": true, "changes_requested": true, "rejected": true, "approved": true,
	}
	search := strings.ToLower(d.SearchQuery)

	out := make([]*promo.SpecialOffer, 0, len(offers))
	for _, o := range offers {
		if o == nil {
			continue
		}
		if st := d.FilterStatus; st != "" && st != "all" {
			if moderation[st] {
				if o.AdminStatus != st {
					continue
				}
			} else if o.Status != st {
				continue
			}
		}
		if d.BranchID > 0 && (o.BranchID == nil || *o.BranchID != d.BranchID) {
			continue
		}
		switch d.DiscountType {
		case "percentage":
			if o.DiscountPercentage <= 0 {
				continue
			}
		case "amount":
			if !o.DiscountAmount.IsPositive() {
				continue
			}
		}
		// The date filter is on the offer's own window rather than on when it
		// was created: a vendor asking "what is running in October" means the
		// campaign, not the paperwork.
		if from != nil && o.EndDate != nil && o.EndDate.Before(*from) {
			continue
		}
		if to != nil && o.StartDate != nil && !o.StartDate.Before(*to) {
			continue
		}
		if search != "" {
			if !strings.Contains(strings.ToLower(o.Title.Get("ar")), search) &&
				!strings.Contains(strings.ToLower(o.Title.Get("en")), search) &&
				!strings.Contains(strings.ToLower(o.Description.Get("ar")), search) {
				continue
			}
		}
		out = append(out, o)
	}
	return out
}

// pageSlice cuts one page out of an in-memory list, clamping a page number past
// the end to the last page rather than returning nothing -- deleting the last
// row of page three must not leave the reader on an empty page three.
func pageSlice[T any](items []T, page, perPage int) []T {
	if perPage <= 0 {
		perPage = 25
	}
	if page < 1 {
		page = 1
	}
	start := (page - 1) * perPage
	if start >= len(items) {
		if len(items) == 0 {
			return nil
		}
		start = ((len(items) - 1) / perPage) * perPage
	}
	end := start + perPage
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
