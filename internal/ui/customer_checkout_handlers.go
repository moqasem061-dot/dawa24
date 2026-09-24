package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func (h *UIHandler) CustomerCheckoutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	if !actor.IsBuyer() {
		h.redirectWithNotice(w, r, "/catalog", "error", i18n.T(lang, "checkout.pharmacy_only"))
		return
	}

	userID := actor.UserID

	if h.commSvc == nil {
		h.renderPage(ctx, w, "render checkout page", pages.CustomerCheckout(nil, nil, nil, nil, nil, lang, dir))
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID, buyerOrgID(ctx))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if cart == nil || len(cart.Items) == 0 {
		h.redirectWithNotice(w, r, "/cart", "info", i18n.TDefault("سلة المشتريات فارغة. الرجاء إضافة منتجات قبل إتمام الشراء."))
		return
	}

	var branches []*pages.CheckoutBranchItem
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if bList, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err != nil {
			h.log.WarnContext(ctx, "checkout: list customer branches", "error", err)
		} else {
			rawBranches := filterCheckoutBranches(bList, actor)
			branches = make([]*pages.CheckoutBranchItem, 0, len(rawBranches))
			for _, b := range rawBranches {
				if b == nil {
					continue
				}
				isAvail, reason := h.evaluateBranchCartAvailability(ctx, actor, lang, cart, b.ID)
				branches = append(branches, &pages.CheckoutBranchItem{
					Branch:      b,
					IsAvailable: isAvail,
					Reason:      reason,
				})
			}
		}
	}

	if buying, has := authctx.BuyingBranchFrom(ctx); has && buying.Active != nil && !buying.IsLocked {
		activeIsAvail := false
		var firstAvailID int64
		for _, b := range branches {
			if b.IsAvailable {
				if firstAvailID == 0 {
					firstAvailID = b.Branch.ID
				}
				if b.Branch.ID == *buying.Active {
					activeIsAvail = true
					break
				}
			}
		}
		if !activeIsAvail && firstAvailID > 0 {
			http.SetCookie(w, &http.Cookie{
				Name:     buyingBranchCookie,
				Value:    strconv.FormatInt(firstAvailID, 10),
				Path:     "/",
				MaxAge:   60 * 60 * 24 * 30,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			newActive := firstAvailID
			buying.Active = &newActive
			ctx = authctx.WithBuyingBranch(ctx, buying)
		}
	}

	var checkoutMethods []*billing.PlatformPaymentMethod
	var userPaymentMethods []*billing.UserPaymentMethod
	var walletCheckoutAllowed bool
	var wallet *billing.Wallet
	if h.billSvc != nil {
		if pms, err := h.billSvc.ListPlatformPaymentMethods(ctx, true); err != nil {
			h.logLoadErr(ctx, "billSvc.ListPlatformPaymentMethods", err)
		} else {
			for _, pm := range pms {
				if pm != nil && pm.IsActive && pm.IsCheckoutEnabled {
					checkoutMethods = append(checkoutMethods, pm)
					if pm.ID == "wallet" {
						walletCheckoutAllowed = true
					}
				}
			}
		}

		walletUserID, candidateUIDs := resolveTenantUserIDs(ctx, h, actor)
		seenPM := make(map[int64]bool)
		for _, uid := range candidateUIDs {
			if upms, err := h.billSvc.ListPaymentMethods(ctx, uid); err != nil {
				h.logLoadErr(ctx, "billSvc.ListPaymentMethods", err)
			} else {
				for _, upm := range upms {
					if upm != nil && !seenPM[upm.ID] {
						seenPM[upm.ID] = true
						userPaymentMethods = append(userPaymentMethods, upm)
					}
				}
			}
		}

		if walletCheckoutAllowed && actor.OrganizationID > 0 {
			if wItem, err := h.billSvc.GetWallet(ctx, walletUserID, "EGP"); err != nil {
				h.logLoadErr(ctx, "billSvc.GetWallet", err)
			} else {
				wallet = wItem
			}
		}
	}

	h.renderPage(ctx, w, "render checkout page", pages.CustomerCheckout(cart, branches, wallet, checkoutMethods, userPaymentMethods, lang, dir))
}

func (h *UIHandler) CheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)
	if _, err := authctx.UserID(ctx); err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}
	lang := langOf(r)

	formBranchID := strings.TrimSpace(r.PostFormValue("branch_id"))
	if formBranchID != "" && h.orgSvc != nil {
		if bID, err := strconv.ParseInt(formBranchID, 10, 64); err == nil && bID > 0 {
			if b, err := h.orgSvc.GetBranch(ctx, bID); err == nil && b != nil && b.OrganizationID == actor.OrganizationID && b.Status != "inactive" && b.Status != "suspended" {
				if buying, has := authctx.BuyingBranchFrom(ctx); !has || !buying.IsLocked {
					ctx = authctx.WithBuyingBranch(ctx, authctx.BuyingBranch{
						Active:   &bID,
						IsLocked: false,
					})
					http.SetCookie(w, &http.Cookie{
						Name:     buyingBranchCookie,
						Value:    formBranchID,
						Path:     "/",
						MaxAge:   60 * 60 * 24 * 30,
						HttpOnly: true,
						SameSite: http.SameSiteLaxMode,
					})
				}
			}
		}
	}

	var senderPMID int64
	if sIDStr := strings.TrimSpace(r.PostFormValue("sender_payment_method_id")); sIDStr != "" {
		if parsedID, err := strconv.ParseInt(sIDStr, 10, 64); err == nil { // nolint:errswallow: optional input; an unparseable value leaves the default
			senderPMID = parsedID
		}
	}

	plan, failure := h.planCheckout(ctx, actor, lang, checkoutRequest{
		FormBranchID:          formBranchID,
		PaymentMethod:         strings.TrimSpace(r.PostFormValue("payment_method")),
		SenderPaymentMethodID: senderPMID,
		PaymentReference:      strings.TrimSpace(r.PostFormValue("payment_reference")),
		Notes:                 r.PostFormValue("notes"),
	})
	if failure == nil {
		var order *commerce.Order
		order, failure = h.placeCheckout(ctx, actor, lang, plan)
		if failure == nil {
			http.Redirect(w, r, "/orders/"+strconv.FormatInt(order.ID, 10), http.StatusSeeOther)
			return
		}
	}
	switch {
	case failure.Err != nil:
		h.renderError(w, r, failure.Err)
	case failure.Message != "":
		h.redirectWithNotice(w, r, failure.Back, "error", failure.Message)
	default:
		http.Redirect(w, r, failure.Back, http.StatusSeeOther)
	}
}
