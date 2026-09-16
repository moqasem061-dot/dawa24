package ui

// The review screen's bulk actions.
//
// One handler per action, each taking the row ids the vendor ticked on the page
// they were looking at. The ids are parsed defensively and the service scopes
// every statement to the import, so a tampered form reaches nothing.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// bulkRowIDs reads the ticked rows out of a submitted review form.
//
// Capped, because the field is repeated once per row and a page shows at most a
// hundred; a form carrying five thousand is not a vendor clicking checkboxes.
const maxBulkRows = 5000

func bulkRowIDs(r *http.Request) []int64 {
	values := r.PostForm["row_ids"]
	out := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, raw := range values {
		// A single field may carry a comma-separated list, which is how the
		// "select every row still awaiting a decision" control submits a
		// selection larger than the page.
		for _, part := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || id <= 0 {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
			if len(out) >= maxBulkRows {
				return out
			}
		}
	}
	return out
}

// reviewFilterFrom rebuilds the review screen's filter from the request the
// bulk form was posted with, so the server selects the same rows the vendor was
// reading. Only the predicates matter here; paging deliberately does not.
func reviewFilterFrom(r *http.Request) ingest.RowFilter {
	return ingest.RowFilter{
		Outcome:    r.PostFormValue("f_outcome"),
		MatchLevel: r.PostFormValue("f_match"),
		Search:     r.PostFormValue("f_q"),
	}
}

// VendorIngestBulkSubmit applies one bulk action to the selected staged rows.
//
// The action arrives as a form value rather than as a separate route because
// the three of them share every other input, and three routes differing in one
// word is how two of them end up with different id parsing.
func (h *UIHandler) VendorIngestBulkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	back := buildReviewRedirect(publicID, r)

	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	// "Apply to everything matching the current filter" is resolved on the
	// server from the same predicates that rendered the page, rather than from
	// a list the browser assembled. A selection of nine thousand rows is not
	// something a form should be carrying, and a client-side list of them would
	// silently disagree with the tab the vendor is reading the moment either
	// changed.
	var (
		ids []int64
		err error
	)
	if r.PostFormValue("select_scope") == "filter" {
		ids, err = h.ingSvc.RowIDsForFilter(ctx, publicID, reviewFilterFrom(r))
		if err != nil {
			h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
			return
		}
	} else {
		ids = bulkRowIDs(r)
	}
	if len(ids) == 0 {
		h.redirectWithNotice(w, r, back, "error",
			i18n.T(langOf(r), "vendor.ingest.no_items_selected"))
		return
	}

	action := strings.TrimSpace(r.PostFormValue("bulk_action"))
	var msg string
	switch action {
	case "confirm":
		out, e := h.ingSvc.ConfirmRowMatches(ctx, publicID, ids)
		err = e
		msg = fmt.Sprintf(i18n.T(langOf(r), "vendor.ingest.bulk_confirmed"), out.Applied)
		if out.Skipped > 0 {
			msg += fmt.Sprintf(i18n.T(langOf(r), "vendor.ingest.bulk_confirmed_skipped"), out.Skipped)
		}
	case "unlink":
		out, e := h.ingSvc.ClearRowMatches(ctx, publicID, ids)
		err = e
		msg = fmt.Sprintf(i18n.T(langOf(r), "vendor.ingest.bulk_unlinked"), out.Applied)
	case "exclude":
		out, e := h.ingSvc.SetRowsExcluded(ctx, publicID, ids, true)
		err = e
		msg = fmt.Sprintf(i18n.T(langOf(r), "vendor.ingest.bulk_excluded"), out.Applied)
	case "include":
		out, e := h.ingSvc.SetRowsExcluded(ctx, publicID, ids, false)
		err = e
		msg = fmt.Sprintf(i18n.T(langOf(r), "vendor.ingest.bulk_included"), out.Applied)
	default:
		h.redirectWithNotice(w, r, back, "error", i18n.T(langOf(r), "vendor.ingest.unknown_action"))
		return
	}

	if err != nil {
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, back, "success", msg)
}
