package smartorder

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// DefaultCriteria is applied when the buyer enables nothing.
//
// Followed suppliers first: the platform's bias is toward existing commercial
// relationships, and a pharmacy that has chosen to follow a supplier has usually
// done so for credit terms or reliability that no price column captures. The
// tolerance band below stops that preference from becoming expensive.
var DefaultCriteria = []Criterion{
	CriterionFollowedSuppliers,
	CriterionHighestDiscount,
	CriterionLowestPrice,
}

// DefaultTolerancePct caps how much more the preferred supplier may cost.
//
// Five percent is a judgement, not a measurement: wide enough that a followed
// supplier is not lost to a piastre, narrow enough that "prefer my suppliers"
// never quietly means "pay a third more". Buyers can change it.
const DefaultTolerancePct = 5.00

// Config is the snapshot a run executed under.
//
// It is copied from the buyer's profile at start and then frozen. Their saved
// defaults keep changing; a finished run must still be able to explain why it
// chose what it chose, and it cannot do that if the inputs move underneath it.
type Config struct {
	RunID             int64         `json:"run_id"`
	OrganizationID    int64         `json:"organization_id"`
	Criteria          []Criterion   `json:"criteria"`
	TolerancePct      float64       `json:"tolerance_pct"`
	DefaultQuantity   int           `json:"default_quantity"`
	MaxBudget         *money.Amount `json:"max_budget,omitempty"`
	UseSavingProducts bool          `json:"use_saving_products"`
	UseAIMatching     bool          `json:"use_ai_matching"`
	// MinMatchScore is the shared أقل نسبة مطابقة control, 0–1. It decides what
	// the scorer offers for review; pipeline.Cutoff decides what is bought
	// unasked and is deliberately not configurable. Zero means the platform
	// default.
	MinMatchScore float64 `json:"min_match_score,omitempty"`
	MatchLanguage string  `json:"match_language,omitempty"`

	// CriteriaDefaulted records that the buyer enabled nothing and DefaultCriteria
	// was applied, so the results screen can say so rather than implying the
	// buyer chose this order.
	CriteriaDefaulted bool `json:"criteria_defaulted"`
}

// Profile is a buyer's remembered configuration, offered as the next run's start.
type Profile struct {
	OrganizationID    int64       `json:"organization_id"`
	Criteria          []Criterion `json:"criteria"`
	TolerancePct      float64     `json:"tolerance_pct"`
	DefaultQuantity   int         `json:"default_quantity"`
	UseSavingProducts bool        `json:"use_saving_products"`
	UseAIMatching     bool        `json:"use_ai_matching"`
	MinMatchScore     float64     `json:"min_match_score,omitempty"`
	MatchLanguage     string      `json:"match_language,omitempty"`
	LastBranchID      *int64      `json:"last_branch_id,omitempty"`
}

// NewConfig builds a validated snapshot, applying defaults where the buyer
// expressed no preference.
func NewConfig(runID, orgID int64, p Profile, maxBudget *money.Amount) (*Config, error) {
	c := &Config{
		RunID:             runID,
		OrganizationID:    orgID,
		Criteria:          p.Criteria,
		TolerancePct:      p.TolerancePct,
		DefaultQuantity:   p.DefaultQuantity,
		MaxBudget:         maxBudget,
		UseSavingProducts: p.UseSavingProducts,
		UseAIMatching:     p.UseAIMatching,
		MinMatchScore:     p.MinMatchScore,
		MatchLanguage:     p.MatchLanguage,
	}
	if len(c.Criteria) == 0 {
		c.Criteria = append([]Criterion(nil), DefaultCriteria...)
		c.CriteriaDefaulted = true
	}
	if c.TolerancePct == 0 {
		c.TolerancePct = DefaultTolerancePct
	}
	if c.DefaultQuantity <= 0 {
		c.DefaultQuantity = 1
	}
	if c.MinMatchScore <= 0 {
		c.MinMatchScore = productmatch.DefaultMinStrong
	}
	c.MinMatchScore = min(max(c.MinMatchScore, productmatch.DefaultMinReview), 1)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate enforces the rules the database also enforces, so a bad
// configuration is rejected with a usable message instead of a constraint
// violation.
func (c *Config) Validate() error {
	seen := make(map[Criterion]bool, len(c.Criteria))
	for _, cr := range c.Criteria {
		switch cr {
		case CriterionLowestPrice, CriterionHighestDiscount, CriterionFollowedSuppliers:
		default:
			return apperr.Validation("smartorder.unknown_criterion",
				fmt.Sprintf("unknown supplier selection criterion %q", cr), nil)
		}
		if seen[cr] {
			return apperr.Validation("smartorder.duplicate_criterion",
				fmt.Sprintf("criterion %q listed twice", cr), nil)
		}
		seen[cr] = true
	}
	if c.TolerancePct < 0 || c.TolerancePct > 100 {
		return apperr.Validation("smartorder.tolerance_range",
			"tolerance must be between 0 and 100 percent", nil)
	}
	if c.DefaultQuantity < 0 {
		return apperr.Validation("smartorder.default_quantity_negative",
			"default quantity cannot be negative", nil)
	}
	if c.MaxBudget != nil && !c.MaxBudget.IsPositive() {
		return apperr.Validation("smartorder.budget_not_positive",
			"maximum budget must be greater than zero", nil)
	}
	if c.MatchLanguage != "" && c.MatchLanguage != "ar" && c.MatchLanguage != "en" && c.MatchLanguage != "auto" {
		return apperr.Validation("smartorder.invalid_match_language",
			"match language must be 'ar', 'en', 'auto' or empty", nil)
	}
	return nil
}

// ToleranceBps expresses the band in basis points, so the arithmetic that
// applies it stays in integers alongside money.Amount.
func (c *Config) ToleranceBps() int64 {
	return int64(c.TolerancePct * 100)
}

// BudgetStatus compares a total against the ceiling the buyer set.
//
// It never blocks anything. The budget exists so the buyer can see the answer,
// not so the system can refuse the order — a pharmacy that needs the stock needs
// it whether or not it fits a number typed at the start.
func (c *Config) BudgetStatus(total money.Amount) (exceeded bool, overage money.Amount, hasBudget bool) {
	if c.MaxBudget == nil {
		return false, money.Amount{}, false
	}
	if total.Minor() <= c.MaxBudget.Minor() {
		return false, money.Amount{}, true
	}
	over, err := total.Sub(*c.MaxBudget)
	if err != nil {
		// Sub only fails on overflow, which a difference of two order totals
		// cannot reach. Report exceeded without an amount rather than lose the
		// signal entirely.
		return true, money.Amount{}, true
	}
	return true, over, true
}

// EffectiveQty applies the default quantity to a line.
//
// Precedence is: what the buyer edited, then what the file said, then the
// default. The default only ever fills a gap — a row that carried a quantity
// keeps it, which is why a default of 5 does not rewrite a file that already
// says 2.
func (c *Config) EffectiveQty(l *Line) float64 {
	if l.EditedQty != nil {
		return *l.EditedQty
	}
	if l.ImportedQty != nil {
		return *l.ImportedQty
	}
	return float64(c.DefaultQuantity)
}
