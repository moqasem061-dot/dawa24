package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// A tenant's identity on the Gateway.
//
// Every organisation gets one Gateway user and one virtual key, and every
// employee of that organisation spends against it. That is the whole point: an
// import a vendor's warehouse clerk runs and a question their pharmacist asks
// the assistant come out of the same budget, attributed to the same منشأة,
// visible on the same usage screen.
//
// What this file exists to fix is that provisioning used to be neither
// idempotent nor safe to call concurrently. Every call POSTed /api/keys, and
// issuing a key for a user REVOKES the one before it. So two browser tabs
// opening a dashboard at the same moment minted two keys, the second killed the
// first, and whichever one got written to org.organizations last was the only
// one that worked — until the next render did it again. A stored key was
// trusted without ever being checked, so a revoked one stayed in the column
// silently failing every AI call.
//
// EnsureOrganization is therefore written to be callable as often as anyone
// likes: it validates before it reuses, reuses before it mints, and mints only
// when there is genuinely nothing to reuse.

// FallbackPlanID is the Gateway plan an organisation gets when its billing plan
// names none.
//
// One constant, because the codebase previously disagreed with itself in three
// places — "plan-pos-free" in the admin client, "plan-basic" in the billing
// repository, "plan-dev" in the dashboard — which meant the plan a tenant ended
// up on depended on which code path happened to provision them first.
const FallbackPlanID = "plan-pos-free"

// OrganizationUserID is the Gateway user id for an organisation.
//
// Derived rather than stored so it cannot drift: the Gateway is keyed by this
// string, and an org.organizations row holding a different one would orphan a
// tenant's whole usage history.
func OrganizationUserID(orgID int64) string {
	return fmt.Sprintf("org-%d", orgID)
}

// OrganizationSpec is what a tenant's Gateway account should look like.
type OrganizationSpec struct {
	OrganizationID int64
	// Name and Email are cosmetic on the Gateway side but are what an operator
	// reads in its admin screens, so a blank one is filled in rather than sent.
	Name  string
	Email string
	// PlanID is the Gateway plan the organisation's billing plan maps to.
	PlanID string
	// ExistingKey is whatever org.organizations already holds. It is checked,
	// not trusted: see the package note above.
	ExistingKey string
}

// OrganizationIdentity is the resolved account.
type OrganizationIdentity struct {
	UserID     string
	VirtualKey string
	PlanID     string
	// KeyIssued reports whether this call minted a new key, which the caller
	// must persist. When false the stored key was reused and no write is
	// needed — the distinction is what keeps a dashboard render from writing to
	// org.organizations on every page view.
	KeyIssued bool
}

// EnsureOrganization brings a tenant's Gateway account into the state the spec
// describes, and returns a key that has been proven to work.
//
// It is idempotent and safe to call on every request. Callers should still hold
// a per-organisation lock around it (see the tenant key provisioner), because
// two simultaneous first-time provisions would each correctly find nothing to
// reuse and each correctly mint — and the second would revoke the first.
func (c *AdminClient) EnsureOrganization(ctx context.Context, spec OrganizationSpec) (OrganizationIdentity, error) {
	id := OrganizationIdentity{
		UserID: OrganizationUserID(spec.OrganizationID),
		PlanID: strings.TrimSpace(spec.PlanID),
	}
	if spec.OrganizationID <= 0 {
		return id, fmt.Errorf("gateway admin: organisation id required")
	}
	if id.PlanID == "" {
		id.PlanID = FallbackPlanID
	}

	// 1. The account itself, with its plan. Doing this first means a tenant
	//    whose subscription changed gets the new quota even when their key is
	//    already valid and nothing below this line runs.
	if err := c.upsertUser(ctx, GatewayUser{
		ID:     id.UserID,
		Name:   organizationName(spec),
		Email:  organizationEmail(spec),
		PlanID: id.PlanID,
		Status: "active",
	}); err != nil {
		return id, err
	}

	// 2. The key we already have, if it still works.
	if stored := strings.TrimSpace(spec.ExistingKey); stored != "" {
		switch c.keyState(ctx, stored) {
		case keyValid, keyUnknown:
			// Unknown means the Gateway could not be reached to check. Treating
			// that as valid is deliberate: a transient outage must not cause a
			// re-mint that revokes a key which was fine, turning a five-second
			// blip into a permanent credential rotation.
			id.VirtualKey = stored
			return id, nil
		case keyRejected:
			// Fall through and find or mint a replacement.
		}
	}

	// 3. A key the Gateway already holds for this user. Reusing one costs a
	//    request; minting one revokes whatever else exists.
	if reused, ok := c.reusableKey(ctx, id.UserID); ok {
		id.VirtualKey = reused
		id.KeyIssued = true // it was not what the caller had stored
		return id, nil
	}

	// 4. Nothing to reuse.
	key, err := c.issueKey(ctx, id.UserID, fmt.Sprintf("Dawa24-Org-%d-Key", spec.OrganizationID))
	if err != nil {
		return id, err
	}
	id.VirtualKey = key
	id.KeyIssued = true
	return id, nil
}

// SyncOrganizationPlan moves a tenant onto the Gateway plan their subscription
// now entitles them to.
//
// This is the call that was missing entirely: UpdateOrganizationPlan existed
// and had no callers, so an organisation kept whatever AI quota it was given the
// first time it was provisioned no matter how many times it upgraded.
//
// Unlike its predecessor it reports failure. The old one performed the PUT and
// returned nil regardless of the status code, so a rejected update looked
// exactly like a successful one.
func (c *AdminClient) SyncOrganizationPlan(ctx context.Context, spec OrganizationSpec) error {
	if spec.OrganizationID <= 0 {
		return fmt.Errorf("gateway admin: organisation id required")
	}
	planID := strings.TrimSpace(spec.PlanID)
	if planID == "" {
		planID = FallbackPlanID
	}
	return c.upsertUser(ctx, GatewayUser{
		ID:     OrganizationUserID(spec.OrganizationID),
		Name:   organizationName(spec),
		Email:  organizationEmail(spec),
		PlanID: planID,
		Status: "active",
	})
}

// ListUserKeys returns the virtual keys the Gateway holds for a user.
//
// The Gateway does not necessarily return the secret for a key it issued
// earlier — most do not, by design — so a listed key is only useful to us when
// it carries one. Callers must handle an empty result as "mint a new one"
// rather than as an error.
func (c *AdminClient) ListUserKeys(ctx context.Context, userID string) ([]GatewayVirtualKey, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("gateway admin: empty user id")
	}
	path := "/api/keys?user_id=" + url.QueryEscape(userID)
	status, raw, err := c.send(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("gateway admin: list keys for %q failed (%d): %s",
			userID, status, truncateBody(raw))
	}

	// The surface has been seen to answer with a bare array and with an
	// envelope. Accepting both costs four lines and saves a support call.
	var keys []GatewayVirtualKey
	if err := json.Unmarshal(raw, &keys); err == nil {
		return keys, nil
	}
	var envelope struct {
		Keys []GatewayVirtualKey `json:"keys"`
		Data []GatewayVirtualKey `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("gateway admin: decode key list: %w", err)
	}
	if len(envelope.Keys) > 0 {
		return envelope.Keys, nil
	}
	return envelope.Data, nil
}

// reusableKey finds an existing key for the user that is active, carries its
// secret, and still authenticates.
func (c *AdminClient) reusableKey(ctx context.Context, userID string) (string, bool) {
	keys, err := c.ListUserKeys(ctx, userID)
	if err != nil {
		return "", false
	}
	for _, k := range keys {
		if k.UserID != "" && k.UserID != userID {
			continue
		}
		if k.Status != "" && !strings.EqualFold(k.Status, "active") {
			continue
		}
		secret := k.Secret()
		if secret == "" {
			continue
		}
		if c.keyState(ctx, secret) == keyValid {
			return secret, true
		}
	}
	return "", false
}

// keyCheck is the outcome of validating a virtual key.
type keyCheck int

const (
	// keyValid means the Gateway accepted it.
	keyValid keyCheck = iota
	// keyRejected means the Gateway answered and refused it.
	keyRejected
	// keyUnknown means the Gateway could not be asked.
	keyUnknown
)

// keyState separates "the Gateway says no" from "the Gateway did not answer".
//
// Collapsing those two is what would make an outage rotate every tenant's
// credentials, so the distinction is load-bearing rather than pedantic.
func (c *AdminClient) keyState(ctx context.Context, key string) keyCheck {
	err := c.ValidateVirtualKey(ctx, key)
	switch {
	case err == nil:
		return keyValid
	case strings.Contains(err.Error(), "rejected"):
		return keyRejected
	default:
		return keyUnknown
	}
}

func organizationName(spec OrganizationSpec) string {
	if name := strings.TrimSpace(spec.Name); name != "" {
		return name
	}
	return fmt.Sprintf("Organization %d", spec.OrganizationID)
}

func organizationEmail(spec OrganizationSpec) string {
	if email := strings.TrimSpace(spec.Email); email != "" {
		return email
	}
	return fmt.Sprintf("org-%d@dawa24.app", spec.OrganizationID)
}
