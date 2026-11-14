package ui

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/erpgate"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// S5: the organization number a buyer gives a supplier is the ERP customer code.
// Every writer of that number runs through erpGuardCustomerCode, so the code is
// checked and written in one transaction under the supplier's lock (D2).

// erpGuardCustomerCode runs write only when the code is free. Without an ERP
// service it runs write as it did before S5.
func (h *UIHandler) erpGuardCustomerCode(ctx context.Context, vendorOrgID int64, customerOrgID *int64, userID int64, code string, excludeLinkID int64, write func(context.Context) error) error {
	if h.erpSvc == nil {
		return write(ctx)
	}
	return h.erpSvc.GuardCustomerCodeWrite(ctx, vendorOrgID, customerOrgID, userID, code, excludeLinkID, write)
}

// erpCodeTaken reports whether err is the guard's refusal: another customer
// already holds the code with this supplier.
func erpCodeTaken(err error) bool {
	var coded *erp.Error
	return errors.As(err, &coded) && coded.Code == "ERP_CODE_TAKEN"
}

// erpOn reports whether the checkout should treat suppliers as ERP-connected:
// the service exists on this deployment and an admin has not switched the
// integration off. With it off, no buyer is asked for a customer code.
func (h *UIHandler) erpOn(ctx context.Context) bool {
	return h.erpSvc != nil && erpgate.Enabled(ctx)
}

// erpBuyerOrg is the buyer organization of the actor, or 0 for a user with none.
func erpBuyerOrg(actor authctx.Actor) int64 {
	if actor.OrganizationID > 0 {
		return actor.OrganizationID
	}
	return actor.OrgID
}

// erpMissingCodeNeeds lists the ERP suppliers in the cart that the buyer has no
// customer code for. The checkout page and the checkout plan both read it here,
// so neither trusts a query string.
func (h *UIHandler) erpMissingCodeNeeds(ctx context.Context, actor authctx.Actor, cart *commerce.Cart, lang string) ([]pages.ErpCodeNeed, error) {
	if !h.erpOn(ctx) || cart == nil {
		return nil, nil
	}
	var ids []int64
	names := map[int64]string{}
	for _, it := range cart.Items {
		if it.OrganizationID <= 0 {
			continue
		}
		if _, seen := names[it.OrganizationID]; !seen {
			ids = append(ids, it.OrganizationID)
			names[it.OrganizationID] = it.SupplierName.Get(i18n.Lang(lang))
		}
	}
	missing, err := h.erpSvc.MissingCodes(ctx, erpBuyerOrg(actor), actor.UserID, ids)
	if err != nil {
		return nil, err
	}
	needs := make([]pages.ErpCodeNeed, 0, len(missing))
	for _, id := range missing {
		needs = append(needs, pages.ErpCodeNeed{VendorOrgID: id, SupplierName: names[id]})
	}
	return needs, nil
}

// erpMissingCodeFailure stops a checkout while an ERP supplier has no customer
// code for this buyer. The checkout page then opens the code modal (SC-009: a
// supplier without ERP is never asked).
func (h *UIHandler) erpMissingCodeFailure(ctx context.Context, actor authctx.Actor, items []commerce.CheckoutLineItem) *checkoutFailure {
	if !h.erpOn(ctx) {
		return nil
	}
	var ids []int64
	seen := map[int64]bool{}
	for _, it := range items {
		if it.VendorOrgID > 0 && !seen[it.VendorOrgID] {
			seen[it.VendorOrgID] = true
			ids = append(ids, it.VendorOrgID)
		}
	}
	missing, err := h.erpSvc.MissingCodes(ctx, erpBuyerOrg(actor), actor.UserID, ids)
	if err != nil {
		return &checkoutFailure{Back: "/checkout", Err: err}
	}
	if len(missing) > 0 {
		return &checkoutFailure{Back: "/checkout?erp_code=1"}
	}
	return nil
}

// CheckoutErpCodeSubmit saves the customer number a buyer gives one ERP supplier
// from the checkout modal (POST /checkout/erp-customer-code). The cart is not
// touched. A number another customer holds is refused with a notice.
func (h *UIHandler) CheckoutErpCodeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !erpgate.Enabled(ctx) {
		http.NotFound(w, r)
		return
	}
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}
	lang := langOf(r)

	vendorOrgID, err := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("vendor_org_id")), 10, 64)
	code := strings.TrimSpace(r.PostFormValue("organization_number"))
	if err != nil || vendorOrgID <= 0 || code == "" {
		redirectCheckoutNotice(w, r, "error", i18n.T(lang, "customer.userorg.org_number_required"))
		return
	}
	if h.orgSvc == nil {
		http.Redirect(w, r, "/checkout?erp_code=1", http.StatusSeeOther)
		return
	}

	customerOrg := customerOrgFor(actor)
	sysCtx := database.AsSystem(ctx)
	links, err := h.orgSvc.ListUserOrganizationsByUser(sysCtx, actor.UserID)
	if err != nil {
		h.log.ErrorContext(ctx, "checkout erp code: list links", "error", err)
		redirectCheckoutNotice(w, r, "error", h.safeMessage(err, lang))
		return
	}
	var existingID int64
	for _, l := range links {
		if l.VendorOrgID == vendorOrgID {
			existingID = l.ID
			break
		}
	}

	err = h.erpGuardCustomerCode(ctx, vendorOrgID, customerOrg, actor.UserID, code, existingID, func(txCtx context.Context) error {
		if existingID > 0 {
			return h.orgSvc.UpdateUserOrgLink(database.AsSystem(txCtx), existingID, code, "")
		}
		_, err := h.orgSvc.CreateUserOrgLink(database.AsSystem(txCtx), actor.UserID, customerOrg, vendorOrgID, code, org.UserOrgStatusPending)
		return err
	})
	switch {
	case erpCodeTaken(err):
		redirectCheckoutNotice(w, r, "error", i18n.T(lang, "customer.userorg.number_taken"))
	case err != nil:
		h.log.ErrorContext(ctx, "checkout erp code: save failed", "error", err)
		redirectCheckoutNotice(w, r, "error", h.safeMessage(err, lang))
	default:
		http.Redirect(w, r, "/checkout", http.StatusSeeOther)
	}
}

// customerOrgFor is the customer organization pointer a link carries, nil for a
// user with no organization.
func customerOrgFor(actor authctx.Actor) *int64 {
	orgID := erpBuyerOrg(actor)
	if orgID <= 0 {
		return nil
	}
	return &orgID
}

func redirectCheckoutNotice(w http.ResponseWriter, r *http.Request, kind, msg string) {
	target := "/checkout?notice_type=" + url.QueryEscape(kind) + "&notice_msg=" + url.QueryEscape(msg)
	http.Redirect(w, r, target, http.StatusSeeOther)
}
