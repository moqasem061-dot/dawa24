package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// S5 (US5) in the UI package: the customer code is checked and written in one
// step (D2). The handlers run over the real erp and org services on the
// in-memory fakes below, because the erp package's fakes live in its own test
// binary.

// codeStore is the state the erp and org fakes share: which suppliers use ERP,
// and the customer links the handlers read and write.
type codeStore struct {
	erpSuppliers map[int64]bool
	links        map[int64]*org.UserOrganization
	nextID       int64
	creates      int
	updates      int
}

func newCodeStore() *codeStore {
	return &codeStore{erpSuppliers: map[int64]bool{}, links: map[int64]*org.UserOrganization{}, nextID: 900}
}

// addCodeLink stores a customer link with a fixed id.
func (s *codeStore) addCodeLink(id, userID int64, customerOrg *int64, vendorOrg int64, number string, status org.UserOrganizationStatus) {
	s.links[id] = &org.UserOrganization{ID: id, UserID: userID, CustomerOrgID: customerOrg, VendorOrgID: vendorOrg, OrganizationNumber: number, Status: status}
}

func codeLive(status org.UserOrganizationStatus) bool {
	return status == org.UserOrgStatusPending || status == org.UserOrgStatusApproved
}

func codeHas(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// codeSameCustomer: a holder and a customer are one customer when they share the
// organization, or, for a user-only link, the same user.
func codeSameCustomer(holderOrg *int64, holderUser int64, customerOrg *int64, user int64) bool {
	if customerOrg != nil && *customerOrg > 0 {
		return holderOrg != nil && *holderOrg == *customerOrg
	}
	return holderOrg == nil && holderUser == user
}

// codeErpRepo answers the erp.Repository calls the customer-code guard makes.
type codeErpRepo struct {
	erp.Repository
	s *codeStore
}

func (c codeErpRepo) SupplierERPStatusAsSystem(_ context.Context, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	for _, id := range ids {
		if c.s.erpSuppliers[id] {
			out[id] = true
		}
	}
	return out, nil
}

func (c codeErpRepo) CodedSuppliers(_ context.Context, buyerOrg, buyerUser int64, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	for _, l := range c.s.links {
		if !codeLive(l.Status) || strings.TrimSpace(l.OrganizationNumber) == "" || !codeHas(ids, l.VendorOrgID) {
			continue
		}
		if (l.CustomerOrgID != nil && buyerOrg > 0 && *l.CustomerOrgID == buyerOrg) || (l.CustomerOrgID == nil && l.UserID == buyerUser) {
			out[l.VendorOrgID] = true
		}
	}
	return out, nil
}

func (c codeErpRepo) GuardCustomerCode(ctx context.Context, vendor int64, cust *int64, user int64, code string, exclude int64, write func(context.Context) error) (bool, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code != "" && c.s.erpSuppliers[vendor] {
		for _, l := range c.s.links {
			if l.VendorOrgID != vendor || l.ID == exclude || !codeLive(l.Status) || strings.ToLower(strings.TrimSpace(l.OrganizationNumber)) != code {
				continue
			}
			var custOrg int64
			if cust != nil {
				custOrg = *cust
			}
			var holderOrg *int64 = l.CustomerOrgID
			if !codeSameCustomer(holderOrg, l.UserID, pointerOrNil(custOrg), user) {
				return true, nil
			}
		}
	}
	return false, write(ctx)
}

func pointerOrNil(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}

// codeOrgRepo answers the org.Repository calls the link writers make.
type codeOrgRepo struct {
	org.Repository
	s *codeStore
}

func (c codeOrgRepo) CreateUserOrganization(_ context.Context, uo *org.UserOrganization) error {
	c.s.nextID++
	uo.ID = c.s.nextID
	saved := *uo
	c.s.links[uo.ID] = &saved
	c.s.creates++
	return nil
}

func (c codeOrgRepo) GetUserOrganizationByID(_ context.Context, id int64) (*org.UserOrganization, error) {
	if l, ok := c.s.links[id]; ok {
		cp := *l
		return &cp, nil
	}
	return nil, nil
}

func (c codeOrgRepo) UpdateUserOrganization(_ context.Context, id int64, orgNumber string, _ org.UserOrganizationStatus, _ string) error {
	c.s.updates++
	if l, ok := c.s.links[id]; ok && orgNumber != "" {
		l.OrganizationNumber = orgNumber
	}
	return nil
}

func (c codeOrgRepo) ListUserOrganizationsByUser(_ context.Context, userID int64) ([]*org.UserOrganization, error) {
	var out []*org.UserOrganization
	for _, l := range c.s.links {
		if l.UserID == userID {
			cp := *l
			out = append(out, &cp)
		}
	}
	return out, nil
}

// codeHandler wires a UIHandler over the fakes. withERP false leaves the ERP
// service unset, as a deployment with the API off does.
func codeHandler(s *codeStore, withERP bool) *UIHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orgSvc := org.NewService(codeOrgRepo{s: s}, logger)
	h := NewUIHandler(nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
	if withERP {
		h.SetERP(erp.NewService(codeErpRepo{s: s}, erp.Ports{}, erp.Config{Provider: "testprovider"}, logger))
	}
	return h
}

// codeRequest runs one handler with a form body, URL params and an actor.
func codeRequest(form url.Values, actor authctx.Actor, vars map[string]string, handler func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	for k, v := range vars {
		rctx.URLParams.Add(k, v)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(authctx.WithActor(ctx, actor))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// takenNotice reports whether a redirect carries the number_taken notice. The
// handler replies in the request language, so both translations are accepted.
func takenNotice(location string) bool {
	return strings.Contains(location, url.QueryEscape(i18n.T("ar", "customer.userorg.number_taken"))) ||
		strings.Contains(location, url.QueryEscape(i18n.T("en", "customer.userorg.number_taken")))
}

func TestErpCustomerCodeHelperNoOp(t *testing.T) {
	h := codeHandler(newCodeStore(), false)
	ran := false
	err := h.erpGuardCustomerCode(context.Background(), 700, nil, 41, "C-1", 0, func(context.Context) error {
		ran = true
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("without an ERP service the write must run: ran=%t err=%v", ran, err)
	}
}

func TestErpCustomerCodeSubmit(t *testing.T) {
	const vendor, holderOrg, buyerOrg int64 = 700, 200, 300
	buyer := authctx.Actor{UserID: 41, OrganizationID: buyerOrg, OrgID: buyerOrg}
	newFixture := func() *codeStore {
		s := newCodeStore()
		s.erpSuppliers[vendor] = true
		s.addCodeLink(501, 40, pointerOrNil(holderOrg), vendor, "C-1", org.UserOrgStatusApproved)
		return s
	}

	t.Run("a code another customer holds is refused and changes nothing", func(t *testing.T) {
		s := newFixture()
		rec := codeRequest(url.Values{"vendor_org_id": {"700"}, "organization_number": {"c-1"}}, buyer, nil, codeHandler(s, true).CheckoutErpCodeSubmit)
		if loc := rec.Header().Get("Location"); !takenNotice(loc) {
			t.Fatalf("Location = %q, want the number_taken notice", loc)
		}
		if s.creates != 0 || s.updates != 0 {
			t.Fatalf("creates=%d updates=%d, want no write", s.creates, s.updates)
		}
	})

	t.Run("a free code creates a pending link and returns to checkout", func(t *testing.T) {
		s := newFixture()
		rec := codeRequest(url.Values{"vendor_org_id": {"700"}, "organization_number": {"C-2"}}, buyer, nil, codeHandler(s, true).CheckoutErpCodeSubmit)
		if loc := rec.Header().Get("Location"); loc != "/checkout" {
			t.Fatalf("Location = %q, want /checkout", loc)
		}
		if s.creates != 1 {
			t.Fatalf("creates = %d, want 1", s.creates)
		}
	})

	t.Run("a buyer's existing link for the supplier is updated", func(t *testing.T) {
		s := newFixture()
		s.addCodeLink(502, 41, pointerOrNil(buyerOrg), vendor, "OLD-1", org.UserOrgStatusPending)
		rec := codeRequest(url.Values{"vendor_org_id": {"700"}, "organization_number": {"C-3"}}, buyer, nil, codeHandler(s, true).CheckoutErpCodeSubmit)
		if loc := rec.Header().Get("Location"); loc != "/checkout" {
			t.Fatalf("Location = %q, want /checkout", loc)
		}
		if s.updates != 1 || s.creates != 0 || s.links[502].OrganizationNumber != "C-3" {
			t.Fatalf("updates=%d creates=%d number=%q, want one update to C-3", s.updates, s.creates, s.links[502].OrganizationNumber)
		}
	})
}
