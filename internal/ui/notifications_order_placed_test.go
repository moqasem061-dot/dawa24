package ui

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// A supplier buying from another supplier is a buyer like any pharmacy: its
// own copy of "order received" must carry the key its members can hold, the
// selling supplier must be told, and so must the platform.
func TestOrderPlacedBySupplierReachesBuyerSellerAndAdmin(t *testing.T) {
	const (
		adminID      = int64(10)
		buyerOwnerID = int64(20)
		buyerStaffID = int64(21)
		buyerOrgID   = int64(300) // a supplier, buying
		sellerOrgID  = int64(200) // the supplier it buys from
	)
	h, repo := newOrderPlacedHandler(t, adminID, buyerOwnerID, buyerStaffID, []int64{buyerOrgID, sellerOrgID})

	h.notifyOrderPlaced(context.Background(), placedOrder(buyerStaffID, buyerOrgID, sellerOrgID, false), "شركة المورد المشتري")

	buyer := logsFor(repo, buyerStaffID, buyerOrgID)
	require.NotEmpty(t, buyer, "the supplier's buyer got no order-placed notification")
	for _, l := range buyer {
		assert.Equal(t, "vendor.buying.order.view", l.RequiredPermission,
			"a supplier's purchase must be filtered by its buying key, never pharmacy.order.view")
	}
	assert.NotEmpty(t, logsFor(repo, buyerOwnerID, sellerOrgID), "the selling supplier was not told")

	admin := logsFor(repo, adminID, 0)
	require.Len(t, admin, 1, "the platform must be told about every order")
	assert.Equal(t, "commerce.order.view", admin[0].RequiredPermission)
	assert.Contains(t, admin[0].Body, "مورد", "the platform copy names the buyer as a supplier")
}

// A negotiation reaches the supplier as a negotiation offer, from the form; the
// buyer and the platform still hear about the order.
func TestOrderPlacedNegotiationDoesNotDoubleNotifySupplier(t *testing.T) {
	const (
		adminID, ownerID, staffID = int64(10), int64(20), int64(21)
		buyerOrgID, sellerOrgID   = int64(100), int64(200)
	)
	h, repo := newOrderPlacedHandler(t, adminID, ownerID, staffID, []int64{sellerOrgID})

	h.notifyOrderPlaced(context.Background(), placedOrder(staffID, buyerOrgID, sellerOrgID, true), "صيدلية النور")

	assert.Empty(t, logsFor(repo, ownerID, sellerOrgID), "the supplier gets the negotiation offer instead")
	assert.NotEmpty(t, logsFor(repo, staffID, buyerOrgID))
	assert.Len(t, logsFor(repo, adminID, 0), 1)
}

func newOrderPlacedHandler(t *testing.T, adminID, ownerID, staffID int64, vendorOrgs []int64) (*UIHandler, *mockNotifRepo) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &mockNotifRepo{}
	orgRepo := &mockOrgRepoForOrderPlaced{
		mockOrgRepoForTableTest: mockOrgRepoForTableTest{ownerID: ownerID, staffID: staffID, orgName: "منشأة"},
		vendors:                 map[int64]bool{},
	}
	for _, id := range vendorOrgs {
		orgRepo.vendors[id] = true
	}
	return &UIHandler{
		notifSvc: notifications.NewService(repo, logger),
		idSvc:    identity.NewService(&mockIdentityRepoForTableTest{adminUser: adminID}, nil, logger),
		orgSvc:   org.NewService(orgRepo, logger),
		log:      logger,
	}, repo
}

type mockOrgRepoForOrderPlaced struct {
	mockOrgRepoForTableTest
	vendors map[int64]bool
}

func (m *mockOrgRepoForOrderPlaced) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	o, err := m.mockOrgRepoForTableTest.GetOrganizationByID(ctx, id)
	if err == nil && m.vendors[id] {
		o.Type = org.TypeVendor
	}
	return o, err
}

func placedOrder(customerID, buyerOrgID, sellerOrgID int64, negotiation bool) *commerce.Order {
	total := money.MustParse("1500.00")
	prodID := int64(1)
	return &commerce.Order{
		ID:             77,
		OrderNumber:    "ORD-0077",
		CustomerID:     customerID,
		OrganizationID: &buyerOrgID,
		TotalAmount:    total,
		IsNegotiation:  negotiation,
		Shipments: []*commerce.OrderShipment{{
			OrganizationID: sellerOrgID,
			Subtotal:       total,
			Lines:          []*commerce.OrderLine{{ProductID: &prodID, Quantity: 2}},
		}},
	}
}

// logsFor returns the rows written to userID about orgID (0: no organization).
func logsFor(repo *mockNotifRepo, userID, orgID int64) []*notifications.NotificationLog {
	var out []*notifications.NotificationLog
	for _, l := range repo.logs {
		if l.UserID != userID {
			continue
		}
		got := int64(0)
		if l.OrganizationID != nil {
			got = *l.OrganizationID
		}
		if got == orgID {
			out = append(out, l)
		}
	}
	return out
}
