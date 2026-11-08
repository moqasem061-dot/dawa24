package erp

import (
	"context"
	"strings"
	"time"
)

// fakeCode is the in-memory CodeRepository the S5 unit tests run against. It
// keeps the rules of postgres/customer_code.go: a supplier uses ERP when its
// integration and key are active and the key is unexpired; a code is compared
// trimmed and case-insensitively; only pending and approved links count; and a
// holder and a customer are the same when they share the organisation, or, for
// a user-only link, the same user.
type fakeCode struct {
	erp       map[int64]fakeERP
	codeLinks []fakeCodeLink
	nextLink  int64
}

type fakeERP struct {
	integration, key string
	expires          *time.Time
}

type fakeCodeLink struct {
	id, vendorOrg, userID int64
	customerOrg           *int64
	code, status          string
}

func newFakeCode() *fakeCode {
	return &fakeCode{erp: map[int64]fakeERP{}, nextLink: 1000}
}

// setERP records the integration and key state of one supplier.
func (f *fakeCode) setERP(vendor int64, integration, key string, expires *time.Time) {
	f.erp[vendor] = fakeERP{integration: integration, key: key, expires: expires}
}

// addLink records a customer link and returns its id.
func (f *fakeCode) addLink(vendor int64, customerOrg *int64, user int64, code, status string) int64 {
	f.nextLink++
	f.codeLinks = append(f.codeLinks, fakeCodeLink{id: f.nextLink, vendorOrg: vendor, userID: user, customerOrg: customerOrg, code: code, status: status})
	return f.nextLink
}

func (f *fakeCode) usesERP(vendor int64) bool {
	e, ok := f.erp[vendor]
	if !ok || e.integration != "active" || e.key != "active" {
		return false
	}
	return e.expires == nil || e.expires.After(time.Now())
}

func fakeLinkLive(status string) bool { return status == "pending" || status == "approved" }

func fakeSameCustomer(holderOrg *int64, holderUser int64, customerOrg *int64, user int64) bool {
	if customerOrg != nil && *customerOrg > 0 {
		return holderOrg != nil && *holderOrg == *customerOrg
	}
	return holderOrg == nil && holderUser == user
}

func (f *fakeCode) SupplierERPStatusAsSystem(_ context.Context, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	for _, id := range ids {
		if f.usesERP(id) {
			out[id] = true
		}
	}
	return out, nil
}

func (f *fakeCode) CodedSuppliers(_ context.Context, buyerOrg, buyerUser int64, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	for _, l := range f.codeLinks {
		if !fakeLinkLive(l.status) || strings.TrimSpace(l.code) == "" || !containsID(ids, l.vendorOrg) {
			continue
		}
		mine := (l.customerOrg != nil && buyerOrg > 0 && *l.customerOrg == buyerOrg) || (l.customerOrg == nil && l.userID == buyerUser)
		if mine {
			out[l.vendorOrg] = true
		}
	}
	return out, nil
}

func (f *fakeCode) GuardCustomerCode(ctx context.Context, vendor int64, customerOrg *int64, user int64, code string, exclude int64, write func(context.Context) error) (bool, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code != "" && f.usesERP(vendor) {
		for _, l := range f.codeLinks {
			if l.vendorOrg != vendor || l.id == exclude || !fakeLinkLive(l.status) || strings.ToLower(strings.TrimSpace(l.code)) != code {
				continue
			}
			if !fakeSameCustomer(l.customerOrg, l.userID, customerOrg, user) {
				return true, nil
			}
		}
	}
	return false, write(ctx)
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
