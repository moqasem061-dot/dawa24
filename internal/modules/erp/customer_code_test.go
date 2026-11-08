package erp

import (
	"context"
	"testing"
	"time"
)

// S5 (US5): the customer code a supplier uses for a buyer. These tests run the
// service over fakeRepo, whose fakeCode keeps the rules of the SQL in
// postgres/customer_code.go. The SQL itself is proven by the integration tests
// in test/integration/erp_customer_code_test.go.

func customerCodeFixture() (*Service, *fakeRepo) {
	repo := newFakeRepo()
	return NewService(repo, Ports{}, Config{Provider: "testprovider"}, nil), repo
}

func customerRef(v int64) *int64 { return &v }

func TestCustomerCodeSuppliersUsingERP(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	for _, tc := range []struct {
		name             string
		integration, key string
		expires          *time.Time
		want             bool
	}{
		{"active integration and active key", "active", "active", nil, true},
		{"key expires in the future", "active", "active", &future, true},
		{"key already expired", "active", "active", &past, false},
		{"key revoked", "active", "revoked", nil, false},
		{"integration paused", "paused", "active", nil, false},
		{"integration closed", "closed", "active", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := customerCodeFixture()
			repo.setERP(7, tc.integration, tc.key, tc.expires)
			got, err := svc.SuppliersUsingERP(context.Background(), []int64{7, 8})
			if err != nil {
				t.Fatal(err)
			}
			if got[7] != tc.want || got[8] {
				t.Fatalf("SuppliersUsingERP = %v, want supplier 7 = %t and supplier 8 (no integration) = false", got, tc.want)
			}
		})
	}
}

func TestCustomerCodeMissingCodes(t *testing.T) {
	svc, repo := customerCodeFixture()
	const buyerOrg, buyerUser int64 = 100, 100
	ctx := context.Background()
	for _, id := range []int64{1, 2, 3, 5, 6} {
		repo.setERP(id, "active", "active", nil)
	}
	// 1: ERP supplier, no link at all: missing.
	// 2: ERP supplier, a link with a blank number: missing.
	repo.addLink(2, nil, buyerUser, "   ", "approved")
	// 3: ERP supplier with a code for this buyer's organisation: not missing.
	repo.addLink(3, customerRef(buyerOrg), buyerUser, "C-3", "approved")
	// 4: no ERP at all, no link: never asked for a code.
	// 5: ERP supplier whose only link is rejected: missing.
	repo.addLink(5, customerRef(buyerOrg), buyerUser, "C-5", "rejected")
	// 6: ERP supplier whose code belongs to another buyer's organisation: missing.
	repo.addLink(6, customerRef(200), 200, "C-6", "approved")

	missing, err := svc.MissingCodes(ctx, buyerOrg, buyerUser, []int64{1, 2, 3, 4, 5, 6})
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{1, 2, 5, 6}
	if len(missing) != len(want) {
		t.Fatalf("MissingCodes = %v, want %v", missing, want)
	}
	for i := range want {
		if missing[i] != want[i] {
			t.Fatalf("MissingCodes = %v, want %v", missing, want)
		}
	}
}

func TestCustomerCodeGuard(t *testing.T) {
	const vendor int64 = 1
	for _, tc := range []struct {
		name          string
		useERP        bool
		holderOrg     *int64
		holderUser    int64
		holderCode    string
		holderStatus  string
		excludeHolder bool
		customerOrg   *int64
		userID        int64
		code          string
		wantTaken     bool
	}{
		{"another customer holds the code", true, customerRef(200), 40, "C-1", "approved", false, customerRef(300), 31, "C-1", true},
		{"code is compared without case", true, customerRef(200), 40, "C-1", "approved", false, customerRef(300), 31, "c-1", true},
		{"a pending link also holds the code", true, customerRef(200), 40, "C-1", "pending", false, customerRef(300), 31, "C-1", true},
		{"a rejected link does not hold the code", true, customerRef(200), 40, "C-1", "rejected", false, customerRef(300), 31, "C-1", false},
		{"the same organisation may keep the code", true, customerRef(200), 40, "C-1", "approved", false, customerRef(200), 41, "C-1", false},
		{"the excluded link is ignored", true, customerRef(200), 40, "C-1", "approved", true, customerRef(300), 31, "C-1", false},
		{"a user-only link holds the code for that user", true, nil, 51, "C-9", "approved", false, nil, 52, "C-9", true},
		{"the same user keeps a user-only code", true, nil, 51, "C-9", "approved", false, nil, 51, "C-9", false},
		{"a blank code is never taken", true, customerRef(200), 40, "C-1", "approved", false, customerRef(300), 31, "  ", false},
		{"a free code is accepted", true, customerRef(200), 40, "C-1", "approved", false, customerRef(300), 31, "C-2", false},
		{"a supplier without ERP is a no-op", false, customerRef(200), 40, "C-1", "approved", false, customerRef(300), 31, "C-1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := customerCodeFixture()
			if tc.useERP {
				repo.setERP(vendor, "active", "active", nil)
			}
			holderID := repo.addLink(vendor, tc.holderOrg, tc.holderUser, tc.holderCode, tc.holderStatus)
			exclude := int64(0)
			if tc.excludeHolder {
				exclude = holderID
			}
			err := svc.GuardCustomerCode(context.Background(), vendor, tc.customerOrg, tc.userID, tc.code, exclude)
			if !tc.wantTaken {
				if err != nil {
					t.Fatalf("GuardCustomerCode = %v, want nil", err)
				}
				return
			}
			if e := requireERPCode(t, err, "ERP_CODE_TAKEN"); e.Status != 409 {
				t.Fatalf("status = %d, want 409", e.Status)
			}
		})
	}
}
