package ui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/aiusage"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// EnsureOrgAIGatewayProvisioned returns the Gateway identity the organisation
// spends against, provisioning it on first use.
//
// The provisioning itself lives in one place now. This used to read the
// organisation, read its subscription, resolve a plan and mint a key inline —
// and so did three other call sites, each minting a key that revoked the one
// the others had just stored. It delegates instead, so the per-organisation
// lock, the validated cache and the plan-follows-subscription rule apply
// wherever the identity is asked for.
func (h *UIHandler) EnsureOrgAIGatewayProvisioned(ctx context.Context, orgID int64) (userID, virtualKey string) {
	if orgID <= 0 || h.orgSvc == nil {
		return "", ""
	}

	// A منشأة approved before any of this worked has nothing: no starter roles,
	// no subscription, and so no Gateway plan to be provisioned under. Approval
	// is no longer the only chance to fix that — signing in repairs whatever is
	// missing, so an organisation that was already approved does not have to be
	// approved a second time to become usable.
	h.healOrgWiring(ctx, orgID)

	if h.tenantKeys != nil {
		virtualKey = h.tenantKeys.Key(ctx, orgID)
	}
	if virtualKey != "" {
		return gateway.OrganizationUserID(orgID), virtualKey
	}
	return "", ""
}

// healOrgWiring gives an already-approved organisation the roles and the
// subscription it should have received at approval.
//
// It is called on the request path, so it is written to cost one indexed read
// when there is nothing to do, and to write only when something is genuinely
// absent. Nothing here is fatal: a منشأة with no subscription can still sign in
// and browse, and reporting the failure is the approvals screen's job.
func (h *UIHandler) healOrgWiring(ctx context.Context, orgID int64) {
	sysCtx := database.AsSystem(ctx)

	o, err := h.orgSvc.GetOrganization(sysCtx, orgID)
	if err != nil || o == nil || o.Status != org.StatusApproved {
		return
	}

	if h.billSvc == nil || o.OwnerID <= 0 {
		return
	}
	if sub, err := h.billSvc.GetActiveSubscriptionByOrg(sysCtx, orgID); err == nil && sub != nil {
		return
	}

	// Only an organisation that is actually missing its wiring reaches here, so
	// seeding roles costs nothing on the common path.
	h.ensureCompanyRoles(sysCtx, orgID, string(o.Type))

	if _, err := h.billSvc.AssignDefaultSubscription(sysCtx, o.OwnerID, &orgID); err != nil {
		h.log.WarnContext(ctx, "could not repair missing subscription on sign-in",
			"org_id", orgID, "owner_id", o.OwnerID, "error", err)
		return
	}
	h.log.InfoContext(ctx, "repaired missing subscription for approved organisation",
		"org_id", orgID, "owner_id", o.OwnerID)
}

// EnsureAIGatewayProvisioned mirrors the org-scoped call for user accounts.
func (h *UIHandler) EnsureAIGatewayProvisioned(ctx context.Context, actor authctx.Actor) (userID, virtualKey string) {
	if actor.OrganizationID > 0 {
		return h.EnsureOrgAIGatewayProvisioned(ctx, actor.OrganizationID)
	}
	return "", ""
}

// AIConsumptionLogsPage renders the audit log for AI spend (US-16).
//
// Every line on this screen comes from platform.ai_usage_ledger in Postgres.
// We used to query the Gateway's live proxy log, which created three bugs at
// once: the page hung whenever the Gateway was slow, every token cost was shown
// as zero because the upstream router had no price catalog, and the tenant
// could not see spend from runs that failed inside our own workers.
func (h *UIHandler) AIConsumptionLogsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (actor.OrganizationID <= 0 && actor.UserID <= 0) {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.Path, http.StatusSeeOther)
		return
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	lang, dir := h.localeAndDir(r)
	subView := h.loadOrgSubscriptionView(ctx, actor, lang)
	isVendor := actor.IsVendor()

	featureFilter := strings.TrimSpace(r.URL.Query().Get("feature"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	searchFilter := strings.TrimSpace(r.URL.Query().Get("q"))

	pageData := pages.AIConsumptionLogsPageData{
		IsVendor:         isVendor,
		IsCustomer:       actor.IsCustomer(),
		FilterFeature:    featureFilter,
		FilterStatus:     statusFilter,
		FilterSearch:     searchFilter,
		FeatureBreakdown: map[string]int{},
		AIUserID:         gateway.OrganizationUserID(actor.OrganizationID),
		PlanName:         i18n.T(lang, "sub.default_plan_name"),
		PlanSlug:         "basic",
		AIPlanID:         gateway.FallbackPlanID,
		Page:             page,
		PerPage:          limit,
	}
	if subView != nil {
		pageData.AIUserID = subView.AIUserID
		pageData.PlanName = subView.PlanName
		pageData.PlanSlug = subView.PlanSlug
		pageData.AIPlanID = subView.AIPlanID
		pageData.ResetTime = subView.AIBudgetResetTime
		pageData.ActiveBudgetLimit = subView.AIBudgetLimitUSD
		pageData.ActiveBudgetSpent = subView.AIBudgetSpentUSD
		pageData.UsagePercentage = subView.AIPercentage()
		pageData.HasBudget = subView.HasAIBudget
	}

	if h.aiUsage != nil && actor.OrganizationID > 0 {
		h.fillAIUsageFromLedger(ctx, &pageData, actor.OrganizationID, isVendor, limit, offset, lang)
	}

	h.renderPage(ctx, w, "render ai consumption logs", pages.AIConsumptionLogsPage(pageData, lang, dir))
}

// fillAIUsageFromLedger populates the page from the local record.
//
// The listing runs under the caller's own transaction context, so row-level
// security scopes it to their organisation. That is the isolation guarantee;
// the previous version enforced it by comparing a user id string in a loop
// after fetching, which is a filter rather than a boundary.
func (h *UIHandler) fillAIUsageFromLedger(ctx context.Context, data *pages.AIConsumptionLogsPageData, orgID int64, isVendor bool, limit, offset int, lang ...string) {
	since := time.Now().Add(-aiLedgerWindow)

	entries, total, err := h.aiUsage.List(ctx, aiusage.Filter{
		OrganizationID: orgID,
		Feature:        data.FilterFeature,
		Status:         data.FilterStatus,
		Search:         data.FilterSearch,
		Since:          since,
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		h.log.WarnContext(ctx, "could not read ai usage ledger", "org_id", orgID, "error", err)
		return
	}
	data.TotalCount = total

	data.Logs = make([]*pages.AILogItemView, 0, len(entries))
	for _, e := range entries {
		featName, featKey := mapGatewayCapabilityToName(e.Capability, e.Feature, isVendor, lang...)
		modelTier := ""
		lowerModel := strings.ToLower(e.Model)
		if strings.Contains(lowerModel, "flash") || strings.Contains(lowerModel, "fast") || strings.Contains(lowerModel, "turbo") {
			modelTier = "fast"
		} else if strings.Contains(lowerModel, "quality") || strings.Contains(lowerModel, "plus") || strings.Contains(lowerModel, "pro") {
			modelTier = "quality"
		}
		modelAlias := "محرك دوا 24 الذكي"
		data.Logs = append(data.Logs, &pages.AILogItemView{
			ID:            fmt.Sprintf("%d", e.ID),
			Timestamp:     pages.FormatDateTimeSec(e.CreatedAt),
			TimeFormatted: pages.FormatDateTime(e.CreatedAt),
			FeatureName:   featName,
			FeatureKey:    featKey,
			ModelAlias:    modelAlias,
			ModelTier:     modelTier,
			InputTokens:   e.InputTokens,
			OutputTokens:  e.OutputTokens,
			TotalTokens:   e.TotalTokens(),
			CostUSD:       e.CostUSD(),
			CostKnown:     e.CostKnown,
			DurationMs:    int64(e.DurationMS),
			DurationKnown: e.DurationMS > 0,
			Status:        e.Status,
			StatusLabel:   aiStatusLabel(e.Status, e.FromCache, e.Fallback, lang...),
		})
	}

	summary, err := h.aiUsage.Summarize(ctx, orgID, since)
	if err != nil {
		h.log.WarnContext(ctx, "could not summarise ai usage", "org_id", orgID, "error", err)
		return
	}
	data.TotalRequests = summary.Requests
	data.TotalTokens = int(summary.TotalTokens())
	data.TotalCostUSD = summary.CostUSD()
	data.CostIsComplete = summary.CostIsComplete()

	byFeature, err := h.aiUsage.ByFeature(ctx, orgID, since)
	if err != nil {
		h.log.WarnContext(ctx, "could not break ai usage down by feature", "org_id", orgID, "error", err)
		return
	}
	for _, f := range byFeature {
		_, key := mapGatewayCapabilityToName(f.Feature, f.Feature, isVendor, lang...)
		data.FeatureBreakdown[key] += f.Requests
	}
}

// aiStatusLabel renders an outcome in the tenant's language.
//
// A cached answer and a fallback are both "successful" in the sense that the
// user got a result, and both cost nothing — but they mean different things and
// a usage screen that calls them the same thing hides how often AI was actually
// reached.
func aiStatusLabel(status string, cached, fallback bool, langOptional ...string) string {
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}
	switch {
	case fallback:
		return i18n.T(lang, "ai.status.fallback")
	case cached:
		return i18n.T(lang, "ai.status.cached")
	}
	switch status {
	case "success":
		return i18n.T(lang, "ai.status.success")
	case "disabled":
		return i18n.T(lang, "ai.status.disabled")
	case "timeout":
		return i18n.T(lang, "ai.status.timeout")
	case "quota_exceeded":
		return i18n.T(lang, "ai.status.quota_exceeded")
	case "rate_limited":
		return i18n.T(lang, "ai.status.rate_limited")
	case "unauthorized":
		return i18n.T(lang, "ai.status.unauthorized")
	case "circuit_open", "unavailable":
		return i18n.T(lang, "ai.status.unavailable")
	case "abandoned":
		return i18n.T(lang, "ai.status.abandoned")
	default:
		return i18n.T(lang, "ai.status.incomplete")
	}
}

func mapGatewayCapabilityToName(cap, feat string, isVendor bool, langOptional ...string) (string, string) {
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}
	if feat != "" {
		switch feat {
		case "smart_order", "smartorder":
			return i18n.T(lang, "ai.feat.smart_order"), "smart_order"
		case "savings", "saving_products":
			return i18n.T(lang, "ai.feat.savings"), "savings"
		case "assistant":
			return i18n.T(lang, "ai.feat.assistant"), "assistant"
		case "voice_ocr", "voice", "ocr":
			return i18n.T(lang, "ai.feat.voice_ocr"), "voice_ocr"
		case "variant_match", "variants":
			return i18n.T(lang, "ai.feat.variant_match"), "variant_match"
		case "savings_import":
			return i18n.T(lang, "ai.feat.savings_import"), "savings_import"
		case "column_detect":
			return i18n.T(lang, "ai.feat.column_detect"), "column_detect"
		case "compare_match", "compare", "compare_discounts":
			return i18n.T(lang, "ai.feat.compare_match"), "compare_match"
		case "catalog_import", "vendor_import":
			return i18n.T(lang, "ai.feat.catalog_import"), "catalog_import"
		}
	}
	switch cap {
	case "matching.enhance":
		if isVendor {
			return i18n.T(lang, "ai.feat.vendor_catalog_enhance"), "variant_match"
		}
		return i18n.T(lang, "ai.feat.smart_order_enhance"), "smart_order"
	case "import.detect_columns":
		return i18n.T(lang, "ai.feat.auto_detect_columns"), "column_detect"
	case "product.match", "matching.adjudicate":
		if isVendor {
			return i18n.T(lang, "ai.feat.vendor_alt_match"), "variant_match"
		}
		return i18n.T(lang, "ai.feat.saving_alt_match"), "savings"
	case "catalog.chat", "assistant":
		return i18n.T(lang, "ai.feat.auto_assistant"), "assistant"
	case "voice.transcribe":
		return i18n.T(lang, "ai.feat.voice_transcribe"), "voice_ocr"
	default:
		if isVendor {
			return i18n.T(lang, "ai.feat.vendor_catalog_generate"), "variant_match"
		}
		return i18n.T(lang, "ai.feat.smart_order"), "smart_order"
	}
}
