package pages

import (
	"fmt"
	"strings"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// TenantActivity contains presentation text only. Technical audit data stays in the admin log.
type TenantActivity struct {
	ID           int64                  `json:"id"`
	Action       string                 `json:"action"`
	Entity       string                 `json:"entity"`
	Outcome      string                 `json:"outcome"`
	Actor        string                 `json:"actor"`
	Organization string                 `json:"organization"`
	Date         string                 `json:"date"`
	Changes      []TenantActivityChange `json:"changes"`
}

type TenantActivityChange struct {
	Label  string `json:"label"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func activityText(lang, key string) string {
	if text, ok := i18n.Lookup(i18n.ParseLang(lang), key); ok {
		return text
	}
	return i18n.T(lang, "activity.action.recorded")
}

func tenantActivity(e *platformadmin.AuditEntry, lang string) TenantActivity {
	entity := activityEntity(e.EntityType)
	var action string
	if e.Action == "http.operation" {
		route := e.Route
		if route == "" {
			route, _ = e.After["route"].(string)
		}
		if route == "" {
			route = e.EntityType
		}
		entity = activityEntity(route)
		verb := activityVerb(route, fmt.Sprint(e.After["method"]))
		action = strings.ReplaceAll(activityText(lang, "activity.verb."+verb), "%s", activityText(lang, "activity.entity."+entity))
	} else {
		action, _ = formatAuditAction(e.Action, e.ActionLabelAr, lang)
	}
	outcome := "recorded"
	if e.Action == "http.operation" {
		outcome = "processed"
		if fmt.Sprint(e.After["outcome"]) == "rejected" {
			outcome = "rejected"
		}
	}
	return TenantActivity{ID: e.ID, Action: action, Entity: activityText(lang, "activity.entity."+entity),
		Outcome: activityText(lang, "activity.outcome."+outcome), Actor: e.ActorName, Organization: e.OrganizationName,
		Date: e.CreatedAt.Format("2006-01-02 15:04:05"), Changes: activityChanges(e, lang)}
}

func formatAuditAction(action, labelAr string, langOpt ...string) (string, string) {
	lang := "ar"
	if len(langOpt) > 0 {
		lang = langOpt[0]
	}
	for _, candidate := range []string{action, strings.TrimPrefix(action, "identity."), strings.TrimPrefix(action, "catalog.")} {
		if label, ok := i18n.Lookup(i18n.ParseLang(lang), "audit.action."+candidate); ok {
			return label, badgeClassForAction(action)
		}
	}
	if labelAr != "" && lang != "en" && !strings.ContainsAny(labelAr, "/{}") && labelAr != action {
		return labelAr, badgeClassForAction(action)
	}
	parts := strings.Split(action, ".")
	verb := activityVerb("/"+strings.Join(parts, "/"), "")
	if len(parts) > 1 {
		switch parts[len(parts)-1] {
		case "created":
			verb = "create"
		case "updated":
			verb = "update"
		case "deleted":
			verb = "delete"
		case "approved", "confirmed":
			verb = "approve"
		case "rejected":
			verb = "reject"
		case "cancelled":
			verb = "cancel"
		case "uploaded":
			verb = "upload"
		}
		if activityEntity(action) != "general" {
			return strings.ReplaceAll(activityText(lang, "activity.verb."+verb), "%s", activityText(lang, "activity.entity."+activityEntity(action))), badgeClassForAction(action)
		}
	}
	return i18n.T(lang, "activity.action.recorded"), "badge-slate"
}

func formatEntityType(entityType, entityTypeAr string, langOpt ...string) string {
	lang := "ar"
	if len(langOpt) > 0 {
		lang = langOpt[0]
	}
	return activityText(lang, "activity.entity."+activityEntity(entityType))
}

func activityEntity(raw string) string {
	if strings.Contains(raw, "user-organization") {
		return "customer"
	}
	tokens := strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool { return r == '/' || r == '.' || r == '_' })
	entities := map[string]string{
		"branches": "branch", "branch": "branch", "warehouses": "warehouse", "warehouse": "warehouse",
		"products": "product", "product": "product", "catalog": "product", "variants": "offer", "variant": "offer", "offers": "offer", "offer": "offer",
		"orders": "order", "order": "order", "delivery": "delivery", "shipments": "delivery", "shipment": "delivery",
		"documents": "document", "document": "document", "staff": "member", "team": "member", "members": "member", "member": "member", "user": "account", "users": "account",
		"invoice": "invoice", "invoices": "invoice", "payments": "payment", "payment": "payment", "wallet": "wallet", "deposits": "wallet", "withdrawals": "wallet",
		"coverage": "coverage", "coverages": "coverage", "settings": "settings", "organization": "organization", "org": "organization",
		"cart": "cart", "returns": "return", "return": "return", "notifications": "notification", "tickets": "ticket", "support": "ticket",
		"promo": "promotion", "promotions": "promotion", "ads": "promotion", "sponsorship": "promotion", "quotas": "quota", "quota": "quota", "import": "import", "ingest": "import",
		"jobs": "job", "applications": "job", "subscription": "subscription", "subscriptions": "subscription", "sessions": "session", "devices": "device",
		"storefront": "storefront", "favorites": "favorite", "reviews": "review", "requests": "request", "negotiations": "negotiation", "bands": "coverage",
	}
	for _, token := range tokens {
		if len(tokens) > 1 && (token == "org" || token == "catalog") {
			continue
		}
		if entity, ok := entities[token]; ok {
			return entity
		}
	}
	return "general"
}

func activityVerb(route, method string) string {
	if strings.Contains(route, "user-organization") {
		route = strings.ReplaceAll(route, "user-organization", "customer")
	}
	parts := strings.Split(strings.Trim(route, "/"), "/")
	verbs := map[string]string{"edit": "update", "update": "update", "new": "create", "create": "create", "add": "create",
		"delete": "delete", "delete-all": "delete", "remove": "delete", "approve": "approve", "confirm": "approve", "reject": "reject",
		"cancel": "cancel", "withdraw": "cancel", "upload": "upload", "import": "import", "export": "export", "status": "status", "toggle": "status",
		"assign": "assign", "manager": "assign", "assign-courier": "assign_courier", "unassign": "unassign", "unassign-courier": "unassign_courier",
		"password": "password", "set-branch": "switch", "active": "switch", "match": "match", "commit": "commit", "verify": "verify", "release": "release", "restore": "restore",
		"purchase": "purchase", "pay": "pay", "renew": "renew", "revoke": "revoke", "accept": "approve", "read": "read", "read-all": "read", "clear": "delete", "delete-request": "delete_request", "received": "receive", "reorder": "reorder"}
	for n := len(parts) - 1; n >= 0; n-- {
		if verb, ok := verbs[parts[n]]; ok {
			return verb
		}
	}
	if method == "DELETE" {
		return "delete"
	}
	if activityEntity(route) == "general" {
		return "recorded"
	}
	if method == "POST" && !strings.Contains(route, "{") {
		switch activityEntity(route) {
		case "job", "coverage", "ticket", "order", "payment", "invoice", "customer":
			return "create"
		}
	}
	return "update"
}

func activityChanges(e *platformadmin.AuditEntry, lang string) []TenantActivityChange {
	after := e.After
	if e.Action == "http.operation" {
		after = map[string]any{}
		switch params := e.After["parameters"].(type) {
		case map[string]any:
			after = params
		case map[string][]string:
			for key, value := range params {
				after[key] = value
			}
		}
	}
	fields := []struct{ raw, key string }{{"price", "price"}, {"quantity", "quantity"}, {"qty", "quantity"}, {"discount", "discount"}, {"status", "status"}, {"name", "name"}, {"title", "name"}, {"document_type", "document_type"}, {"original_name", "original_name"}, {"branch_id", "branch_id"}, {"product_id", "product_id"}, {"variant_id", "variant_id"}, {"warehouse_id", "warehouse_id"}, {"request_id", "request_id"}, {"vendor_org_id", "vendor_org_id"}, {"mode", "mode"}}
	changes := make([]TenantActivityChange, 0)
	seen := make(map[string]bool)
	for _, field := range fields {
		before, hasBefore := e.Before[field.raw]
		value, hasAfter := after[field.raw]
		if !hasBefore && !hasAfter {
			continue
		}
		if seen[field.key] {
			continue
		}
		seen[field.key] = true
		changes = append(changes, TenantActivityChange{Label: activityText(lang, "activity.field."+field.key), Before: activityValue(before, field.key, lang), After: activityValue(value, field.key, lang)})
	}
	return changes
}

func activityValue(value any, key, lang string) string {
	if value == nil {
		return "—"
	}
	switch v := value.(type) {
	case map[string]any:
		for _, locale := range []string{lang, "ar", "en"} {
			if translated, ok := v[locale].(string); ok && translated != "" {
				return translated
			}
		}
		return i18n.T(lang, "activity.value.unspecified")
	case []any:
		if len(v) > 0 {
			return activityValue(v[0], key, lang)
		}
		return "—"
	case []string:
		if len(v) > 0 {
			return activityValue(v[0], key, lang)
		}
		return "—"
	}
	text := fmt.Sprint(value)
	if key == "status" || key == "mode" {
		if localized, ok := i18n.Lookup(i18n.ParseLang(lang), "activity.value."+text); ok {
			return localized
		}
		return i18n.T(lang, "activity.value.unspecified")
	}
	if key == "document_type" {
		if localized, ok := i18n.Lookup(i18n.ParseLang(lang), "doc.type."+text); ok {
			return localized
		}
		return i18n.T(lang, "activity.entity.document")
	}
	if strings.Contains(text, "/") || strings.Contains(text, "\\") {
		return i18n.T(lang, "activity.value.unspecified")
	}
	return text
}
