package ui

import (
	"log/slog"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
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
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/aiusage"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/platform/mailer"
	"github.com/muhiya/dawa24-store/internal/platform/pagecontrol"
	"github.com/muhiya/dawa24-store/internal/platform/progress"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/platform/safe"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/platform/telegramgateway"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// SetRoleSeeder wires starter-role provisioning for companies that have none.
func (h *UIHandler) SetRoleSeeder(fn RoleSeederFunc) { h.roleSeeder = fn }

// SetPermissionResolver wires live permission resolution. Optional: without
// it the UI falls back to the session's permission copy, which is correct but
// cannot see a revocation until the session ends.
func (h *UIHandler) SetPermissionResolver(r *rbac.Resolver) { h.resolver = r }

// SetPageControlStore wires the /admin/system-pages screen. Optional: the screen
// reports itself unavailable when it is nil.
func (h *UIHandler) SetPageControlStore(s *pagecontrol.Store) { h.pageControl = s }

// SetMatchEnhancer attaches the shared AI matching stage.
func (h *UIHandler) SetMatchEnhancer(e matchflow.Enhancer) { h.matchEnhancer = e }

// SetMatchMemory attaches the shared decision cache, so the saving-list import
// reads and writes the same catalog.match_decisions rows the vendor import and
// the smart order do.
func (h *UIHandler) SetMatchMemory(m matchflow.Memory) { h.matchMemory = m }

// SetAssistantRepository attaches the Assistant database repository for auditing and history.
func (h *UIHandler) SetAssistantRepository(repo assistant.Repository) {
	h.assistantRepo = repo
}

// SetSmartOrder wires the smart ordering wizard.
func (h *UIHandler) SetSmartOrder(svc *smartorder.Service, enqueue SmartOrderEnqueueFunc) {
	h.smartOrderSvc = svc
	h.smartOrderEnqueue = enqueue
	if h.smartOrderStale == nil {
		h.smartOrderStale = newSmartOrderStaleStore()
	}
}

// SetGatewayClient attaches the Gateway client instance for health probes and AI services.
func (h *UIHandler) SetGatewayClient(ai gateway.Client) {
	h.aiClient = ai
}

// SetGatewayKeyCache installs the credential cache the settings screen resets.
func (h *UIHandler) SetGatewayKeyCache(cache GatewayKeyCache) {
	h.gatewayKeys = cache
}

// SetTenantGatewayKeys installs the per-organisation credential resolver.
func (h *UIHandler) SetTenantGatewayKeys(keys TenantGatewayKeys) {
	h.tenantKeys = keys
}

// SetBaseURL is the site's configured public origin (APP_BASE_URL). Links the
// site publishes about itself — sitemap, llms.txt, agents index — use it rather
// than the request's Host header, which the client chooses and which those
// publicly cached documents would otherwise repeat to everyone.
func (h *UIHandler) SetBaseURL(u string) {
	h.baseURL = strings.TrimRight(strings.TrimSpace(u), "/")
}

// SetSecureCookie controls whether session cookies include the Secure flag.
// Should be true in production (behind TLS) and false in development.
func (h *UIHandler) SetSecureCookie(secure bool) {
	h.secureCookie = secure
}

// SetSessionCookieName sets the cookie name used for user sessions.
func (h *UIHandler) SetSessionCookieName(name string) {
	h.sessionCookieName = name
}

// cookieName returns the configured session cookie name, defaulting to "dawa24_session".
func (h *UIHandler) cookieName() string {
	if h.sessionCookieName != "" {
		return h.sessionCookieName
	}
	return "dawa24_session"
}

// SetAIUsage installs the local AI consumption ledger.
//
// The usage screens read from it rather than calling the Gateway on every
// render. That is what gives a tenant a history longer than one API page, makes
// the figures survive a Gateway outage, and removes the need for the invented
// costs and latencies the screens used to fill the gaps with.
func (h *UIHandler) SetAIUsage(repo aiusage.Repository) {
	h.aiUsage = repo
}

// NewUIHandler creates a new UI page handler with all platform domain services wired.
func NewUIHandler(
	catSvc *catalog.Service,
	orgSvc *org.Service,
	ingSvc *ingest.Service,
	commSvc *commerce.Service,
	invSvc *inventory.Service,
	idSvc *identity.Service,
	notifSvc *notifications.Service,
	promoSvc *promo.Service,
	adminSvc *platformadmin.Service,
	billSvc *billing.Service,
	chatSvc *chat.Service,
	wfSvc *workflow.Service,
	hrSvc *hr.Service,
	attSvc *attachments.Service,
	log *slog.Logger,
) *UIHandler {
	if log == nil {
		log = slog.Default()
	}
	return &UIHandler{
		catSvc:   catSvc,
		orgSvc:   orgSvc,
		ingSvc:   ingSvc,
		commSvc:  commSvc,
		invSvc:   invSvc,
		idSvc:    idSvc,
		notifSvc: notifSvc,
		promoSvc: promoSvc,
		adminSvc: adminSvc,
		billSvc:  billSvc,
		chatSvc:  chatSvc,
		wfSvc:    wfSvc,
		hrSvc:    hrSvc,
		attSvc:   attSvc,
		log:      log,
	}
}

// SetStorage configures object storage (MinIO/S3) for UI handlers.
func (h *UIHandler) SetStorage(s *storage.Client) {
	h.storage = s
}

// SetCompareService configures the compare module service for UI handlers.
func (h *UIHandler) SetCompareService(s *compare.Service) {
	h.compareSvc = s
}

// SetCoverageService configures the workflow coverage service for UI handlers.
func (h *UIHandler) SetCoverageService(cs *workflow.CoverageService) {
	h.coverageSvc = cs
}

// SetImportRunRepo wires the durable import run repository.
func (h *UIHandler) SetImportRunRepo(repo importrun.Repository) {
	h.importRunRepo = repo
}

// SetProgressHub wires the live-progress fan-out.
//
// Optional on purpose: a deployment without it, or a process that failed to
// reach Redis, simply serves the JSON poll and every bar keeps working.
func (h *UIHandler) SetProgressHub(hub *progress.Hub) {
	h.progressHub = hub
}

// SetImportQueue wires background queue dispatch functions for imports.
func (h *UIHandler) SetImportQueue(stage ImportStageEnqueueFunc, commit ImportCommitEnqueueFunc) {
	h.importStageEnqueue = stage
	h.importCommitEnqueue = commit
}

// SetTrustedProxyHops configures the number of reverse proxies in front of the process
// so client IP resolution reads X-Forwarded-For from the trusted hop.
func (h *UIHandler) SetTrustedProxyHops(n int) {
	h.trustedProxyHops = n
}

// SetNotificationEnqueue wires asynchronous background queue dispatch for notifications.
func (h *UIHandler) SetNotificationEnqueue(fn NotificationEnqueueFunc) {
	h.notificationEnqueue = fn
}

// SetTelegramGatewayClient wires the Telegram Gateway client for phone verification OTPs.
func (h *UIHandler) SetTelegramGatewayClient(c telegramgateway.Client) {
	h.tgGatewayClient = c
}

// SetSessionSecret sets the session secret used for signing verified tokens.
func (h *UIHandler) SetSessionSecret(s string) {
	h.sessionSecret = s
}

// safeGo executes a background task with panic recovery and structured error logging.
func (h *UIHandler) safeGo(name string, fn func()) {
	safe.Go(h.log, name, fn)
}

// SetMailer wires the transactional mailer service and initializes the email OTP engine.
func (h *UIHandler) SetMailer(m mailer.Mailer) {
	h.mailer = m
	if h.emailOTP == nil {
		h.emailOTP = mailer.NewOTPEngine()
	}
}

// Mailer returns the configured mailer or a mock fallback when nil.
func (h *UIHandler) Mailer() mailer.Mailer {
	if h.mailer != nil {
		return h.mailer
	}
	return mailer.NewMockMailer(h.log)
}

// EmailOTP returns the configured email OTP engine.
func (h *UIHandler) EmailOTP() *mailer.OTPEngine {
	if h.emailOTP == nil {
		h.emailOTP = mailer.NewOTPEngine()
	}
	return h.emailOTP
}

