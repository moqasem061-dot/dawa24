package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"

	"github.com/go-chi/chi/v5"

	attachmentsPostgres "github.com/muhiya/dawa24-store/internal/modules/attachments/postgres"
	identityPostgres "github.com/muhiya/dawa24-store/internal/modules/identity/postgres"
	orgPostgres "github.com/muhiya/dawa24-store/internal/modules/org/postgres"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	attachmentsHttp "github.com/muhiya/dawa24-store/internal/modules/attachments/http"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	orgHttp "github.com/muhiya/dawa24-store/internal/modules/org/http"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/storage"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/redis/go-redis/v9"
)

// mountModuleRoutes registers domain handlers across all platform bounded contexts.
func mountModuleRoutesAPI(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	deps *dependencies,
	ai gateway.Client,
	adminKeys *adminKeyProvisioner,
	tenantKeys *tenantKeyProvisioner,
) (*database.DB, *identity.Service, *attachments.Service, func(ctx context.Context, orgID int64, orgType string) error, *storage.Client, *rbac.Resolver) {
	db := deps.Handle()

	// Initialize dynamic feature flags engine
	if _, err := features.Init(deps.Context(), db, log); err != nil {
		log.Warn("failed to initialize features engine", "error", err)
	}

	// 1. Storage & Attachments
	var storageClient *storage.Client
	if sClient, err := storage.New(context.Background(), cfg.Storage); err == nil {
		storageClient = sClient
		log.Info("object storage client initialized", "bucket", cfg.Storage.Bucket)
	} else {
		log.Warn("object storage client not initialized", "error", err)
	}

	attachRepo := attachmentsPostgres.NewRepository(db)
	attachSvc := attachments.NewService(attachRepo, storageClient, log)

	// §4.2 documents gate: an organization with missing mandatory documents
	// cannot check out (customer) or publish offers (vendor). Composed here
	// because modules must not import each other. Fail closed: if the
	// documents service errors, trading is refused until it recovers.
	docsGate := func(ctx context.Context, orgID int64, orgType string) error {
		if orgID <= 0 {
			return nil
		}
		missing, err := attachSvc.MissingRequiredDocuments(ctx, orgID, orgType)
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			return apperr.Validation("documents.incomplete", "Organization must attach its mandatory documents before trading.", nil)
		}
		return nil
	}

	// 2. Identity & Auth
	idRepo := identityPostgres.NewRepository(db)
	sessionStore := identity.NewSessionStore(deps.CacheHandle(), cfg.Session)
	idSvc := identity.NewService(idRepo, sessionStore, log)

	// The permission resolver is shared by every gate in the process. It reads
	// a caller's effective permissions from the database and caches them
	// against identity.rbac_version, so a role change is visible to every
	// process within seconds rather than at the end of a 720-hour session.
	permissions := rbac.NewResolver(db)
	idSvc.SetPermissionResolver(permissions)

	identityHandler := identityHttp.NewHandler(idSvc, cfg.Session, log)
	identityHandler.SetTrustedProxyHops(cfg.HTTP.TrustedProxyHops)
	identityHandler.SetResolver(permissions)
	identityHandler.SetLimiter(httpx.NewLazyLimiter(func() *redis.Client {
		return deps.CacheHandle().Redis()
	}, "dawa24:ratelimit:auth:"))
	identityHandler.RegisterRoutes(r)

	// CSP violation reporting endpoint: collects browser-reported policy blocks,
	// rate-limited per IP to prevent flood attacks.
	cspLimiter := httpx.NewLazyLimiter(func() *redis.Client {
		if deps != nil && deps.CacheHandle() != nil {
			return deps.CacheHandle().Redis()
		}
		return nil
	}, "dawa24:ratelimit:csp:").LimitByIP(30, time.Minute)
	// The collector usually sits on its own origin (CSP_REPORT_URL), so it
	// answers the Reporting API's CORS preflight for the site's origin.
	cspReports := httpx.CSPReportHandler(log, cfg.SiteOrigin())
	r.With(cspLimiter).Post(httpx.CSPReportPath, cspReports)
	r.With(cspLimiter).Options(httpx.CSPReportPath, cspReports)

	// Dynamic API rate limiting: 240 req/min for authenticated users, 60 req/min for unauthenticated callers.
	// Calibrated so fast-paced pharmacists and high-frequency UI actions never encounter limits,
	// while immediately halting spam bots, scrape scripts, and abusive bursts.
	apiLimiter := httpx.NewLazyLimiter(func() *redis.Client {
		return deps.CacheHandle().Redis()
	}, "dawa24:ratelimit:api:")
	apiLimiter.SetUserExtractor(func(req *http.Request) int64 {
		if actor, ok := authctx.From(req.Context()); ok && actor.UserID > 0 {
			return actor.UserID
		}
		if uid, err := authctx.UserID(req.Context()); err == nil && uid > 0 {
			return uid
		}
		return 0
	})
	apiRateLimit := apiLimiter.LimitUserOrIP(240, 60, time.Minute)

	// Authenticated API routes — Pre-approval / Onboarding allowlist
	// (Document upload and own organisation status queries needed to achieve approval)
	r.Group(func(preApproval chi.Router) {
		preApproval.Use(apiRateLimit)
		preApproval.Use(httpx.CSRF(cfg.Env.IsProd()))
		preApproval.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		preApproval.Use(identityHttp.ResolveTenant(idSvc, log))

		// Attachments API (document uploads and confirmation for verification)
		attachmentsHttp.NewHandler(attachSvc, log).RegisterRoutes(preApproval)

		// Organization registration & own org query during onboarding
		orgRepoAPI := orgPostgres.NewRepository(db)
		orgSvcAPI := org.NewService(orgRepoAPI, log)
		orgHttp.NewHandler(orgSvcAPI, log).RegisterPreApprovalRoutes(preApproval)
	})

	// Authenticated API routes — Approved organizations only
	r.Group(func(approved chi.Router) {
		approved.Use(apiRateLimit)
		approved.Use(httpx.CSRF(cfg.Env.IsProd()))
		approved.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		approved.Use(identityHttp.ResolveTenant(idSvc, log))
		approved.Use(authctx.RequireApproved(log))

		mountAuthenticatedModules(approved, cfg, log, deps, ai, adminKeys, tenantKeys, storageClient, docsGate)
	})

	return db, idSvc, attachSvc, docsGate, storageClient, permissions
}
