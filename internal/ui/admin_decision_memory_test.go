package ui

import (
	"bytes"
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
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

type mockDecisionRepo struct {
	mockCatalogImageRepo
	decisions []*catalog.MatchDecisionView
	enabled   bool
	prefs     map[int64]bool
	promoted  []int64
	demoted   []int64
	relinked  map[int64]*int64
	deleted   []int64
}

func newMockDecisionRepo() *mockDecisionRepo {
	now := time.Now()
	prodID1 := int64(101)
	prodID2 := int64(102)
	orgID := int64(55)
	userID := int64(1)

	return &mockDecisionRepo{
		mockCatalogImageRepo: *newMockCatalogImageRepo(),
		enabled:              true,
		prefs:                make(map[int64]bool),
		relinked:             make(map[int64]*int64),
		decisions: []*catalog.MatchDecisionView{
			{
				ID:                1,
				OrganizationID:    &orgID,
				UserID:            &userID,
				DecisionKey:       "key-1",
				NormName:          "panadol extra 500mg",
				ChosenProductID:   &prodID1,
				ChosenProductName: "بنادول إكسترا 500 مجم",
				ChosenProductSKU:  "PAN-EXT-500",
				Confidence:        0.95,
				Reason:            "AI High Confidence Match",
				PromptVersion:     "v1",
				HitCount:          42,
				Scope:             "platform",
				Source:            "admin",
				OrganizationName:  "مستودع الأمل",
				UserName:          "أحمد علي",
				CreatedAt:         now.Add(-48 * time.Hour),
				LastUsedAt:        now.Add(-1 * time.Hour),
			},
			{
				ID:                  2,
				OrganizationID:      &orgID,
				UserID:              &userID,
				DecisionKey:         "key-2",
				NormName:            "augmentin 1g 14tab",
				ChosenProductID:     &prodID2,
				ChosenProductName:   "أوجمنتين 1 جم 14 قرص",
				ChosenProductSKU:    "AUG-1G",
				Confidence:          0.82,
				Reason:              "Manual Mapping",
				PromptVersion:       "v1",
				HitCount:            5,
				Scope:               "org",
				Source:              "manual",
				OrganizationName:    "مستودع الأمل",
				UserName:            "أحمد علي",
				CreatedAt:           now.Add(-24 * time.Hour),
				LastUsedAt:          now.Add(-2 * time.Hour),
				IsPlatformInherited: false,
			},
		},
	}
}

func (m *mockDecisionRepo) ListMatchDecisionsFiltered(_ context.Context, f catalog.DecisionMemoryFilter) ([]*catalog.MatchDecisionView, int, error) {
	var out []*catalog.MatchDecisionView
	for _, d := range m.decisions {
		if f.Scope != "" && d.Scope != f.Scope {
			continue
		}
		if f.Source != "" && d.Source != f.Source {
			continue
		}
		if f.Search != "" && !strings.Contains(strings.ToLower(d.NormName), strings.ToLower(f.Search)) {
			continue
		}
		out = append(out, d)
	}
	return out, len(out), nil
}

func (m *mockDecisionRepo) ListMatchDecisionsForOrgWithPlatform(_ context.Context, _ int64, _ string, _, _ int) ([]*catalog.MatchDecisionView, int, error) {
	return m.decisions, len(m.decisions), nil
}

func (m *mockDecisionRepo) GetDecisionMemoryPreference(_ context.Context, orgID int64) (bool, error) {
	if p, ok := m.prefs[orgID]; ok {
		return p, nil
	}
	return true, nil
}

func (m *mockDecisionRepo) SetDecisionMemoryPreference(_ context.Context, orgID int64, enabled bool, _ int64) error {
	m.prefs[orgID] = enabled
	return nil
}

func (m *mockDecisionRepo) PromoteMatchDecision(_ context.Context, id int64, _ int64) error {
	m.promoted = append(m.promoted, id)
	for _, d := range m.decisions {
		if d.ID == id {
			d.Scope = "platform"
		}
	}
	return nil
}

func (m *mockDecisionRepo) DemoteMatchDecision(_ context.Context, id int64) error {
	m.demoted = append(m.demoted, id)
	for _, d := range m.decisions {
		if d.ID == id {
			d.Scope = "org"
		}
	}
	return nil
}

func (m *mockDecisionRepo) RelinkMatchDecision(_ context.Context, id int64, productID *int64, _ int64) error {
	m.relinked[id] = productID
	for _, d := range m.decisions {
		if d.ID == id {
			d.ChosenProductID = productID
			d.Source = "admin"
		}
	}
	return nil
}

func (m *mockDecisionRepo) RelinkMatchDecisionForOrg(_ context.Context, _ int64, id int64, productID *int64, _ int64) error {
	m.relinked[id] = productID
	return nil
}

func (m *mockDecisionRepo) BulkPromoteMatchDecisions(_ context.Context, ids []int64, _ int64) (int64, error) {
	m.promoted = append(m.promoted, ids...)
	return int64(len(ids)), nil
}

func (m *mockDecisionRepo) BulkDeleteMatchDecisions(_ context.Context, ids []int64) (int64, error) {
	m.deleted = append(m.deleted, ids...)
	return int64(len(ids)), nil
}

func (m *mockDecisionRepo) DeleteMatchDecision(_ context.Context, id int64) error {
	m.deleted = append(m.deleted, id)
	return nil
}

func (m *mockDecisionRepo) ClearMatchDecisions(_ context.Context) error {
	m.decisions = nil
	return nil
}

func (m *mockDecisionRepo) IsDecisionMemoryEnabled(_ context.Context) bool {
	return m.enabled
}

func (m *mockDecisionRepo) SetDecisionMemoryEnabled(_ context.Context, en bool) error {
	m.enabled = en
	return nil
}

func (m *mockDecisionRepo) ListDosageForms(_ context.Context) ([]string, error) {
	return nil, nil
}

func setupDecisionTestHandler() (*UIHandler, *mockDecisionRepo) {
	repo := newMockDecisionRepo()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	catSvc := catalog.NewService(repo, logger)
	handler := &UIHandler{log: logger, catSvc: catSvc}
	return handler, repo
}

func TestAdminMatchDecisions_PageAndFilter(t *testing.T) {
	h, _ := setupDecisionTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/admin/match-decisions?scope=platform", nil)
	rec := httptest.NewRecorder()

	h.AdminMatchDecisionsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "panadol extra 500mg") {
		t.Errorf("expected platform decision in response, got body: %s", body)
	}
	if strings.Contains(body, "augmentin 1g 14tab") {
		t.Errorf("expected org decision to be filtered out when scope=platform")
	}
}

func TestAdminMatchDecisions_ExportXLSX(t *testing.T) {
	h, _ := setupDecisionTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/admin/match-decisions/export.xlsx", nil)
	rec := httptest.NewRecorder()

	h.AdminMatchDecisionsExportXLSX(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "spreadsheetml.sheet") {
		t.Errorf("expected xlsx content-type, got %q", contentType)
	}

	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("failed to open generated excel sheet: %v", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("failed to get rows from excel sheet: %v", err)
	}

	if len(rows) < 3 {
		t.Fatalf("expected at least 3 rows (header + 2 data rows), got %d", len(rows))
	}

	headers := rows[0]
	if len(headers) < 13 || headers[0] != "#" || headers[3] != "اسم الصنف الوارد" {
		t.Errorf("unexpected headers: %+v", headers)
	}

	row1 := rows[1]
	if row1[3] != "panadol extra 500mg" || row1[4] != "بنادول إكسترا 500 مجم" {
		t.Errorf("unexpected row 1 data: %+v", row1)
	}
}

func TestAdminMatchDecision_Actions(t *testing.T) {
	h, repo := setupDecisionTestHandler()

	// 1. Promote
	r := chi.NewRouter()
	r.Post("/admin/match-decisions/{id}/promote", h.AdminMatchDecisionPromoteSubmit)
	r.Post("/admin/match-decisions/{id}/demote", h.AdminMatchDecisionDemoteSubmit)
	r.Post("/admin/match-decisions/{id}/relink", h.AdminMatchDecisionRelinkSubmit)
	r.Post("/admin/match-decisions/bulk", h.AdminMatchDecisionBulkSubmit)
	r.Post("/admin/match-decisions/{id}/delete", h.AdminMatchDecisionDeleteSubmit)

	req := httptest.NewRequest(http.MethodPost, "/admin/match-decisions/2/promote", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("promote expected 303, got %d", rec.Code)
	}
	if len(repo.promoted) != 1 || repo.promoted[0] != 2 {
		t.Errorf("expected promoted id 2, got %v", repo.promoted)
	}

	// 2. Demote
	req = httptest.NewRequest(http.MethodPost, "/admin/match-decisions/1/demote", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("demote expected 303, got %d", rec.Code)
	}
	if len(repo.demoted) != 1 || repo.demoted[0] != 1 {
		t.Errorf("expected demoted id 1, got %v", repo.demoted)
	}

	// 3. Relink
	relinkForm := url.Values{"product_id": {"303"}}
	req = httptest.NewRequest(http.MethodPost, "/admin/match-decisions/1/relink", strings.NewReader(relinkForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("relink expected 303, got %d", rec.Code)
	}
	if repo.relinked[1] == nil || *repo.relinked[1] != 303 {
		t.Errorf("expected relinked product 303, got %v", repo.relinked[1])
	}

	// 4. Bulk Delete
	bulkForm := url.Values{"action": {"delete"}, "ids": {"1", "2"}}
	req = httptest.NewRequest(http.MethodPost, "/admin/match-decisions/bulk", strings.NewReader(bulkForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("bulk delete expected 303, got %d", rec.Code)
	}
	if len(repo.deleted) != 2 {
		t.Errorf("expected 2 deleted items, got %v", repo.deleted)
	}
}

func TestOrgDecisionMemory_Preferences(t *testing.T) {
	h, repo := setupDecisionTestHandler()

	r := chi.NewRouter()
	r.Post("/customer/decision-memory/preference", h.CustomerDecisionMemoryTogglePlatformSubmit)
	r.Post("/vendor/decision-memory/preference", h.VendorDecisionMemoryTogglePlatformSubmit)

	actor := authctx.Actor{
		OrgID:          88,
		OrganizationID: 88,
		UserID:         9,
		Role:           "customer",
	}

	// Customer opts out
	prefForm := url.Values{"use_platform_memory": {"false"}}
	req := httptest.NewRequest(http.MethodPost, "/customer/decision-memory/preference", strings.NewReader(prefForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("preference toggle expected 303, got %d", rec.Code)
	}
	if repo.prefs[88] != false {
		t.Errorf("expected preference for org 88 to be false, got %v", repo.prefs[88])
	}
}
