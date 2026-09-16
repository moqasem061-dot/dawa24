package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockGeographyRepo struct {
	platformadmin.Repository
	governorates []*platformadmin.Governorate
	cities       []*platformadmin.City
}

func newMockGeographyRepo() *mockGeographyRepo {
	govID := int64(1)
	return &mockGeographyRepo{
		governorates: []*platformadmin.Governorate{
			{
				ID:                   govID,
				CountryID:            1,
				Name:                 i18n.New("القاهرة", "Cairo"),
				Latitude:             30.0444,
				Longitude:            31.2357,
				CoverageRadiusMeters: 25000,
				IsActive:             true,
				CityCount:            2,
			},
		},
		cities: []*platformadmin.City{
			{
				ID:                   1,
				CountryID:            1,
				GovernorateID:        &govID,
				GovernorateName:      &i18n.Text{"ar": "القاهرة", "en": "Cairo"},
				Name:                 i18n.New("مدينة نصر", "Nasr City"),
				Latitude:             30.0500,
				Longitude:            31.3300,
				CoverageRadiusMeters: 5000,
				IsActive:             true,
				IsCapital:            false,
			},
			{
				ID:                   2,
				CountryID:            1,
				GovernorateID:        &govID,
				GovernorateName:      &i18n.Text{"ar": "القاهرة", "en": "Cairo"},
				Name:                 i18n.New("حلوان", "Helwan"),
				Latitude:             29.8500,
				Longitude:            31.3000,
				CoverageRadiusMeters: 4000,
				IsActive:             false, // Inactive / disabled city
				IsCapital:            false,
			},
		},
	}
}

func (m *mockGeographyRepo) ListAllGovernorates(_ context.Context, _ int64) ([]*platformadmin.Governorate, error) {
	return m.governorates, nil
}
func (m *mockGeographyRepo) ListGovernorates(_ context.Context, _ int64) ([]*platformadmin.Governorate, error) {
	var out []*platformadmin.Governorate
	for _, g := range m.governorates {
		if g.IsActive {
			out = append(out, g)
		}
	}
	return out, nil
}
func (m *mockGeographyRepo) GetGovernorate(_ context.Context, id int64) (*platformadmin.Governorate, error) {
	for _, g := range m.governorates {
		if g.ID == id {
			return g, nil
		}
	}
	return nil, nil
}
func (m *mockGeographyRepo) CreateGovernorate(_ context.Context, g *platformadmin.Governorate) error {
	m.governorates = append(m.governorates, g)
	return nil
}
func (m *mockGeographyRepo) UpdateGovernorate(_ context.Context, g *platformadmin.Governorate) error {
	for i, ex := range m.governorates {
		if ex.ID == g.ID {
			m.governorates[i] = g
		}
	}
	return nil
}
func (m *mockGeographyRepo) ToggleGovernorateStatus(_ context.Context, id int64) error {
	for _, g := range m.governorates {
		if g.ID == id {
			g.IsActive = !g.IsActive
		}
	}
	return nil
}

func (m *mockGeographyRepo) ListAllCities(_ context.Context, _ int64) ([]*platformadmin.City, error) {
	return m.cities, nil
}
func (m *mockGeographyRepo) ListCities(_ context.Context, _ int64) ([]*platformadmin.City, error) {
	var out []*platformadmin.City
	for _, c := range m.cities {
		if c.IsActive {
			out = append(out, c)
		}
	}
	return out, nil
}
func (m *mockGeographyRepo) ListCitiesByGovernorate(_ context.Context, govID int64) ([]*platformadmin.City, error) {
	var out []*platformadmin.City
	for _, c := range m.cities {
		if c.GovernorateID != nil && *c.GovernorateID == govID && c.IsActive {
			out = append(out, c)
		}
	}
	return out, nil
}
func (m *mockGeographyRepo) GetCity(_ context.Context, id int64) (*platformadmin.City, error) {
	for _, c := range m.cities {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}
func (m *mockGeographyRepo) ToggleCityStatus(_ context.Context, id int64) error {
	for _, c := range m.cities {
		if c.ID == id {
			c.IsActive = !c.IsActive
		}
	}
	return nil
}
func (m *mockGeographyRepo) CreateCity(_ context.Context, c *platformadmin.City) error {
	m.cities = append(m.cities, c)
	return nil
}
func (m *mockGeographyRepo) UpdateCity(_ context.Context, c *platformadmin.City) error {
	for i, ex := range m.cities {
		if ex.ID == c.ID {
			m.cities[i] = c
		}
	}
	return nil
}

func (m *mockGeographyRepo) ListCountries(_ context.Context) ([]*platformadmin.Country, error) {
	return []*platformadmin.Country{
		{ID: 1, Code: "EG", Name: i18n.New("مصر", "Egypt"), IsActive: true},
	}, nil
}

func setupGeoTestHandler() (*UIHandler, *mockGeographyRepo) {
	repo := newMockGeographyRepo()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	adminSvc := platformadmin.NewService(repo, logger)
	handler := &UIHandler{log: logger, adminSvc: adminSvc}
	return handler, repo
}

// WO-14: Disabling a city must not hide it on /admin/cities.
// A disabled city must be displayed with a "معطلة" badge and a "تفعيل" button.
func TestAdminCitiesPage_ShowsDisabledCityWithBadgeAndActivateButton(t *testing.T) {
	h, _ := setupGeoTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/admin/cities", nil)
	rec := httptest.NewRecorder()

	h.AdminCitiesPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()

	// 1. Active city is visible with "مفعلة" badge and "تعطيل" action
	if !strings.Contains(body, "مدينة نصر") {
		t.Errorf("expected active city 'مدينة نصر' to be visible")
	}
	if !strings.Contains(body, "مفعلة") {
		t.Errorf("expected 'مفعلة' badge for active city")
	}

	// 2. Disabled city is VISIBLE with "معطلة" badge and "تفعيل" action
	if !strings.Contains(body, "حلوان") {
		t.Fatalf("BUG DEFECT #14: disabled city 'حلوان' was hidden from /admin/cities!")
	}
	if !strings.Contains(body, "معطلة") {
		t.Errorf("expected 'معطلة' badge for disabled city")
	}
	if !strings.Contains(body, "تفعيل") {
		t.Errorf("expected 'تفعيل' button for disabled city")
	}
	if !strings.Contains(body, "/admin/cities/2/toggle") {
		t.Errorf("expected toggle action for disabled city ID 2")
	}
}

// WO-14: Status filter (الكل / مفعّلة / معطّلة) works as expected.
func TestAdminCitiesPage_StatusFilter(t *testing.T) {
	h, _ := setupGeoTestHandler()

	// 1. status=active: shows only active cities
	reqActive := httptest.NewRequest(http.MethodGet, "/admin/cities?status=active", nil)
	recActive := httptest.NewRecorder()
	h.AdminCitiesPage(recActive, reqActive)

	bodyActive := recActive.Body.String()
	if !strings.Contains(bodyActive, "/admin/cities/1/toggle") {
		t.Errorf("expected active city ID 1 in active filter")
	}
	if strings.Contains(bodyActive, "/admin/cities/2/toggle") {
		t.Errorf("expected disabled city ID 2 to be excluded from status=active")
	}

	// 2. status=inactive: shows only disabled cities
	reqInactive := httptest.NewRequest(http.MethodGet, "/admin/cities?status=inactive", nil)
	recInactive := httptest.NewRecorder()
	h.AdminCitiesPage(recInactive, reqInactive)

	bodyInactive := recInactive.Body.String()
	if !strings.Contains(bodyInactive, "/admin/cities/2/toggle") {
		t.Errorf("expected disabled city ID 2 in inactive filter")
	}
	if strings.Contains(bodyInactive, "/admin/cities/1/toggle") {
		t.Errorf("expected active city ID 1 to be excluded from status=inactive")
	}
}

// WO-14: Consumers (such as registration city dropdown) keep using ListCities,
// ensuring disabled cities remain excluded from registration.
func TestRegistrationCityDropdown_ExcludesDisabledCities(t *testing.T) {
	h, _ := setupGeoTestHandler()

	cities := h.listCities(context.Background())
	if len(cities) != 1 {
		t.Fatalf("expected 1 active city in consumer dropdown, got %d", len(cities))
	}
	if cities[0].Name["ar"] != "مدينة نصر" {
		t.Errorf("expected only active city 'مدينة نصر', got %q", cities[0].Name["ar"])
	}
}

// WO-14: Toggling a city updates its status and preserves the referer.
func TestAdminCityToggleSubmit_TogglesStatus(t *testing.T) {
	h, repo := setupGeoTestHandler()

	r := chi.NewRouter()
	r.Post("/admin/cities/{id}/toggle", h.AdminCityToggleSubmit)

	req := httptest.NewRequest(http.MethodPost, "/admin/cities/2/toggle", nil)
	req.Header.Set("Referer", "/admin/cities?status=inactive")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}

	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/admin/cities") || !strings.Contains(loc, "status=inactive") {
		t.Errorf("expected redirect to preserve referer query params, got %q", loc)
	}

	// Verify status toggled in repository
	if !repo.cities[1].IsActive {
		t.Errorf("expected city 2 to be toggled to active=true")
	}
}
