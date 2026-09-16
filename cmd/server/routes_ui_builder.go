package main

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"

	assistantPostgres "github.com/muhiya/dawa24-store/internal/modules/assistant/postgres"
	billingPostgres "github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	catalogPostgres "github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	chatPostgres "github.com/muhiya/dawa24-store/internal/modules/chat/postgres"
	commercePostgres "github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
	comparePostgres "github.com/muhiya/dawa24-store/internal/modules/compare/postgres"
	hrPostgres "github.com/muhiya/dawa24-store/internal/modules/hr/postgres"
	ingestPostgres "github.com/muhiya/dawa24-store/internal/modules/ingest/postgres"
	inventoryPostgres "github.com/muhiya/dawa24-store/internal/modules/inventory/postgres"
	notificationsPostgres "github.com/muhiya/dawa24-store/internal/modules/notifications/postgres"
	orgPostgres "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	platformadminPostgres "github.com/muhiya/dawa24-store/internal/modules/platform_admin/postgres"
	promoPostgres "github.com/muhiya/dawa24-store/internal/modules/promo/postgres"
	workflowPostgres "github.com/muhiya/dawa24-store/internal/modules/workflow/postgres"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	cataloggw "github.com/muhiya/dawa24-store/internal/modules/catalog/gateway"
	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"

	aiusagePostgres "github.com/muhiya/dawa24-store/internal/platform/aiusage/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/antiscrape"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/mailer"
	"github.com/muhiya/dawa24-store/internal/platform/pagecontrol"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/platform/telegramgateway"
	"github.com/muhiya/dawa24-store/internal/ui"

	"github.com/redis/go-redis/v9"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/progress"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func buildUIHandler(
	cfg *config.Config,
	log *slog.Logger,
	deps *dependencies,
	db *database.DB,
	idSvc *identity.Service,
	attachSvc *attachments.Service,
	docsGate func(ctx context.Context, orgID int64, orgType string) error,
	tenantKeys *tenantKeyProvisioner,
	adminKeys *adminKeyProvisioner,
	storageClient *storage.Client,
	ai gateway.Client,
	permissions *rbac.Resolver,
) *ui.UIHandler {
	catRepoUI := catalogPostgres.NewRepository(db)
	orgRepoUI := orgPostgres.NewRepository(db)
	ingRepoUI := ingestPostgres.NewRepository(db)
	commRepoUI := commercePostgres.NewRepository(db)
	invRepoUI := inventoryPostgres.NewRepository(db)
	notifRepoUI := notificationsPostgres.NewRepository(db)
	promoRepoUI := promoPostgres.NewRepository(db)
	adminRepoUI := platformadminPostgres.NewRepository(db)
	billRepoUI := billingPostgres.NewRepository(db)
	chatRepoUI := chatPostgres.NewRepository(db)
	wfRepoUI := workflowPostgres.NewRepository(db)
	hrRepoUI := hrPostgres.NewRepository(db)

	orgSvcUI := org.NewService(orgRepoUI, log)

	// Live progress, established before the import stores are wired because
	// each of them is decorated with it.
	//
	// One hub per process, fed locally by whatever is doing the work and across
	// processes by Redis. Every import store below publishes through it, so a
	// screen watching a run is written to when a number moves instead of asking
	// twice a second for one that has not.
	progressHub := progress.NewHub()
	// The Redis handle is passed as a RESOLVER, not a client: this runs before
	// Redis has been dialled, so a client read here would be nil for the life of
	// the process and every cross-process message would vanish silently.
	redisFor := func() *redis.Client { return redisHandle(deps) }
	uiProgress := progress.NewPublisher(redisFor, progressHub, log)
	go progress.Bridge(deps.Context(), redisFor, progressHub, log)

	instGate := catalog.InstitutionalGateFunc(func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return orgSvcUI.AllowedWorkIDs(ctx, userID, org.InstitutionalFilterMode(mode))
	})

	catSvcUI := catalog.NewService(catRepoUI, log)
	catSvcUI.SetInstitutionalGate(instGate)
	// The staged catalogue import: the admin reviews a parsed file before any of
	// it is written. Without a store the wizard reports itself unavailable
	// rather than falling back to writing blind.
	catSvcUI.SetImportStore(catalog.WithProgressNotifications(catRepoUI, uiProgress))
	// The row is written once per phase; the bar is fed every batch.
	catSvcUI.SetProgressNotifier(func(publicID string, p catalog.ImportProgress) {
		uiProgress.Publish(context.Background(), progress.Snapshot{
			ID:      publicID,
			Percent: p.Percent(),
			Message: p.Message,
			Current: p.Current,
			Total:   p.Total,
			State:   string(catalog.SessionProcessing),
		})
	})
	if cacheHandle := deps.CacheHandle(); cacheHandle != nil {
		catSvcUI.SetCache(cacheHandle)
	}

	commSvcUI := commerce.NewService(commRepoUI, log)
	commSvcUI.SetRequiredDocsChecker(docsGate)
	// §1.2 availability gate: stock, supplier approval, branch ownership and
	// weekly coverage are checked in one place for every buying surface.
	// Composed here because commerce must not import catalog/org/workflow.
	uiAvailability := newAvailabilityProbe(
		catalog.NewService(catRepoUI, log),
		org.NewService(orgRepoUI, log),
		workflow.NewCoverageService(db),
		inventory.NewService(invRepoUI, log),
	)
	commSvcUI.SetAvailabilityProbe(uiAvailability)

	promoSvcUI := promo.NewService(promoRepoUI, log)
	promoSvcUI.SetRequiredDocsChecker(docsGate)
	// Wallet debiter for UI-driven sponsorship purchases.
	billSvcUIForPromo := billing.NewService(billRepoUI, log)
	promoSvcUI.SetWalletDebiter(func(ctx context.Context, orgID int64, amount money.Amount, description string) (*int64, error) {
		uid, _ := authctx.UserID(ctx)
		if uid <= 0 {
			return nil, apperr.Validation("auth.required", i18n.TDefault("w4_cmd.w4str_263_263"), nil)
		}
		wallet, err := billSvcUIForPromo.GetWallet(ctx, uid, "EGP")
		if err != nil {
			return nil, err
		}
		if wallet.Available().Minor() < amount.Minor() {
			return nil, apperr.Conflict("wallet.insufficient_funds", i18n.TDefault("w4_cmd.w4str_264_264"))
		}
		tx, err := billSvcUIForPromo.Withdraw(ctx, uid, "EGP", amount, "sponsorship_package", nil, description)
		if err != nil {
			return nil, err
		}
		txID := tx.ID
		return &txID, nil
	})

	wfSvcUI := workflow.NewService(wfRepoUI, log)
	wfSvcUI.SetInstitutionalGate(workflow.InstitutionalGateFunc(func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return orgSvcUI.AllowedWorkIDs(ctx, userID, org.InstitutionalFilterMode(mode))
	}))

	invSvcUI := inventory.NewService(invRepoUI, log)

	ingSvcUI := ingest.NewService(ingRepoUI, log)
	if storageClient != nil {
		ingSvcUI.SetStorage(storageClient)
	}
	// The rebuilt vendor catalogue import. All three ports are required for the
	// screen to be offered: without the store there is nowhere to hold a review,
	// and without the catalogue there is nothing to match against. Wiring them
	// here keeps the module free of the composition root.
	ingSvcUI.SetImportStore(ingest.WithProgressNotifications(ingRepoUI, uiProgress))
	// The decision cache the AI stage reads before it asks anything. Shared
	// with the smart order, on purpose: both ask the same question through the
	// same prompt, so an answer either bought is free to the other.
	ingSvcUI.SetMatchMemory(ingRepoUI)
	// The same cache, in the shared vocabulary, for the two import paths that
	// had none: the saving-list import and the administrator's master-catalogue
	// import. Four tools, one table, one key — an answer bought by any of them
	// is free to the other three.
	sharedMatchMemory := newMatchMemory(ingRepoUI)
	catSvcUI.SetMatchMemory(sharedMatchMemory)
	ingSvcUI.SetCatalogPort(catSvcUI)
	ingSvcUI.SetInventoryPort(invSvcUI)

	// A subscription bought or assigned here has to move the tenant's AI quota
	// with it, which is what the sync port does. Without it an upgrade changed
	// the invoice and nothing else.
	billSvcUI := billing.NewService(billRepoUI, log)
	billSvcUI.SetAIPlanSync(tenantKeys)

	uiHandler := ui.NewUIHandler(
		catSvcUI,
		orgSvcUI,
		ingSvcUI,
		commSvcUI,
		invSvcUI,
		idSvc,
		notifications.NewService(notifRepoUI, log),
		promoSvcUI,
		platformadmin.NewService(adminRepoUI, log),
		billSvcUI,
		chat.NewService(chatRepoUI, log),
		wfSvcUI,
		hr.NewService(hrRepoUI, log),
		attachSvc,
		log,
	)
	uiHandler.SetTrustedProxyHops(cfg.HTTP.TrustedProxyHops)
	uiHandler.SetSecureCookie(cfg.Session.SecureOnly)
	uiHandler.SetBaseURL(cfg.BaseURL)
	uiHandler.SetSessionCookieName(cfg.Session.CookieName)
	uiHandler.SetSessionSecret(cfg.Session.Secret)
	uiHandler.SetTelegramGatewayClient(telegramgateway.New(telegramgateway.Config{
		Token: cfg.Telegram.GatewayToken,
		Log:   log,
	}))
	uiHandler.SetMailer(mailer.New(cfg.SMTP, log))

	// Every employee of a منشأة spends against that منشأة's own Gateway key.
	//
	// The four hand-rolled copies of this — two here, one in the dashboard
	// handler, one in the admin handler — each provisioned inline on the request
	// path and each minted a fresh key every time, which revoked the one before
	// it. tenantKeyProvisioner does the same job once, with a per-organisation
	// lock, a validated cache, and a plan that follows the subscription.
	keyResolverUI := func(ctx context.Context, orgID int64) (string, error) {
		return tenantKeys.Key(ctx, orgID), nil
	}

	// The saving-list import reaches the shared cache through the handler,
	// because that is where its staging runs.
	uiHandler.SetMatchMemory(sharedMatchMemory)
	uiHandler.SetCoverageService(workflow.NewCoverageService(db))

	// Tenant key resolution is wired unconditionally, outside the `ai != nil`
	// block below. Approving a منشأة provisions its Gateway identity through
	// this port, and that has to keep working on a deployment where the
	// completion client is absent — otherwise approval silently skips
	// provisioning and the tenant is left with no user and no key.
	uiHandler.SetTenantGatewayKeys(tenantKeys)

	compareRepoUI := comparePostgres.NewRepository(db)
	compareSvcUI := compare.NewService(compareRepoUI, log)
	// The public خصومات السوق board's total and supplier list. Without this the
	// board still works and still uses its indexes — it just pays for the count
	// on every page view instead of once a minute. See market_board_service.go.
	if cacheHandle := deps.CacheHandle(); cacheHandle != nil {
		compareSvcUI.SetCache(cacheHandle)
	}
	if ai != nil {
		uiHandler.SetGatewayClient(ai)
		// The settings screen resets this when an operator changes the Gateway
		// credentials the admin panel's key was issued from.
		uiHandler.SetGatewayKeyCache(adminKeys)
		// The usage screens read the local ledger rather than calling the
		// Gateway on every render.
		uiHandler.SetAIUsage(aiusagePostgres.NewRepository(db))
		aiCapabilitiesSvc := aicapabilities.NewService(ai, log)
		aiCapabilitiesSvc.SetKeyResolver(keyResolverUI)
		compareSvcUI.SetAIMatcher(aiCapabilitiesSvc)

		// The catalogue import's three mapping calls: which column is which
		// field, and which existing category and pharmaceutical form each of the
		// file's distinct words means. Three requests per import regardless of
		// how many rows it has; the rows themselves never reach a model.
		mapper := cataloggw.NewMapper(ai, log)
		mapper.SetKeyResolver(cataloggw.KeyResolver(keyResolverUI))
		catSvcUI.SetAIMapper(mapper)
		// The fourth call, and the only one that is about rows: the products
		// similarity could not tie to an existing catalogue entry, adjudicated
		// in batches of twenty-five against a shortlist the importer retrieved.
		// Without it the import matched barcodes and identical spellings only,
		// and staged everything else as a new product.
		// The administrator's import asks the same question as the other three,
		// so it asks it through the same capability rather than through a third
		// prompt of its own. The mapper still answers the two questions that are
		// genuinely its own — which column is which, and what a category word
		// means — and those are asked once per file, not once per row.
		catSvcUI.SetMatchAdjudicator(&catalogAdjudicateAdapter{caps: aiCapabilitiesSvc})
		// The vendor import runs the smart order's enhancement stage: the same
		// system prompt, the same shared catalogue window, the same guards, and
		// the same decision cache in catalog.match_decisions — so an answer
		// bought by a pharmacy's order is free to the vendor whose price list
		// asks the same question, and there is one prompt to tune rather than
		// two that drift.
		enhancer := &ingestEnhanceAdapter{caps: aiCapabilitiesSvc}
		ingSvcUI.SetEnhancer(ingest.NewGatewayEnhancer(enhancer))
		// The saving-list import runs the same stage through the same adapter.
		// It was the last of the four importers without one, and giving it a
		// second implementation would have been the mistake this whole refactor
		// exists to undo.
		uiHandler.SetMatchEnhancer(enhancer)
	}
	if storageClient != nil {
		compareSvcUI.SetStorage(storageClient)
	}
	uiHandler.SetCompareService(compareSvcUI)

	if storageClient != nil {
		uiHandler.SetStorage(storageClient)
	}
	// The public surface publishes supplier identity, net supply price, stock
	// and expiry to callers who have not signed in. The guard meters that; the
	// Redis handle is resolved per request because the listener opens before
	// Redis is dialled, and a client captured here would be nil forever.
	uiHandler.SetScrapeGuard(antiscrape.New(antiscrape.Options{
		Enabled: cfg.Scrape.Enabled,
		Redis: func() *redis.Client {
			if c := deps.CacheHandle(); c != nil {
				return c.Redis()
			}
			return nil
		},
		Log:              log,
		KeyPrefix:        "dawa24:" + string(cfg.Env) + ":antiscrape:",
		TrustedProxyHops: cfg.HTTP.TrustedProxyHops,
		PenaltyTTL:       cfg.Scrape.PenaltyTTL,
	}))
	uiHandler.SetGuestListingLimits(cfg.Scrape.GuestMaxPage, cfg.Scrape.GuestMaxPageSize)

	uiHandler.SetAssistantRepository(assistantPostgres.NewRepository(db))
	uiHandler.SetPermissionResolver(permissions)
	// The /admin/system-pages screen. The enforcement engine itself is started
	// in newRouter; this is only the store the screen reads and writes.
	uiHandler.SetPageControlStore(pagecontrol.NewStore(db))
	pagecontrol.SetDisabledHandler(uiHandler.PageMaintenanceHandler)
	// A company that has no roles yet gets them the first time its owner opens
	// the roles or team screen. The boot seeder covers companies that already
	// existed; this covers one registered while the process was running, and a
	// registration whose seeding step failed.
	uiHandler.SetRoleSeeder(func(ctx context.Context, orgID int64, orgType string) error {
		return rbac.EnsureCompanyRoles(ctx, db, orgID, orgType)
	})

	// Smart ordering (specs/001-smart-ordering-system).
	wireSmartOrder(db, uiHandler, orgSvcUI, workflow.NewCoverageService(db), commSvcUI, uiAvailability, ai, uiProgress, log)

	// Unified durable imports (Task 18).
	wireImports(db, uiHandler, catSvcUI, progressHub, uiProgress, log)

	// Audience-gated UI groups (Rebuild V2 §1.3). Every route is registered
	// under exactly one group; a route living outside these groups means it is
	// reachable by anyone regardless of account type — test/route_audience_test.go
	// walks the app the same way admin_guard_test.go does and forbids that.
	return uiHandler
}

// redisHandle resolves the cache client, which may not be dialled yet.
//
// The HTTP listener opens before Redis is connected, so a client captured at
// wiring time would be nil for the life of the process. Progress degrades to
// the JSON poll when it comes back nil, which is the same trade the rest of the
// platform makes with the cache.
func redisHandle(deps *dependencies) *redis.Client {
	if c := deps.CacheHandle(); c != nil {
		return c.Redis()
	}
	return nil
}
