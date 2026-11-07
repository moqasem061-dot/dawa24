package erp

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAuthenticate(t *testing.T) {
	ctx := context.Background()
	for _, scenario := range []string{"unknown", "wrong", "revoked", "expired", "valid", "malformed", "paused", "closed", "unapproved"} {
		t.Run(scenario, func(t *testing.T) {
			svc, repo, in := keyServiceFixture()
			created, err := svc.CreateKey(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			raw := created.FullKey
			key := repo.keys[created.Key.ID]
			switch scenario {
			case "unknown":
				raw = strings.Replace(raw, key.KeyPrefix, "zzzzzzzz", 1)
				if key.KeyPrefix == "zzzzzzzz" {
					t.Fatal("test prefix collision")
				}
			case "wrong":
				parts := strings.Split(raw, "_")
				if parts[3][0] == 'A' {
					parts[3] = "B" + parts[3][1:]
				} else {
					parts[3] = "A" + parts[3][1:]
				}
				raw = strings.Join(parts, "_")
			case "revoked":
				key.Status = "revoked"
				repo.keys[key.ID] = key
			case "expired":
				expired := time.Now().Add(-time.Hour)
				key.ExpiresAt = &expired
				repo.keys[key.ID] = key
			case "malformed":
				raw = "invalid"
			case "paused", "closed":
				i := repo.integrations[key.IntegrationID]
				i.Status = scenario
				repo.integrations[i.ID] = i
			case "unapproved":
				repo.orgStatus = "pending"
			}
			calls := 0
			realHash := svc.hashSecret
			svc.hashSecret = func(secret string) []byte { calls++; return realHash(secret) }
			principal, err := svc.Authenticate(ctx, raw, "127.0.0.1")
			if calls != 1 {
				t.Errorf("HMAC calls=%d, want 1", calls)
			}
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				if principal.OrganizationID != in.OrganizationID || principal.KeyID != created.Key.ID || principal.WarehouseID != in.WarehouseID || principal.BranchID != in.BranchID || principal.OrgType != "vendor" || !reflect.DeepEqual(principal.Scopes, created.Key.Scopes) || repo.touches != 1 {
					t.Errorf("principal or touch incorrect: %+v", principal)
				}
			} else {
				requireERPCode(t, err, "ERP_AUTH_INVALID")
				if repo.touches != 0 {
					t.Error("failed authentication touched key")
				}
			}
			if scenario == "unapproved" {
				repo.orgStatus = "approved"
				if _, err := svc.Authenticate(ctx, created.FullKey, ""); err != nil {
					t.Fatalf("approval restored: %v", err)
				}
			}
		})
	}
}
func TestAuthenticateUnknownAndWrongAreIdentical(t *testing.T) {
	ctx := context.Background()
	svc, _, in := keyServiceFixture()
	created, err := svc.CreateKey(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(created.FullKey, "_")
	wrong := append([]string(nil), parts...)
	if wrong[3][0] == 'A' {
		wrong[3] = "B" + wrong[3][1:]
	} else {
		wrong[3] = "A" + wrong[3][1:]
	}
	unknown := append([]string(nil), parts...)
	unknown[2] = "zzzzzzzz"
	if unknown[2] == parts[2] {
		t.Fatal("test prefix collision")
	}
	_, unknownErr := svc.Authenticate(ctx, strings.Join(unknown, "_"), "")
	_, wrongErr := svc.Authenticate(ctx, strings.Join(wrong, "_"), "")
	a := requireERPCode(t, unknownErr, "ERP_AUTH_INVALID")
	b := requireERPCode(t, wrongErr, "ERP_AUTH_INVALID")
	if !reflect.DeepEqual(a, b) || a.Error() != b.Error() {
		t.Error("authentication failures differ")
	}
}
