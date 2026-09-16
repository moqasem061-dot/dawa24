package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// buildVersion is stamped at build time via -ldflags. It makes "which version is
// actually running" answerable from outside the container, which matters most
// during a cutover when two versions briefly coexist.
var (
	buildVersion = "dev"
	buildCommit  = "unknown"
)

type healthHandler struct {
	deps       *dependencies
	ai         gateway.Client
	migrations []database.Migration
	env        string
	log        *slog.Logger
}

type componentStatus struct {
	Status  string `json:"status"` // ok | degraded | down | disabled | connecting
	Detail  string `json:"detail,omitempty"`
	Latency int64  `json:"latency_ms,omitempty"`
}

type statusResponse struct {
	Status     string                     `json:"status"`
	Version    string                     `json:"version"`
	Commit     string                     `json:"commit"`
	Env        string                     `json:"env"`
	Time       string                     `json:"time"`
	Pending    int                        `json:"pending_migrations"`
	Components map[string]componentStatus `json:"components"`
}

// live answers whether the process is alive. It touches no dependencies, so a
// database outage cannot cause the platform to restart a healthy container.
func (h *healthHandler) live(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "dawa24-store",
		"version": buildVersion,
	})
}

// root gives a human hitting the domain something better than a bare 404.
//
// The UI does not exist yet; until it does, this states plainly what is running
// and where to look. It is removed when the Phase 2 templ layout lands.
func (h *healthHandler) root(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"service": "dawa24-store",
		"status":  "foundation only — no marketplace UI yet",
		"version": buildVersion,
		"endpoints": map[string]string{
			"liveness":  "/health",
			"readiness": "/ready",
			"status":    "/api/v1/status",
		},
	})
}

// ready answers whether this instance should receive traffic.
//
// The AI Gateway is excluded from the verdict on purpose. It is an enhancement,
// not a dependency: removing the marketplace from rotation because an LLM
// provider is slow would be a self-inflicted outage.
func (h *healthHandler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	components := map[string]componentStatus{
		"database": h.checkDatabase(ctx),
		"cache":    h.checkCache(ctx),
	}

	pending, migrationStatus := h.checkMigrations(ctx)
	components["migrations"] = migrationStatus

	status, overall := http.StatusOK, "ok"
	for _, c := range components {
		if c.Status == "down" || c.Status == "connecting" {
			status, overall = http.StatusServiceUnavailable, "not_ready"
			break
		}
	}

	if status != http.StatusOK {
		h.log.WarnContext(ctx, "readiness check failed", "components", components)
	}

	httpx.JSON(w, status, statusResponse{
		Status: overall, Version: buildVersion, Commit: buildCommit, Env: h.env,
		Time: time.Now().UTC().Format(time.RFC3339), Pending: pending,
		Components: components,
	})
}

// status is the operator view: readiness plus the Gateway, which is reported but
// never affects the verdict.
func (h *healthHandler) status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	components := map[string]componentStatus{
		"database": h.checkDatabase(ctx),
		"cache":    h.checkCache(ctx),
	}
	pending, migrationStatus := h.checkMigrations(ctx)
	components["migrations"] = migrationStatus
	components["ai_gateway"] = h.checkGateway(ctx)

	overall := "ok"
	for name, c := range components {
		if name == "ai_gateway" {
			continue
		}
		if c.Status != "ok" {
			overall = "degraded"
		}
	}

	httpx.JSON(w, http.StatusOK, statusResponse{
		Status: overall, Version: buildVersion, Commit: buildCommit, Env: h.env,
		Time: time.Now().UTC().Format(time.RFC3339), Pending: pending,
		Components: components,
	})
}

func (h *healthHandler) checkDatabase(ctx context.Context) componentStatus {
	db, err := h.deps.DB()
	if err != nil {
		return connectingOrDown(err)
	}
	start := time.Now()
	if err := db.Health(ctx); err != nil {
		return componentStatus{Status: "down", Detail: err.Error(),
			Latency: time.Since(start).Milliseconds()}
	}

	// A pool at its ceiling answers SELECT 1 perfectly well and is nonetheless
	// the reason every other request is queuing. Reporting it as degraded is
	// what makes the condition findable before it presents as a scattering of
	// "context canceled" with nothing to tie them together.
	stats := db.Stats()
	if stats.Saturated {
		return componentStatus{
			Status: "degraded",
			Detail: fmt.Sprintf("connection pool saturated: %d/%d in use, %d waits so far",
				stats.Acquired, stats.Max, stats.Waiting),
			Latency: time.Since(start).Milliseconds(),
		}
	}
	return componentStatus{Status: "ok", Latency: time.Since(start).Milliseconds()}
}

func (h *healthHandler) checkCache(ctx context.Context) componentStatus {
	c, err := h.deps.Cache()
	if err != nil {
		return connectingOrDown(err)
	}
	start := time.Now()
	if err := c.Health(ctx); err != nil {
		return componentStatus{Status: "down", Detail: err.Error(),
			Latency: time.Since(start).Milliseconds()}
	}
	return componentStatus{Status: "ok", Latency: time.Since(start).Milliseconds()}
}

func (h *healthHandler) checkMigrations(ctx context.Context) (int, componentStatus) {
	db, err := h.deps.DB()
	if err != nil {
		return -1, componentStatus{Status: "down",
			Detail: "cannot check migrations: database unavailable"}
	}

	pending, err := db.PendingCount(ctx, h.migrations)
	if err != nil {
		return -1, componentStatus{Status: "down", Detail: err.Error()}
	}
	if pending > 0 {
		// Refusing traffic here is the safety net for a deploy that skipped the
		// migrate step. Serving against a schema this build does not expect is
		// worse than serving nothing.
		return pending, componentStatus{Status: "down",
			Detail: "pending migrations; run: cli migrate"}
	}
	return 0, componentStatus{Status: "ok"}
}

func (h *healthHandler) checkGateway(ctx context.Context) componentStatus {
	if !h.ai.Enabled() {
		return componentStatus{Status: "disabled",
			Detail: "GATEWAY_ENABLED=false; capabilities serve deterministic fallbacks"}
	}
	start := time.Now()
	if err := h.ai.Health(ctx); err != nil {
		// Degraded, never down: fallbacks cover it.
		return componentStatus{Status: "degraded", Detail: err.Error(),
			Latency: time.Since(start).Milliseconds()}
	}
	return componentStatus{Status: "ok", Latency: time.Since(start).Milliseconds()}
}

// connectingOrDown distinguishes "still dialling after boot" from "dialled and
// failed", so an operator can tell a slow start from a misconfiguration.
func connectingOrDown(err error) componentStatus {
	if _, isNotConnected := err.(notConnected); isNotConnected {
		return componentStatus{Status: "connecting", Detail: "establishing connection"}
	}
	return componentStatus{Status: "down", Detail: err.Error()}
}
