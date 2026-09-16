package ui_test

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
	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// بيانات المنشأة, section by section.
//
// The bug this replaced: one form, one button, and a POST that rewrote every
// field of every section from whatever the browser happened to send. The
// property worth pinning is that saving one section leaves the others exactly
// as they were, and that the identity section does not touch the organization
// at all until a moderator says so.

type profileRepoStub struct {
	org.Repository
	fields        map[org.ProfileSection]org.ProfileFields
	requests      []*org.ProfileChangeRequest
	savedPolicies []*org.Policy
	nextID        int64
}

func newProfileRepoStub() *profileRepoStub {
	return &profileRepoStub{fields: map[org.ProfileSection]org.ProfileFields{
		org.SectionIdentity: {
			"legal_name": "شركة سمارت كودز", "trade_name_ar": "سمارت كودز",
			"trade_name_en": "Smart Codes", "commercial_register": "CR-1", "tax_number": "TX431256",
		},
		org.SectionLimits:      {"min_order_price": "10.00", "max_order_price": "50.00"},
		org.SectionContact:     {"email": "info@sc.com", "phone": "01099887766", "address": "Giza", "organization_number": "267244"},
		org.SectionDescription: {"description_ar": "وصف", "description_en": "Description"},
		org.SectionMedia:       {"image": "/logo.png", "coverage_image": ""},
	}}
}

func (s *profileRepoStub) GetOrganizationByID(_ context.Context, id int64) (*org.Organization, error) {
	return &org.Organization{
		ID:        id,
		LegalName: "شركة سمارت كودز",
		TradeName: i18n.Text{i18n.AR: "سمارت كودز"},
		Status:    org.StatusApproved,
	}, nil
}

func (s *profileRepoStub) GetPendingOrgDeletionRequest(_ context.Context, _ int64) (*org.OrganizationDeletionRequest, error) {
	return nil, nil
}

func (s *profileRepoStub) ReadProfileSection(
	_ context.Context, _ int64, section org.ProfileSection,
) (org.ProfileFields, error) {
	out := org.ProfileFields{}
	for k, v := range s.fields[section] {
		out[k] = v
	}
	return out, nil
}

func (s *profileRepoStub) ApplyProfileSection(
	_ context.Context, _ int64, section org.ProfileSection, fields org.ProfileFields,
) error {
	if s.fields[section] == nil {
		s.fields[section] = org.ProfileFields{}
	}
	for k, v := range fields {
		s.fields[section][k] = v
	}
	return nil
}

func (s *profileRepoStub) ApplyApprovedProfileChange(
	ctx context.Context, _ pgx.Tx, req *org.ProfileChangeRequest,
) error {
	return s.ApplyProfileSection(ctx, req.OrganizationID, req.Section, req.Proposed)
}

func (s *profileRepoStub) CreateProfileChangeRequest(_ context.Context, req *org.ProfileChangeRequest) error {
	s.nextID++
	req.ID = s.nextID
	req.Status = org.ChangePending
	s.requests = append(s.requests, req)
	return nil
}

func (s *profileRepoStub) PendingProfileChanges(
	_ context.Context, _ int64,
) (map[org.ProfileSection]*org.ProfileChangeRequest, error) {
	out := map[org.ProfileSection]*org.ProfileChangeRequest{}
	for _, req := range s.requests {
		if req.Status == org.ChangePending {
			out[req.Section] = req
		}
	}
	return out, nil
}

func (s *profileRepoStub) GetProfileChangeRequest(_ context.Context, id int64) (*org.ProfileChangeRequest, error) {
	for _, req := range s.requests {
		if req.ID == id {
			return req, nil
		}
	}
	return nil, nil
}

func (s *profileRepoStub) ListProfileChangeRequests(
	_ context.Context, _ string, _, _ int,
) ([]*org.ProfileChangeRequest, int, error) {
	return s.requests, len(s.requests), nil
}

func (s *profileRepoStub) ListProfileChangeRequestsWithFilter(
	_ context.Context, _ org.ProfileChangeFilter, _, _ int,
) ([]*org.ProfileChangeRequest, int, error) {
	return s.requests, len(s.requests), nil
}

func (s *profileRepoStub) ProfileChangeRequestCounts(
	_ context.Context,
) (org.ProfileChangeCounts, error) {
	var c org.ProfileChangeCounts
	for _, req := range s.requests {
		switch req.Status {
		case org.ChangePending:
			c.Pending++
		case org.ChangeApproved:
			c.Approved++
		case org.ChangeRejected:
			c.Rejected++
		case org.ChangeWithdrawn:
			c.Withdrawn++
		}
		c.All++
	}
	return c, nil
}

func (s *profileRepoStub) DecideProfileChangeRequest(
	_ context.Context, _, _ int64, _ bool, _ string,
	_ func(context.Context, pgx.Tx, *org.ProfileChangeRequest) error,
) (*org.ProfileChangeRequest, error) {
	return nil, nil
}

func (s *profileRepoStub) WithdrawProfileChangeRequest(_ context.Context, _, id int64) error {
	for _, req := range s.requests {
		if req.ID == id {
			req.Status = org.ChangeWithdrawn
		}
	}
	return nil
}

func (s *profileRepoStub) SavePolicies(_ context.Context, _ int64, policies []*org.Policy) error {
	s.savedPolicies = policies
	return nil
}

func (s *profileRepoStub) ListPoliciesByOrg(_ context.Context, _ int64) ([]*org.Policy, error) {
	return s.savedPolicies, nil
}

func profileHandler(repo org.Repository) *ui.UIHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return ui.NewUIHandler(
		nil, org.NewService(repo, logger), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
}

func vendorProfileActor() authctx.Actor {
	return authctx.Actor{
		UserID: 10, OrganizationID: 42, OrgType: "vendor",
		Permissions: []string{"vendor.organization.view", "vendor.organization.update"},
	}
}

func TestOrganizationProfilePage_RendersEverySection(t *testing.T) {
	repo := newProfileRepoStub()
	h := profileHandler(repo)

	req := httptest.NewRequest("GET", "/vendor/organization", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), vendorProfileActor()))
	rec := httptest.NewRecorder()
	h.OrganizationProfilePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"سمارت كودز", "Smart Codes", "TX431256", "267244", "10.00", "50.00",
		"section-identity", "section-limits", "section-contact",
		"section-description", "section-media",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not show %q", want)
		}
	}
	// Each section posts on its own; one form for all five is the bug.
	if !strings.Contains(body, `action="/vendor/organization/contact"`) {
		t.Error("the contact section does not post to its own URL")
	}
}

// Saving one section must not touch the others. This is the whole point of the
// change: the old page's single POST rewrote the trade name whenever anyone
// corrected a phone number.
func TestOrganizationProfile_SectionSaveLeavesOtherSectionsAlone(t *testing.T) {
	repo := newProfileRepoStub()
	h := profileHandler(repo)

	form := url.Values{}
	form.Set("email", "new@sc.com")
	form.Set("phone", "0100000000")
	form.Set("address", "Cairo")
	form.Set("organization_number", "998877")

	req := httptest.NewRequest("POST", "/vendor/organization/contact", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), vendorProfileActor()))
	req = withChiParam(req, "section", "contact")
	rec := httptest.NewRecorder()
	h.OrganizationProfileSectionSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := repo.fields[org.SectionContact]["email"]; got != "new@sc.com" {
		t.Errorf("the contact section was not saved, email is %q", got)
	}
	if got := repo.fields[org.SectionIdentity]["trade_name_ar"]; got != "سمارت كودز" {
		t.Errorf("saving contact details rewrote the trade name to %q", got)
	}
	if got := repo.fields[org.SectionLimits]["min_order_price"]; got != "10.00" {
		t.Errorf("saving contact details rewrote the order limits to %q", got)
	}
}

// The identity section opens a request instead of writing the row.
func TestOrganizationProfile_IdentityWaitsForReview(t *testing.T) {
	repo := newProfileRepoStub()
	h := profileHandler(repo)

	form := url.Values{}
	form.Set("legal_name", "شركة سمارت كودز الدولية")
	form.Set("trade_name_ar", "سمارت كودز الدولية")
	form.Set("trade_name_en", "Smart Codes Global")
	form.Set("commercial_register", "CR-1")
	form.Set("tax_number", "TX431256")

	req := httptest.NewRequest("POST", "/vendor/organization/identity", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), vendorProfileActor()))
	req = withChiParam(req, "section", "identity")
	rec := httptest.NewRecorder()
	h.OrganizationProfileSectionSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	if got := repo.fields[org.SectionIdentity]["legal_name"]; got != "شركة سمارت كودز" {
		t.Errorf("the identity change was applied without review, legal name is %q", got)
	}
	if len(repo.requests) != 1 {
		t.Fatalf("expected one pending request, got %d", len(repo.requests))
	}
	if repo.requests[0].Section != org.SectionIdentity {
		t.Errorf("the request is for %q", repo.requests[0].Section)
	}
}

// An unknown section is not a section, and a 404 says so. Falling through to a
// default branch would let /vendor/organization/anything write something.
func TestOrganizationProfile_UnknownSectionIs404(t *testing.T) {
	h := profileHandler(newProfileRepoStub())

	req := httptest.NewRequest("POST", "/vendor/organization/whatever", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), vendorProfileActor()))
	req = withChiParam(req, "section", "whatever")
	rec := httptest.NewRecorder()
	h.OrganizationProfileSectionSubmit(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for an unknown section, got %d", rec.Code)
	}
}

// withChiParam attaches a URL parameter the way the router would, so a handler
// can be exercised without standing up the whole route tree.
func withChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestVendorPoliciesSubmit_SavesWarrantyAndTrims(t *testing.T) {
	repo := newProfileRepoStub()
	h := profileHandler(repo)

	form := url.Values{
		"shipping_policy": {"   الشحن السريع للمحافظات   "},
		"returns_policy":  {"   قبول المرتجعات خلال 14 يوم   "},
		"terms_policy":    {""}, // Empty should be omitted
		"warranty_policy": {"   ضمان هيئة الدواء وسلسلة التبريد   "},
	}

	req := httptest.NewRequest("POST", "/vendor/policies", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), vendorProfileActor()))
	rec := httptest.NewRecorder()

	h.VendorPoliciesSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", rec.Code)
	}

	if len(repo.savedPolicies) != 3 {
		t.Fatalf("expected 3 non-empty policies saved, got %d", len(repo.savedPolicies))
	}

	foundWarranty := false
	for _, p := range repo.savedPolicies {
		if p.PolicyType == org.PolicyTypeWarranty {
			foundWarranty = true
			if p.Content != "ضمان هيئة الدواء وسلسلة التبريد" {
				t.Errorf("expected trimmed content, got %q", p.Content)
			}
			if p.Title != "سياسة الضمان والجودة" {
				t.Errorf("expected warranty title, got %q", p.Title)
			}
		}
	}
	if !foundWarranty {
		t.Errorf("expected policy with PolicyType %q to be saved", org.PolicyTypeWarranty)
	}
}

func TestVendorPoliciesPage_BackwardCompatibilityWithPrivacy(t *testing.T) {
	repo := newProfileRepoStub()
	repo.savedPolicies = []*org.Policy{
		{
			Title:      "سياسة الضمان والجودة",
			Content:    "ضمان معتمد",
			PolicyType: org.PolicyTypePrivacy, // Legacy fallback
			IsActive:   true,
		},
	}
	h := profileHandler(repo)

	req := httptest.NewRequest("GET", "/vendor/policies", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), vendorProfileActor()))
	rec := httptest.NewRecorder()

	h.VendorPoliciesPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ضمان معتمد") {
		t.Errorf("expected page to contain warranty policy content from privacy fallback, but did not")
	}
}
