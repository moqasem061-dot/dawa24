package billing

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AssignDefaultSubscription guarantees an organization or user has an active basic subscription.
func (s *Service) AssignDefaultSubscription(ctx context.Context, userID int64, orgID *int64) (*Subscription, error) {
	if orgID != nil && *orgID > 0 {
		if sub, err := s.repo.GetActiveSubscriptionByOrg(ctx, *orgID); err == nil && sub != nil {
			return sub, nil
		}
	} else if userID > 0 {
		if sub, err := s.repo.GetActiveSubscription(ctx, userID); err == nil && sub != nil {
			return sub, nil
		}
	}

	plan, err := s.repo.GetDefaultPlan(ctx)
	if err != nil || plan == nil {
		plan, err = s.repo.GetPlanBySlug(ctx, "basic")
		if err != nil || plan == nil {
			return nil, fmt.Errorf("default plan not found: %w", err)
		}
	}

	now := time.Now().UTC()
	duration := time.Duration(plan.DurationDays) * 24 * time.Hour
	if duration <= 0 {
		duration = 3650 * 24 * time.Hour
	}

	sub := &Subscription{
		UserID:         userID,
		OrganizationID: orgID,
		PlanID:         plan.ID,
		Status:         SubActive,
		StartsAt:       now,
		ExpiresAt:      now.Add(duration),
		SourceSystem:   "registration_default",
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}
	s.syncAIPlan(ctx, orgID)
	s.log.InfoContext(ctx, "default subscription assigned", "user_id", userID, "org_id", orgID, "plan", plan.Slug)
	return sub, nil
}

// Subscribe grants a user a subscription to a plan tier.
func (s *Service) Subscribe(
	ctx context.Context,
	userID int64,
	orgID *int64,
	planSlug string,
	sourceSystem string,
	sourceID *int64,
) (*Subscription, error) {
	plan, err := s.repo.GetPlanBySlug(ctx, planSlug)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	duration := time.Duration(plan.DurationDays) * 24 * time.Hour
	if duration <= 0 {
		duration = 30 * 24 * time.Hour
	}

	sub := &Subscription{
		UserID:         userID,
		OrganizationID: orgID,
		PlanID:         plan.ID,
		Status:         SubActive,
		BillingCycle:   "monthly",
		AutoRenew:      false,
		StartsAt:       now,
		ExpiresAt:      now.Add(duration),
		SourceSystem:   sourceSystem,
		SourceID:       sourceID,
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}

	if userID > 0 {
		if wallet, err := s.repo.GetOrCreateWallet(ctx, userID, "EGP"); err == nil && wallet != nil {
			desc := fmt.Sprintf(i18n.TDefault("billing.sub.activate_desc"), plan.Name.Get("ar"), sourceSystem)
			_, _ = s.repo.RecordTransaction(ctx, wallet.ID, TxPurchase, money.Zero, "subscription_"+sourceSystem, &sub.ID, desc)
		}
	}

	s.syncAIPlan(ctx, orgID)
	s.log.InfoContext(ctx, "subscription activated", "user_id", userID, "plan", planSlug, "expires", sub.ExpiresAt)
	return sub, nil
}

// SubscribeWithWallet purchases or upgrades a subscription plan using the user's/org's wallet balance.
func (s *Service) SubscribeWithWallet(
	ctx context.Context,
	userID int64,
	orgID *int64,
	planSlug string,
	cycle string,
	autoRenew bool,
) (*Subscription, error) {
	plan, err := s.repo.GetPlanBySlug(ctx, planSlug)
	if err != nil {
		return nil, err
	}

	if cycle != "annual" && cycle != "monthly" {
		cycle = "monthly"
	}

	cost := plan.PriceMonth
	durationDays := 30
	if cycle == "annual" {
		cost = plan.PriceYear
		durationDays = 365
	}

	wallet, err := s.repo.GetOrCreateWallet(ctx, userID, "EGP")
	if err != nil {
		return nil, err
	}

	// 1. Identify existing subscription to detect upgrade vs renewal vs initial subscription
	var currentSub *Subscription
	if orgID != nil && *orgID > 0 {
		currentSub, _ = s.repo.GetActiveSubscriptionByOrg(ctx, *orgID)
	} else if userID > 0 {
		currentSub, _ = s.repo.GetActiveSubscription(ctx, userID)
	}

	now := time.Now().UTC()
	isRenewal := false
	isUpgrade := false
	var startsAt time.Time
	var expiresAt time.Time

	// Enforce subscription change cooldown (WO-15: 24 hours for plan changes/upgrades)
	if currentSub != nil {
		currentPlan, _ := s.repo.GetPlanByID(ctx, currentSub.PlanID)
		if currentPlan != nil && !currentPlan.IsDefault && !currentPlan.PriceMonth.IsZero() {
			// Current subscription is on a paid plan
			isPlanChange := (currentSub.PlanID != plan.ID || currentSub.BillingCycle != cycle)
			settings := s.GetCooldownSettings(ctx)
			if isPlanChange {
				cooldownHours := settings.CooldownHours
				if cooldownHours <= 0 {
					cooldownHours = 24
				}
				earliestAllowed := currentSub.StartsAt.Add(time.Duration(cooldownHours) * time.Hour)
				if now.Before(earliestAllowed) {
					msg := fmt.Sprintf(i18n.TDefault("billing.sub.cooldown_downgrade"), earliestAllowed.Format("2006-01-02 15:04"))
					return nil, apperr.Conflict("subscription.change_cooldown", msg)
				}
			} else if settings.MinHours > 0 {
				// Same plan & cycle renewal: minimum wait after any purchase
				earliestRenewal := currentSub.StartsAt.Add(time.Duration(settings.MinHours) * time.Hour)
				if now.Before(earliestRenewal) {
					msg := fmt.Sprintf(i18n.TDefault("billing.sub.cooldown_renewal"), settings.MinDays, earliestRenewal.Format("2006-01-02 15:04"))
					return nil, apperr.Conflict("subscription.change_cooldown", msg)
				}
			}
		}
	}

	if currentSub != nil && currentSub.PlanID == plan.ID && currentSub.BillingCycle == cycle && currentSub.ExpiresAt.After(now) {
		// Early renewal: maintain original start, extend expiration date by plan duration
		isRenewal = true
		startsAt = currentSub.StartsAt
		expiresAt = currentSub.ExpiresAt.Add(time.Duration(durationDays) * 24 * time.Hour)
	} else if currentSub != nil {
		// Upgrade or plan switch: resets old consumption window and starts fresh quota immediately
		isUpgrade = true
		startsAt = now
		expiresAt = now.Add(time.Duration(durationDays) * 24 * time.Hour)
	} else {
		// First-time subscription
		startsAt = now
		expiresAt = now.Add(time.Duration(durationDays) * 24 * time.Hour)
	}

	planName := plan.Name.Get("ar")
	if planName == "" {
		planName = plan.Name.Get("en")
	}
	if planName == "" {
		planName = plan.Slug
	}

	cycleStr := i18n.TDefault("billing.sub.cycle_monthly")
	if cycle == "annual" {
		cycleStr = i18n.TDefault("billing.sub.cycle_yearly")
	}

	var refType string
	var desc string
	if isRenewal {
		refType = "subscription_renewal"
		desc = fmt.Sprintf(i18n.TDefault("billing.sub.renew_desc"), planName, cycleStr, expiresAt.Format("2006-01-02"))
	} else if isUpgrade {
		if cost.IsZero() {
			refType = "subscription_change"
			desc = fmt.Sprintf(i18n.TDefault("billing.sub.modify_desc"), planName, cycleStr)
		} else {
			refType = "subscription_upgrade"
			desc = fmt.Sprintf(i18n.TDefault("billing.sub.upgrade_desc"), planName, cycleStr)
		}
	} else {
		refType = "subscription_checkout"
		desc = fmt.Sprintf(i18n.TDefault("billing.sub.new_desc"), planName, cycleStr)
	}

	// 2. Validate wallet available balance if non-free plan
	if !cost.IsZero() && !cost.IsNegative() {
		if wallet.AvailableBalance.Minor() < cost.Minor() {
			return nil, apperr.Conflict("wallet.insufficient_funds", i18n.T("ar", "billing.err.insufficient_funds"))
		}
	}

	// 3. Create the subscription record
	sub := &Subscription{
		UserID:         userID,
		OrganizationID: orgID,
		PlanID:         plan.ID,
		Status:         SubActive,
		BillingCycle:   cycle,
		AutoRenew:      autoRenew,
		StartsAt:       startsAt,
		ExpiresAt:      expiresAt,
		SourceSystem:   "wallet_checkout",
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}

	// 4. Record wallet ledger transaction with the subscription ID reference (always recorded for complete transaction history)
	negCost := money.Zero
	if !cost.IsZero() && !cost.IsNegative() {
		negCost = money.FromMinor(-cost.Minor())
	}
	_, err = s.repo.RecordTransaction(ctx, wallet.ID, TxPurchase, negCost, refType, &sub.ID, desc)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to record subscription transaction in wallet", "error", err, "sub_id", sub.ID)
		return nil, err
	}

	// 5. File the transition.
	//
	// Only renewals used to be recorded, so the trail an administrator reads on
	// /admin/plans?tab=subscriptions was empty for every first purchase and
	// every plan change -- the two transitions anyone actually asks about.
	// refType already names the transition and is the same string the wallet
	// ledger carries, so the two records agree.
	histOrgID := int64(0)
	if orgID != nil {
		histOrgID = *orgID
	}
	s.recordSubscriptionHistory(ctx, SubscriptionHistoryEntry{
		SubscriptionID: sub.ID,
		OrganizationID: histOrgID,
		UserID:         userID,
		PlanID:         plan.ID,
		Action:         refType,
		AmountMinor:    cost.Minor(),
		Currency:       "EGP",
		Details:        desc,
	})

	// 6. Synchronise AI quota to Gateway so the new plan limits and quota apply
	s.syncAIPlan(ctx, orgID)

	s.log.InfoContext(ctx, "subscription activated via wallet",
		"user_id", userID,
		"org_id", orgID,
		"sub_id", sub.ID,
		"plan", planSlug,
		"cycle", cycle,
		"is_upgrade", isUpgrade,
		"is_renewal", isRenewal,
		"cost", cost.String(),
		"auto_renew", autoRenew,
	)
	return sub, nil
}

// ProcessDueSubscriptionRenewals checks and executes auto-renewals for all due subscriptions.
func (s *Service) ProcessDueSubscriptionRenewals(ctx context.Context) (renewed int, failed int, err error) {
	now := time.Now().UTC()
	dueSubs, err := s.repo.ListDueSubscriptionsForRenewal(ctx, now)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to list due subscriptions for renewal", "error", err)
		return 0, 0, err
	}

	for _, sub := range dueSubs {
		plan, err := s.repo.GetPlanByID(ctx, sub.PlanID)
		if err != nil {
			s.log.ErrorContext(ctx, "renewal: failed to fetch plan", "sub_id", sub.ID, "plan_id", sub.PlanID, "error", err)
			continue
		}

		cost := plan.PriceMonth
		durationDays := 30
		if sub.BillingCycle == "annual" {
			cost = plan.PriceYear
			durationDays = 365
		}

		// Calculate new expiration date anchored on current expiration (or now if past)
		baseTime := sub.ExpiresAt
		if baseTime.Before(now.Add(-24 * time.Hour)) {
			baseTime = now
		}
		newExpiresAt := baseTime.Add(time.Duration(durationDays) * 24 * time.Hour)

		wallet, err := s.repo.GetOrCreateWallet(ctx, sub.UserID, "EGP")
		if err != nil {
			s.log.ErrorContext(ctx, "renewal: failed to get wallet", "sub_id", sub.ID, "user_id", sub.UserID, "error", err)
			continue
		}

		desc := fmt.Sprintf(i18n.T("ar", "billing.renewal_desc"), plan.Name.Get("ar"), sub.BillingCycle)
		err = s.repo.RenewSubscription(ctx, sub.ID, wallet.ID, cost, newExpiresAt, desc)
		if err != nil {
			s.log.WarnContext(ctx, "subscription renewal failed", "sub_id", sub.ID, "user_id", sub.UserID, "error", err)
			_ = s.repo.UpdateSubscriptionStatus(ctx, sub.ID, SubPastDue, sub.RenewalAttempts+1)
			failed++
		} else {
			// A renewal can follow a plan change made while the old term ran,
			// and it reopens the Gateway's budget window either way.
			s.syncAIPlan(ctx, sub.OrganizationID)
			s.log.InfoContext(ctx, "subscription renewed successfully", "sub_id", sub.ID, "user_id", sub.UserID, "new_expires_at", newExpiresAt)
			renewed++
		}
	}

	return renewed, failed, nil
}

// CheckEntitlement resolves whether a user has access to a specific feature key.
func (s *Service) CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error) {
	return s.repo.CheckEntitlement(ctx, userID, featureKey)
}

// CheckOrgEntitlement resolves whether an organization/user has active access to featureKey.
func (s *Service) CheckOrgEntitlement(ctx context.Context, orgID, userID int64, featureKey string) (bool, error) {
	return s.repo.CheckOrgEntitlement(ctx, orgID, userID, featureKey)
}
