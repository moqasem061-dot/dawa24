package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	billingPG "github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	orgPG "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	platformadminPG "github.com/muhiya/dawa24-store/internal/modules/platform_admin/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// Giving every منشأة a Gateway identity, without waiting for someone to open a
// page.
//
// Provisioning used to happen only when an organisation's dashboard rendered.
// An organisation approved months ago whose staff work entirely from the mobile
// order screen had no Gateway user and no key at all, and every AI call anyone
// in it made was billed to the platform's own budget and invisible in that
// tenant's usage. Three of seven organisations on the live database had an
// identity; four did not.
//
// This walks the whole table once. It is idempotent — an organisation that
// already has a working key is left exactly as it is — so it is safe to run on
// a schedule or after a Gateway migration.

// aiIdentities provisions Gateway users and keys for organisations that lack
// them.
func aiIdentities(ctx context.Context, db *database.DB, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("ai-identities", flag.ContinueOnError)
	apply := fs.Bool("apply", false, "provision; without this the command only reports what it would do")
	only := fs.Int64("org", 0, "restrict to one organisation id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Organisation identities are platform-wide bookkeeping, not the work of
	// any one tenant, so this runs outside row-level security deliberately.
	sysCtx := database.AsSystem(ctx)

	adminSvc := platformadmin.NewService(platformadminPG.NewRepository(db), log)
	orgSvc := org.NewService(orgPG.NewRepository(db), log)
	billSvc := billing.NewService(billingPG.NewRepository(db), log)

	gw, err := adminSvc.GetGatewaySettings(sysCtx)
	if err != nil {
		return fmt.Errorf("read gateway settings: %w", err)
	}

	// Missing Gateway credentials stop the identity half of the repair, not the
	// whole command. A منشأة approved before the subscription was assigned on
	// approval has no plan row either, and that half needs nothing but the
	// database — refusing to run it because a third-party credential is absent
	// would leave the tenant broken in a second way for no reason.
	var client *gateway.AdminClient
	if gw != nil && gw.CanProvision() {
		username, password := gw.AdminCredentials()
		client = gateway.NewAdminClient(gw.EndpointURL, username, password)

		// Fail on bad credentials here rather than after touching half the
		// table, so the output names the real problem.
		if err := client.Ping(sysCtx); err != nil {
			return fmt.Errorf("gateway administrator credentials rejected: %w", err)
		}
	} else {
		fmt.Println(i18n.TDefault("w4_mod.s_455_455"))
		fmt.Println(i18n.TDefault("w4_cmd.w4str_261_261"))
	}

	orgs, err := orgSvc.ListOrganizations(sysCtx, nil, nil, 10000, 0)
	if err != nil {
		return fmt.Errorf("list organisations: %w", err)
	}

	var provisioned, unchanged, failed int
	for _, o := range orgs {
		if o == nil || o.ID <= 0 {
			continue
		}
		if *only > 0 && o.ID != *only {
			continue
		}

		// The subscription comes first: it decides which Gateway plan, and so
		// which quota, the identity below is created under.
		subscribed := true
		if sub, err := billSvc.GetActiveSubscriptionByOrg(sysCtx, o.ID); err != nil || sub == nil {
			subscribed = false
			// Booked against the owner: billing.subscriptions.user_id is NOT
			// NULL and references identity.users, so an org-scoped row still
			// has to name a person.
			if *apply && o.Status == org.StatusApproved && o.OwnerID > 0 {
				orgID := o.ID
				if _, err := billSvc.AssignDefaultSubscription(sysCtx, o.OwnerID, &orgID); err != nil {
					fmt.Printf("  org %-7d subscription FAILED: %v\n", o.ID, err)
				} else {
					fmt.Printf("  org %-7d default subscription assigned\n", o.ID)
					subscribed = true
				}
			}
		}

		planID := aiPlanFor(sysCtx, billSvc, o.ID)
		if !*apply {
			state := "has key"
			if o.AIVirtualKey == "" {
				state = "NO KEY"
			}
			if !subscribed {
				state += ", NO SUBSCRIPTION"
			}
			fmt.Printf("  org %-7d %-28s %-10s plan=%-16s %s\n",
				o.ID, truncate(o.LegalName, 28), o.Status, planID, state)
			continue
		}

		if client == nil {
			continue
		}

		identity, err := client.EnsureOrganization(sysCtx, gateway.OrganizationSpec{
			OrganizationID: o.ID,
			Name:           o.LegalName,
			PlanID:         planID,
			ExistingKey:    o.AIVirtualKey,
		})
		if err != nil {
			fmt.Printf("  org %-7d FAILED: %v\n", o.ID, err)
			failed++
			continue
		}

		if !identity.KeyIssued && o.AIUserID == identity.UserID {
			unchanged++
			continue
		}
		if err := orgSvc.UpdateOrganizationAICredentials(sysCtx, o.ID, identity.UserID, identity.VirtualKey); err != nil {
			fmt.Printf("  org %-7d provisioned but NOT SAVED: %v\n", o.ID, err)
			failed++
			continue
		}
		fmt.Printf("  org %-7d provisioned  user=%s plan=%s\n", o.ID, identity.UserID, identity.PlanID)
		provisioned++
	}

	if !*apply {
		fmt.Printf("\n%d organisation(s) inspected. Re-run with --apply to provision.\n", len(orgs))
		return nil
	}
	fmt.Printf("\nprovisioned %d, already correct %d, failed %d\n", provisioned, unchanged, failed)
	if failed > 0 {
		return fmt.Errorf("%d organisation(s) could not be provisioned", failed)
	}
	return nil
}

// aiPlanFor resolves the Gateway plan an organisation's subscription entitles
// it to, mirroring the server's own resolution so the CLI cannot put a tenant
// on a different plan than a page render would.
func aiPlanFor(ctx context.Context, billSvc *billing.Service, orgID int64) string {
	if sub, err := billSvc.GetActiveSubscriptionByOrg(ctx, orgID); err == nil && sub != nil {
		if plan, err := billSvc.GetPlanByID(ctx, sub.PlanID); err == nil && plan != nil && plan.AIPlanID != "" {
			return plan.AIPlanID
		}
	}
	if plan, err := billSvc.GetDefaultPlan(ctx); err == nil && plan != nil && plan.AIPlanID != "" {
		return plan.AIPlanID
	}
	return gateway.FallbackPlanID
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
