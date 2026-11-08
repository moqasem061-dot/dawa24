package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// The supplier's review handlers: list open items, resolve (a product picked
// through the search box), dismiss. Permission checks live on the route gate
// (vendor.integration.review); the handlers scope everything by the signed-in
// actor's own organization.

func (h *UIHandler) ErpReviewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := resolveLocale(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		erpLoginRedirect(w, r)
		return
	}
	data := pages.ErpReviewData{Audience: "vendor"}
	data.NoticeKind, data.NoticeMessage = noticeFrom(r)
	if h.erpSvc != nil {
		items, err := h.erpSvc.ReviewPage(ctx, erpActorOrg(actor), erp.Page{Limit: erp.PageDefault})
		if err != nil {
			h.renderError(w, r, err)
			return
		}
		data.Items = items
	}
	h.renderPage(ctx, w, "render erp review page", pages.ErpReviewPage(data, lang, dir))
}

func (h *UIHandler) ErpReviewResolve(w http.ResponseWriter, r *http.Request) {
	h.reviewAction(w, r, func(ctx context.Context, orgID, id, actor int64) error {
		productID, perr := strconv.ParseInt(strings.TrimSpace(r.FormValue("product_id")), 10, 64)
		if perr != nil || productID < 1 {
			return erp.NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"productId"}})
		}
		return h.erpSvc.ResolveReview(ctx, orgID, id, productID, actor)
	})
}

func (h *UIHandler) ErpReviewDismiss(w http.ResponseWriter, r *http.Request) {
	h.reviewAction(w, r, func(ctx context.Context, orgID, id, actor int64) error {
		return h.erpSvc.DismissReview(ctx, orgID, id, actor)
	})
}

// reviewAction is the shared frame: resolve the actor, refuse a disabled
// service, parse the route id, then run the mutation.
func (h *UIHandler) reviewAction(w http.ResponseWriter, r *http.Request, run func(ctx context.Context, orgID, id, actor int64) error) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		erpLoginRedirect(w, r)
		return
	}
	back := "/vendor/integrations/review"
	if h.erpSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "common.service_unavailable"))
		return
	}
	id, perr := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if perr != nil || id < 1 {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "erp.err.not_found"))
		return
	}
	if runErr := run(ctx, erpActorOrg(actor), id, actor.UserID); runErr != nil {
		h.redirectWithNotice(w, r, back, "error", h.erpMessage(runErr, lang))
		return
	}
	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "erp.review.done"))
}
