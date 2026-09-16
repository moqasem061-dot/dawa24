package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorReviewsPage renders all reviews submitted by pharmacies for this vendor.
func (h *UIHandler) VendorReviewsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/reviews", http.StatusSeeOther)
		return
	}

	viewData := pages.VendorReviewsViewData{}
	if h.orgSvc != nil {
		reviews, err := h.orgSvc.ListReviewsForVendor(ctx, actor.OrganizationID, 100, 0)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to list vendor reviews", "error", err, "vendor_org_id", actor.OrganizationID)
		} else {
			viewData.Reviews = reviews
			viewData.TotalCount = len(reviews)
			if viewData.TotalCount > 0 {
				totalScore := 0
				totalRep := 0
				totalQuality := 0
				totalSpeed := 0
				for _, rv := range reviews {
					totalScore += rv.Rating
					if rv.ScoreRep > 0 {
						totalRep += rv.ScoreRep
					} else {
						totalRep += rv.Rating
					}
					if rv.ScoreQuality > 0 {
						totalQuality += rv.ScoreQuality
					} else {
						totalQuality += rv.Rating
					}
					if rv.ScoreSpeed > 0 {
						totalSpeed += rv.ScoreSpeed
					} else {
						totalSpeed += rv.Rating
					}
				}
				viewData.AverageRating = float64(totalScore) / float64(viewData.TotalCount)
				viewData.AvgRep = float64(totalRep) / float64(viewData.TotalCount)
				viewData.AvgQuality = float64(totalQuality) / float64(viewData.TotalCount)
				viewData.AvgSpeed = float64(totalSpeed) / float64(viewData.TotalCount)
			}
		}
	}

	h.renderPage(ctx, w, "render vendor reviews page", pages.VendorReviewsPage(viewData, lang, dir))
}

// VendorReviewReplySubmit records or updates the vendor's official reply to a pharmacy review.
func (h *UIHandler) VendorReviewReplySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/reviews", http.StatusSeeOther)
		return
	}

	reviewID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || reviewID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/reviews", "error", i18n.T(langOf(r), "common.invalid_id"))
		return
	}

	_ = r.ParseForm()
	replyText := strings.TrimSpace(r.PostFormValue("response"))
	if replyText == "" {
		h.redirectWithNotice(w, r, "/vendor/reviews", "error", "يرجى كتابة نص الرد قبل الإرسال.")
		return
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.ReplyToReview(ctx, reviewID, actor.OrganizationID, replyText, actor.UserID); err != nil {
			h.log.ErrorContext(ctx, "failed to reply to review", "error", err, "review_id", reviewID)
			h.redirectWithNotice(w, r, "/vendor/reviews", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/reviews", "success", "تم نشر ردك الرسمي على تقييم الصيدلية بنجاح.")
}
