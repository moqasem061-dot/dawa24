package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/antiscrape"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// SetScrapeGuard installs the anti-scraping guard for /catalog.
//
// A setter rather than a constructor argument for the same reason as every
// other optional dependency here: NewUIHandler already takes fifteen, and the
// guard needs a Redis handle that is not dialled when routes are mounted.
//
// Leaving it unset is a supported state, not a mistake: the guard is a property
// of the deployment, and an absent one must not change which routes exist. A
// test harness relies on that.
func (h *UIHandler) SetScrapeGuard(g *antiscrape.Guard) { h.scrape = g }

// SetGuestListingLimits bounds how far a signed-out caller may page into the
// catalogue listing.
//
// This is the ceiling on total exposure, and it is the part of the defence a
// forged User-Agent cannot walk around: the request budgets decide how fast the
// catalogue can be read, this decides how much of it is readable at all. Zero
// for either value leaves that dimension unbounded.
func (h *UIHandler) SetGuestListingLimits(maxPage, maxPageSize int) {
	h.guestMaxPage, h.guestMaxPageSize = maxPage, maxPageSize
}

// guestListingBounds returns the page and page-size ceilings that apply to this
// request: the configured guest limits when nobody is signed in, and the
// caller-supplied defaults otherwise.
func (h *UIHandler) guestListingBounds(r *http.Request, maxPage, maxPageSize int) (int, int) {
	if actor, ok := authctx.From(r.Context()); ok && actor.UserID > 0 {
		return maxPage, maxPageSize
	}
	if h.guestMaxPage > 0 && h.guestMaxPage < maxPage {
		maxPage = h.guestMaxPage
	}
	if h.guestMaxPageSize > 0 && h.guestMaxPageSize < maxPageSize {
		maxPageSize = h.guestMaxPageSize
	}
	return maxPage, maxPageSize
}
