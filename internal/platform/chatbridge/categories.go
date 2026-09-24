package chatbridge

import "strings"

// Category groups notifications so a user can silence a kind of message on a
// chat channel without touching the in-app feed.
//
// The feed row carries no event key, only the permission its producer
// required, so the category is read from that permission's resource segment.
// The permission is the right signal: it is what the producer chose as "who
// is entitled to know this", and it is stable where titles are copy.
type Category string

const (
	CategoryOrders   Category = "orders"
	CategoryPayments Category = "payments"
	CategoryDelivery Category = "delivery"
	CategoryOffers   Category = "offers"
	CategoryAccount  Category = "account"
	CategoryGeneral  Category = "general"
)

// Categories is every category, in the order the bots list them.
var Categories = []Category{
	CategoryOrders, CategoryPayments, CategoryDelivery,
	CategoryOffers, CategoryAccount, CategoryGeneral,
}

// Label is the Arabic name shown to the user.
func (c Category) Label() string {
	switch c {
	case CategoryOrders:
		return "الطلبات وطلبات التسعير"
	case CategoryPayments:
		return "المدفوعات والمحفظة والاشتراك"
	case CategoryDelivery:
		return "الشحن والتوصيل"
	case CategoryOffers:
		return "العروض والإعلانات"
	case CategoryAccount:
		return "الحساب والمنشأة والفروع"
	default:
		return "إشعارات أخرى"
	}
}

// ParseCategory accepts a category key.
func ParseCategory(s string) (Category, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, c := range Categories {
		if string(c) == s {
			return c, true
		}
	}
	return "", false
}

var resourceCategory = map[string]Category{
	"order":            CategoryOrders,
	"purchase_request": CategoryOrders,
	"quote":            CategoryOrders,
	"negotiation":      CategoryOrders,
	"smart_order":      CategoryOrders,
	"cart":             CategoryOrders,
	"wallet":           CategoryPayments,
	"billing":          CategoryPayments,
	"payment":          CategoryPayments,
	"invoice":          CategoryPayments,
	"subscription":     CategoryPayments,
	"refund":           CategoryPayments,
	"delivery":         CategoryDelivery,
	"shipment":         CategoryDelivery,
	"offer":            CategoryOffers,
	"offer_package":    CategoryOffers,
	"ad":               CategoryOffers,
	"sponsorship":      CategoryOffers,
	"promo":            CategoryOffers,
	"organization":     CategoryAccount,
	"organizations":    CategoryAccount,
	"document":         CategoryAccount,
	"branch":           CategoryAccount,
	"member":           CategoryAccount,
	"users":            CategoryAccount,
	"role":             CategoryAccount,
}

// CategoryFor classifies a notification by the permission it required, and by
// its title only when no permission was recorded.
func CategoryFor(requiredPermission, title string) Category {
	parts := strings.Split(strings.TrimSpace(requiredPermission), ".")
	if len(parts) >= 2 {
		if c, ok := resourceCategory[parts[1]]; ok {
			return c
		}
		return CategoryGeneral
	}
	switch {
	case strings.Contains(title, "شحنة") || strings.Contains(title, "توصيل") || strings.Contains(title, "تسليم"):
		return CategoryDelivery
	case strings.Contains(title, "طلب"):
		return CategoryOrders
	case strings.Contains(title, "محفظ") || strings.Contains(title, "رصيد") || strings.Contains(title, "دفع"):
		return CategoryPayments
	}
	return CategoryGeneral
}

// Muted reports whether category is among a chat's muted category keys.
func Muted(muted []string, category Category) bool {
	for _, c := range muted {
		if c == string(category) {
			return true
		}
	}
	return false
}

// CategoryKeys validates and de-duplicates categories into stored keys.
func CategoryKeys(cs []Category) []string {
	out := make([]string, 0, len(cs))
	seen := map[Category]bool{}
	for _, c := range cs {
		if _, ok := ParseCategory(string(c)); ok && !seen[c] {
			seen[c] = true
			out = append(out, string(c))
		}
	}
	return out
}
