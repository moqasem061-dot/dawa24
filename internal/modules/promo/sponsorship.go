// Package promo — sponsorship domain types.
//
// Sponsorship packages (باقات الرعاية) give a vendor a pool of credits they
// spend to sponsor individual products or offers. A sponsorship request
// (طلب الرعاية) is the approval workflow row: the vendor submits, the admin
// approves or rejects, and on approval the item is ranked at the top of the
// catalog/offers page by the package tier level.
package promo

import (
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// fmtInt64 formats an int64 as a decimal string without importing fmt into domain.
func fmtInt64(n int64) string { return strconv.FormatInt(n, 10) }

// SponsorshipItemType identifies whether a sponsorship targets a product or an offer.
type SponsorshipItemType string

const (
	SponsorItemProduct SponsorshipItemType = "product"
	SponsorItemOffer   SponsorshipItemType = "offer"
)

// AdMediaType identifies the media kind of an advertisement.
type AdMediaType string

const (
	MediaImage AdMediaType = "image"
	MediaVideo AdMediaType = "video"
)

// AdClickTarget identifies what happens when a user clicks an ad.
type AdClickTarget string

// An advertisement may only point somewhere inside the platform. The
// "external_url" target is gone: it let a supplier send a pharmacy to a page
// nobody here moderates, and nothing validated the URL it carried.
const (
	ClickTargetVendor  AdClickTarget = "vendor_page"
	ClickTargetProduct AdClickTarget = "product"
	ClickTargetOffer   AdClickTarget = "offer"
)

// ValidClickTarget folds an untrusted click_target_type onto the three the
// platform accepts. Removing the option from the <select> stops a person, not
// a crafted POST; this is what stops the POST.
func ValidClickTarget(raw string) AdClickTarget {
	switch AdClickTarget(raw) {
	case ClickTargetVendor:
		return ClickTargetVendor
	case ClickTargetOffer:
		return ClickTargetOffer
	default:
		return ClickTargetProduct
	}
}

// Standard advertisement placement positions on the storefront and platform.
const (
	PositionHomeHero   = "home_hero"
	PositionHomeBanner = "home_banner"
	PositionHomeDeals  = "home_deals"
	PositionHomeBottom = "home_bottom"
	PositionCatalogTop = "catalog_top"

	AdCreditCost          = 2
	SponsorshipCreditCost = 1
)

// AdminStatus is the approval lifecycle of a moderation-gated row.
type AdminStatus string

const (
	AdminPending  AdminStatus = "pending"
	AdminApproved AdminStatus = "approved"
	AdminRejected AdminStatus = "rejected"
)

// PurchaseStatus tracks the lifecycle of a sponsorship package purchase.
type PurchaseStatus string

const (
	PurchaseActive    PurchaseStatus = "active"
	PurchaseExpired   PurchaseStatus = "expired"
	PurchaseCancelled PurchaseStatus = "cancelled"
	PurchasePending   PurchaseStatus = "pending"
)

// SponsorshipRequestStatus is the operational status of a request.
type SponsorshipRequestStatus string

const (
	SRSPending   SponsorshipRequestStatus = "pending"
	SRSActive    SponsorshipRequestStatus = "active"
	SRSExpired   SponsorshipRequestStatus = "expired"
	SRSCancelled SponsorshipRequestStatus = "cancelled"
	SRSRejected  SponsorshipRequestStatus = "rejected"
)

// SponsorshipPurchase records a vendor's acquisition of a sponsorship package
// and the remaining credits. One row per purchase; credits_used climbs as
// the vendor submits sponsorship requests against this purchase.
type SponsorshipPurchase struct {
	ID               int64          `json:"id"`
	PublicID         string         `json:"public_id"`
	OrganizationID   int64          `json:"organization_id"`
	PackageID        int64          `json:"package_id"`
	Package          *OfferPackage  `json:"package,omitempty"`
	CreditsTotal     int            `json:"credits_total"`
	CreditsUsed      int            `json:"credits_used"`
	CreditsRemaining int            `json:"credits_remaining"`
	StartsAt         time.Time      `json:"starts_at"`
	ExpiresAt        time.Time      `json:"expires_at"`
	Status           PurchaseStatus `json:"status"`
	AutoRenew        bool           `json:"auto_renew"`
	BillingCycle     string         `json:"billing_cycle"`
	Amount           money.Amount   `json:"amount"`
	PaymentID        *int64         `json:"payment_id,omitempty"`
	SourceSystem     string         `json:"source_system"`
	SourceID         *int64         `json:"source_id,omitempty"`
	ApprovedBy       *int64         `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time     `json:"approved_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// CreditsRemaining returns the unused credits on this purchase.
func (p *SponsorshipPurchase) CreditsRemainingInt() int {
	r := p.CreditsTotal - p.CreditsUsed
	if r < 0 {
		return 0
	}
	return r
}

// SponsorshipRequest is one vendor's request to sponsor a product or offer
// using credits from a purchase. Admin approval gates activation.
type SponsorshipRequest struct {
	ID             int64                    `json:"id"`
	PublicID       string                   `json:"public_id"`
	OrganizationID int64                    `json:"organization_id"`
	PurchaseID     *int64                   `json:"purchase_id,omitempty"`
	PackageID      int64                    `json:"package_id"`
	Package        *OfferPackage            `json:"package,omitempty"`
	ItemType       SponsorshipItemType      `json:"item_type"`
	ItemID         int64                    `json:"item_id"`
	CreditsUsed    int                      `json:"credits_used"`
	AdminStatus    AdminStatus              `json:"admin_status"`
	AdminNotes     string                   `json:"admin_notes,omitempty"`
	ReviewedBy     *int64                   `json:"reviewed_by,omitempty"`
	ReviewedAt     *time.Time               `json:"reviewed_at,omitempty"`
	StartsAt       time.Time                `json:"starts_at"`
	ExpiresAt      time.Time                `json:"expires_at"`
	Status         SponsorshipRequestStatus `json:"status"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
}

// IsApproved reports whether the admin cleared this request for activation.
func (r *SponsorshipRequest) IsApproved() bool {
	return r.AdminStatus == AdminApproved && r.Status == SRSActive
}

// AdStats holds the engagement metrics for a single advertisement.
type AdStats struct {
	Impressions int64   `json:"impressions"`
	Clicks      int64   `json:"clicks"`
	CTR         float64 `json:"ctr"` // clicks / impressions * 100, 0 when no impressions
}

// ComputeCTR derives the click-through rate from impressions and clicks.
func ComputeCTR(impressions, clicks int64) float64 {
	if impressions <= 0 {
		return 0
	}
	return float64(clicks) / float64(impressions) * 100
}
