package ui

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The subscriber log's request handling.
//
// The filters are read here and echoed straight back into the page so the
// controls stay populated, the pager carries them, and the URL is the question.
// Parsing them in one place is what stops the form, the pager and the query
// from drifting into three slightly different ideas of what was asked.

// adminSubscriptionFiltersFrom reads the filter bar off the query string.
//
// Unparseable values are dropped rather than defaulted: a malformed date in a
// bookmark should widen the answer, not silently narrow it to something the
// reader did not ask for.
func adminSubscriptionFiltersFrom(r *http.Request) pages.AdminSubscriptionFilters {
	q := r.URL.Query()
	f := pages.AdminSubscriptionFilters{
		OrganizationQuery: strings.TrimSpace(q.Get("org_q")),
		Status:            strings.TrimSpace(q.Get("status")),
		BillingCycle:      strings.TrimSpace(q.Get("cycle")),
		AutoRenew:         strings.TrimSpace(q.Get("auto_renew")),
		StartsFrom:        strings.TrimSpace(q.Get("starts_from")),
		StartsTo:          strings.TrimSpace(q.Get("starts_to")),
		ExpiresFrom:       strings.TrimSpace(q.Get("expires_from")),
		ExpiresTo:         strings.TrimSpace(q.Get("expires_to")),
	}
	if len(f.OrganizationQuery) > 80 {
		f.OrganizationQuery = f.OrganizationQuery[:80]
	}
	if f.AutoRenew != "1" && f.AutoRenew != "0" {
		f.AutoRenew = ""
	}
	if id, err := strconv.ParseInt(q.Get("plan_id"), 10, 64); err == nil && id > 0 {
		f.PlanID = id
	}
	// Only the windows the control offers are honoured, for the same reason
	// pagination.RowsPerPage honours only its own set: the query string is
	// user-supplied and an arbitrary window is a question the UI never asked.
	if n, err := strconv.Atoi(q.Get("expiring_days")); err == nil {
		switch n {
		case 7, 30, 90:
			f.ExpiringWithinDays = n
		}
	}
	return f
}

// parseFilterDate reads a date input's value. An empty or malformed value is
// no filter at all.
func parseFilterDate(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil
	}
	return &t
}

// exclusiveEndOfDay turns a "to" date into the instant after it.
//
// A reader who types 2026-09-09 in a "to" box means the whole of that day. A
// naive `<= 2026-09-09` compares against midnight and silently excludes every
// subscription that started during it.
func exclusiveEndOfDay(raw string) *time.Time {
	t := parseFilterDate(raw)
	if t == nil {
		return nil
	}
	end := t.AddDate(0, 0, 1)
	return &end
}

// AdminSubscriptionHistoryFragment serves one subscription's trail into the
// history modal.
//
// A fragment rather than data embedded in the page: a hundred-row page would
// otherwise carry a hundred trails it will never show.
func (h *UIHandler) AdminSubscriptionHistoryFragment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.renderError(w, r, err)
		return
	}
	if h.billSvc == nil {
		h.renderError(w, r, nil)
		return
	}

	rows, err := h.billSvc.AdminSubscriptionHistory(database.AsSystem(ctx), id)
	if err != nil {
		h.log.ErrorContext(ctx, "load subscription history", "subscription_id", id, "error", err)
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if renderErr := pages.AdminSubscriptionHistoryFragment(rows, lang).Render(ctx, w); renderErr != nil {
		h.log.ErrorContext(ctx, "render subscription history", "error", renderErr)
	}
}

// toBillingFilter converts the page's echoed filter state into the query the
// repository runs.
func adminSubscriptionBillingFilter(f pages.AdminSubscriptionFilters) billing.AdminSubscriptionFilter {
	out := billing.AdminSubscriptionFilter{
		OrganizationQuery:  f.OrganizationQuery,
		PlanID:             f.PlanID,
		Status:             f.Status,
		BillingCycle:       f.BillingCycle,
		ExpiringWithinDays: f.ExpiringWithinDays,
		StartsFrom:         parseFilterDate(f.StartsFrom),
		StartsTo:           exclusiveEndOfDay(f.StartsTo),
		ExpiresFrom:        parseFilterDate(f.ExpiresFrom),
		ExpiresTo:          exclusiveEndOfDay(f.ExpiresTo),
	}
	switch f.AutoRenew {
	case "1":
		yes := true
		out.AutoRenew = &yes
	case "0":
		no := false
		out.AutoRenew = &no
	}
	return out
}
