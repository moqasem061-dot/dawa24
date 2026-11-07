package erp

import "testing"

func TestPrincipalHasScope(t *testing.T) {
	for _, tc := range []struct {
		name, scope string
		scopes      []string
		want        bool
	}{
		{"present", ScopeOrdersRead, []string{ScopeOrdersRead, ScopeOrdersAck}, true},
		{"absent", ScopeOrdersDecide, []string{ScopeOrdersRead}, false},
		{"empty", ScopeOrdersRead, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Principal{Scopes: tc.scopes}).HasScope(tc.scope); got != tc.want {
				t.Errorf("HasScope(%q) = %v", tc.scope, got)
			}
		})
	}
}
