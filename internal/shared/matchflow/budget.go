package matchflow

import (
	"sync"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Budget defines and tracks spending ceilings for an AI match run.
// It tracks requests, token allowances, and exact monetary costs in micro-units.
type Budget struct {
	MaxRequests     int          `json:"max_requests"`
	MaxInputTokens  int          `json:"max_input_tokens"`
	MaxOutputTokens int          `json:"max_output_tokens"`
	MaxCost         money.Amount `json:"max_cost"`

	mu           sync.Mutex
	Requests     int          `json:"requests"`
	InputTokens  int          `json:"input_tokens"`
	OutputTokens int          `json:"output_tokens"`
	SpentCost    money.Amount `json:"spent_cost"`
	ReservedCost money.Amount `json:"reserved_cost"`
	CostKnown    bool         `json:"cost_known"`
}

// NewBudget initializes a new Budget tracker with given limits.
// CostKnown is initially true until an attempt finishes with unknown cost.
func NewBudget(maxRequests, maxInputTokens, maxOutputTokens int, maxCost money.Amount) *Budget {
	return &Budget{
		MaxRequests:     maxRequests,
		MaxInputTokens:  maxInputTokens,
		MaxOutputTokens: maxOutputTokens,
		MaxCost:         maxCost,
		CostKnown:       true,
	}
}

// Reserve attempts to reserve perAttempt cost from the remaining budget.
// It returns false if dispatching another attempt would violate request,
// token, or monetary ceilings.
func (b *Budget) Reserve(perAttempt money.Amount) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.MaxRequests > 0 && b.Requests >= b.MaxRequests {
		return false
	}
	if b.MaxInputTokens > 0 && b.InputTokens >= b.MaxInputTokens {
		return false
	}
	if b.MaxOutputTokens > 0 && b.OutputTokens >= b.MaxOutputTokens {
		return false
	}

	if b.MaxCost.IsPositive() {
		if perAttempt.IsNegative() {
			return false
		}
		currentMinor := b.SpentCost.Minor() + b.ReservedCost.Minor()
		if currentMinor+perAttempt.Minor() > b.MaxCost.Minor() {
			return false
		}
		b.ReservedCost = money.FromMinor(b.ReservedCost.Minor() + perAttempt.Minor())
	}

	b.Requests++
	return true
}

// Settle reconciles a previously reserved amount with the actual cost incurred.
// When known is false, it settles at the reserved amount and marks CostKnown=false.
func (b *Budget) Settle(reserved, actual money.Amount, known bool) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// Release reservation.
	remReserved := b.ReservedCost.Minor() - reserved.Minor()
	if remReserved < 0 {
		remReserved = 0
	}
	b.ReservedCost = money.FromMinor(remReserved)

	if known {
		b.SpentCost = money.FromMinor(b.SpentCost.Minor() + actual.Minor())
	} else {
		b.CostKnown = false
		b.SpentCost = money.FromMinor(b.SpentCost.Minor() + reserved.Minor())
	}
}

// RecordTokens records token consumption against the budget.
func (b *Budget) RecordTokens(input, output int) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.InputTokens += input
	b.OutputTokens += output
}

// Stats returns a snapshot of the current budget consumption.
func (b *Budget) Stats() (requests, inTokens, outTokens int, spent money.Amount, costKnown bool) {
	if b == nil {
		return 0, 0, 0, money.Zero, true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Requests, b.InputTokens, b.OutputTokens, b.SpentCost, b.CostKnown
}
