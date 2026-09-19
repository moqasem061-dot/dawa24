package ui

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

var (
	reOrderNumber = regexp.MustCompile(`\b(ORD-[A-Za-z0-9-]+)\b`)
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

	// 2. Orders & Commerce
	if isOrderNotification(combined, notif.RequiredPermission) {
		if dest := h.resolveOrderDestination(ctx, actor, combined); dest != "" {
			return dest
		}
	}

	// 3. Delivery & Courier
	if isDeliveryNotification(combined, notif.RequiredPermission, actor) {
		if actor.Role == "courier" || actor.Role == "org_courier" || actor.Can("vendor.delivery.view") {
			return "/vendor/delivery"
		}
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/orders"
		}
		return "/vendor/delivery"
	}

	// 4. Purchase Requests & RFQs
	if isPurchaseRequestNotification(combined, notif.RequiredPermission) {
		if actor.Can("vendor.purchase_request.view") || actor.Role == "vendor" {
			return "/vendor/purchase-requests"
		}
		if actor.Can("pharmacy.order.view") || actor.Role == "customer" || actor.Role == "pharmacy" {
			return "/customer/purchase-request"
		}
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/orders"
		}
	}

	// 5. Wallet & Financials (Deposits, Withdrawals, Refunds)
	if isWalletNotification(combined, notif.RequiredPermission) {
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/billing/wallets"
		}
		if isVendor(actor) {
			if actor.Can("vendor.wallet.view") {
				return "/vendor/wallet"
			}
			return "/vendor/dashboard"
		}
		return "/customer/wallet"
	}

	// 6. Platform Admin Approvals & Registration
	if isApprovalNotification(combined, notif.RequiredPermission) {
		if actor.IsSuperAdmin() || actor.Can("org.approval.view") {
			return "/admin/approvals"
		}
		if isVendor(actor) {
			return "/vendor/organization"
		}
		return "/customer/dashboard"
	}

	// 7. Job Seekers & HR
	if isJobNotification(combined) {
		if actor.IsSuperAdmin() || actor.Can("org.approval.view") {
			return "/admin/approvals/job-seekers"
		}
		return "/jobs"
	}

	// 8. Documents & Verification
	if isDocumentNotification(combined, notif.RequiredPermission) {
		if actor.IsSuperAdmin() || actor.Can("hr.document.view") {
			return "/admin/documents"
		}
		if isVendor(actor) && actor.Can("vendor.document.view") {
			return "/vendor/documents"
		}
		return "/documents"
	}

	// 9. Smart Order
	if strings.Contains(combined, "الطلب الذكي") || strings.Contains(combined, "smart_order") || strings.Contains(combined, "smart order") {
		return "/customer/smart-order"
	}

	// 10. Vendor Ingest / Catalog Import
	if isCatalogImportNotification(combined) {
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/products/import"
		}
		if isVendor(actor) {
			return "/vendor/ingest"
		}
		return "/customer/saving-products"
	}

	// 11. Promotions, Sponsorships, Offers & Ads
	if isPromoNotification(combined, notif.RequiredPermission) {
		if actor.IsSuperAdmin() || actor.IsStaff {
			if strings.Contains(combined, "إعلان") || strings.Contains(combined, "ad.") {
				return "/admin/ads"
			}
			return "/admin/sponsorships"
		}
		if isVendor(actor) {
			if strings.Contains(combined, "إعلان") || strings.Contains(combined, "ad.") {
				return "/vendor/ads"
			}
			return "/vendor/sponsorships"
		}
		return "/customer/dashboard"
	}

	// 12. Subscriptions & Sessions
	if strings.Contains(combined, "اشتراك") || strings.Contains(combined, "باقة") || strings.Contains(combined, "subscription") {
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/subscriptions"
		}
		if isVendor(actor) {
			return "/vendor/subscription"
		}
		return "/settings?tab=sessions"
	}

	// 13. Team, Branches & Roles
	if isTeamBranchNotification(combined) {
		if isVendor(actor) {
			if strings.Contains(combined, "فرع") {
				return "/vendor/branches"
			}
			return "/vendor/team"
		}
		if strings.Contains(combined, "فرع") {
			return "/customer/branches"
		}
		return "/customer/team"
	}

	// 14. Reviews & Ratings
	if strings.Contains(combined, "تقييم") || strings.Contains(combined, "review") {
		if actor.IsSuperAdmin() || actor.IsStaff {
			return "/admin/reviews"
		}
		if isVendor(actor) {
			return "/vendor/reviews"
		}
		return "/customer/dashboard"
	}

	// 15. Chat Messages
	if strings.Contains(combined, "محادثة") || strings.Contains(combined, "رسالة") || strings.Contains(combined, "chat") {
		return "/chat"
	}

	// 16. Security & Password Changes
	if strings.Contains(combined, "أمان") || strings.Contains(combined, "كلمة المرور") || strings.Contains(combined, "security") || strings.Contains(combined, "password") {
		return "/settings?tab=security"
	}

	return fallbackDashboard(actor)
}

func (h *UIHandler) resolveOrderDestination(ctx context.Context, actor authctx.Actor, text string) string {
	orderNum := ""
	if matches := reOrderNumber.FindStringSubmatch(text); len(matches) > 1 {
		orderNum = matches[1]
	}

	if orderNum != "" && h.commSvc != nil {
		sysCtx := database.AsSystem(ctx)
		if ord, err := h.commSvc.GetOrderByNumber(sysCtx, orderNum); err == nil && ord != nil && ord.ID > 0 {
			if actor.IsSuperAdmin() || actor.IsStaff {
				return fmt.Sprintf("/admin/orders/%d", ord.ID)
			}
			if isVendor(actor) {
				for _, sh := range ord.Shipments {
					if sh != nil && sh.OrganizationID == actor.OrganizationID {
						return fmt.Sprintf("/vendor/orders/%d", sh.ID)
					}
				}
				return fmt.Sprintf("/vendor/orders/%d", ord.ID)
			}
			if actor.Role == "courier" || actor.Role == "org_courier" {
				return fmt.Sprintf("/vendor/delivery/%d", ord.ID)
			}
			return fmt.Sprintf("/orders/%d", ord.ID)
		}
	}

	if actor.IsSuperAdmin() || actor.IsStaff {
		return "/admin/orders"
	}
	if isVendor(actor) {
		if actor.Can("vendor.order.view") {
			return "/vendor/orders"
		}
		return "/vendor/dashboard"
	}
	return "/orders"
}

func extractExplicitURL(text string) string {
	if strings.Contains(text, "http://") || strings.Contains(text, "https://") {
		parts := strings.Fields(text)
		for _, p := range parts {
			if strings.HasPrefix(p, "https://dawa24.com/") || strings.HasPrefix(p, "http://dawa24.com/") {
				idx := strings.Index(p, "dawa24.com")
				if idx != -1 {
					return p[idx+len("dawa24.com"):]
				}
			}
		}
	}
	return ""
}

func isAllowedDestination(actor authctx.Actor, url string) bool {
	if url == "" || !strings.HasPrefix(url, "/") {
		return false
	}
	if strings.HasPrefix(url, "/admin") && !actor.IsSuperAdmin() && !actor.IsStaff {
		return false
	}
	if strings.HasPrefix(url, "/vendor") && !isVendor(actor) && !actor.IsSuperAdmin() {
		return false
	}
	return true
}

func isVendor(actor authctx.Actor) bool {
	return actor.IsVendor() || actor.DashboardScope() == rbac.ScopeVendor || actor.Role == "vendor" || actor.OrgType == "vendor" || actor.OrgType == "supplier"
}

func fallbackDashboard(actor authctx.Actor) string {
	if actor.IsSuperAdmin() || actor.IsStaff {
		return "/admin/dashboard"
	}
	if isVendor(actor) {
		return "/vendor/dashboard"
	}
	if actor.DashboardScope() == rbac.ScopePharmacy || actor.IsCustomer() {
		return "/customer/dashboard"
	}
	return "/notifications"
}

func isOrderNotification(text, perm string) bool {
	return perm == "vendor.order.view" || perm == "pharmacy.order.view" || perm == "commerce.order.view" ||
		strings.Contains(text, "طلب توريد") || strings.Contains(text, "توريد جديد") ||
		strings.Contains(text, "تم تأكيد طلبك") || strings.Contains(text, "تم استلام طلبك") ||
		strings.Contains(text, "تم شحن طلبك") || strings.Contains(text, "تم تسليم طلبك") ||
		strings.Contains(text, "إلغاء الطلب") || strings.Contains(text, "تعديل طلب") ||
		strings.Contains(text, "order.placed") || strings.Contains(text, "order.status")
}

func isDeliveryNotification(text, perm string, actor authctx.Actor) bool {
	return perm == "vendor.delivery.view" || actor.Role == "courier" ||
		strings.Contains(text, "طرد") || strings.Contains(text, "توصيل") ||
		strings.Contains(text, "مندوب") || strings.Contains(text, "delivery") ||
		strings.Contains(text, "courier") || strings.Contains(text, "استلام الطرد")
}

func isPurchaseRequestNotification(text, perm string) bool {
	return perm == "vendor.purchase_request.view" || strings.Contains(text, "طلب شراء") ||
		strings.Contains(text, "طلب تسعير") || strings.Contains(text, "عرض أسعار") ||
		strings.Contains(text, "تفاوض") || strings.Contains(text, "purchase_request") ||
		strings.Contains(text, "quote.") || strings.Contains(text, "negotiation.")
}

func isWalletNotification(text, perm string) bool {
	return perm == "vendor.wallet.view" || perm == "pharmacy.wallet.view" ||
		strings.Contains(text, "محفظة") || strings.Contains(text, "شحن رصيد") ||
		strings.Contains(text, "إيداع") || strings.Contains(text, "سحب رصيد") ||
		strings.Contains(text, "سحب أرباح") || strings.Contains(text, "wallet.") ||
		strings.Contains(text, "deposit") || strings.Contains(text, "withdrawal")
}

func isApprovalNotification(text, perm string) bool {
	return perm == "org.approval.view" || strings.Contains(text, "اعتماد منشأة") ||
		strings.Contains(text, "تسجيل منشأة جديدة") || strings.Contains(text, "طلب اعتماد") ||
		strings.Contains(text, "تم تفعيل حساب") || strings.Contains(text, "موافقة على المنشأة") ||
		strings.Contains(text, "org.approved") || strings.Contains(text, "org.rejected")
}

func isJobNotification(text string) bool {
	return strings.Contains(text, "باحث عن عمل") || strings.Contains(text, "وظيفة") ||
		strings.Contains(text, "توظيف") || strings.Contains(text, "job_seeker") ||
		strings.Contains(text, "job_application")
}

func isDocumentNotification(text, perm string) bool {
	return perm == "hr.document.view" || perm == "vendor.document.view" ||
		strings.Contains(text, "ترخيص") || strings.Contains(text, "مستند") ||
		strings.Contains(text, "وثائق") || strings.Contains(text, "document.")
}

func isCatalogImportNotification(text string) bool {
	return strings.Contains(text, "استيراد") || strings.Contains(text, "ملف أسعار") ||
		strings.Contains(text, "كتالوج") || strings.Contains(text, "import.run")
}

func isPromoNotification(text, perm string) bool {
	return perm == "vendor.offer.view" || perm == "vendor.offer_package.view" || perm == "vendor.ad.view" ||
		strings.Contains(text, "عرض خاص") || strings.Contains(text, "باقة رعاية") ||
		strings.Contains(text, "إعلان") || strings.Contains(text, "sponsorship") ||
		strings.Contains(text, "special_offer") || strings.Contains(text, "ad.status")
}

func isTeamBranchNotification(text string) bool {
	return strings.Contains(text, "فرع") || strings.Contains(text, "فريق") ||
		strings.Contains(text, "عضو") || strings.Contains(text, "صلاحية") ||
		strings.Contains(text, "branch.") || strings.Contains(text, "org.user")
}
