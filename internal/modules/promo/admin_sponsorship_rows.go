package promo

import (
	"context"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Product sponsorship, as the platform administers it.
//
// The screen loaded five hundred requests in one go, then resolved the product,
// the organisation and the package one id at a time in a Go loop. That is a
// query per distinct id per page view, an arbitrary ceiling nobody could page
// past, and no way to ask when something ran or how it performed.
//
// One query answers the page. The performance figures come from the tables that
// already record them, so a number on this screen can be traced to a row rather
// than being a plausible zero.

// AdminSponsorshipFilter narrows the product-sponsorship listing.
type AdminSponsorshipFilter struct {
	// Tab is one of "", "all", "active", "pending", "expired", "rejected".
	Tab            string
	Search         string
	OrganizationID int64
	PackageID      int64
	TierLevel      int
	StartsFrom     *time.Time
	StartsTo       *time.Time
	Limit          int
	Offset         int
}

// Normalize clamps the window and drops values the query cannot use.
func (f *AdminSponsorshipFilter) Normalize() {
	f.Search = strings.TrimSpace(f.Search)
	f.Tab = strings.ToLower(strings.TrimSpace(f.Tab))
	switch f.Tab {
	case "active", "pending", "expired", "rejected":
	default:
		f.Tab = "all"
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 25
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
}

// AdminSponsorshipRow is one line of the product-sponsorship listing.
type AdminSponsorshipRow struct {
	ID             int64  `json:"id"`
	PublicID       string `json:"public_id"`
	OrganizationID int64  `json:"organization_id"`
	// OrganizationName is resolved in the query. It was resolved with a
	// GetOrganization call per row before, which is a query per sponsor per
	// page view for a column that is one join.
	OrganizationName i18n.Text `json:"organization_name"`
	OrganizationType string    `json:"organization_type"`

	ProductID    int64     `json:"product_id"`
	ProductName  i18n.Text `json:"product_name"`
	ProductSKU   string    `json:"product_sku"`
	ProductImage string    `json:"product_image"`

	PackageID   int64     `json:"package_id"`
	PackageName i18n.Text `json:"package_name"`
	TierLevel   int       `json:"tier_level"`
	PurchaseID  *int64    `json:"purchase_id,omitempty"`
	ItemID      int64     `json:"item_id"`

	CreditsUsed  int          `json:"credits_used"`
	CreditsTotal int          `json:"credits_total"`
	Amount       money.Amount `json:"amount"`

	AdminStatus string     `json:"admin_status"`
	Status      string     `json:"status"`
	AdminNotes  string     `json:"admin_notes"`
	ReviewedBy  *int64     `json:"reviewed_by,omitempty"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`

	// Impressions and Clicks come from promo.ad_impressions and
	// promo.offer_clicks. A sponsorship with neither reports zero because it
	// has not been seen, not because the screen could not find the number.
	Impressions int `json:"impressions"`
	Clicks      int `json:"clicks"`
}

// Expired reports whether the sponsorship's window has closed.
func (r *AdminSponsorshipRow) Expired(now time.Time) bool {
	if r == nil {
		return false
	}
	if r.Status == string(SRSExpired) {
		return true
	}
	return r.ExpiresAt != nil && !r.ExpiresAt.IsZero() && r.ExpiresAt.Before(now)
}

// Live reports whether the sponsorship is running right now.
func (r *AdminSponsorshipRow) Live(now time.Time) bool {
	if r == nil || r.Expired(now) {
		return false
	}
	return r.AdminStatus == string(AdminApproved) || r.Status == string(SRSActive)
}

// ClickRate is clicks per hundred impressions, for the screen's only derived
// figure. Zero impressions is zero rather than a division by nothing.
func (r *AdminSponsorshipRow) ClickRate() float64 {
	if r == nil || r.Impressions <= 0 {
		return 0
	}
	return float64(r.Clicks) * 100 / float64(r.Impressions)
}

// AdminSponsorshipCounts is the tab bar's badges, counted over the whole set
// rather than over the page.
type AdminSponsorshipCounts struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Pending  int `json:"pending"`
	Expired  int `json:"expired"`
	Rejected int `json:"rejected"`
}

// AdminSponsorshipEvent represents an impression or click tracking record.
type AdminSponsorshipEvent struct {
	EventType string    `json:"event_type"` // "impression" or "click"
	ID        int64     `json:"id"`
	UserID    *int64    `json:"user_id,omitempty"`
	IPAddress string    `json:"ip_address,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AdminSponsorshipBackend is the listing's persistence.
type AdminSponsorshipBackend interface {
	ListAdminSponsorshipRows(ctx context.Context, f AdminSponsorshipFilter) ([]*AdminSponsorshipRow, int, error)
	AdminSponsorshipCounts(ctx context.Context, f AdminSponsorshipFilter) (AdminSponsorshipCounts, error)
	GetAdminSponsorshipRow(ctx context.Context, id int64) (*AdminSponsorshipRow, error)
	GetAdminSponsorshipCreditEntries(ctx context.Context, purchaseID, requestID int64) ([]*CreditEntry, error)
	GetAdminSponsorshipEvents(ctx context.Context, itemID int64) ([]*AdminSponsorshipEvent, error)
}

// ListAdminSponsorshipRows returns one filtered page of product sponsorships.
func (s *Service) ListAdminSponsorshipRows(
	ctx context.Context, f AdminSponsorshipFilter,
) ([]*AdminSponsorshipRow, int, error) {
	backend, ok := s.repo.(AdminSponsorshipBackend)
	if !ok {
		return nil, 0, nil
	}
	return backend.ListAdminSponsorshipRows(ctx, f)
}

// AdminSponsorshipCounts returns the tab badges for the current filter.
func (s *Service) AdminSponsorshipCounts(
	ctx context.Context, f AdminSponsorshipFilter,
) (AdminSponsorshipCounts, error) {
	backend, ok := s.repo.(AdminSponsorshipBackend)
	if !ok {
		return AdminSponsorshipCounts{}, nil
	}
	return backend.AdminSponsorshipCounts(ctx, f)
}

// GetAdminSponsorshipRow returns one sponsorship for its detail page.
func (s *Service) GetAdminSponsorshipRow(ctx context.Context, id int64) (*AdminSponsorshipRow, error) {
	backend, ok := s.repo.(AdminSponsorshipBackend)
	if !ok {
		return nil, nil
	}
	return backend.GetAdminSponsorshipRow(ctx, id)
}

// GetAdminSponsorshipCreditEntries returns credit ledger entries for a sponsorship request and purchase.
func (s *Service) GetAdminSponsorshipCreditEntries(ctx context.Context, purchaseID, requestID int64) ([]*CreditEntry, error) {
	backend, ok := s.repo.(AdminSponsorshipBackend)
	if !ok {
		return nil, nil
	}
	return backend.GetAdminSponsorshipCreditEntries(ctx, purchaseID, requestID)
}

// GetAdminSponsorshipEvents returns recent impressions and clicks for an item.
func (s *Service) GetAdminSponsorshipEvents(ctx context.Context, itemID int64) ([]*AdminSponsorshipEvent, error) {
	backend, ok := s.repo.(AdminSponsorshipBackend)
	if !ok {
		return nil, nil
	}
	return backend.GetAdminSponsorshipEvents(ctx, itemID)
}

// GetSponsorshipPurchaseByID returns a single sponsorship purchase by ID.
func (s *Service) GetSponsorshipPurchaseByID(ctx context.Context, id int64) (*SponsorshipPurchase, error) {
	return s.repo.GetSponsorshipPurchaseByID(ctx, id)
}
