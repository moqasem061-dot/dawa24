package database

import "testing"

func TestAuditActorKindDefault(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "user"}, {"api_key", "api_key"}, {"system", "system"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := auditActorKind(AuditEntry{ActorKind: tc.input}); got != tc.want {
				t.Errorf("auditActorKind(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
