package ui

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

const (
	// RevisionConflictNoticeAr is the Arabic notice displayed on optimistic concurrency mismatch.
	RevisionConflictNoticeAr = "تعارض في التعديل: تم تحديث هذا الصنف من مراجع آخر"
	// RevisionConflictNoticeEn is the English notice displayed on optimistic concurrency mismatch.
	RevisionConflictNoticeEn = "Revision conflict: this row was updated by another reviewer"
	// RevisionConflictNoticeBilingual is the bilingual combined notice.
	RevisionConflictNoticeBilingual = "تعارض في التعديل: تم تحديث هذا الصنف من مراجع آخر (Revision conflict: updated by another reviewer)"
)

// parseReviewAction extracts ReviewAction from an incoming HTTP form request.
func parseReviewAction(r *http.Request, row productmatch.Row, curDec matchflow.RowDecision) matching.ReviewAction {
	action := strings.TrimSpace(r.FormValue("action"))
	if action == "" {
		action = "select"
	}
	targetID, _ := strconv.ParseInt(r.FormValue("target_id"), 10, 64)
	if targetID == 0 {
		targetID, _ = strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	}
	expectedRev, _ := strconv.Atoi(r.FormValue("expected_rev"))
	if expectedRev == 0 {
		expectedRev = curDec.Revision
	}

	var corrections []matchflow.Correction
	strengthVal := strings.TrimSpace(r.FormValue("strength_val"))
	if strengthVal == "" {
		strengthVal = strings.TrimSpace(r.FormValue("corr_strength"))
	}
	strengthUnit := strings.TrimSpace(r.FormValue("strength_unit"))
	dosageForm := strings.TrimSpace(r.FormValue("dosage_form"))
	if dosageForm == "" {
		dosageForm = strings.TrimSpace(r.FormValue("corr_form"))
	}
	packUnit := strings.TrimSpace(r.FormValue("pack_unit"))
	if packUnit == "" {
		packUnit = strings.TrimSpace(r.FormValue("corr_pack"))
	}
	brand := strings.TrimSpace(r.FormValue("brand"))
	if brand == "" {
		brand = strings.TrimSpace(r.FormValue("corr_brand"))
	}
	reason := strings.TrimSpace(r.FormValue("correction_reason"))

	if strengthVal != "" || strengthUnit != "" {
		corrections = append(corrections, matchflow.Correction{
			Attribute: "strength",
			Value:     strings.TrimSpace(strengthVal + " " + strengthUnit),
			Reason:    reason,
		})
	}
	if dosageForm != "" {
		corrections = append(corrections, matchflow.Correction{
			Attribute: "dosage_form",
			Value:     dosageForm,
			Reason:    reason,
		})
	}
	if packUnit != "" {
		corrections = append(corrections, matchflow.Correction{
			Attribute: "pack_unit",
			Value:     packUnit,
			Reason:    reason,
		})
	}
	if brand != "" {
		corrections = append(corrections, matchflow.Correction{
			Attribute: "brand",
			Value:     brand,
			Reason:    reason,
		})
	}

	return matching.ReviewAction{
		Action:      action,
		TargetID:    targetID,
		ExpectedRev: expectedRev,
		Row:         row,
		CurrentDec:  curDec,
		Corrections: corrections,
		Reason:      reason,
	}
}

// executeReviewAction applies the reviewer action via matching service if wired,
// or performs an in-memory decision fallback.
func executeReviewAction(
	ctx context.Context,
	svc matching.Service,
	scope productmatch.Scope,
	act matching.ReviewAction,
) (matchflow.RowDecision, error) {
	if svc != nil {
		return svc.Review(ctx, scope, act)
	}

	// Fallback when matching service is not wired
	dec := act.CurrentDec
	dec.Revision = act.ExpectedRev + 1
	switch act.Action {
	case "select", "correct":
		dec.State = matchflow.StateConfirmed
		dec.Basis = matchflow.BasisReviewer
		if act.TargetID > 0 {
			dec.ProductID = &act.TargetID
		}
	case "reject":
		dec.State = matchflow.StateNoCandidate
		dec.Basis = matchflow.BasisReviewer
		dec.ProductID = nil
	case "exclude":
		dec.State = matchflow.StateExcluded
		dec.Basis = matchflow.BasisReviewer
	case "undo":
		dec.State = matchflow.StateNeedsReview
		dec.Basis = matchflow.BasisReviewer
		dec.ProductID = nil
	default:
		return dec, errors.New("unknown review action: " + act.Action)
	}
	return dec, nil
}
