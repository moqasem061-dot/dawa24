package erp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func keyServiceFixture() (*Service, *fakeRepo, CreateKeyInput) {
	repo := newFakeRepo()
	svc := NewService(repo, Ports{}, Config{Pepper: []byte("test pepper"), Tag: "test", Provider: "testprovider", DefaultRate: 600, MaxBatchItems: 50000}, nil)
	return svc, repo, CreateKeyInput{OrganizationID: 1, ActorUserID: 9, ActorIsOwner: true, Name: "key", WarehouseID: 2, BranchID: 3}
}
func requireERPCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
	return e
}
func TestKeyServiceCreateValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fakeRepo, *CreateKeyInput)
		field  string
	}{
		{"non-owner", func(_ *fakeRepo, in *CreateKeyInput) { in.ActorIsOwner = false }, "actor"},
		{"warehouse", func(r *fakeRepo, _ *CreateKeyInput) { r.validWarehouse = false }, "warehouseId"},
		{"branch", func(r *fakeRepo, _ *CreateKeyInput) { r.validBranch = false }, "branchId"},
		{"buying user", func(r *fakeRepo, in *CreateKeyInput) { id := int64(4); in.BuyingUserID = &id; r.validMember = false }, "buyingUserId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, r, in := keyServiceFixture()
			tc.change(r, &in)
			e := requireERPCode(t, svcCreateError(svc, in), "ERP_VALIDATION")
			fields, ok := e.Details["fields"].([]string)
			if !ok || len(fields) == 0 || fields[0] != tc.field {
				t.Errorf("fields=%v", e.Details["fields"])
			}
			if len(r.keys) != 0 || len(r.audits) != 0 {
				t.Error("refusal mutated state")
			}
		})
	}
}
func svcCreateError(s *Service, in CreateKeyInput) error {
	_, err := s.CreateKey(context.Background(), in)
	return err
}
func TestKeyServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, r, in := keyServiceFixture()
	created, err := svc.CreateKey(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	prefix, secret, ok := ParseKey(created.FullKey)
	stored := r.keys[created.Key.ID]
	if !ok || prefix != stored.KeyPrefix || !VerifyHash(svc.cfg.Pepper, secret, stored.KeyHash) {
		t.Fatal("stored key does not match returned key")
	}
	if len(created.Key.Scopes) != 4 || created.Key.RateLimitPerMinute != 600 || created.Key.MaxBatchItems != 50000 {
		t.Errorf("defaults: %+v", created.Key)
	}
	before := len(r.audits)
	if _, err = svc.RotateKey(ctx, 1, created.Key.ID, time.Hour, 9, false); err == nil {
		t.Fatal("non-owner rotated")
	}
	e := requireERPCode(t, err, "ERP_VALIDATION")
	if len(e.Details["fields"].([]string)) == 0 || e.Details["fields"].([]string)[0] != "actor" || len(r.audits) != before || len(r.keys) != 1 {
		t.Fatal("rotate refusal mutated state")
	}
	if _, err = svc.RotateKey(ctx, 1, created.Key.ID, RotateGraceMax+time.Hour, 9, true); err == nil {
		t.Fatal("excess grace accepted")
	}
	rotated, err := svc.RotateKey(ctx, 1, created.Key.ID, time.Hour, 9, true)
	if err != nil {
		t.Fatal(err)
	}
	old := r.keys[created.Key.ID]
	if old.ExpiresAt == nil || time.Until(*old.ExpiresAt) < 59*time.Minute {
		t.Error("old key expiry missing")
	}
	newKey := rotated.Key
	if newKey.Status != "active" || newKey.WarehouseID != created.Key.WarehouseID || newKey.BranchID != created.Key.BranchID || newKey.RateLimitPerMinute != created.Key.RateLimitPerMinute || newKey.MaxBatchItems != created.Key.MaxBatchItems || len(newKey.Scopes) != len(created.Key.Scopes) {
		t.Error("rotation did not copy key settings")
	}
	before = len(r.audits)
	if err = svc.UpdateKeyLimits(ctx, 1, newKey.ID, 700, 9, false); err == nil {
		t.Fatal("non-owner changed limit")
	}
	e = requireERPCode(t, err, "ERP_VALIDATION")
	if e.Details["fields"].([]string)[0] != "actor" || len(r.audits) != before || r.keys[newKey.ID].RateLimitPerMinute != 600 {
		t.Error("limit refusal mutated state")
	}
	for _, n := range []int{9, 6001} {
		err = svc.UpdateKeyLimits(ctx, 1, newKey.ID, n, 9, true)
		e = requireERPCode(t, err, "ERP_VALIDATION")
		if e.Details["fields"].([]string)[0] != "ratePerMinute" {
			t.Error("wrong validation field")
		}
	}
	before = len(r.audits)
	if err = svc.UpdateKeyLimits(ctx, 1, newKey.ID, 700, 9, true); err != nil || r.keys[newKey.ID].RateLimitPerMinute != 700 || len(r.audits) != before+1 {
		t.Fatalf("update limit: %v", err)
	}
	otherErr := svc.UpdateKeyLimits(ctx, 2, newKey.ID, 700, 10, true)
	missingErr := svc.UpdateKeyLimits(ctx, 2, 999999, 700, 10, true)
	requireERPCode(t, otherErr, "ERP_NOT_FOUND")
	requireERPCode(t, missingErr, "ERP_NOT_FOUND")
	if otherErr.Error() != missingErr.Error() {
		t.Error("cross-tenant and missing differ")
	}
	requireERPCode(t, svc.RevokeKey(ctx, 2, newKey.ID, "", 10), "ERP_NOT_FOUND")
	if err = svc.RevokeKey(ctx, 1, newKey.ID, "reason", 9); err != nil {
		t.Fatal(err)
	}
	own, err := svc.ListKeys(ctx, 1)
	if err != nil || len(own) != 2 {
		t.Fatalf("own list: %v", err)
	}
	in.OrganizationID = 2
	if _, err = svc.CreateKey(ctx, in); err != nil {
		t.Fatal(err)
	}
	own, err = svc.ListKeys(ctx, 1)
	if err != nil || len(own) != 2 {
		t.Fatalf("tenant list: %v", err)
	}
	all, err := svc.ListKeysAsStaff(ctx)
	if err != nil || len(all) != 3 {
		t.Fatalf("staff list: %v", err)
	}
	if err = svc.RevokeKeyAsStaff(ctx, created.Key.ID, "staff", 9); err != nil || r.keys[created.Key.ID].Status != "revoked" {
		t.Fatalf("staff revoke: %v", err)
	}
}
