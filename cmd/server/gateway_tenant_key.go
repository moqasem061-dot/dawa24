package main

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/reqcache"
)

// One Gateway identity per منشأة, resolved once and shared by its employees.
//
// This replaces four copy-pasted blocks — two key resolvers in routes.go, one in
// the dashboard handler, one in the admin handler — which each read the
// organisation, read its subscription, read its plan, and then called
// ProvisionOrganization inline on the request path. That arrangement had three
// faults and this type exists to fix all three:
//
//  1. Every call minted a new virtual key, and minting revokes the previous one.
//     Two concurrent dashboard renders left the tenant with a key that had
//     already been revoked by the render next to it.
//  2. A stored key was trusted without being checked, so once that happened the
//     column held a dead credential and every AI call failed for good.
//  3. The organisation's Gateway plan was set at first provision and never
//     again. Upgrading a subscription changed nothing about the tenant's AI
//     quota.
//
// It mirrors adminKeyProvisioner deliberately: same TTL cache, same
// validate-before-trust rule, same failure back-off. The difference is that it
// is keyed by organisation and holds a per-organisation lock, so a first-time
// provision for one tenant does not serialise every other tenant behind it.

// errGatewayNotConfigured is returned when an operator has not supplied
// credentials the Gateway's management API will accept. It is a configuration
// state, not a failure, so callers log it once and use their fallback rather
// than retrying it.
var errGatewayNotConfigured = errors.New("gateway admin credentials not configured")

// tenantKeyTTL is how long a resolved key is reused without re-reading the
// organisation and re-checking the key. Long enough that a nine-thousand-row
// import does not re-resolve per batch; short enough that a plan change takes
// effect within a page refresh or two even if the explicit sync missed it.
const tenantKeyTTL = 5 * time.Minute

// tenantRetryAfter is the quiet period after a failed provisioning attempt, so
// an unreachable Gateway is not dialled on every request.
const tenantRetryAfter = 60 * time.Second

// tenantKeyProvisioner resolves, caches and repairs per-organisation keys.
type tenantKeyProvisioner struct {
	orgs     *org.Service
	bill     *billing.Service
	admin    *platformadmin.Service
	fallback *adminKeyProvisioner
	log      *slog.Logger

	mu      sync.Mutex
	entries map[int64]*tenantKeyEntry
}

// tenantKeyEntry is one organisation's cached identity, with the lock that
// serialises provisioning for that organisation only.
type tenantKeyEntry struct {
	mu sync.Mutex

	key    string
	planID string
	// resolvedAt is when key was last proven good, not when it was issued.
	resolvedAt  time.Time
	lastFailure time.Time
}

func newTenantKeyProvisioner(
	orgs *org.Service,
	bill *billing.Service,
	admin *platformadmin.Service,
	fallback *adminKeyProvisioner,
	log *slog.Logger,
) *tenantKeyProvisioner {
	return &tenantKeyProvisioner{
		orgs:     orgs,
		bill:     bill,
		admin:    admin,
		fallback: fallback,
		log:      log.With("component", "gateway_tenant_key"),
		entries:  make(map[int64]*tenantKeyEntry),
	}
}

// Key returns the organisation's virtual key, provisioning one if needed.
//
// It returns an empty string rather than an error when no key can be had:
// every caller's contract is to fall back to its deterministic path, and an
// error here would only be translated into that same decision one layer up.
func (p *tenantKeyProvisioner) Key(ctx context.Context, orgID int64) string {
	if p == nil {
		return ""
	}
	if orgID <= 0 {
		return p.platformKey(ctx)
	}

	entry := p.entryFor(orgID)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	sysCtx := database.AsSystem(ctx)
	wantPlan := p.planFor(sysCtx, orgID)

	// A cached key is reused only while the plan it was provisioned under is
	// still the plan the tenant is on. That check is what makes an upgrade take
	// effect without waiting for the TTL.
	if entry.key != "" && entry.planID == wantPlan && time.Since(entry.resolvedAt) < tenantKeyTTL {
		return entry.key
	}

	if time.Since(entry.lastFailure) < tenantRetryAfter {
		if entry.key != "" {
			return entry.key
		}
		return p.platformKey(ctx)
	}

	key, err := p.provision(sysCtx, orgID, wantPlan)
	if err != nil {
		entry.lastFailure = time.Now()
		p.log.WarnContext(ctx, "could not resolve organisation gateway key",
			"org_id", orgID, "plan_id", wantPlan, "error", err)
		if entry.key != "" {
			return entry.key
		}
		return p.platformKey(ctx)
	}

	entry.key, entry.planID, entry.resolvedAt = key, wantPlan, time.Now()
	entry.lastFailure = time.Time{}
	return key
}

// SyncPlan moves an organisation onto the Gateway plan its current subscription
// entitles it to, and drops the cached key so the next Key call re-resolves.
//
// Billing calls this the moment a subscription is created, changed or renewed.
// It is the answer to the plan never following the subscription.
func (p *tenantKeyProvisioner) SyncPlan(ctx context.Context, orgID int64) error {
	if p == nil || orgID <= 0 {
		return nil
	}
	sysCtx := database.AsSystem(ctx)

	client, err := p.client(sysCtx)
	if err != nil {
		return err
	}

	o, err := p.orgs.GetOrganization(sysCtx, orgID)
	if err != nil {
		return err
	}
	planID := p.planFor(sysCtx, orgID)

	spec := gateway.OrganizationSpec{
		OrganizationID: orgID,
		PlanID:         planID,
	}
	if o != nil {
		spec.Name = o.LegalName
	}
	if err := client.SyncOrganizationPlan(sysCtx, spec); err != nil {
		return err
	}

	entry := p.entryFor(orgID)
	entry.mu.Lock()
	entry.planID = ""
	entry.resolvedAt = time.Time{}
	entry.mu.Unlock()

	p.log.InfoContext(ctx, "organisation gateway plan synchronised",
		"org_id", orgID, "plan_id", planID)
	return nil
}

// Invalidate drops an organisation's cached key. The settings screen calls it
// after an operator changes Gateway credentials.
func (p *tenantKeyProvisioner) Invalidate(orgID int64) {
	if p == nil {
		return
	}
	if orgID <= 0 {
		p.mu.Lock()
		p.entries = make(map[int64]*tenantKeyEntry)
		p.mu.Unlock()
		return
	}
	entry := p.entryFor(orgID)
	entry.mu.Lock()
	entry.key, entry.planID = "", ""
	entry.resolvedAt, entry.lastFailure = time.Time{}, time.Time{}
	entry.mu.Unlock()
}

// provision brings the organisation's Gateway account into line and returns a
// key that has been proven to work.
func (p *tenantKeyProvisioner) provision(ctx context.Context, orgID int64, planID string) (string, error) {
	client, err := p.client(ctx)
	if err != nil {
		return "", err
	}
	o, err := p.orgs.GetOrganization(ctx, orgID)
	if err != nil {
		return "", err
	}

	spec := gateway.OrganizationSpec{OrganizationID: orgID, PlanID: planID}
	if o != nil {
		spec.Name = o.LegalName
		spec.ExistingKey = o.AIVirtualKey
	}

	identity, err := client.EnsureOrganization(ctx, spec)
	if err != nil {
		return "", err
	}

	// Persist only when something actually changed. A dashboard render must not
	// write to org.organizations on every page view.
	if o == nil || identity.KeyIssued || o.AIUserID != identity.UserID {
		if err := p.orgs.UpdateOrganizationAICredentials(ctx, orgID, identity.UserID, identity.VirtualKey); err != nil {
			// The key works even if it could not be stored; the next resolve
			// will find the same one on the Gateway and reuse it.
			p.log.WarnContext(ctx, "provisioned organisation gateway key but could not persist it",
				"org_id", orgID, "error", err)
		} else {
			p.log.InfoContext(ctx, "organisation gateway identity provisioned",
				"org_id", orgID, "gateway_user", identity.UserID, "plan_id", identity.PlanID)
		}
	}
	return identity.VirtualKey, nil
}

// client builds an admin client from the credentials an operator configured.
func (p *tenantKeyProvisioner) client(ctx context.Context) (*gateway.AdminClient, error) {
	gw, err := p.admin.GetGatewaySettings(ctx)
	if err != nil {
		return nil, err
	}
	if gw == nil || !gw.IsActive || gw.EndpointURL == "" {
		return nil, errGatewayNotConfigured
	}
	if !gw.CanProvision() {
		return nil, errGatewayNotConfigured
	}
	username, password := gw.AdminCredentials()
	return gateway.NewAdminClient(gw.EndpointURL, username, password), nil
}

// planFor resolves the Gateway plan an organisation's subscription entitles it
// to, falling back to the platform's default plan and then to the shared
// constant.
//
// A page asks for the key once per AI-backed widget, so the answer is memoised
// for the request: three widgets used to read the subscription three times.
func (p *tenantKeyProvisioner) planFor(ctx context.Context, orgID int64) string {
	if p.bill == nil {
		return gateway.FallbackPlanID
	}
	plan, _ := reqcache.Get(ctx, "gateway.plan:"+strconv.FormatInt(orgID, 10), func() (string, error) {
		return p.resolvePlan(ctx, orgID), nil
	})
	return plan
}

func (p *tenantKeyProvisioner) resolvePlan(ctx context.Context, orgID int64) string {
	if sub, err := p.bill.GetActiveSubscriptionByOrg(ctx, orgID); err == nil && sub != nil {
		if plan, err := p.bill.GetPlanByID(ctx, sub.PlanID); err == nil && plan != nil && plan.AIPlanID != "" {
			return plan.AIPlanID
		}
	}
	if plan, err := p.bill.GetDefaultPlan(ctx); err == nil && plan != nil && plan.AIPlanID != "" {
		return plan.AIPlanID
	}
	return gateway.FallbackPlanID
}

// platformKey is the last resort: the admin panel's own key.
//
// Using it spends the platform's budget on a tenant's work and hides that work
// from the tenant's usage screen, so it is logged every time. It is kept rather
// than removed because the alternative — no AI at all for an organisation the
// Gateway could not be asked about — is worse for the pharmacy standing at the
// counter, and because provisioning now happens on approval rather than only on
// a page render, so this path should be rare.
func (p *tenantKeyProvisioner) platformKey(ctx context.Context) string {
	if p.fallback == nil {
		return ""
	}
	return p.fallback.Key(ctx)
}

func (p *tenantKeyProvisioner) entryFor(orgID int64) *tenantKeyEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.entries[orgID]
	if !ok {
		entry = &tenantKeyEntry{}
		p.entries[orgID] = entry
	}
	return entry
}
