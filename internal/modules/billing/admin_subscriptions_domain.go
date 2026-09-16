package billing

import (
	"context"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The subscriber log, as an administrator reads it.
//
// The screen behind this listed `billing.subscriptions` verbatim: an id for the
// organisation, an id for the plan, no cycle, no renewal setting, no filters
// and a page-size control over an unfiltered table. An operator asked "which
// pharmacies are on the annual plan and which expire this month" had to read
// the ids and look each one up.
//
// A row here is the join an operator actually reads, and the filter is the
// question they actually ask.

// AdminSubscriptionFilter narrows the subscriber log.
//
// Every field is optional and they compose. Organization is a name search
// rather than an id because that is what the client asked for and what a person
// has in front of them.
type AdminSubscriptionFilter struct {
	OrganizationQuery string
	PlanID            int64
	Status            string
	BillingCycle      string
	// AutoRenew filters on the setting when set; nil means "either".
	AutoRenew *bool
	// ExpiringWithinDays limits to subscriptions whose expiry falls inside the
	// next N days and has not already passed. Zero means no limit.
	ExpiringWithinDays int
	StartsFrom         *time.Time
	StartsTo           *time.Time
	ExpiresFrom        *time.Time
	ExpiresTo          *time.Time

	Limit  int
	Offset int
}

// Normalize clamps the page window and drops filter values the query cannot use.
func (f *AdminSubscriptionFilter) Normalize() {
	f.OrganizationQuery = strings.TrimSpace(f.OrganizationQuery)
	f.Status = strings.TrimSpace(f.Status)
	f.BillingCycle = strings.TrimSpace(f.BillingCycle)
	if f.BillingCycle != "monthly" && f.BillingCycle != "annual" {
		f.BillingCycle = ""
	}
	if f.ExpiringWithinDays < 0 {
		f.ExpiringWithinDays = 0
	}
	if f.Limit <= 0 {
		f.Limit = 25
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
}

// AdminSubscriptionRow is one line of the subscriber log.
type AdminSubscriptionRow struct {
	Subscription

	OrganizationName i18n.Text `json:"organization_name"`
	OrganizationType string    `json:"organization_type"`
	UserName         i18n.Text `json:"user_name"`
	UserEmail        string    `json:"user_email"`

	PlanName    i18n.Text    `json:"plan_name"`
	PlanSlug    string       `json:"plan_slug"`
	PlanIsFree  bool         `json:"plan_is_free"`
	AmountPaid  money.Amount `json:"amount_paid"`
	HistoryRows int          `json:"history_rows"`
}

// DaysRemaining is how long the subscription still runs, floored at zero.
//
// Floored rather than negative because "expired 12 days ago" is the status
// column's job; this column answers "how long have I got", and a negative
// number there reads as a bug.
func (r *AdminSubscriptionRow) DaysRemaining(now time.Time) int {
	if r == nil || r.ExpiresAt.IsZero() || !r.ExpiresAt.After(now) {
		return 0
	}
	return int(r.ExpiresAt.Sub(now).Hours() / 24)
}

// Expired reports whether the term has run out, whatever the status column says.
//
// The two disagree in practice: nothing sweeps `status` when a term lapses, so
// a row can read `active` months after it expired. The screen shows both and
// this is the one that is true.
func (r *AdminSubscriptionRow) Expired(now time.Time) bool {
	return r != nil && !r.ExpiresAt.IsZero() && !r.ExpiresAt.After(now)
}

// DisplayOrganizationName prefers Arabic, then English, then the buyer's own
// name, so a row is never rendered as a bare id.
func (r *AdminSubscriptionRow) DisplayOrganizationName(lang string) string {
	if r == nil {
		return ""
	}
	if name := r.OrganizationName.Get(i18n.Lang(lang)); name != "" {
		return name
	}
	if name := r.OrganizationName.Get(i18n.AR); name != "" {
		return name
	}
	if name := r.OrganizationName.Get(i18n.EN); name != "" {
		return name
	}
	if name := r.UserName.Get(i18n.Lang(lang)); name != "" {
		return name
	}
	return r.UserEmail
}

// SubscriptionHistoryRow is one entry of a subscription's trail: the purchase,
// each upgrade or downgrade, and every renewal.
type SubscriptionHistoryRow struct {
	ID             int64        `json:"id"`
	SubscriptionID int64        `json:"subscription_id"`
	OrganizationID *int64       `json:"organization_id,omitempty"`
	UserID         *int64       `json:"user_id,omitempty"`
	PlanID         *int64       `json:"plan_id,omitempty"`
	PlanName       i18n.Text    `json:"plan_name"`
	Action         string       `json:"action"`
	Amount         money.Amount `json:"amount"`
	Currency       string       `json:"currency"`
	Details        string       `json:"details"`
	CreatedAt      time.Time    `json:"created_at"`
}

// AdminSubscriptionBackend is the subscriber log's persistence.
//
// An optional interface on the repository rather than a member of Repository,
// so the in-memory repositories the service tests use keep compiling: a backend
// that does not implement it simply has no subscriber log, and the screen says
// so instead of the process failing to build.
type AdminSubscriptionBackend interface {
	AdminListSubscriptionRows(ctx context.Context, f AdminSubscriptionFilter) ([]*AdminSubscriptionRow, int, error)
	AdminSubscriptionHistory(ctx context.Context, subscriptionID int64) ([]*SubscriptionHistoryRow, error)
}

// AdminListSubscriptionRows returns one filtered page of the subscriber log.
func (s *Service) AdminListSubscriptionRows(
	ctx context.Context, f AdminSubscriptionFilter,
) ([]*AdminSubscriptionRow, int, error) {
	backend, ok := s.repo.(AdminSubscriptionBackend)
	if !ok {
		return nil, 0, nil
	}
	return backend.AdminListSubscriptionRows(ctx, f)
}

// AdminSubscriptionHistory returns one subscription's upgrade and renewal trail.
func (s *Service) AdminSubscriptionHistory(
	ctx context.Context, subscriptionID int64,
) ([]*SubscriptionHistoryRow, error) {
	backend, ok := s.repo.(AdminSubscriptionBackend)
	if !ok {
		return nil, nil
	}
	return backend.AdminSubscriptionHistory(ctx, subscriptionID)
}

// SubscriptionHistoryEntry is one transition to file.
type SubscriptionHistoryEntry struct {
	SubscriptionID int64
	OrganizationID int64
	UserID         int64
	PlanID         int64
	Action         string
	AmountMinor    int64
	Currency       string
	Details        string
}

// SubscriptionHistoryWriter records subscription transitions. Optional for the
// same reason AdminSubscriptionBackend is: an in-memory repository has no trail
// and must still compile.
type SubscriptionHistoryWriter interface {
	RecordSubscriptionHistory(ctx context.Context, h SubscriptionHistoryEntry) error
}

// recordSubscriptionHistory files a transition, if the repository keeps a trail.
//
// A failure is logged rather than returned: the subscription is already active
// and the money already moved, so refusing here would leave the caller with a
// successful purchase reported as an error.
func (s *Service) recordSubscriptionHistory(ctx context.Context, h SubscriptionHistoryEntry) {
	writer, ok := s.repo.(SubscriptionHistoryWriter)
	if !ok {
		return
	}
	if err := writer.RecordSubscriptionHistory(ctx, h); err != nil {
		s.log.ErrorContext(ctx, "could not file the subscription history entry",
			"subscription_id", h.SubscriptionID, "action", h.Action, "error", err)
	}
}
