package promo

import "testing"

func TestResolveClickURLStaysOnSite(t *testing.T) {
	id := int64(7)
	org := int64(3)
	cases := map[string]string{
		"/offers/9":               "/offers/9",
		"https://evil.example/x":  "/catalog/7",
		"//evil.example":          "/catalog/7",
		`/\evil.example`:          "/catalog/7",
		"javascript:alert(1)":     "/catalog/7",
		"/products?variant_id=11": "/catalog/11",
	}
	for target, want := range cases {
		a := &Ad{TargetURL: target, ClickTargetType: ClickTargetProduct, ClickTargetID: &id, OrganizationID: &org}
		if got := a.ResolveClickURL(); got != want {
			t.Errorf("ResolveClickURL(%q) = %q; want %q", target, got, want)
		}
	}
}
