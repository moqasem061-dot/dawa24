package gateway

import (
	"context"
	"sync"
	"time"
)

// Settings are the Gateway credentials and model choices in effect right now.
type Settings struct {
	BaseURL string
	// VirtualKey authenticates /v1 traffic. It is a Bearer token the Gateway
	// issued (sk-virt-…), never the admin credential used for /api management —
	// sending the latter is a 401 and reads to a caller as "AI is broken".
	VirtualKey string
	ClientApp  string
	Enabled    bool
	// FastModel and QualityModel name the models serving each tier. Empty means
	// "use this package's default".
	FastModel    string
	QualityModel string
	// RoleModels maps each Role to its operator-configured model name.
	RoleModels map[string]string
	// RoleDisabled tracks roles explicitly toggled off by the operator.
	RoleDisabled map[string]bool
	// RoleMaxTokens stores max-token overrides per role.
	RoleMaxTokens map[string]int
}

// SettingsSource supplies live credentials. The admin panel writes them to the
// database; the process environment is the fallback so a fresh deployment boots
// before an operator has visited the settings screen.
//
// It is an interface because platform/ must not import modules/ (ADR 0002) —
// the composition root injects an implementation backed by platform_admin.
type SettingsSource interface {
	GatewaySettings(ctx context.Context) (Settings, error)
}

type cachedSettings struct {
	settings  Settings
	expiresAt time.Time
}

type settingsCache struct {
	mu     sync.RWMutex
	cached cachedSettings
	valid  bool
	ttl    time.Duration
}

func newSettingsCache(ttl time.Duration) *settingsCache {
	return &settingsCache{
		ttl: ttl,
	}
}

func (c *settingsCache) get() (Settings, bool) {
	if c == nil {
		return Settings{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.valid || time.Now().After(c.cached.expiresAt) {
		return Settings{}, false
	}
	return c.cached.settings, true
}

func (c *settingsCache) set(s Settings) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cached = cachedSettings{
		settings:  s,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.valid = true
}

// WithSettingsSource configures an optional dynamic SettingsSource on the client.
func (c *HTTPClient) WithSettingsSource(src SettingsSource) *HTTPClient {
	c.source = src
	return c
}

// resolve returns the credentials in effect, preferring the operator's saved
// settings over the boot-time environment. Cached briefly (30s) to avoid a
// settings read per streamed token.
func (c *HTTPClient) resolve(ctx context.Context) Settings {
	if cached, ok := c.cache.get(); ok {
		return cached
	}

	if c.source != nil {
		if s, err := c.source.GatewaySettings(ctx); err == nil {
			if s.Enabled {
				if s.ClientApp == "" {
					s.ClientApp = c.cfg.ClientApp
				}
				if s.BaseURL == "" {
					s.BaseURL = c.cfg.BaseURL
				}
				c.cache.set(s)
				c.logSourceChange("admin_settings")
				return s
			}
		}
	}

	envSettings := Settings{
		BaseURL:    c.cfg.BaseURL,
		VirtualKey: c.cfg.VirtualKey,
		ClientApp:  c.cfg.ClientApp,
		Enabled:    c.cfg.Enabled,
	}
	c.cache.set(envSettings)
	c.logSourceChange("environment")
	return envSettings
}

func (c *HTTPClient) logSourceChange(source string) {
	c.sourceMu.Lock()
	defer c.sourceMu.Unlock()
	if c.lastSource != source {
		c.lastSource = source
		c.log.Info("gateway credentials source updated", "source", source)
	}
}
