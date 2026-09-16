package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminReviewsPage renders platform-wide ratings and reviews submitted by pharmacies to vendors.
func (h *UIHandler) AdminReviewsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	ratingStr := strings.TrimSpace(r.URL.Query().Get("rating"))
	statusStr := strings.TrimSpace(r.URL.Query().Get("status"))

	filter := org.AdminReviewFilter{
		Search: q,
		Status: statusStr,
		Limit:  limit,
		Offset: offset,
	}

	if ratingStr != "" {
		if rVal, err := strconv.Atoi(ratingStr); err == nil && rVal >= 1 && rVal <= 5 {
			filter.Rating = &rVal
		}
	}

	var reviews []*org.Review
	var totalCount int
	var stats *org.AdminReviewStats

	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		var err error
		reviews, totalCount, err = h.orgSvc.ListAdminReviewsWithTotal(sysCtx, filter)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to list admin reviews", "error", err)
		}
		stats, err = h.orgSvc.GetAdminReviewStats(sysCtx)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to get admin review stats", "error", err)
		}
	}

	data := pages.AdminReviewsPageData{
		Reviews:      reviews,
		Stats:        stats,
		SearchQuery:  q,
		RatingFilter: ratingStr,
		StatusFilter: statusStr,
		Page:         page,
		PerPage:      limit,
		TotalCount:   totalCount,
	}

	h.renderPage(ctx, w, "render admin reviews page", pages.AdminReviewsPage(data, lang, dir))
}

// AdminReviewStatusSubmit toggles a review between approved and hidden/pending.
func (h *UIHandler) AdminReviewStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	reviewID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || reviewID <= 0 {
		h.redirectWithNotice(w, r, "/admin/reviews", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	approved := r.PostFormValue("approved") == "true"

	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		if err := h.orgSvc.UpdateReviewStatus(sysCtx, reviewID, approved); err != nil {
			h.log.ErrorContext(ctx, "failed to update review status", "error", err, "review_id", reviewID)
			h.redirectWithNotice(w, r, "/admin/reviews", "error", h.safeMessage(err, lang))
			return
		}
	}

	actor, _ := authctx.From(ctx)
	h.log.InfoContext(ctx, "admin updated review status", "actor_id", actor.UserID, "review_id", reviewID, "approved", approved)

	h.redirectWithNotice(w, r, "/admin/reviews", "success", i18n.T(lang, "common.saved"))
}

// AdminReviewDeleteSubmit soft-deletes a review from the platform.
func (h *UIHandler) AdminReviewDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	reviewID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || reviewID <= 0 {
		h.redirectWithNotice(w, r, "/admin/reviews", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		if err := h.orgSvc.SoftDeleteReview(sysCtx, reviewID); err != nil {
			h.log.ErrorContext(ctx, "failed to delete review", "error", err, "review_id", reviewID)
			h.redirectWithNotice(w, r, "/admin/reviews", "error", h.safeMessage(err, lang))
			return
		}
	}

	actor, _ := authctx.From(ctx)
	h.log.InfoContext(ctx, "admin deleted review", "actor_id", actor.UserID, "review_id", reviewID)

	h.redirectWithNotice(w, r, "/admin/reviews", "success", i18n.T(lang, "common.deleted"))
}
