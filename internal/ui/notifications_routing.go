package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

// ResolveNotificationDestination determines the optimal platform destination URL
// when a user clicks on a notification, based on the notification metadata, title, body,
// and the user's role and RBAC permissions.
func (h *UIHandler) ResolveNotificationDestination(ctx context.Context, actor authctx.Actor, notif *notifications.NotificationLog) string {
	if notif == nil {
		return fallbackDashboard(actor)
	}

	title := notif.Title
	body := notif.Body
	combined := strings.ToLower(title + " " + body)

	// 1. Direct explicit URL embedded in notification body
	if url := extractExplicitURL(body); url != "" {
		if isAllowedDestination(actor, url) {
			return url
		}
	}

	// 2. Account & Organization Deletions
	if isDeletionNotification(combined) {
		if strings.Contains(combined, "منشأة") || strings.Contains(combined, "org.deletion") {
			if actor.IsSuperAdmin() || actor.IsStaff {
				return "/admin/deletion-requests?tab=organizations"
			}
			if isVendor(actor) {
				return "/vendor/organization"
			}
			return "/customer/organization"
		}
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/deletion-requests?tab=users"
		}
		return "/settings?tab=security"
	}

	// 3. Orders & Commerce
	if isOrderNotification(combined, notif.RequiredPermission) {
		if dest := h.resolveOrderOrDeliveryDestination(ctx, actor, combined, false); dest != "" {
			return dest
		}
	}

	// 4. Delivery & Courier
	if isDeliveryNotification(combined, notif.RequiredPermission, actor) {
		if dest := h.resolveOrderOrDeliveryDestination(ctx, actor, combined, true); dest != "" {
			return dest
		}
	}

	// 5. Purchase Requests & RFQs
	if isPurchaseRequestNotification(combined, notif.RequiredPermission) {
		if actor.Can("vendor.purchase_request.view") || actor.Role == "vendor" {
			if reqID := extractRequestID(combined); reqID != "" {
				return "/vendor/purchase-requests/" + reqID
			}
			return "/vendor/purchase-requests"
		}
		if actor.Can("pharmacy.order.view") || actor.Role == "customer" || actor.Role == "pharmacy" {
			return "/customer/purchase-request"
		}
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/orders"
		}
	}

	// 6. Wallet & Financials (Deposits, Withdrawals, Refunds)
	if isWalletNotification(combined, notif.RequiredPermission) {
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/finance/wallets"
		}
		if isVendor(actor) {
			if actor.Can("vendor.wallet.view") {
				return "/vendor/wallet"
			}
			return "/vendor/dashboard"
		}
		return "/customer/wallet"
	}

	// 7. Platform Admin Approvals, Registration & Profile Changes
	if isApprovalNotification(combined, notif.RequiredPermission) {
		if actor.IsSuperAdmin() || actor.Can("org.approval.view") {
			if strings.Contains(combined, "طلب تعديل") || strings.Contains(combined, "profile_change") {
				return "/admin/approvals?tab=requests"
			}
			return "/admin/approvals?tab=organizations"
		}
		if isVendor(actor) {
			return "/vendor/organization"
		}
		return "/customer/organization"
	}

	// 8. Job Seekers & HR
	if isJobNotification(combined) {
		if actor.IsSuperAdmin() || actor.Can("org.approval.view") {
			return "/admin/approvals?tab=job_seekers"
		}
		return "/jobs"
	}

	// 9. Documents & Verification
	if isDocumentNotification(combined, notif.RequiredPermission) {
		if actor.IsSuperAdmin() || actor.Can("hr.document.view") {
			return "/admin/approvals?tab=documents"
		}
		if isVendor(actor) && actor.Can("vendor.document.view") {
			return "/vendor/documents"
		}
		return "/customer/documents"
	}

	// 10. Smart Order
	if strings.Contains(combined, "الطلب الذكي") || strings.Contains(combined, "smart_order") || strings.Contains(combined, "smart order") {
		return "/customer/smart-order"
	}

	// 11. Vendor Ingest / Catalog Import
	if isCatalogImportNotification(combined) {
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/products/import"
		}
		if isVendor(actor) {
			return "/vendor/ingest"
		}
		return "/customer/saving-products"
	}

	// 12. Promotions, Sponsorships, Offers & Ads
	if isPromoNotification(combined, notif.RequiredPermission) {
		if actor.IsSuperAdmin() || actor.IsStaff {
			if strings.Contains(combined, "إعلان") || strings.Contains(combined, "ad.") {
				return "/admin/offers-packages?tab=ads"
			}
			if strings.Contains(combined, "باقة رعاية") || strings.Contains(combined, "sponsorship") {
				return "/admin/offers-packages/sponsorships"
			}
			return "/admin/offers-packages"
		}
		if isVendor(actor) {
			if strings.Contains(combined, "إعلان") || strings.Contains(combined, "ad.") {
				return "/vendor/ads"
			}
			if strings.Contains(combined, "باقة رعاية") || strings.Contains(combined, "sponsorship") {
				return "/vendor/offers-packages/sponsorships"
			}
			return "/vendor/offers"
		}
		return "/customer/dashboard"
	}

	// 13. Subscriptions & Sessions
	if strings.Contains(combined, "اشتراك") || strings.Contains(combined, "باقة") || strings.Contains(combined, "subscription") {
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/plans?tab=subscriptions"
		}
		if isVendor(actor) {
			return "/vendor/subscription"
		}
		return "/customer/subscription"
	}

	// 14. Team, Branches & Roles
	if isTeamBranchNotification(combined) {
		if isVendor(actor) {
			if strings.Contains(combined, "فرع") || strings.Contains(combined, "quota") {
				return "/vendor/branches"
			}
			return "/vendor/team"
		}
		if strings.Contains(combined, "فرع") || strings.Contains(combined, "quota") {
			return "/customer/branches"
		}
		return "/customer/team"
	}

	// 15. Reviews & Ratings
	if strings.Contains(combined, "تقييم") || strings.Contains(combined, "review") {
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/reviews"
		}
		if isVendor(actor) {
			return "/vendor/reviews"
		}
		return "/orders"
	}

	// 16. Chat Messages
	if strings.Contains(combined, "محادثة") || strings.Contains(combined, "رسالة") || strings.Contains(combined, "chat") {
		return "/chat"
	}

	// 17. Security & Password Changes
	if strings.Contains(combined, "أمان") || strings.Contains(combined, "كلمة المرور") || strings.Contains(combined, "security") || strings.Contains(combined, "password") {
		return "/settings?tab=security"
	}

	return fallbackDashboard(actor)
}

func (h *UIHandler) resolveOrderOrDeliveryDestination(ctx context.Context, actor authctx.Actor, text string, isDelivery bool) string {
	orderNum, shipmentNum := extractOrderAndShipmentIdentifiers(text)

	if (orderNum != "" || shipmentNum != "") && h.commSvc != nil {
		sysCtx := database.AsSystem(ctx)
		lookupNum := orderNum
		if lookupNum == "" {
			lookupNum = shipmentNum
		}
		if ord, err := h.commSvc.GetOrderByNumber(sysCtx, lookupNum); err == nil && ord != nil && ord.ID > 0 {
			if actor.IsSuperAdmin() || actor.IsStaff {
				return fmt.Sprintf("/admin/orders/%d", ord.ID)
			}
			if isVendor(actor) {
				for _, sh := range ord.Shipments {
					if sh != nil {
						if shipmentNum != "" && sh.ShipmentNumber == shipmentNum {
							return fmt.Sprintf("/vendor/orders/%d", sh.ID)
						}
						if sh.OrganizationID == actor.OrganizationID {
							return fmt.Sprintf("/vendor/orders/%d", sh.ID)
						}
					}
				}
				// A supplier that bought this order sees it on the buying
				// surface; /vendor/orders/{id} takes a shipment it sells.
				if ord.OrganizationID != nil && *ord.OrganizationID == actor.OrganizationID {
					return fmt.Sprintf("/orders/%d", ord.ID)
				}
				return fmt.Sprintf("/vendor/orders/%d", ord.ID)
			}
			if actor.Role == "courier" || actor.Role == "org_courier" {
				for _, sh := range ord.Shipments {
					if sh != nil && (shipmentNum == "" || sh.ShipmentNumber == shipmentNum) {
						return fmt.Sprintf("/vendor/delivery/%d", sh.ID)
					}
				}
				return fmt.Sprintf("/vendor/delivery/%d", ord.ID)
			}
			return fmt.Sprintf("/orders/%d", ord.ID)
		}
	}

	// Direct numeric fallback if database lookup returned not found or was unavailable
	idNum := ""
	if orderNum != "" && isAllDigits(orderNum) {
		idNum = orderNum
	}
	if idNum != "" {
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/orders/" + idNum
		}
		if isVendor(actor) {
			return "/vendor/orders/" + idNum
		}
		if actor.Role == "courier" || actor.Role == "org_courier" {
			return "/vendor/delivery/" + idNum
		}
		return "/orders/" + idNum
	}

	if actor.IsSuperAdmin() || actor.IsStaff {
		return "/admin/orders"
	}
	if actor.Role == "courier" || actor.Role == "org_courier" {
		return "/vendor/delivery"
	}
	if isVendor(actor) {
		if isDelivery && actor.Can("vendor.delivery.view") && !actor.Can("vendor.order.view") {
			return "/vendor/delivery"
		}
		if actor.Can("vendor.order.view") {
			return "/vendor/orders"
		}
		if actor.Can(rbac.BuyOrderView.Vendor) {
			return "/orders"
		}
		return "/vendor/dashboard"
	}
	// Customer / Buyer ALWAYS lands on /orders, NEVER /vendor/delivery
	return "/orders"
}
