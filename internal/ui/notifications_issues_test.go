package ui_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockIssueNotifRepo struct {
	notifications.Repository
	sentLogs []*notifications.NotificationLog
}

func (m *mockIssueNotifRepo) CreateLog(_ context.Context, l *notifications.NotificationLog) error {
	l.ID = int64(len(m.sentLogs) + 1)
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	m.sentLogs = append(m.sentLogs, l)
	return nil
}

func (m *mockIssueNotifRepo) HasNotificationWithTitle(_ context.Context, userID int64, title string, since time.Time) (bool, error) {
	for _, l := range m.sentLogs {
		if l.UserID == userID && l.Title == title && (since.IsZero() || !l.CreatedAt.Before(since)) {
			return true, nil
		}
	}
	return false, nil
}


type mockIssueWorkflowRepo struct {
	workflow.Repository
	issues map[int64]*workflow.ReportIssue
	nextID int64
}

func (m *mockIssueWorkflowRepo) CreateIssue(_ context.Context, i *workflow.ReportIssue) error {
	if m.nextID == 0 {
		m.nextID = 1
	}
	i.ID = m.nextID
	m.nextID++
	i.CreatedAt = time.Now()
	m.issues[i.ID] = i
	return nil
}

func (m *mockIssueWorkflowRepo) GetIssueByID(_ context.Context, id int64) (*workflow.ReportIssue, error) {
	if iss, ok := m.issues[id]; ok {
		return iss, nil
	}
	return nil, fmt.Errorf("issue not found")
}

func (m *mockIssueWorkflowRepo) UpdateIssueStatus(_ context.Context, id int64, status, notes string) error {
	if iss, ok := m.issues[id]; ok {
		iss.Status = status
		iss.ResponseNotes = notes
		return nil
	}
	return fmt.Errorf("issue not found")
}

type mockIssueIdentityRepo struct {
	identity.Repository
	staffIDs []int64
	users    map[int64]*identity.User
}

func (m *mockIssueIdentityRepo) ListStaffUserIDs(_ context.Context) ([]int64, error) {
	return m.staffIDs, nil
}

func (m *mockIssueIdentityRepo) GetUserByID(_ context.Context, id int64) (*identity.User, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return &identity.User{ID: id, Name: i18n.New("مستخدم تجريبي", "Test User")}, nil
}

func (m *mockIssueIdentityRepo) AdminGetUser(ctx context.Context, id int64) (*identity.User, error) {
	return m.GetUserByID(ctx, id)
}

func setupIssueNotificationFixture() (*ui.UIHandler, *mockIssueNotifRepo, *mockIssueWorkflowRepo) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	notifRepo := &mockIssueNotifRepo{}
	notifSvc := notifications.NewService(notifRepo, logger)

	wfRepo := &mockIssueWorkflowRepo{issues: make(map[int64]*workflow.ReportIssue)}
	wfSvc := workflow.NewService(wfRepo, logger)

	idRepo := &mockIssueIdentityRepo{
		staffIDs: []int64{99, 100},
		users: map[int64]*identity.User{
			25: {ID: 25, Name: i18n.New("د. أحمد الصيدلي", "Dr. Ahmed")},
		},
	}
	idSvc := identity.NewService(idRepo, nil, logger)

	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, idSvc, notifSvc, nil, nil, nil, nil, wfSvc, nil, nil, logger)
	return handler, notifRepo, wfRepo
}

func TestIssueNotification_CustomerReportNotifiesAdmins(t *testing.T) {
	handler, notifRepo, _ := setupIssueNotificationFixture()

	form := url.Values{}
	form.Set("issue_type", "technical")
	form.Set("priority", "high")
	form.Set("description", "زر الدفع لا يستجيب في صفحة الطلبات")

	req := httptest.NewRequest(http.MethodPost, "/report-issue", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	actor := authctx.Actor{
		UserID:         25,
		OrganizationID: 200,
		Role:           "customer",
		OrgType:        "customer",
		Scope:          rbac.ScopePharmacy,
	}
	ctx := authctx.WithActor(req.Context(), actor)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.CustomerReportIssueSubmit(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rr.Code)
	}

	if len(notifRepo.sentLogs) == 0 {
		t.Fatalf("expected admin notifications to be dispatched, got 0")
	}

	staffNotified := false
	for _, l := range notifRepo.sentLogs {
		if l.UserID == 99 || l.UserID == 100 {
			staffNotified = true
			if !strings.Contains(l.Title, "بلاغ دعم جديد") {
				t.Errorf("expected title to mention new support report, got %q", l.Title)
			}
			if !strings.Contains(l.Body, "زر الدفع لا يستجيب") {
				t.Errorf("expected body to contain issue description, got %q", l.Body)
			}
			if !strings.Contains(l.Body, "عاجلة") {
				t.Errorf("expected body to contain priority label, got %q", l.Body)
			}
			if l.RequiredPermission != "workflow.issue.view" {
				t.Errorf("expected required permission workflow.issue.view, got %q", l.RequiredPermission)
			}
		}
	}

	if !staffNotified {
		t.Errorf("expected platform staff IDs (99 or 100) to be notified")
	}
}

func TestIssueNotification_AdminResponseNotifiesReporter(t *testing.T) {
	handler, notifRepo, wfRepo := setupIssueNotificationFixture()

	existing := &workflow.ReportIssue{
		ID:          10,
		ReportedBy:  25,
		IssueType:   "billing",
		Priority:    "medium",
		Description: "مشكلة في خصم رصيد المحفظة",
		Status:      "pending",
	}
	wfRepo.issues[10] = existing

	form := url.Values{}
	form.Set("status", "resolved")
	form.Set("response_notes", "تم مراجعة العملية وإعادة الرصيد إلى محفظتك بنجاح.")

	req := httptest.NewRequest(http.MethodPost, "/admin/report-issues/10/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	adminActor := authctx.Actor{
		UserID:      99,
		Role:        "admin",
		IsStaff:     true,
		Permissions: []string{"workflow.issue.update"},
	}
	ctx := authctx.WithActor(req.Context(), adminActor)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "10")
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.AdminReportIssueUpdateSubmit(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rr.Code)
	}

	userNotified := false
	for _, l := range notifRepo.sentLogs {
		if l.UserID == 25 {
			userNotified = true
			if !strings.Contains(l.Title, "تم الرد وحل بلاغك") {
				t.Errorf("expected title to mention resolution, got %q", l.Title)
			}
			if !strings.Contains(l.Body, "تم مراجعة العملية وإعادة الرصيد إلى محفظتك بنجاح") {
				t.Errorf("expected body to contain admin response notes, got %q", l.Body)
			}
			if !strings.Contains(l.Body, "تم الحل بنجاح") {
				t.Errorf("expected body to mention status, got %q", l.Body)
			}
		}
	}

	if !userNotified {
		t.Errorf("expected reporting user (ID 25) to receive the admin response notification")
	}
}
