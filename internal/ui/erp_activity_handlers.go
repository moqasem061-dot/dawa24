package ui

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// The integration activity screen (plan section 3c): the overview, the product
// syncs (supplier only), the call log and the acknowledged orders, plus one
// sync's items and the live panel fragment. Every read is scoped by the
// signed-in actor's organization; a batch id from another tenant is not found.

// erpPurgeAfter is the item retention the batch page reports (plan 3c).
const erpPurgeAfter = 7 * 24 * time.Hour

// erpItemsPage is the items table's page size (plan 3c: 100 per page).
const erpItemsPage = 100

// erpOverviewCalls is how many calls the overview lists.
const erpOverviewCalls = 10

// erpItemStates are the item states the batch filter accepts.
var erpItemStates = map[string]bool{"created": true, "updated": true, "unchanged": true, "needs_review": true, "failed": true}

// ErpActivityPage renders /vendor/integrations/activity or
// /customer/integrations/activity. The pharmacy has no syncs tab, so tab=syncs
// there reads as the overview.
func (h *UIHandler) ErpActivityPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vendor := erpAudience(r)
	lang, dir := resolveLocale(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		erpLoginRedirect(w, r)
		return
	}
	tab := r.URL.Query().Get("tab")
	switch {
	case tab == "syncs" && vendor:
	case tab == "calls" || tab == "orders":
	default:
		tab = "overview"
	}
	data := pages.ErpActivityData{Audience: erpAudienceName(vendor), Owner: actor.IsOwner, Tab: tab}
	data.NoticeKind, data.NoticeMessage = noticeFrom(r)
	if h.erpSvc != nil {
		if err := h.fillActivity(ctx, r, &data, erpActorOrg(actor), vendor); err != nil {
			h.renderError(w, r, err)
			return
		}
	}
	if vendor {
		h.renderPage(ctx, w, "render erp activity page", pages.ErpActivityVendorPage(data, lang, dir))
		return
	}
	h.renderPage(ctx, w, "render erp activity page", pages.ErpActivityCustomerPage(data, lang, dir, actor))
}

// fillActivity loads the summary and the rows of the chosen tab. Each list asks
// for one row more than its page to learn whether a next page exists.
func (h *UIHandler) fillActivity(ctx context.Context, r *http.Request, data *pages.ErpActivityData, org int64, vendor bool) error {
	summary, err := h.erpSvc.ActivitySummary(ctx, org)
	if err != nil {
		return err
	}
	data.Summary = summary
	switch data.Tab {
	case "syncs":
		rows, err := h.erpSvc.ListBatches(ctx, org, erpQueryID(r, "before"), erp.BatchesDefaultLimit+1)
		if err != nil {
			return err
		}
		rows, next := trimPage(rows, erp.BatchesDefaultLimit, func(b erp.BatchRow) int64 { return b.ID })
		data.Batches = rows
		if next > 0 {
			data.BatchNextURL = activityURL(data.Audience, url.Values{"tab": {"syncs"}, "before": {strconv.FormatInt(next, 10)}})
		}
	case "calls":
		filter, view := callFilterFrom(r)
		data.Filter = view
		filter.Limit = erp.CallsDefaultLimit + 1
		rows, err := h.erpSvc.ListCalls(ctx, org, filter)
		if err != nil {
			return err
		}
		rows, next := trimPage(rows, erp.CallsDefaultLimit, func(c erp.CallRow) int64 { return c.ID })
		data.Calls = rows
		if next > 0 {
			values := callFilterValues(view)
			values.Set("tab", "calls")
			values.Set("before", strconv.FormatInt(next, 10))
			data.CallNextURL = activityURL(data.Audience, values)
		}
	case "orders":
		rows, err := h.erpSvc.ListOrderImports(ctx, org)
		if err != nil {
			return err
		}
		data.Orders = rows
	default:
		recent, err := h.erpSvc.ListCalls(ctx, org, erp.CallFilter{Limit: erpOverviewCalls})
		if err != nil {
			return err
		}
		data.Recent = recent
		if vendor {
			live, err := h.erpSvc.ListActiveBatches(ctx, org)
			if err != nil {
				return err
			}
			data.Live = live
		}
	}
	return nil
}

// ErpActivityLive renders the syncs-in-progress panel for polling. The panel
// keeps polling only while a batch is still queued or processing.
func (h *UIHandler) ErpActivityLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := resolveLocale(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		erpLoginRedirect(w, r)
		return
	}
	data := pages.ErpActivityData{Audience: "vendor"}
	if h.erpSvc != nil {
		live, err := h.erpSvc.ListActiveBatches(ctx, erpActorOrg(actor))
		if err != nil {
			h.renderError(w, r, err)
			return
		}
		data.Live = live
	}
	w.Header().Set("Cache-Control", "no-store")
	h.renderPage(ctx, w, "render erp live panel", pages.ErpLivePanel(data, lang))
}

// ErpBatchPage renders one product-sync batch with a page of its items. Only
// the supplier has this page; another tenant's id is not found.
func (h *UIHandler) ErpBatchPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := resolveLocale(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		erpLoginRedirect(w, r)
		return
	}
	back := "/vendor/integrations/activity?tab=syncs"
	if h.erpSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "common.service_unavailable"))
		return
	}
	notFound := func() {
		h.redirectWithNotice(w, r, back, "error", h.erpMessage(erp.NewError("ERP_SYNC_NOT_FOUND", 404, nil), lang))
	}
	id := chi.URLParam(r, "id")
	if _, isID := erp.ParseShortID(id); !isID {
		notFound()
		return
	}
	org := erpActorOrg(actor)
	batch, err := h.erpSvc.BatchStatus(ctx, org, id)
	if err != nil {
		notFound()
		return
	}
	state := r.URL.Query().Get("state")
	if !erpItemStates[state] {
		state = ""
	}
	items, err := h.erpSvc.BatchItems(ctx, org, batch.ID, erpQueryID(r, "after"), state, erpItemsPage+1)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	data := pages.ErpBatchPageData{Batch: erp.BatchRow{SyncBatch: batch}, State: state}
	data.NoticeKind, data.NoticeMessage = noticeFrom(r)
	if len(items) > erpItemsPage {
		items = items[:erpItemsPage]
		values := url.Values{"state": {state}, "after": {strconv.FormatInt(items[len(items)-1].ID, 10)}}
		data.NextURL = "/vendor/integrations/activity/syncs/" + id + "?" + values.Encode()
	}
	data.Items = items
	data.Purged = batch.FinishedAt != nil && time.Since(*batch.FinishedAt) > erpPurgeAfter
	h.renderPage(ctx, w, "render erp batch page", pages.ErpBatchVendorPage(data, lang, dir))
}

// trimPage cuts a fetched page of one extra row down to its size and returns the
// id to continue from (0 when there is no next page).
func trimPage[T any](rows []T, size int, id func(T) int64) ([]T, int64) {
	if len(rows) <= size {
		return rows, 0
	}
	rows = rows[:size]
	return rows, id(rows[size-1])
}

// erpQueryID reads a positive numeric query parameter, or 0.
func erpQueryID(r *http.Request, name string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get(name)), 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}

// callFilterFrom reads the call log's filter from the query. Dates are
// YYYY-MM-DD; the "to" day is included in full.
func callFilterFrom(r *http.Request) (erp.CallFilter, pages.ErpCallFilterView) {
	q := r.URL.Query()
	view := pages.ErpCallFilterView{
		Result:    q.Get("result"),
		KeyPrefix: clipText(q.Get("key"), 8),
		Route:     clipText(q.Get("route"), 200),
		From:      q.Get("from"),
		To:        q.Get("to"),
	}
	filter := erp.CallFilter{BeforeID: erpQueryID(r, "before"), Result: view.Result, KeyPrefix: view.KeyPrefix, Route: view.Route}
	if from, err := time.Parse("2006-01-02", view.From); err == nil {
		filter.From = &from
	}
	if to, err := time.Parse("2006-01-02", view.To); err == nil {
		end := to.AddDate(0, 0, 1)
		filter.To = &end
	}
	return filter, view
}

// callFilterValues puts the call filter back into query parameters, so the
// "older" link keeps the filter.
func callFilterValues(v pages.ErpCallFilterView) url.Values {
	values := url.Values{}
	for name, value := range map[string]string{"result": v.Result, "key": v.KeyPrefix, "route": v.Route, "from": v.From, "to": v.To} {
		if value != "" {
			values.Set(name, value)
		}
	}
	return values
}

// clipText shortens user text to n runes.
func clipText(s string, n int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) > n {
		return string(runes[:n])
	}
	return string(runes)
}

// activityURL builds a link into the audience's activity screen.
func activityURL(audience string, values url.Values) string {
	return "/" + audience + "/integrations/activity?" + values.Encode()
}
