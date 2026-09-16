package promo

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// استهلاك الباقة — the ledger behind a purchase's credit counter.
//
// credits_used on promo.sponsorship_purchases is a number with no history. A
// supplier could see that 19 of 50 credits were gone and had no way to learn to
// what, and support had no way to answer. Every movement of that counter now
// writes one entry beside it, in the same transaction.

// CreditReason says why credits moved. The set is closed and mirrored by a CHECK
// constraint, so a new kind of movement is a deliberate change in both places
// rather than a new string appearing in the statement.
type CreditReason string

const (
	CreditAdCreated            CreditReason = "ad_created"
	CreditAdRefunded           CreditReason = "ad_refunded"
	CreditSponsorshipRequested CreditReason = "sponsorship_requested"
	CreditSponsorshipBatch     CreditReason = "sponsorship_batch"
	CreditSponsorshipRejected  CreditReason = "sponsorship_rejected"
	CreditAdjustment           CreditReason = "adjustment"
)

// Valid reports whether the reason is one the ledger accepts.
func (r CreditReason) Valid() bool {
	switch r {
	case CreditAdCreated, CreditAdRefunded, CreditSponsorshipRequested,
		CreditSponsorshipBatch, CreditSponsorshipRejected, CreditAdjustment:
		return true
	}
	return false
}

// CreditEntry is one movement on a purchase.
type CreditEntry struct {
	ID             int64  `json:"id"`
	PublicID       string `json:"public_id"`
	OrganizationID int64  `json:"organization_id"`
	PurchaseID     int64  `json:"purchase_id"`
	// Delta is signed the way the remaining balance moves: a consumption is
	// negative, a refund positive.
	Delta        int          `json:"delta"`
	BalanceAfter int          `json:"balance_after"`
	Reason       CreditReason `json:"reason"`
	EntityType   string       `json:"entity_type,omitempty"`
	EntityID     *int64       `json:"entity_id,omitempty"`
	ActorUserID  *int64       `json:"actor_user_id,omitempty"`
	Note         string       `json:"note,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

// IsRefund reports whether this entry gave credits back.
func (e *CreditEntry) IsRefund() bool { return e.Delta > 0 }

// ConsumeCredits is one request to move a purchase's balance.
//
// Credits is always positive and Refund says which way it goes, rather than the
// caller passing a negative number. The old API took a signed count and four of
// its six call sites passed `-AdCreditCost` to mean a refund, which reads as
// arithmetic rather than as an intention.
type ConsumeCredits struct {
	OrganizationID int64
	PurchaseID     int64
	Credits        int
	Refund         bool
	Reason         CreditReason
	EntityType     string
	EntityID       *int64
	ActorUserID    *int64
	Note           string
}

// Validate checks a movement before it reaches the database.
func (c ConsumeCredits) Validate() error {
	if c.PurchaseID <= 0 {
		return apperr.Validation("promo.credit_invalid_purchase",
			"A valid purchase is required.", nil)
	}
	if c.Credits <= 0 {
		return apperr.Validation("promo.credit_invalid_amount",
			"The number of credits must be positive.", nil)
	}
	if !c.Reason.Valid() {
		return apperr.Validation("promo.credit_invalid_reason",
			"Unknown credit movement reason.", nil)
	}
	return nil
}

// Delta is the signed change this movement makes to the remaining balance.
func (c ConsumeCredits) Delta() int {
	if c.Refund {
		return c.Credits
	}
	return -c.Credits
}

// CreditStatement is one purchase's ledger with its running totals.
type CreditStatement struct {
	Purchase  *SponsorshipPurchase `json:"purchase"`
	Entries   []*CreditEntry       `json:"entries"`
	Total     int                  `json:"total"`
	Consumed  int                  `json:"consumed"`
	Refunded  int                  `json:"refunded"`
	Remaining int                  `json:"remaining"`
}

// CreditAccount is one company's package position, for the administrator's
// per-organization view of /admin/offers-packages.
//
// CreditsRemaining counts live purchases only: an expired package's unused
// credits cannot be spent, and folding them into the figure would report budget
// that does not exist.
type CreditAccount struct {
	OrganizationID   int64      `json:"organization_id"`
	OrganizationName string     `json:"organization_name"`
	Purchases        int        `json:"purchases"`
	ActivePurchases  int        `json:"active_purchases"`
	CreditsTotal     int        `json:"credits_total"`
	CreditsUsed      int        `json:"credits_used"`
	CreditsRemaining int        `json:"credits_remaining"`
	LastPurchaseAt   *time.Time `json:"last_purchase_at,omitempty"`
}
