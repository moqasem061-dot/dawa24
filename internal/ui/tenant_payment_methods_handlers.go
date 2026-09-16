package ui

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// TenantPaymentMethodAddSubmit saves a new payment method from the dedicated wallet page.
func (h *UIHandler) TenantPaymentMethodAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "payment.service_unavailable"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "common.invalid_form_data"))
		return
	}
	in, err := readPaymentMethodForm(r)
	if err != nil {
		h.redirectWithNotice(w, r, dest, "error", err.Error())
		return
	}

	pm := &billing.UserPaymentMethod{
		UserID:            actor.UserID,
		Provider:          in.Provider,
		AccountIdentifier: in.Identifier,
		Details:           in.Details,
		IsDefault:         r.PostFormValue("is_default") == "1",
	}

	if err := h.billSvc.AddPaymentMethod(ctx, pm); err != nil {
		h.log.ErrorContext(ctx, "failed to add payment method", "error", err)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "payment.created_success"))
}

// TenantPaymentMethodDeleteSubmit deletes a saved payment method from the dedicated wallet page.
func (h *UIHandler) TenantPaymentMethodDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "payment.invalid_id"))
		return
	}

	if h.billSvc != nil {
		_, candidateUIDs := resolveTenantUserIDs(ctx, h, actor)
		deleted := false
		for _, uid := range candidateUIDs {
			if err := h.billSvc.DeletePaymentMethod(ctx, uid, id); err == nil {
				deleted = true
				break
			}
		}
		if !deleted {
			h.log.ErrorContext(ctx, "failed to delete payment method", "id", id)
			h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "payment.invalid_id"))
			return
		}
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "payment.deleted_success"))
}

// resolveTenantUserIDs returns the primary wallet user ID and all candidate user IDs for the tenant org.
func resolveTenantUserIDs(ctx context.Context, h *UIHandler, actor authctx.Actor) (int64, []int64) {
	walletUserID := actor.UserID
	seen := make(map[int64]bool)
	var list []int64

	addUID := func(uid int64) {
		if uid > 0 && !seen[uid] {
			seen[uid] = true
			list = append(list, uid)
		}
	}
	addUID(actor.UserID)

	if actor.OrganizationID > 0 && h.orgSvc != nil {
		// 1. Prioritize organization owner
		if org, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID); err == nil && org != nil && org.OwnerID > 0 {
			walletUserID = org.OwnerID
			addUID(org.OwnerID)
		}

		// 2. Scan employees, strictly prioritizing org_owner / owner
		if emps, err := h.orgSvc.ListEmployees(ctx, actor.OrganizationID); err == nil {
			var ownerUID int64
			var adminUID int64
			for _, emp := range emps {
				if emp != nil && emp.Member != nil {
					addUID(emp.Member.UserID)
					if emp.Member.RoleKey == "org_owner" || emp.Member.RoleKey == "owner" {
						if ownerUID == 0 {
							ownerUID = emp.Member.UserID
						}
					} else if emp.Member.RoleKey == "org_admin" {
						if adminUID == 0 {
							adminUID = emp.Member.UserID
						}
					}
				}
			}
			if ownerUID > 0 {
				walletUserID = ownerUID
			} else if adminUID > 0 && walletUserID == actor.UserID {
				walletUserID = adminUID
			}
		}
	}
	addUID(walletUserID)
	return walletUserID, list
}
