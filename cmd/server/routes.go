package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"

	"github.com/go-chi/chi/v5"

	billingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	catalogPostgres "github.com/moqasem061-dot/dawa24/internal/modules/catalog/postgres"
	commercePostgres "github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	hrPostgres "github.com/moqasem061-dot/dawa24/internal/modules/hr/postgres"
	ingestPostgres "github.com/moqasem061-dot/dawa24/internal/modules/ingest/postgres"
	inventoryPostgres "github.com/moqasem061-dot/dawa24/internal/modules/inventory/postgres"
	notificationsPostgres "github.com/moqasem061-dot/dawa24/internal/modules/notifications/postgres"
	orgPostgres "github.com/moqasem061-dot/dawa24/internal/modules/org/postgres"
	platformadminPostgres "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin/postgres"
	promoPostgres "github.com/moqasem061-dot/dawa24/internal/modules/promo/postgres"
	workflowPostgres "github.com/moqasem061-dot/dawa24/internal/modules/workflow/postgres"

	"github.com/moqasem061-dot/dawa24/internal/modules/assistant"
	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	billingHttp "github.com/moqasem061-dot/dawa24/internal/modules/billing/http"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	catalogHttp "github.com/moqasem061-dot/dawa24/internal/modules/catalog/http"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	commerceHttp "github.com/moqasem061-dot/dawa24/internal/modules/commerce/http"
	"github.com/moqasem061-dot/dawa24/internal/modules/hr"
	hrHttp "github.com/moqasem061-dot/dawa24/internal/modules/hr/http"
	identityHttp "github.com/moqasem061-dot/dawa24/internal/modules/identity/http"
	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	ingestHttp "github.com/moqasem061-dot/dawa24/internal/modules/ingest/http"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	inventoryHttp "github.com/moqasem061-dot/dawa24/internal/modules/inventory/http"
	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
	notificationsHttp "github.com/moqasem061-dot/dawa24/internal/modules/notifications/http"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	orgHttp "github.com/moqasem061-dot/dawa24/internal/modules/org/http"
	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	platformadminHttp "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin/http"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	promoHttp "github.com/moqasem061-dot/dawa24/internal/modules/promo/http"
	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	smartorderHttp "github.com/moqasem061-dot/dawa24/internal/modules/smartorder/http"
	smartorderPG "github.com/moqasem061-dot/dawa24/internal/modules/smartorder/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/workflow"
	workflowHttp "github.com/moqasem061-dot/dawa24/internal/modules/workflow/http"

	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/platform/storage"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/redis/go-redis/v9"
)

func mountModuleRoutes(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	deps *dependencies,
	ai gateway.Client,
	adminKeys *adminKeyProvisioner,
	tenantKeys *tenantKeyProvisioner,
) {
	// Created before the assistant is mounted, which binds itself to it.
	deps.capsule = newCapsuleBridge(cfg.BaseURL)
	deps.actions = &lateActions{}
	deps.orders = &lateOrderAnnouncer{}

	db, idSvc, attachSvc, docsGate, storageClient, permissions := mountModuleRoutesAPI(r, cfg, log, deps, ai, adminKeys, tenantKeys)

	uiHandler := buildUIHandler(cfg, log, deps, db, idSvc, attachSvc, docsGate, tenantKeys, adminKeys, storageClient, ai, permissions)
	deps.orders.bind(uiHandler.AnnounceOrderPlaced)
	uiHandler.SetLimiter(httpx.NewLazyLimiter(func() *redis.Client {
		return deps.CacheHandle().Redis()
	}, "dawa24:ratelimit:auth:"))

	// Telegram, WhatsApp, and Marketing bridges: machine-to-machine routes for n8n, mounted on the root
	// router so no session, CSRF or tenant middleware applies to them.
	// Rate-limited by IP to throttle unauthenticated floods and brute-force attempts.
	bridgeLimiter := httpx.NewLazyLimiter(func() *redis.Client {
		if deps != nil && deps.CacheHandle() != nil {
			return deps.CacheHandle().Redis()
		}
		return nil
	}, "dawa24:ratelimit:bridge:").LimitByIP(300, time.Minute)

	bridgeRouter := r.With(bridgeLimiter)
	uiHandler.SetTelegram(mountTelegram(bridgeRouter, cfg, log, db, permissions, deps.capsule))
	uiHandler.SetWhatsApp(mountWhatsApp(bridgeRouter, cfg, log, db, permissions, deps.capsule))
	mountMarketing(bridgeRouter, cfg, log, db)
	deps.actions.bind(uiHandler.AssistantActions())

	if cfg.Session.CookieName != "" {
		httpx.SessionCookieName = cfg.Session.CookieName
	}

	uiHandler.RegisterPublicRoutes(r)

	isProd := cfg.Env.IsProd()

	// baseUIMiddleware returns the middleware stack shared by every
	// authenticated UI route group: CSRF, session auth, tenant resolution,
	// and site-wide settings. Defined once to avoid the maintenance trap of
	// replicating changes across eight route groups.
	baseUIMiddleware := func(router chi.Router) {
		router.Use(httpx.CSRF(isProd))
		router.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		router.Use(identityHttp.ResolveTenant(idSvc, log))
		router.Use(uiHandler.SiteSettingsMiddleware)
	}

	r.Group(func(uiRouter chi.Router) {
		baseUIMiddleware(uiRouter)
		uiRouter.Use(authctx.RequireCustomer(log))
		uiRouter.Use(authctx.RequireApproved(log))
		uiRouter.Use(uiHandler.BuyingBranchSelector)
		uiHandler.RegisterCustomerRoutes(uiRouter)
	})
	// The shared buying surface: the catalogue, the supplier directory, the
	// offers board, the cart, the checkout, the buyer's own orders and Smart
	// Ordering. A pharmacy and a supplier reach the same pages here; which of
	// them each may open is decided per page by the buying capability, against
	// the caller's own dashboard.
	r.Group(func(uiRouter chi.Router) {
		baseUIMiddleware(uiRouter)
		uiRouter.Use(authctx.RequireBuyer(log))
		uiRouter.Use(authctx.RequireApproved(log))
		uiRouter.Use(uiHandler.BuyingBranchSelector)
		uiHandler.RegisterBuyingRoutes(uiRouter)
		uiHandler.RegisterSmartOrderRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		baseUIMiddleware(uiRouter)
		uiRouter.Use(authctx.RequireVendor(log))
		uiRouter.Use(authctx.RequireApproved(log))
		uiRouter.Use(uiHandler.BuyingBranchSelector)
		uiHandler.RegisterVendorRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		baseUIMiddleware(uiRouter)
		uiRouter.Use(authctx.RequireStaff(log))
		uiHandler.RegisterAdminRoutes(uiRouter)
	})
	// Tier A: Pre-approval shared routes (authenticated only, no RequireApproved)
	r.Group(func(uiRouter chi.Router) {
		baseUIMiddleware(uiRouter)
		uiHandler.RegisterPreApprovalRoutes(uiRouter)
	})
	// Tier B: Approved-only shared routes (RequireApproved mounted)
	r.Group(func(uiRouter chi.Router) {
		baseUIMiddleware(uiRouter)
		uiRouter.Use(authctx.RequireApproved(log))
		uiHandler.RegisterApprovedSharedRoutes(uiRouter)
	})
	// Tier C: Audience-specific customer shared routes
	r.Group(func(uiRouter chi.Router) {
		baseUIMiddleware(uiRouter)
		uiRouter.Use(authctx.RequireCustomer(log))
		uiHandler.RegisterCustomerSharedRoutes(uiRouter)
	})
	// Tier C: Audience-specific vendor shared routes
	r.Group(func(uiRouter chi.Router) {
		baseUIMiddleware(uiRouter)
		uiRouter.Use(authctx.RequireVendor(log))
		uiHandler.RegisterVendorSharedRoutes(uiRouter)
	})

}

// mountAuthenticatedModules registers every module whose endpoints require a logged-in caller.
func mountAuthenticatedModules(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	deps *dependencies,
	ai gateway.Client,
	adminKeys *adminKeyProvisioner,
	tenantKeys *tenantKeyProvisioner,
	storageClient *storage.Client,
	docsGate func(ctx context.Context, orgID int64, orgType string) error,
) {
	db := deps.Handle()

	// moduleAPI mounts one legacy module API only when enabled; see
	// config.HTTP.ModuleAPI. The services are built either way, because the
	// assistant and smart ordering depend on them.
	moduleAPI := func(mount func()) {
		if cfg.HTTP.ModuleAPI {
			mount()
		}
	}

	// 2. Catalog
	catRepo := catalogPostgres.NewRepository(db)
	catSvc := catalog.NewService(catRepo, log)
	if cacheHandle := deps.CacheHandle(); cacheHandle != nil {
		catSvc.SetCache(cacheHandle)
	}
	moduleAPI(func() { catalogHttp.NewHandler(catSvc, log).RegisterRoutes(r) })

	// 3. Inventory
	invRepo := inventoryPostgres.NewRepository(db)
	invSvc := inventory.NewService(invRepo, log)
	moduleAPI(func() { inventoryHttp.NewHandler(invSvc, log).RegisterRoutes(r) })

	// 4. Commerce
	commRepo := commercePostgres.NewRepository(db)
	commSvc := commerce.NewService(commRepo, log)
	commSvc.SetRequiredDocsChecker(commerce.RequiredDocsChecker(docsGate))
	commSvc.OnOrderPlaced(deps.orders.announce)
	// The JSON API must enforce the same availability rules as the HTML surface,
	// otherwise it is a way around them.
	commSvc.SetAvailabilityProbe(newAvailabilityProbe(
		catalog.NewService(catalogPostgres.NewRepository(db), log),
		org.NewService(orgPostgres.NewRepository(db), log),
		workflow.NewCoverageService(db),
		inventory.NewService(inventoryPostgres.NewRepository(db), log),
	))
	moduleAPI(func() { commerceHttp.NewHandler(commSvc, log).RegisterRoutes(r) })

	// 5. Billing & Entitlements
	billRepo := billingPostgres.NewRepository(db)
	billSvc := billing.NewService(billRepo, log)
	billSvc.SetAIPlanSync(tenantKeys)
	moduleAPI(func() { billingHttp.NewHandler(billSvc, log).RegisterRoutes(r) })

	// 6. Ingest & AI Matching
	ingRepo := ingestPostgres.NewRepository(db)
	ingSvc := ingest.NewService(ingRepo, log)
	if storageClient != nil {
		ingSvc.SetStorage(storageClient)
	}
	moduleAPI(func() { ingestHttp.NewHandler(ingSvc, log).RegisterRoutes(r) })

	// 7. Promo, Offers & Ads
	promoRepo := promoPostgres.NewRepository(db)
	promoSvc := promo.NewService(promoRepo, log)
	promoSvc.SetRequiredDocsChecker(promo.RequiredDocsChecker(docsGate))
	// Wallet debiter: charges the vendor's wallet for a sponsorship package
	// purchase. Composed here because promo must not import billing (ADR 0002).
	billSvcForPromo := billing.NewService(billRepo, log)
	promoSvc.SetWalletDebiter(func(ctx context.Context, orgID int64, amount money.Amount, description string) (*int64, error) {
		// Find the wallet for the org's first member (owner).
		// The billing service's GetOrCreateWallet is user-scoped; we resolve
		// the org's owner user from the tenant context.
		uid, _ := authctx.UserID(ctx)
		if uid <= 0 {
			return nil, apperr.Validation("auth.required", i18n.TDefault("w4_cmd.w4str_263_263"), nil)
		}
		wallet, err := billSvcForPromo.GetWallet(ctx, uid, "EGP")
		if err != nil {
			return nil, err
		}
		if wallet.Available().Minor() < amount.Minor() {
			return nil, apperr.Conflict("wallet.insufficient_funds", i18n.TDefault("w4_cmd.w4str_264_264"))
		}
		tx, err := billSvcForPromo.Withdraw(ctx, uid, "EGP", amount, "sponsorship_package", nil, description)
		if err != nil {
			return nil, err
		}
		txID := tx.ID
		return &txID, nil
	})
	promoHandler := promoHttp.NewHandler(promoSvc, log)
	promoHandler.SetTrustedProxyHops(cfg.HTTP.TrustedProxyHops)
	moduleAPI(func() { promoHandler.RegisterRoutes(r) })

	// 8. Workflow
	wfRepo := workflowPostgres.NewRepository(db)
	wfSvc := workflow.NewService(wfRepo, log)
	moduleAPI(func() { workflowHttp.NewHandler(wfSvc, log).RegisterRoutes(r) })

	// 9. HR
	hrRepo := hrPostgres.NewRepository(db)
	hrSvc := hr.NewService(hrRepo, log)
	moduleAPI(func() { hrHttp.NewHandler(hrSvc, log).RegisterRoutes(r) })

	// 10. Platform Admin
	paRepo := platformadminPostgres.NewRepository(db)
	paSvc := platformadmin.NewService(paRepo, log)
	moduleAPI(func() { platformadminHttp.NewHandler(paSvc, log).RegisterRoutes(r) })

	// 11. Notifications
	notifRepo := notificationsPostgres.NewRepository(db)
	notifSvc := notifications.NewService(notifRepo, log)
	moduleAPI(func() { notificationsHttp.NewHandler(notifSvc, log).RegisterRoutes(r) })

	// 12. Organizations & Tenants
	orgRepo := orgPostgres.NewRepository(db)
	orgSvc := org.NewService(orgRepo, log)
	moduleAPI(func() { orgHttp.NewHandler(orgSvc, log).RegisterApprovedRoutes(r) })

	// Every employee of a منشأة spends against that منشأة's own Gateway key.
	//
	// The four hand-rolled copies of this — two here, one in the dashboard
	// handler, one in the admin handler — each provisioned inline on the request
	// path and each minted a fresh key every time, which revoked the one before
	// it. tenantKeyProvisioner does the same job once, with a per-organisation
	// lock, a validated cache, and a plan that follows the subscription.
	keyResolverAPI := func(ctx context.Context, orgID int64) (string, error) {
		return tenantKeys.Key(ctx, orgID), nil
	}

	// 13. Assistant (كبسولة)
	mountAssistant(r, assistantDeps{
		db:      db,
		cfg:     cfg,
		log:     log,
		ai:      ai,
		cacheH:  deps.CacheHandle(),
		storage: storageClient,
		admin:   platformadmin.NewService(platformadminPostgres.NewRepository(db), log),
		keys:    assistant.KeyResolver(keyResolverAPI),
		bridge:  deps.capsule,
		actions: deps.actions,
		// The assistant answers coverage questions through the same service the
		// catalogue and checkout resolve them with, never a second copy.
		coverage: &assistantCoverageProbe{
			coverage: workflow.NewCoverageService(db),
			workflow: wfSvc,
			orgs:     orgSvc,
		},
	})

	// 14. Smart Order API
	smartorderRepo := smartorderPG.New(db)
	smartorderSvc := smartorder.NewService(smartorderRepo, log)
	// The same purchase rule the UI service uses. Without it this service's
	// match correction had no coverage check at all.
	smartorderSvc.SetAvailabilityGate(newCommerceAvailabilityGate(commSvc))
	soHandler := smartorderHttp.NewHandler(smartorderSvc, log)
	soFinalizer := smartorder.NewFinalizer(
		smartorderRepo,
		placeSmartOrder(commSvc, orgSvc, workflow.NewCoverageService(db), log),
		&reverifier{gate: newCommerceAvailabilityGate(commSvc)},
	)
	smartorderHttp.RegisterRoutes(r, soHandler, smartorderHttp.NewReviewer(soHandler, smartorderSvc, soFinalizer))

	// The institutional gate resolves which institutional work IDs a user
	// may access. Defined once and cast to each module's adapter type
	// (modules cannot import each other, so each defines its own GateFunc).
	allowedWorkIDs := func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return orgSvc.AllowedWorkIDs(ctx, userID, org.InstitutionalFilterMode(mode))
	}
	catSvc.SetInstitutionalGate(catalog.InstitutionalGateFunc(allowedWorkIDs))
	wfSvc.SetInstitutionalGate(workflow.InstitutionalGateFunc(allowedWorkIDs))
}
