package pages

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// OrgSubscriptionView encapsulates subscription and AI Gateway usage details for tenant dashboards.
type OrgSubscriptionView struct {
	HasSubscription    bool
	PlanName           string
	PlanSlug           string
	Status             string
	ExpiresAt          string
	MaxLoginSessions       int
	MaxDevices             int
	BaseLoginSessions      int
	ExtraLoginSessions     int
	HasExtraSessions       bool
	ExtraSessionsExpires   *time.Time
	ExtraSessionsExpiresAt string
	AIPlanID               string
	AIUserID           string
	AIVirtualKeyMasked string
	IsDefaultPlan      bool
	AIRequestsCount    int
	AITokensUsed       int
	AIBudgetLimitUSD   float64
	AIBudgetSpentUSD   float64
	AIBudgetName       string
	AIBudgetResetTime  string
	RawExpiresAt       time.Time
	DaysRemaining      int
	HasDaysRemaining   bool
	RenewalCost        money.Amount
	// HasAIUsage reports whether the Gateway actually answered with this
	// tenant's consumption.
	//
	// It used to be assigned true unconditionally at the end of the loader,
	// after every branch, so a screen with no data at all still claimed to be
	// showing usage. A card that cannot say what a tenant has spent must say
	// so; inventing a number is worse than an empty state, because a pharmacy
	// makes decisions about its plan from this figure.
	HasAIUsage bool
	// HasAIBudget reports whether the Gateway publishes a spending ceiling for
	// this tenant's plan. When false there is no percentage to draw: the limit
	// was previously invented from the plan slug ($15 / $50 / $200), numbers
	// that existed nowhere but one switch statement.
	HasAIBudget bool
}

// TenantSubscriptionPageData represents the full subscription management view model.
type TenantSubscriptionPageData struct {
	Subscription      *OrgSubscriptionView
	Plans             []*billing.Plan
	CurrentPlanID     int64
	WalletBalance     money.Amount
	AutoRenew         bool
	BillingCycle      string
	NoticeType        string
	NoticeMsg         string
	CooldownActive    bool
	CooldownUntil     time.Time
	CooldownUntilText string
	CooldownDays      int
	CooldownHours     int
}

// CooldownPeriodLabel returns the human-readable cooldown period (e.g. "24 ساعة" or "N يوماً").
func (d *TenantSubscriptionPageData) CooldownPeriodLabel() string {
	if d == nil {
		return "24 ساعة"
	}
	if d.CooldownHours > 0 {
		if d.CooldownHours == 24 {
			return "24 ساعة"
		}
		if d.CooldownHours < 24 {
			return fmt.Sprintf("%d ساعة", d.CooldownHours)
		}
		if d.CooldownHours%24 == 0 {
			return fmt.Sprintf("%d يوماً", d.CooldownHours/24)
		}
		return fmt.Sprintf("%d ساعة", d.CooldownHours)
	}
	if d.CooldownDays > 1 {
		return fmt.Sprintf("%d يوماً", d.CooldownDays)
	}
	return "24 ساعة"
}

func (d *TenantSubscriptionPageData) CurrentPlan() *billing.Plan {
	if d == nil {
		return nil
	}
	for _, p := range d.Plans {
		if d.CurrentPlanID > 0 && p.ID == d.CurrentPlanID {
			return p
		}
		if d.CurrentPlanID <= 0 && p.IsDefault {
			return p
		}
	}
	return nil
}

func (d *TenantSubscriptionPageData) NextRenewalAmount() money.Amount {
	p := d.CurrentPlan()
	if p == nil {
		return money.Zero
	}
	if d.BillingCycle == "annual" {
		return p.PriceYear
	}
	return p.PriceMonth
}

func (d *TenantSubscriptionPageData) HasSufficientForRenewal() bool {
	if d == nil {
		return false
	}
	cost := d.NextRenewalAmount()
	if cost.IsZero() || cost.IsNegative() {
		return true
	}
	return d.WalletBalance.Minor() >= cost.Minor()
}

func (d *TenantSubscriptionPageData) ShortfallAmount() money.Amount {
	if d == nil {
		return money.Zero
	}
	cost := d.NextRenewalAmount()
	if d.WalletBalance.Minor() >= cost.Minor() {
		return money.Zero
	}
	return money.FromMinor(cost.Minor() - d.WalletBalance.Minor())
}

func (v *OrgSubscriptionView) DaysRemainingText() string {
	if v == nil || v.IsDefaultPlan || !v.HasDaysRemaining {
		return "باقة دائمة بدون موعد انتهاء"
	}
	if v.DaysRemaining < 0 {
		return "انتهت فترة الاشتراك"
	}
	if v.DaysRemaining == 0 {
		return "اليوم موعد التجديد التلقائي"
	}
	if v.DaysRemaining == 1 {
		return "متبقي يوم واحد على موعد التجديد"
	}
	if v.DaysRemaining == 2 {
		return "متبقي يومان على موعد التجديد"
	}
	if v.DaysRemaining >= 3 && v.DaysRemaining <= 10 {
		return fmt.Sprintf("متبقي %d أيام على موعد التجديد", v.DaysRemaining)
	}
	return fmt.Sprintf("متبقي %d يوماً على موعد التجديد", v.DaysRemaining)
}

// AIPercentage returns how much of the published budget has been spent, 0-100.
//
// Callers must check HasAIBudget first: a zero here means "no ceiling is
// known", not "nothing has been spent", and rendering it as a full-width empty
// progress bar tells the reader the opposite of the truth.
func (v *OrgSubscriptionView) AIPercentage() int {
	if v == nil || !v.HasAIBudget || v.AIBudgetLimitUSD <= 0 {
		return 0
	}
	pct := int((v.AIBudgetSpentUSD / v.AIBudgetLimitUSD) * 100)
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// FormatRelativeResetTime converts a reset timestamp into friendly countdown text.
func FormatRelativeResetTime(rawTime string) string {
	cleaned := strings.TrimSpace(rawTime)
	if cleaned == "" {
		return i18n.T("ar", "ai.periodic_renewal")
	}
	layouts := []string{
		"2006-01-02 03:04 PM",
		"2006-01-02 03:04:05 PM",
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
		"03:04 PM 02-01-2006",
		"02-01-2006 03:04 PM",
		"15:04 02-01-2006",
		"02-01-2006 15:04",
		"02-01-2006",
	}
	var resetTime time.Time
	for _, l := range layouts {
		if t, err := time.Parse(l, cleaned); err == nil {
			resetTime = t
			break
		}
	}
	if resetTime.IsZero() {
		return i18n.T("ar", "ai.periodic_renewal")
	}
	now := time.Now()
	diff := resetTime.Sub(now)
	if diff <= 0 {
		return i18n.T("ar", "ai.today_renewing")
	}
	hours := int(diff.Hours())
	days := hours / 24
	if days > 1 {
		return fmt.Sprintf(i18n.T("ar", "ai.remaining_days_format"), days)
	} else if days == 1 {
		return i18n.T("ar", "ai.remaining_one_day")
	} else if hours > 1 {
		return fmt.Sprintf(i18n.T("ar", "ai.remaining_hours_format"), hours)
	} else if hours == 1 {
		return i18n.T("ar", "ai.remaining_one_hour")
	}
	mins := int(diff.Minutes())
	if mins > 1 {
		return fmt.Sprintf(i18n.T("ar", "ai.remaining_mins_format"), mins)
	}
	return i18n.T("ar", "ai.remaining_less_min")
}

// AIResetText says when the Gateway's budget window reopens.
func (v *OrgSubscriptionView) AIResetText() string {
	if v == nil || v.AIBudgetResetTime == "" {
		return i18n.T("ar", "ai.unspecified")
	}
	return FormatRelativeResetTime(v.AIBudgetResetTime)
}

// AIUsageText renders the spend against the ceiling, or says which of the two
// is unknown.
func (v *OrgSubscriptionView) AIUsageText() string {
	if v == nil || !v.HasAIUsage {
		return i18n.T("ar", "ai.no_usage_data")
	}
	if !v.HasAIBudget {
		return fmt.Sprintf(i18n.T("ar", "ai.consumed_no_ceiling_format"), v.AIBudgetSpentUSD)
	}
	return fmt.Sprintf(i18n.T("ar", "ai.consumed_of_total_format"), v.AIBudgetSpentUSD, v.AIBudgetLimitUSD)
}

// AILogItemView represents one unified, high-density AI request log entry.
type AILogItemView struct {
	ID            string
	Timestamp     string
	TimeFormatted string
	FeatureName   string
	FeatureKey    string
	ModelAlias    string
	ModelTier     string
	InputTokens   int
	OutputTokens  int
	TotalTokens   int
	CostUSD       float64
	DurationMs    int64
	Status        string // "success", "cached", "failed", "completed"
	StatusLabel   string
	SourceContext string
	// CostKnown and DurationKnown separate a real zero from an absent value.
	//
	// Rows drawn from the assistant's own message table used to synthesise both
	// — a cost from invented per-token rates and a flat 280 ms latency — which
	// put fabricated figures in the same column as the Gateway's measured ones,
	// indistinguishable to the reader.
	CostKnown     bool
	DurationKnown bool
}

// CostText renders the billed cost, or says it is not published.
func (l *AILogItemView) CostText() string {
	if l == nil || !l.CostKnown {
		return "—"
	}
	return fmt.Sprintf("%.6f$", l.CostUSD)
}

// DurationText renders the measured latency, or says it is not published.
func (l *AILogItemView) DurationText() string {
	if l == nil || !l.DurationKnown {
		return "—"
	}
	return fmt.Sprintf("%d ms", l.DurationMs)
}

// ModelText renders the model that served the request.
func (l *AILogItemView) ModelText() string {
	if l == nil || strings.TrimSpace(l.ModelAlias) == "" {
		return "محرك دوا 24 الذكي"
	}
	return l.ModelAlias
}

// AIConsumptionLogsPageData represents the full view model for the AI Consumption Logs page.
type AIConsumptionLogsPageData struct {
	Logs              []*AILogItemView
	TotalRequests     int
	TotalTokens       int
	TotalCostUSD      float64
	ActiveBudgetLimit float64
	ActiveBudgetSpent float64
	UsagePercentage   int
	AIUserID          string
	PlanName          string
	PlanSlug          string
	AIPlanID          string
	ResetTime         string
	IsVendor          bool
	IsCustomer        bool
	FeatureBreakdown  map[string]int
	HasBudget         bool
	CostIsComplete    bool
	Page              int
	PerPage           int
	TotalCount        int
	FilterFeature     string
	FilterStatus      string
	FilterSearch      string
}

// FeatureCount returns the request count for a given feature key from FeatureBreakdown.
func (d *AIConsumptionLogsPageData) FeatureCount(key string) int {
	if d == nil || d.FeatureBreakdown == nil {
		return 0
	}
	return d.FeatureBreakdown[key]
}

// QueryParams returns url.Values with active filters preserved for pagination links.
func (d *AIConsumptionLogsPageData) QueryParams() url.Values {
	v := url.Values{}
	if d == nil {
		return v
	}
	if d.FilterFeature != "" {
		v.Set("feature", d.FilterFeature)
	}
	if d.FilterStatus != "" {
		v.Set("status", d.FilterStatus)
	}
	if d.FilterSearch != "" {
		v.Set("q", d.FilterSearch)
	}
	return v
}

// TotalCostText renders the window's spend, marked as a floor when some of the
// requests in it carried no published price.
func (d *AIConsumptionLogsPageData) TotalCostText() string {
	if d == nil {
		return "—"
	}
	if d.TotalRequests == 0 {
		return i18n.T("ar", "ai.no_operations")
	}
	if !d.CostIsComplete {
		return fmt.Sprintf(i18n.T("ar", "ai.at_least_cost_format"), d.TotalCostUSD)
	}
	return fmt.Sprintf("%.4f$", d.TotalCostUSD)
}

// QuotaText renders the quota headline, or says the Gateway published none.
func (d *AIConsumptionLogsPageData) QuotaText() string {
	if d == nil || !d.HasBudget {
		return i18n.T("ar", "ai.no_ceiling_published")
	}
	return fmt.Sprintf("%d%%", d.UsagePercentage)
}

// ResetCountdown returns friendly relative text for quota renewal.
func (d *AIConsumptionLogsPageData) ResetCountdown() string {
	if d == nil || strings.TrimSpace(d.ResetTime) == "" {
		return i18n.T("ar", "ai.unspecified")
	}
	return FormatRelativeResetTime(d.ResetTime)
}
