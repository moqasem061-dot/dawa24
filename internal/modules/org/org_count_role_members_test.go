package org

import (
	"context"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

func (m *mockOrgRepo) CountRoleMembers(_ context.Context, _ int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *mockOrgRepo) AssignMemberRole(_ context.Context, _, _, _ int64) error { return nil }

func (m *mockOrgRepo) GetDeliveryBands(_ context.Context, orgID int64) ([]*DeliveryBand, error) {
	if m.deliveryBands != nil {
		return m.deliveryBands[orgID], nil
	}
	return nil, nil
}

func (m *mockOrgRepo) SaveDeliveryBands(_ context.Context, orgID int64, bands []*DeliveryBand) error {
	if m.deliveryBands == nil {
		m.deliveryBands = make(map[int64][]*DeliveryBand)
	}
	m.deliveryBands[orgID] = bands
	return nil
}

func (m *mockOrgRepo) CreateOrgDeletionRequest(_ context.Context, req *OrganizationDeletionRequest) error {
	req.ID = m.nextID
	m.nextID++
	req.Status = OrgDeletionStatusPending
	return nil
}

func (m *mockOrgRepo) GetPendingOrgDeletionRequest(_ context.Context, _ int64) (*OrganizationDeletionRequest, error) {
	return nil, nil
}

func (m *mockOrgRepo) GetOrgDeletionRequest(_ context.Context, _ int64) (*OrganizationDeletionRequest, error) {
	return nil, nil
}

func (m *mockOrgRepo) CancelOrgDeletionRequest(_ context.Context, _, _ int64) error {
	return nil
}

func (m *mockOrgRepo) ListOrgDeletionRequests(_ context.Context, _ string, _, _ int) ([]*OrganizationDeletionRequest, int, error) {
	return nil, 0, nil
}

func (m *mockOrgRepo) ReviewOrgDeletionRequest(_ context.Context, _, _ int64, _ bool, _ string) (*OrganizationDeletionRequest, error) {
	return nil, nil
}

func (m *mockOrgRepo) SetExtraDevices(_ context.Context, orgID int64, extraDevices int, expiresAt *time.Time) error {
	o, ok := m.orgs[orgID]
	if !ok {
		return apperr.NotFound("organization")
	}
	o.ExtraDevices = extraDevices
	o.ExtraDevicesExpiresAt = expiresAt
	return nil
}

func TestOrganization_ActiveExtraDevices(t *testing.T) {
	// 1. Zero extra devices
	o1 := &Organization{ExtraDevices: 0}
	if got := o1.ActiveExtraDevices(); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}

	// 2. Active extra devices with future expiry
	future := time.Now().Add(24 * time.Hour)
	o2 := &Organization{ExtraDevices: 5, ExtraDevicesExpiresAt: &future}
	if got := o2.ActiveExtraDevices(); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}

	// 3. Expired extra devices
	past := time.Now().Add(-1 * time.Hour)
	o3 := &Organization{ExtraDevices: 5, ExtraDevicesExpiresAt: &past}
	if got := o3.ActiveExtraDevices(); got != 0 {
		t.Fatalf("expected 0 for expired devices, got %d", got)
	}

	// 4. Active extra devices without explicit expiry
	o4 := &Organization{ExtraDevices: 3, ExtraDevicesExpiresAt: nil}
	if got := o4.ActiveExtraDevices(); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}

	// 5. Service SetExtraDevices sets and clears
	repo := newMockOrgRepo()
	svc := NewService(repo, nil)
	ctx := context.Background()
	_ = repo.CreateOrganization(ctx, &Organization{LegalName: "Test Pharmacy"})
	orgID := int64(1)

	if err := svc.SetExtraDevices(ctx, orgID, 4, &future); err != nil {
		t.Fatalf("SetExtraDevices: %v", err)
	}
	fetched, _ := svc.GetOrganization(ctx, orgID)
	if fetched.ActiveExtraDevices() != 4 {
		t.Fatalf("expected 4 active extra devices, got %d", fetched.ActiveExtraDevices())
	}

	// Clear extra devices
	if err := svc.SetExtraDevices(ctx, orgID, 0, nil); err != nil {
		t.Fatalf("SetExtraDevices reset: %v", err)
	}
	fetched2, _ := svc.GetOrganization(ctx, orgID)
	if fetched2.ActiveExtraDevices() != 0 {
		t.Fatalf("expected 0 active extra devices after reset, got %d", fetched2.ActiveExtraDevices())
	}
}
