package ui

import (
	"regexp"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

var (
	// Matches shipment identifiers such as 10542-1, ORD-999-1, 2609230001-1, #10542-1, or (10542-1).
	reShipmentNumber = regexp.MustCompile(`(?i)(?:#|\(|\b)((?:ORD-)?[A-Z0-9]+-(\d{1,4}))(?:\)|\b)`)

	// Matches standard prefixed order numbers such as ORD-1234, #ORD-1234.
	reOrderNumber = regexp.MustCompile(`(?i)(?:#|\b)(ORD-[A-Za-z0-9-]+)\b`)

	// Matches order numbers with # prefix or Arabic keyword prefix.
	reHashOrderNumber = regexp.MustCompile(`(?:#|طلب(?:ك)?\s*#?|شحن(?:تك|ة)?\s*#?)(\d{4,14})\b`)

	// Matches standalone numeric order identifiers.
	reNumericOrderNumber = regexp.MustCompile(`\b(\d{5,14})\b`)

	// Matches request IDs (e.g. #123).
	reRequestID = regexp.MustCompile(`#(\d+)`)
)

// extractOrderAndShipmentIdentifiers parses order numbers and shipment numbers from notification text.
func extractOrderAndShipmentIdentifiers(text string) (orderNum string, shipmentNum string) {
	// 1. Check for shipment number first (e.g., 10542-1, (10542-1), #10542-1, ORD-999-1)
	if m := reShipmentNumber.FindStringSubmatch(text); len(m) > 1 {
		shipmentNum = strings.TrimSpace(m[1])
		lastDash := strings.LastIndex(shipmentNum, "-")
		if lastDash > 0 {
			orderNum = shipmentNum[:lastDash]
		}
	}

	// 2. Check for explicit ORD- prefix order number if orderNum is still empty
	if orderNum == "" {
		if m := reOrderNumber.FindStringSubmatch(text); len(m) > 1 {
			orderNum = strings.TrimSpace(m[1])
		}
	}

	// 3. Check for #<numeric> or Arabic prefixed order numbers
	if orderNum == "" {
		if m := reHashOrderNumber.FindStringSubmatch(text); len(m) > 1 {
			orderNum = strings.TrimSpace(m[1])
		}
	}

	// 4. Check for standalone long numeric order number (e.g. 2609230001)
	if orderNum == "" {
		if m := reNumericOrderNumber.FindStringSubmatch(text); len(m) > 1 {
			orderNum = strings.TrimSpace(m[1])
		}
	}

	return orderNum, shipmentNum
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func extractExplicitURL(text string) string {
	if strings.Contains(text, "http://") || strings.Contains(text, "https://") {
		parts := strings.Fields(text)
		for _, p := range parts {
			clean := strings.TrimRight(p, ".,;:!?)'\"")
			if strings.HasPrefix(clean, "https://dawa24.com/") || strings.HasPrefix(clean, "http://dawa24.com/") {
				idx := strings.Index(clean, "dawa24.com")
				if idx != -1 {
					return clean[idx+len("dawa24.com"):]
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
		strings.Contains(text, "تم تسليم الشحنة") || strings.Contains(text, "تم تسليم شحنتك") ||
		strings.Contains(text, "إلغاء الطلب") || strings.Contains(text, "تعديل طلب") ||
		strings.Contains(text, "شحنة") || strings.Contains(text, "order.placed") || strings.Contains(text, "order.status")
}

func isDeliveryNotification(text, perm string, actor authctx.Actor) bool {
	return perm == "vendor.delivery.view" || actor.Role == "courier" || actor.Role == "org_courier" ||
		strings.Contains(text, "طرد") || strings.Contains(text, "توصيل") ||
		strings.Contains(text, "مندوب") || strings.Contains(text, "delivery") ||
		strings.Contains(text, "courier") || strings.Contains(text, "استلام الطرد") ||
		strings.Contains(text, "تسليم")
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
		perm == "promo.offer.view" || perm == "promo.offer_package.view" || perm == "promo.ad.view" ||
		strings.Contains(text, "عرض خاص") || strings.Contains(text, "باقة رعاية") ||
		strings.Contains(text, "إعلان") || strings.Contains(text, "sponsorship") ||
		strings.Contains(text, "special_offer") || strings.Contains(text, "ad.status")
}

func isTeamBranchNotification(text string) bool {
	return strings.Contains(text, "فرع") || strings.Contains(text, "فريق") ||
		strings.Contains(text, "عضو") || strings.Contains(text, "صلاحية") ||
		strings.Contains(text, "branch.") || strings.Contains(text, "org.user")
}

func isDeletionNotification(text string) bool {
	return strings.Contains(text, "حذف حساب") || strings.Contains(text, "حذف منشأة") ||
		strings.Contains(text, "حذف المنشأة") || strings.Contains(text, "account.deletion") ||
		strings.Contains(text, "org.deletion") || strings.Contains(text, "طلب حذف")
}

func extractRequestID(text string) string {
	if m := reRequestID.FindStringSubmatch(text); len(m) > 1 {
		return m[1]
	}
	return ""
}
