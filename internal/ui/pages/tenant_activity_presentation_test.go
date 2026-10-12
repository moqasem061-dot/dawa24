package pages

import (
	"encoding/json"
	"strings"
	"testing"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
)

func TestTenantActivityReadableBilingualProjection(t *testing.T) {
	for _, lang := range []string{"ar", "en"} {
		for _, route := range []string{"/vendor/branches/{id}/edit", "/customer/branches/new", "/vendor/orders/{id}/assign-courier", "/vendor/documents/upload", "/vendor/coverage/{id}/toggle", "/customer/cart/add", "/vendor/warehouses/{id}/delete", "/vendor/organization/{section}", "/vendor/user-organization/{id}/approve", "/vendor/import/{sessionID}/commit", "/future/unknown"} {
			e := &platformadmin.AuditEntry{Action: "http.operation", EntityType: route, Route: route, After: map[string]any{"method": "POST", "status": 303, "outcome": "response_303", "parameters": map[string]any{"quantity": []any{"12"}, "status": []any{"pending"}, "password": "secret", "token": "private", "route": route}}}
			view := tenantActivity(e, lang)
			encoded, err := json.Marshal(view)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{route, "POST", "response_303", "secret", "private", "http.operation", "activity.verb", "%!(EXTRA"} {
				if strings.Contains(string(encoded), forbidden) {
					t.Fatalf("%s %s leaks %s: %s", lang, route, forbidden, encoded)
				}
			}
			if len(view.Changes) != 2 {
				t.Fatalf("unexpected details: %+v", view)
			}
			if lang == "ar" && route == "/vendor/branches/{id}/edit" && view.Action != "تعديل بيانات الفرع" {
				t.Fatalf("branch action: %s", view.Action)
			}
			if lang == "en" && route == "/vendor/branches/{id}/edit" && view.Action != "Update branch details" {
				t.Fatalf("branch action: %s", view.Action)
			}
		}
	}
}

func TestTenantActivityRejectedAndLegacyFallback(t *testing.T) {
	e := &platformadmin.AuditEntry{Action: "http.operation", After: map[string]any{"outcome": "rejected"}}
	view := tenantActivity(e, "ar")
	if view.Outcome != "لم تكتمل العملية" || view.Changes == nil {
		t.Fatalf("wrong rejected projection: %+v", view)
	}
	for _, action := range []string{"identity.user.login", "product.updated", "document.uploaded", "member.created", "unknown.code"} {
		ar, _ := formatAuditAction(action, "", "ar")
		en, _ := formatAuditAction(action, "", "en")
		if ar == action || en == action || ar == en {
			t.Fatalf("action not localized: %s %s %s", action, ar, en)
		}
	}
}

func TestTenantActivityBilingualNamesAndDuplicateFields(t *testing.T) {
	e := &platformadmin.AuditEntry{Action: "product.updated", After: map[string]any{"name": map[string]any{"ar": "بنادول", "en": "Panadol"}, "qty": "3", "quantity": "3"}}
	for _, lang := range []string{"ar", "en"} {
		view := tenantActivity(e, lang)
		if len(view.Changes) != 2 {
			t.Fatalf("duplicate detail fields: %+v", view.Changes)
		}
		found := false
		for _, change := range view.Changes {
			if change.After == "بنادول" && lang == "ar" || change.After == "Panadol" && lang == "en" {
				found = true
			}
		}
		if !found {
			t.Fatalf("name not localized: %+v", view.Changes)
		}
	}
}
