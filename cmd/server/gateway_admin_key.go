package main

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// The admin panel's own Gateway identity.
//
// Everything the platform does with AI that is not on a tenant's behalf — the
// catalogue import's enrichment, the admin assistant — needs a key. Until now
// the runtime sent the administrator credential from إعدادات النظام as the
// Bearer token, which the Gateway rejects: that credential is basic auth for
// the /api management surface, and /v1 wants a virtual key. Every AI call was a
// 401, and because the import falls back silently on any Gateway error, the
// feature looked switched on and did nothing.
//
// This closes the loop. The credential an operator does have is used the way it
// is meant to be used — to register a user and mint a key — and the key it
// produces is what the runtime then sends.

// adminKeyProvisioner resolves, caches, and repairs the admin panel's key.
type adminKeyProvisioner struct {
	svc *platformadmin.Service
	log *slog.Logger

	mu       sync.Mutex
	cached   string
	cachedAt time.Time
	// lastFailure throttles retries so an unreachable Gateway is not dialled on
	// every request; the import screen asks for this key often.
	lastFailure time.Time
}

// adminKeyTTL is how long a resolved key is trusted without re-reading
// settings. Short enough that an operator changing credentials sees the effect
// promptly, long enough that a nine-thousand-row import does not re-read them
// per batch.
const adminKeyTTL = 5 * time.Minute

// adminKeyRetryAfter is the quiet period after a failed provisioning attempt.
const adminKeyRetryAfter = 60 * time.Second

func newAdminKeyProvisioner(svc *platformadmin.Service, log *slog.Logger) *adminKeyProvisioner {
	return &adminKeyProvisioner{svc: svc, log: log.With("component", "gateway_admin_key")}
}

// Key returns the admin panel's virtual key, provisioning one if needed.
//
// It returns an empty string rather than an error when the Gateway cannot
// supply a key: every caller's contract is to fall back to its deterministic
// path, and an error here would only be translated into that same decision one
// layer up.
func (p *adminKeyProvisioner) Key(ctx context.Context) string {
	if p == nil || p.svc == nil {
		return ""
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cached != "" && time.Since(p.cachedAt) < adminKeyTTL {
		return p.cached
	}

	sysCtx := database.AsSystem(ctx)
	settings, err := p.svc.GetGatewaySettings(sysCtx)
	if err != nil || settings == nil {
		p.log.WarnContext(ctx, "gateway settings unavailable", "error", err)
		return ""
	}
	if !settings.IsActive {
		p.cached, p.cachedAt = "", time.Now()
		return ""
	}

	// A stored key is trusted only after it is checked. Issuing a key for a user
	// revokes the previous one, so a second instance booting — or a re-run of
	// provisioning — leaves whatever is stored here silently dead, and every AI
	// call fails with "invalid or revoked virtual key" until somebody notices.
	if settings.VirtualKey != "" {
		if p.keyWorks(sysCtx, settings) {
			p.cached, p.cachedAt = settings.VirtualKey, time.Now()
			return p.cached
		}
		p.log.WarnContext(ctx, "stored gateway key no longer valid, re-provisioning",
			"gateway_user", settings.AIUserID)
		settings.VirtualKey = ""
	}

	if !settings.CanProvision() {
		p.log.WarnContext(ctx, "cannot provision admin gateway key: no administrator credentials configured")
		return ""
	}
	if time.Since(p.lastFailure) < adminKeyRetryAfter {
		return ""
	}

	key, err := p.provision(sysCtx, settings)
	if err != nil {
		p.lastFailure = time.Now()
		p.log.ErrorContext(ctx, "could not provision admin gateway key",
			"endpoint", settings.EndpointURL, "error", err)
		return ""
	}

	p.cached, p.cachedAt = key, time.Now()
	p.lastFailure = time.Time{}
	return key
}

// provision registers the admin panel with the Gateway and stores the key it
// issues, so the next boot does not have to ask again.
func (p *adminKeyProvisioner) provision(ctx context.Context, settings *platformadmin.GatewaySettings) (string, error) {
	username, password := settings.AdminCredentials()
	client := gateway.NewAdminClient(settings.EndpointURL, username, password)

	// Fail on bad credentials here rather than after creating half an identity,
	// so the log names the real problem.
	if err := client.Ping(ctx); err != nil {
		return "", err
	}

	userID, key, err := client.ProvisionAdminPanel(ctx, settings.AIPlanID)
	if err != nil {
		return "", err
	}

	settings.VirtualKey = key
	settings.AIUserID = userID
	if err := p.svc.SaveGatewaySettings(ctx, settings); err != nil {
		// The key works even if it could not be stored; the next call will
		// simply provision another one.
		p.log.WarnContext(ctx, "provisioned admin gateway key but could not persist it", "error", err)
	}

	p.log.InfoContext(ctx, "provisioned admin panel gateway identity",
		"gateway_user", userID, "endpoint", settings.EndpointURL)
	return key, nil
}

// keyWorks checks a stored key against the Gateway, treating an unreachable
// Gateway as "assume it still works": a transient outage must not cause a
// pointless re-provision that revokes a key which was fine.
func (p *adminKeyProvisioner) keyWorks(ctx context.Context, settings *platformadmin.GatewaySettings) bool {
	if !settings.CanProvision() {
		return true
	}
	username, password := settings.AdminCredentials()
	client := gateway.NewAdminClient(settings.EndpointURL, username, password)

	err := client.ValidateVirtualKey(ctx, settings.VirtualKey)
	if err == nil {
		return true
	}
	return !strings.Contains(err.Error(), "rejected")
}

// Invalidate drops the cached key, so the next call re-reads settings. The
// settings screen calls it after an operator saves new credentials.
func (p *adminKeyProvisioner) Invalidate() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cached, p.cachedAt, p.lastFailure = "", time.Time{}, time.Time{}
}
