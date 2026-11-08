package ui

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
)

// The reveal fixtures are in-memory: each service sits on a fake repository that
// embeds the repository interface and overrides only the methods the reveal path
// calls. No database is needed; the audit fake records every row the handler asks
// the identity service to write.

const (
	revealOrgID        int64 = 3
	revealOtherOrgID   int64 = 4
	revealOrderID      int64 = 77
	revealTaxValue           = "TAX-998877"
	revealCRValue            = "CR-5544332"
	revealLicenseValue       = "PL-7766554"
	revealPhoneValue         = "+201001234567"
	revealEmailValue         = "ops@pharmacy.example"
	revealBranchPhone        = "01099887766"
	revealGatewayUser        = "gwadmin"
	revealGatewayPass        = "gw-secret-never-shown"
)

type revealAuditRepo struct {
	identity.Repository
	mu        sync.Mutex
	entries   []database.AuditEntry
	failAudit bool
}

func (r *revealAuditRepo) RecordAudit(_ context.Context, e database.AuditEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failAudit {
		return errors.New("audit store unavailable")
	}
	r.entries = append(r.entries, e)
	return nil
}

func (r *revealAuditRepo) rows() []database.AuditEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]database.AuditEntry(nil), r.entries...)
}

type revealOrgRepo struct {
	org.Repository
	orgs map[int64]*org.Organization
}

func (r *revealOrgRepo) GetOrganizationByID(_ context.Context, id int64) (*org.Organization, error) {
	o, ok := r.orgs[id]
	if !ok {
		return nil, errors.New("organization not found")
	}
	return o, nil
}

type revealCommerceRepo struct {
	commerce.Repository
	byOrder map[int64]*commerce.OrderShipment
}

func (r *revealCommerceRepo) GetVendorShipment(context.Context, int64, int64) (*commerce.OrderShipment, error) {
	return nil, nil
}

func (r *revealCommerceRepo) GetVendorShipmentByOrderID(_ context.Context, orderID, vendorOrgID int64) (*commerce.OrderShipment, error) {
	sh, ok := r.byOrder[orderID]
	if !ok || sh.OrganizationID != vendorOrgID {
		return nil, nil
	}
	return sh, nil
}

type revealAdminRepo struct {
	platformadmin.Repository
	settings map[string]*platformadmin.SystemSetting
}

func (r *revealAdminRepo) GetSetting(_ context.Context, key string) (*platformadmin.SystemSetting, error) {
	return r.settings[key], nil
}

type revealFixture struct {
	handler *UIHandler
	audit   *revealAuditRepo
}

func newRevealFixture(t *testing.T) *revealFixture {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	audit := &revealAuditRepo{}
	orgs := &revealOrgRepo{orgs: map[int64]*org.Organization{
		revealOrgID: {
			ID:                 revealOrgID,
			TaxNumber:          revealTaxValue,
			CommercialRegister: revealCRValue,
			PharmacistLicense:  revealLicenseValue,
			Phone:              revealPhoneValue,
			Email:              revealEmailValue,
		},
	}}
	comm := &revealCommerceRepo{byOrder: map[int64]*commerce.OrderShipment{
		revealOrderID: {ID: 700, OrderID: revealOrderID, OrganizationID: revealOrgID, CustomerBranchPhone: revealBranchPhone},
	}}
	admin := &revealAdminRepo{settings: map[string]*platformadmin.SystemSetting{
		"gateway_configuration": {Value: map[string]any{
			"endpoint_url": "https://api.dawa24.com",
			"api_key":      revealGatewayUser + ":" + revealGatewayPass,
		}},
	}}
	h := NewUIHandler(nil,
		org.NewService(orgs, log),
		nil,
		commerce.NewService(comm, log),
		nil,
		identity.NewService(audit, nil, log),
		nil, nil,
		platformadmin.NewService(admin, log),
		nil, nil, nil, nil, nil, log)
	return &revealFixture{handler: h, audit: audit}
}

func revealStaff() authctx.Actor {
	return authctx.Actor{UserID: 900, IsStaff: true, Permissions: []string{"platform.reveal.sensitive"}}
}

func revealVendor() authctx.Actor {
	return authctx.Actor{
		UserID: 901, OrganizationID: revealOrgID, OrgID: revealOrgID,
		OrgType: "vendor", OrgStatus: "approved",
		Permissions: []string{"vendor.reveal.customer_contact"},
	}
}

func revealPost(t *testing.T, f *revealFixture, actor authctx.Actor, kind, id string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"kind": {kind}, "id": {id}}
	req := httptest.NewRequest(http.MethodPost, "/reveal", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	f.handler.RevealSubmit(rec, req)
	return rec
}

func lastRevealRow(t *testing.T, f *revealFixture) database.AuditEntry {
	t.Helper()
	rows := f.audit.rows()
	if len(rows) == 0 {
		t.Fatalf("no audit row was written")
	}
	return rows[len(rows)-1]
}

func TestRevealGrantsEachAllowlistedKind(t *testing.T) {
	cases := []struct {
		name, kind, id, value string
		actor                 authctx.Actor
	}{
		{"tax number", "org.tax_number", "3", revealTaxValue, revealStaff()},
		{"commercial register", "org.commercial_register", "3", revealCRValue, revealStaff()},
		{"pharmacist license", "org.pharmacist_license", "3", revealLicenseValue, revealStaff()},
		{"organization phone", "org.phone", "3", revealPhoneValue, revealStaff()},
		{"organization email", "org.email", "3", revealEmailValue, revealStaff()},
		{"gateway admin username", "gateway.admin_username", "0", revealGatewayUser, revealStaff()},
		{"buyer branch phone for a shipped order", "branch.phone", "77", revealBranchPhone, revealVendor()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRevealFixture(t)
			rec := revealPost(t, f, tc.actor, tc.kind, tc.id)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body %q", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.value) {
				t.Fatalf("grant body does not hold the value")
			}
			row := lastRevealRow(t, f)
			if row.Action != "reveal.grant" || row.EntityType != tc.kind || row.EntityID != tc.id {
				t.Fatalf("audit row = %s %s %s, want reveal.grant %s %s", row.Action, row.EntityType, row.EntityID, tc.kind, tc.id)
			}
			if row.Before != nil || row.After != nil {
				t.Fatalf("audit row carries a before or after value")
			}
		})
	}
}

func TestRevealRefusesWithoutTheKindPermission(t *testing.T) {
	cases := []struct {
		name, kind, id, value string
		actor                 authctx.Actor
	}{
		{"vendor cannot reveal an organization tax number", "org.tax_number", "3", revealTaxValue, revealVendor()},
		{"staff cannot reveal a buyer branch phone without the supplier grant", "branch.phone", "77", revealBranchPhone, revealStaff()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRevealFixture(t)
			rec := revealPost(t, f, tc.actor, tc.kind, tc.id)
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "REVEAL_FORBIDDEN") {
				t.Fatalf("status = %d body %q, want 403 REVEAL_FORBIDDEN", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), tc.value) {
				t.Fatalf("refusal body holds the value")
			}
			if row := lastRevealRow(t, f); row.Action != "reveal.deny" || row.EntityType != tc.kind {
				t.Fatalf("audit row = %s %s, want reveal.deny %s", row.Action, row.EntityType, tc.kind)
			}
		})
	}
}

func TestRevealNeverRevealsSecretsOrUnknownKinds(t *testing.T) {
	kinds := []string{
		"gateway.admin_password",
		"gateway.api_key",
		"gateway.virtual_key",
		"ai.provider_key",
		"erp.api_key",
		"org.password_hash",
		"not.a.kind",
	}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			f := newRevealFixture(t)
			staff := revealStaff()
			staff.Permissions = append(staff.Permissions, "vendor.reveal.customer_contact")
			rec := revealPost(t, f, staff, kind, "0")
			if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "REVEAL_NOT_REVEALABLE") {
				t.Fatalf("status = %d body %q, want 422 REVEAL_NOT_REVEALABLE", rec.Code, rec.Body.String())
			}
			for _, secret := range []string{revealGatewayPass, revealGatewayUser + ":" + revealGatewayPass} {
				if strings.Contains(rec.Body.String(), secret) {
					t.Fatalf("refusal body holds the gateway secret")
				}
			}
			if row := lastRevealRow(t, f); row.Action != "reveal.deny" {
				t.Fatalf("audit action = %s, want reveal.deny", row.Action)
			}
		})
	}
}

func TestRevealNotFoundDoesNotConfirmExistence(t *testing.T) {
	t.Run("missing organization", func(t *testing.T) {
		f := newRevealFixture(t)
		rec := revealPost(t, f, revealStaff(), "org.tax_number", "999")
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "REVEAL_NOT_FOUND") {
			t.Fatalf("status = %d body %q, want 404 REVEAL_NOT_FOUND", rec.Code, rec.Body.String())
		}
		if row := lastRevealRow(t, f); row.Action != "reveal.deny" {
			t.Fatalf("audit action = %s, want reveal.deny", row.Action)
		}
	})
	t.Run("order the caller did not ship", func(t *testing.T) {
		f := newRevealFixture(t)
		other := revealVendor()
		other.OrganizationID, other.OrgID = revealOtherOrgID, revealOtherOrgID
		rec := revealPost(t, f, other, "branch.phone", "77")
		if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), revealBranchPhone) {
			t.Fatalf("status = %d, want 404 with no phone in the body", rec.Code)
		}
	})
}

func TestRevealWritesOneAuditRowPerOutcome(t *testing.T) {
	f := newRevealFixture(t)
	revealPost(t, f, revealStaff(), "org.tax_number", "3")
	revealPost(t, f, revealStaff(), "org.tax_number", "3")
	revealPost(t, f, revealVendor(), "org.tax_number", "3")
	revealPost(t, f, revealStaff(), "gateway.admin_password", "0")
	revealPost(t, f, revealStaff(), "org.tax_number", "999")

	want := []string{"reveal.grant", "reveal.grant", "reveal.deny", "reveal.deny", "reveal.deny"}
	rows := f.audit.rows()
	if len(rows) != len(want) {
		t.Fatalf("audit rows = %d, want %d", len(rows), len(want))
	}
	for i, action := range want {
		if rows[i].Action != action {
			t.Fatalf("row %d action = %s, want %s", i, rows[i].Action, action)
		}
	}
}

func TestRevealFailsClosedWhenTheAuditRowCannotBeWritten(t *testing.T) {
	f := newRevealFixture(t)
	f.audit.failAudit = true
	rec := revealPost(t, f, revealStaff(), "org.tax_number", "3")
	if rec.Code == http.StatusOK {
		t.Fatalf("status = 200 with no audit row; the value must not be shown")
	}
	if strings.Contains(rec.Body.String(), revealTaxValue) {
		t.Fatalf("body holds the value although the audit row failed")
	}
}

func TestRevealRequiresTheCSRFToken(t *testing.T) {
	f := newRevealFixture(t)
	form := url.Values{"kind": {"org.tax_number"}, "id": {"3"}}
	req := httptest.NewRequest(http.MethodPost, "/reveal", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), revealStaff()))
	rec := httptest.NewRecorder()
	httpx.CSRF(true)(http.HandlerFunc(f.handler.RevealSubmit)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 without a CSRF token", rec.Code)
	}
	if strings.Contains(rec.Body.String(), revealTaxValue) {
		t.Fatalf("body holds the value without a CSRF token")
	}
	if n := len(f.audit.rows()); n != 0 {
		t.Fatalf("audit rows = %d for a request the CSRF check refused, want 0", n)
	}
}

func TestRevealRouteIsMountedOnTheApprovedTier(t *testing.T) {
	f := newRevealFixture(t)
	r := chi.NewRouter()
	f.handler.RegisterApprovedSharedRoutes(r)
	form := url.Values{"kind": {"org.tax_number"}, "id": {"3"}}
	req := httptest.NewRequest(http.MethodPost, "/reveal", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), revealStaff()))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), revealTaxValue) {
		t.Fatalf("status = %d, want the revealed value from POST /reveal", rec.Code)
	}
}
