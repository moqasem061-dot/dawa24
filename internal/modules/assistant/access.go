package assistant

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The read model the assistant answers from.
//
// Three properties make this safe to hand a language model, and all three are
// structural rather than conventional:
//
//  1. Every method takes the live authctx.Actor as its FIRST argument and
//     derives its scope from that. There is no organisation parameter to get
//     wrong, and nothing the model produces can influence who the caller is.
//
//  2. Every query runs inside db.InReadTx, which sets the Postgres GUC that
//     row-level security reads. A query that forgot its own WHERE clause
//     returns zero rows rather than a competitor's order book — the database
//     refuses, not the code.
//
//  3. Every list is paginated with a server-imposed ceiling. A model asking for
//     "all orders" gets a page and a cursor, so one careless question cannot
//     pull a year of trading into a prompt.
//
// The interface is deliberately read-only. There is no write method here, on
// any type, and adding one would be visible in review as exactly what it is.

// PageLimit is the largest number of rows any tool returns in one call.
//
// Twenty-five, and not more, because these rows are going into a prompt: a
// bigger page costs tokens on every subsequent turn of the conversation and
// buys an answer nobody reads. Anything larger is a job for the dashboard's own
// export.
const PageLimit = 25

// Page is one window over a result set.
type Page[T any] struct {
	Rows       []T  `json:"rows"`
	HasMore    bool `json:"has_more"`
	NextOffset int  `json:"next_offset,omitempty"`
	Total      int  `json:"total,omitempty"`
}

// DateRange bounds a query in time. A zero bound means unbounded.
type DateRange struct {
	From time.Time
	To   time.Time
}

// ---------------------------------------------------------------------------
// Shared row shapes
// ---------------------------------------------------------------------------

// BranchRow is one branch of the caller's own organisation.
type BranchRow struct {
	ID     int64  `json:"-"`
	Handle string `json:"branch"`
	Name   string `json:"name"`
	Phone  string `json:"phone,omitempty"`
	City   string `json:"city,omitempty"`
	IsMain bool   `json:"is_main"`
	Status string `json:"status"`
}

// WalletTxRow is one movement on the wallet.
type WalletTxRow struct {
	Type        string       `json:"type"`
	Amount      money.Amount `json:"amount"`
	BalanceAter money.Amount `json:"balance_after"`
	Description string       `json:"description,omitempty"`
	At          time.Time    `json:"at"`
}

// WalletSummary is the balance and the recent movements behind it.
type WalletSummary struct {
	Currency string        `json:"currency"`
	Balance  money.Amount  `json:"balance"`
	Recent   []WalletTxRow `json:"recent"`
}

// SubscriptionSummary is the plan the organisation is on.
type SubscriptionSummary struct {
	PlanName      string     `json:"plan"`
	Status        string     `json:"status"`
	StartsAt      time.Time  `json:"starts_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	DaysRemaining int        `json:"days_remaining"`
	PriceMonth    string     `json:"price_month,omitempty"`
	RenewedAt     *time.Time `json:"renewed_at,omitempty"`
}

// ---------------------------------------------------------------------------
// Market and platform rows
// ---------------------------------------------------------------------------

// PlatformSummary is the operator's headline numbers.
type PlatformSummary struct {
	Organizations   int          `json:"organizations"`
	Pharmacies      int          `json:"pharmacies"`
	Vendors         int          `json:"vendors"`
	PendingApproval int          `json:"pending_approval"`
	Users           int          `json:"users"`
	Orders          int          `json:"orders"`
	GMV             money.Amount `json:"gmv"`
	LifetimeOrders  int          `json:"lifetime_orders"`
	LifetimeGMV     money.Amount `json:"lifetime_gmv"`
	From            *time.Time   `json:"from,omitempty"`
	To              *time.Time   `json:"to,omitempty"`
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

// Reader is the assistant's whole view of the business. Read-only, actor-scoped,
// paginated. Implemented by assistant/postgres.
type Reader interface {
	// Shared across dashboards.
	Branches(ctx context.Context, actor authctx.Actor) ([]BranchRow, error)
	Wallet(ctx context.Context, actor authctx.Actor) (*WalletSummary, error)
	Subscription(ctx context.Context, actor authctx.Actor) (*SubscriptionSummary, error)

	// Admin: the platform, read-only and permission-gated.
	PlatformOverview(ctx context.Context, actor authctx.Actor, r DateRange) (*PlatformSummary, error)
}
