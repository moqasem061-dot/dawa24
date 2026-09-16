package ui

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// translateSmartOrderError turns a domain error into something a pharmacist can
// act on, rather than a generic or technical error code.
func translateSmartOrderError(err error, langOptional ...string) string {
	if err == nil {
		return ""
	}
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}

	var appErr *apperr.Error
	if errors.As(err, &appErr) && appErr.Msg != "" {
		msg := appErr.Msg
		switch {
		case strings.Contains(msg, "branch_required") || strings.Contains(msg, "branch_invalid") || strings.Contains(msg, i18n.TDefault("w4_ui.s_102_102")):
			return i18n.T(lang, "smartorder.err_branch_required")
		case strings.Contains(msg, "branch_no_location"):
			return i18n.T(lang, "smartorder.err_branch_no_location")
		case strings.Contains(msg, "branch_not_owned"):
			return i18n.T(lang, "smartorder.err_branch_not_owned")
		case strings.Contains(msg, "nothing_to_order"):
			return i18n.T(lang, "smartorder.err_nothing_to_order")
		case strings.Contains(msg, "customer_required"):
			return i18n.T(lang, "smartorder.err_customer_required")
		case strings.Contains(msg, "empty_cart"):
			return i18n.T(lang, "smartorder.err_empty_cart")
		case strings.Contains(msg, "missing_documents") || strings.Contains(msg, "documents"):
			return i18n.T(lang, "smartorder.err_missing_documents")
		case strings.Contains(msg, "min_order_not_met"):
			return i18n.T(lang, "smartorder.err_min_order_not_met")
		case strings.Contains(msg, "branch_institutional_mismatch"):
			return i18n.T(lang, "smartorder.blocked_institutional_hint")
		case strings.Contains(msg, "branch_no_institutional_works"):
			return i18n.T(lang, "smartorder.err_branch_no_institutional_works")
		case strings.Contains(msg, "line_unavailable") || strings.Contains(msg, "not_covered") || strings.Contains(msg, i18n.TDefault("w4_ui.s_103_103")):
			return i18n.T(lang, "smartorder.err_line_unavailable")
		case strings.Contains(msg, "out_of_stock") || strings.Contains(msg, i18n.TDefault("w4_ui.s_104_104")):
			return i18n.T(lang, "smartorder.err_out_of_stock")
		case strings.Contains(msg, "insufficient_stock"):
			return i18n.T(lang, "smartorder.err_insufficient_stock")
		case strings.Contains(msg, "below_minimum"):
			return i18n.T(lang, "smartorder.err_below_minimum")
		case strings.Contains(msg, "mapping_incomplete"):
			return i18n.T(lang, "smartorder.err_mapping_incomplete")
		case strings.Contains(msg, "already_finalized"):
			return i18n.T(lang, "smartorder.err_already_finalized")
		case strings.Contains(msg, "stale"):
			return i18n.T(lang, "smartorder.err_stale")
		}
		return msg
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "branch_required") || strings.Contains(msg, "branch_invalid") || strings.Contains(msg, i18n.TDefault("w4_ui.s_102_102")):
		return i18n.T(lang, "smartorder.err_branch_required")
	case strings.Contains(msg, "branch_no_location"):
		return i18n.T(lang, "smartorder.err_branch_no_location")
	case strings.Contains(msg, "branch_not_owned"):
		return i18n.T(lang, "smartorder.err_branch_not_owned")
	case strings.Contains(msg, "mapping_incomplete"):
		return i18n.T(lang, "smartorder.err_mapping_incomplete")
	case strings.Contains(msg, "already_finalized"):
		return i18n.T(lang, "smartorder.err_already_finalized")
	case strings.Contains(msg, "stale"):
		return i18n.T(lang, "smartorder.err_stale")
	case strings.Contains(msg, "nothing_to_order"):
		return i18n.T(lang, "smartorder.err_nothing_to_order")
	case strings.Contains(msg, "customer_required"):
		return i18n.T(lang, "smartorder.err_customer_required")
	case strings.Contains(msg, "empty_cart"):
		return i18n.T(lang, "smartorder.err_empty_cart")
	case strings.Contains(msg, "missing_documents") || strings.Contains(msg, "documents"):
		return i18n.T(lang, "smartorder.err_missing_documents")
	case strings.Contains(msg, "min_order_not_met"):
		return i18n.T(lang, "smartorder.err_min_order_not_met")
	case strings.Contains(msg, "branch_institutional_mismatch"):
		return i18n.T(lang, "smartorder.blocked_institutional_hint")
	case strings.Contains(msg, "branch_no_institutional_works"):
		return i18n.T(lang, "smartorder.err_branch_no_institutional_works")
	case strings.Contains(msg, "line_unavailable") || strings.Contains(msg, "not_covered") || strings.Contains(msg, i18n.TDefault("w4_ui.s_103_103")):
		return i18n.T(lang, "smartorder.err_line_unavailable")
	case strings.Contains(msg, "out_of_stock") || strings.Contains(msg, i18n.TDefault("w4_ui.s_104_104")):
		return i18n.T(lang, "smartorder.err_out_of_stock")
	case strings.Contains(msg, "insufficient_stock"):
		return i18n.T(lang, "smartorder.err_insufficient_stock")
	case strings.Contains(msg, "below_minimum"):
		return i18n.T(lang, "smartorder.err_below_minimum")
	}
	return fmt.Sprintf(i18n.T(lang, "smartorder.err_operation_failed_format"), msg)
}

// SmartOrderLegacyRedirect handles backward-compatible URLs like:
// /customer/smart-order/review?run_id=64 or /customer/smart-order/results?run_id=64
// and redirects to the canonical RESTful URL: /customer/smart-order/{public_id}/results
func (h *UIHandler) SmartOrderLegacyRedirect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	if h.smartOrderSvc == nil {
		http.Redirect(w, r, "/customer/smart-order/history", http.StatusSeeOther)
		return
	}

	q := r.URL.Query()
	runParam := strings.TrimSpace(q.Get("run_id"))
	if runParam == "" {
		runParam = strings.TrimSpace(q.Get("id"))
	}
	if runParam == "" {
		http.Redirect(w, r, "/customer/smart-order/history", http.StatusSeeOther)
		return
	}

	var run *smartorder.Run
	if numID, err := strconv.ParseInt(runParam, 10, 64); err == nil && numID > 0 {
		run, _ = h.smartOrderSvc.GetByID(ctx, actor.OrganizationID, numID)
	}
	if run == nil {
		run, _ = h.smartOrderSvc.Get(ctx, actor.OrganizationID, runParam)
	}
	if run == nil {
		http.Redirect(w, r, "/customer/smart-order/history", http.StatusSeeOther)
		return
	}

	subpath := "results"
	if run.Status == smartorder.StatusProcessing || run.Status == smartorder.StatusFinalizing || run.Status == smartorder.StatusQueued {
		subpath = "progress"
	} else if run.Status == smartorder.StatusDraft || run.Status == smartorder.StatusMapping {
		subpath = "mapping"
	}

	http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/"+subpath, http.StatusSeeOther)
}
