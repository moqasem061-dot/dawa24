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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type adminOrgChangesRepoStub struct {
	org.Repository
	requests []*org.ProfileChangeRequest
	decided  map[int64]bool
	notes    map[int64]string
}

func newAdminOrgChangesRepoStub() *adminOrgChangesRepoStub {
	now := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	revAt := now.Add(2 * time.Hour)
	revID1 := int64(201)
	revID2 := int64(202)

	return &adminOrgChangesRepoStub{
		decided: make(map[int64]bool),
		notes:   make(map[int64]string),
		requests: []*org.ProfileChangeRequest{
			{
				ID:               1,
				OrganizationID:   42,
				OrganizationName: "مستودع الأمل الحديث",
				RequestedBy:      101,
				RequesterName:    "أحمد محمد",
				Section:          org.SectionIdentity,
				Previous:         org.ProfileFields{"legal_name": "شركة الأمل القديمة", "commercial_register": "12345"},
				Proposed:         org.ProfileFields{"legal_name": "شركة الأمل المتطورة", "commercial_register": "12345"},
				Status:           org.ChangePending,
				CreatedAt:        now,
			},
			{
				ID:               2,
				OrganizationID:   42,
				OrganizationName: "مستودع الأمل الحديث",
				RequestedBy:      102,
				RequesterName:    "محمود حسن",
				ReviewedBy:       &revID1,
				ReviewerName:     "سارة علي",
				ReviewedAt:       &revAt,
				Section:          org.SectionLimits,
				Previous:         org.ProfileFields{"min_order_price": "50.00"},
				Proposed:         org.ProfileFields{"min_order_price": "100.00"},
				Status:           org.ChangeApproved,
				AdminNotes:       "تم التحقق من السجل التجاري المرفق",
				CreatedAt:        now,
			},
			{
				ID:               3,
				OrganizationID:   99,
				OrganizationName: "صيدلية النور",
				RequestedBy:      103,
				RequesterName:    "علي إبراهيم",
				ReviewedBy:       &revID2,
				ReviewerName:     "خالد عمر",
				ReviewedAt:       &revAt,
				Section:          org.SectionContact,
				Previous:         org.ProfileFields{"phone": "01011111111"},
				Proposed:         org.ProfileFields{"phone": "01022222222"},
				Status:           org.ChangeRejected,
				AdminNotes:       "الرقم غير مسجل في الوثيقة الرسمية",
				CreatedAt:        now,
			},
		},
	}
}

func (s *adminOrgChangesRepoStub) GetOrganizationByID(_ context.Context, id int64) (*org.Organization, error) {
	name := "مستودع الأمل الحديث"
	if id == 99 {
		name = "صيدلية النور"
	}
	return &org.Organization{
		ID:        id,
		LegalName: name,
		TradeName: i18n.Text{i18n.AR: name},
		Status:    org.StatusApproved,
	}, nil
}

func (s *adminOrgChangesRepoStub) ListOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus, _, _ int) ([]*org.Organization, error) {
	return []*org.Organization{
		{ID: 42, LegalName: "مستودع الأمل الحديث"},
		{ID: 99, LegalName: "صيدلية النور"},
	}, nil
}

func (s *adminOrgChangesRepoStub) ListProfileChangeRequestsWithFilter(
	_ context.Context, f org.ProfileChangeFilter, _, _ int,
) ([]*org.ProfileChangeRequest, int, error) {
	var filtered []*org.ProfileChangeRequest
	for _, req := range s.requests {
		if f.Status != "" && string(req.Status) != f.Status {
			continue
		}
		if f.Section != "" && string(req.Section) != f.Section {
			continue
		}
		if f.OrganizationID > 0 && req.OrganizationID != f.OrganizationID {
			continue
		}
		if f.Search != "" && !strings.Contains(req.OrganizationName, f.Search) {
			continue
		}
		filtered = append(filtered, req)
	}
	return filtered, len(filtered), nil
}

func (s *adminOrgChangesRepoStub) ProfileChangeRequestCounts(
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

func (s *adminOrgChangesRepoStub) DecideProfileChangeRequest(
	_ context.Context, id, reviewerID int64, approve bool, notes string,
	_ func(context.Context, pgx.Tx, *org.ProfileChangeRequest) error,
) (*org.ProfileChangeRequest, error) {
	s.decided[id] = approve
	s.notes[id] = notes
	for _, req := range s.requests {
		if req.ID == id {
			if approve {
				req.Status = org.ChangeApproved
			} else {
				req.Status = org.ChangeRejected
			}
			req.ReviewedBy = &reviewerID
			req.AdminNotes = notes
			return req, nil
		}
	}
	return nil, nil
}

func adminActor() authctx.Actor {
	return authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		Permissions: []string{"org.approval.decide", "*"},
	}
}

func TestAdminOrgChangesPage_ReadableQueueAndFilters(t *testing.T) {
	repo := newAdminOrgChangesRepoStub()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orgSvc := org.NewService(repo, logger)
	handler := ui.NewUIHandler(nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Get("/admin/organizations/change-requests", handler.AdminOrgChangesPage)

	// 1. Check Pending queue
	req := httptest.NewRequest("GET", "/admin/organizations/change-requests?status=pending", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), adminActor()))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Organization link with target="_blank"
	if !strings.Contains(body, `href="/admin/organizations/42"`) || !strings.Contains(body, `target="_blank"`) {
		t.Error("expected organization link to /admin/organizations/42 with target='_blank'")
	}
	if !strings.Contains(body, "مستودع الأمل الحديث") {
		t.Error("expected organization name 'مستودع الأمل الحديث' in page")
	}

	// Requester link with target="_blank" and requester name
	if !strings.Contains(body, `href="/admin/users/101"`) {
		t.Error("expected requester link to /admin/users/101")
	}
	if !strings.Contains(body, "أحمد محمد") {
		t.Error("expected requester name 'أحمد محمد' in page")
	}

	// Tab count badges from single grouped query
	if !strings.Contains(body, "tab-btn-count") {
		t.Error("expected tab-btn-count badges in tabs")
	}

	// Before -> After diff table
	diffHeaders := []string{"الحقل", "القيمة السابقة", "القيمة المقترحة"}
	for _, h := range diffHeaders {
		if !strings.Contains(body, h) {
			t.Errorf("expected diff table column header %q", h)
		}
	}
	// Localized field label and values
	if !strings.Contains(body, "الاسم القانوني") {
		t.Error("expected localized field label 'الاسم القانوني'")
	}
	if !strings.Contains(body, "شركة الأمل القديمة") {
		t.Error("expected previous value 'شركة الأمل القديمة'")
	}
	if !strings.Contains(body, "شركة الأمل المتطورة") {
		t.Error("expected proposed value 'شركة الأمل المتطورة'")
	}

	// Filter bar controls
	if !strings.Contains(body, `name="q"`) || !strings.Contains(body, `name="section"`) ||
		!strings.Contains(body, `name="org_id"`) ||
		!strings.Contains(body, `name="from"`) || !strings.Contains(body, `name="to"`) {
		t.Error("expected filter toolbar inputs for q, section, org_id, from, to")
	}

	// Approve and reject buttons on pending request
	if !strings.Contains(body, `/admin/organizations/change-requests/1/approve`) {
		t.Error("expected approve form action")
	}
	if !strings.Contains(body, `/admin/organizations/change-requests/1/reject`) {
		t.Error("expected reject form action")
	}
}

func TestAdminOrgChangesPage_ApprovedAndRejectedCards(t *testing.T) {
	repo := newAdminOrgChangesRepoStub()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orgSvc := org.NewService(repo, logger)
	handler := ui.NewUIHandler(nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Get("/admin/organizations/change-requests", handler.AdminOrgChangesPage)

	// Approved tab
	req := httptest.NewRequest("GET", "/admin/organizations/change-requests?status=approved", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), adminActor()))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// Reviewer link and name
	if !strings.Contains(body, `href="/admin/users/201"`) {
		t.Error("expected reviewer link to /admin/users/201")
	}
	if !strings.Contains(body, "سارة علي") {
		t.Error("expected reviewer name 'سارة علي'")
	}
	// Admin notes
	if !strings.Contains(body, "تم التحقق من السجل التجاري المرفق") {
		t.Error("expected admin notes on approved card")
	}
}

func TestAdminOrgChanges_ApproveAndRejectSubmissions(t *testing.T) {
	repo := newAdminOrgChangesRepoStub()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orgSvc := org.NewService(repo, logger)
	handler := ui.NewUIHandler(nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Post("/admin/organizations/change-requests/{id}/approve", handler.AdminOrgChangeApproveSubmit)
	r.Post("/admin/organizations/change-requests/{id}/reject", handler.AdminOrgChangeRejectSubmit)

	// 1. Approve submit
	req := httptest.NewRequest("POST", "/admin/organizations/change-requests/1/approve", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), adminActor()))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", rec.Code)
	}
	if !repo.decided[1] {
		t.Error("expected request 1 to be approved")
	}

	// 2. Reject submit with notes
	form := url.Values{}
	form.Set("notes", "سجل تجاري غير مطابق")
	reqReject := httptest.NewRequest("POST", "/admin/organizations/change-requests/1/reject", strings.NewReader(form.Encode()))
	reqReject.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqReject = reqReject.WithContext(authctx.WithActor(req.Context(), adminActor()))
	recReject := httptest.NewRecorder()
	r.ServeHTTP(recReject, reqReject)

	if recReject.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", recReject.Code)
	}
	if repo.decided[1] {
		t.Error("expected request 1 to be rejected (false)")
	}
	if repo.notes[1] != "سجل تجاري غير مطابق" {
		t.Errorf("expected notes 'سجل تجاري غير مطابق', got %q", repo.notes[1])
	}
}
