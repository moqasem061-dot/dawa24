package billing

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// CooldownSettings configures plan change cooldowns.
type CooldownSettings struct {
	CooldownHours int `json:"cooldown_hours"` // Hours a paid plan must wait before changing plan (default 24)
	CooldownDays  int `json:"cooldown_days"`  // Cooldown in days (default 1)
	MinHours      int `json:"min_hours"`      // Absolute minimum wait after any purchase in hours (default 24)
	MinDays       int `json:"min_days"`       // Absolute minimum wait in days (default 1)
}

// SubscriptionCooldownInfo summarizes the active cooldown status for an organization or user.
type SubscriptionCooldownInfo struct {
	IsOnCooldown      bool          `json:"is_on_cooldown"`
	EarliestAllowedAt time.Time     `json:"earliest_allowed_at"`
	CurrentPlanSlug   string        `json:"current_plan_slug"`
	CurrentPlanName   string        `json:"current_plan_name"`
	CooldownDays      int           `json:"cooldown_days"`
	CooldownHours     int           `json:"cooldown_hours"`
	RemainingDuration time.Duration `json:"remaining_duration"`
}

// GetCooldownSettings reads the configured cooldown thresholds from system settings,
// falling back to defaults (24 hours for paid plan cooldown, 24 hours minimum wait).
func (s *Service) GetCooldownSettings(ctx context.Context) CooldownSettings {
	settings := CooldownSettings{
		CooldownHours: 24,
		CooldownDays:  1,
		MinHours:      24,
		MinDays:       1,
	}
	if s == nil || s.repo == nil {
		return settings
	}

	if val, err := s.repo.GetSetting(ctx, "subscription_change_cooldown_hours"); err == nil && val != "" {
		if h := parseSettingInt(val); h > 0 {
			settings.CooldownHours = h
			settings.CooldownDays = (h + 23) / 24
		}
	} else if val, err := s.repo.GetSetting(ctx, "subscription_change_cooldown_days"); err == nil && val != "" {
		if d := parseSettingInt(val); d > 0 {
			if d == 25 {
				// Legacy default was 25 days; migrate to 24 hours (1 day)
				settings.CooldownHours = 24
				settings.CooldownDays = 1
			} else {
				settings.CooldownDays = d
				settings.CooldownHours = d * 24
			}
		}
	}

	if val, err := s.repo.GetSetting(ctx, "subscription_change_min_hours"); err == nil && val != "" {
		if h := parseSettingInt(val); h > 0 {
			settings.MinHours = h
			settings.MinDays = (h + 23) / 24
		}
	} else if val, err := s.repo.GetSetting(ctx, "subscription_change_min_days"); err == nil && val != "" {
		if d := parseSettingInt(val); d > 0 {
			if d == 2 {
				// Legacy default was 2 days (48h); align with 24 hours
				settings.MinHours = 24
				settings.MinDays = 1
			} else {
				settings.MinDays = d
				settings.MinHours = d * 24
			}
		}
	}

	if settings.MinHours > settings.CooldownHours {
		settings.MinHours = settings.CooldownHours
		settings.MinDays = settings.CooldownDays
	}

	return settings
}

func parseSettingInt(val string) int {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0
	}
	// Try direct int first
	if n, err := strconv.Atoi(val); err == nil {
		return n
	}
	// Try JSON map
	var m map[string]any
	if err := json.Unmarshal([]byte(val), &m); err == nil {
		if v, ok := m["days"].(float64); ok && v > 0 {
			return int(v)
		}
		if v, ok := m["value"].(float64); ok && v > 0 {
			return int(v)
		}
		if v, ok := m["days"].(string); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return n
			}
		}
		if v, ok := m["value"].(string); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return n
			}
		}
	}
	return 0
}

// CheckSubscriptionChangeCooldown determines if an org or user is currently prohibited
// from changing their subscription plan due to the active cooldown window.
func (s *Service) CheckSubscriptionChangeCooldown(
	ctx context.Context,
	userID int64,
	orgID *int64,
	targetPlanSlug string,
) (SubscriptionCooldownInfo, error) {
	info := SubscriptionCooldownInfo{}
	if s == nil || s.repo == nil {
		return info, nil
	}

	var currentSub *Subscription
	if orgID != nil && *orgID > 0 {
		currentSub, _ = s.repo.GetActiveSubscriptionByOrg(ctx, *orgID)
	} else if userID > 0 {
		currentSub, _ = s.repo.GetActiveSubscription(ctx, userID)
	}

	if currentSub == nil {
		return info, nil
	}

	currentPlan, err := s.repo.GetPlanByID(ctx, currentSub.PlanID)
	if err != nil || currentPlan == nil {
		return info, nil
	}

	info.CurrentPlanSlug = currentPlan.Slug
	info.CurrentPlanName = currentPlan.Name.Get("ar")
	if info.CurrentPlanName == "" {
		info.CurrentPlanName = currentPlan.Name.Get("en")
	}

	// Rule 1: Free / default plan is exempt. Upgrading from free is ALWAYS allowed immediately.
	if currentPlan.IsDefault || currentPlan.PriceMonth.IsZero() {
		return info, nil
	}

	// Current plan is a paid plan.
	settings := s.GetCooldownSettings(ctx)
	cooldownHours := settings.CooldownHours
	if cooldownHours <= 0 {
		cooldownHours = 24
	}
	info.CooldownHours = cooldownHours
	info.CooldownDays = (cooldownHours + 23) / 24

	earliestAllowed := currentSub.StartsAt.Add(time.Duration(cooldownHours) * time.Hour)
	info.EarliestAllowedAt = earliestAllowed

	now := time.Now().UTC()
	if now.Before(earliestAllowed) {
		info.IsOnCooldown = true
		info.RemainingDuration = earliestAllowed.Sub(now)
	}

	return info, nil
}
