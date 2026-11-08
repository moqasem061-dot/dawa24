package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/mask"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func (h *UIHandler) getGatewayAdminClient(ctx context.Context) (*gateway.AdminClient, string, bool) {
	var endpointURL, secretKey string
	if h.adminSvc != nil {
		// gateway_configuration is the authority for the administrator
		// credential and is read first. ai_configuration is only a mirror of
		// it, and on the live database that mirror was found holding the
		// production PostgreSQL superuser credential — which this function
		// previously preferred and sent as Basic auth to the Gateway host on
		// every management call. Both sources now pass through the same shape
		// check, so neither can carry a database password out of the process.
		if gwSets, err := h.adminSvc.GetGatewaySettings(ctx); err == nil && gwSets != nil {
			if gwSets.EndpointURL != "" {
				endpointURL = platformadmin.NormalizeGatewayEndpoint(gwSets.EndpointURL)
			}
			// Not gwSets.APIKey directly: AdminCredentials refuses to hand back
			// a value that does not look like a Gateway credential.
			if user, pass := gwSets.AdminCredentials(); pass != "" {
				if user != "" && user != "admin" {
					secretKey = user + ":" + pass
				} else {
					secretKey = pass
				}
			}
		}
		if endpointURL == "" || secretKey == "" {
			if aiSets, err := h.adminSvc.GetAISettings(ctx); err == nil && aiSets != nil {
				if endpointURL == "" && aiSets.EndpointURL != "" {
					endpointURL = platformadmin.NormalizeGatewayEndpoint(aiSets.EndpointURL)
				}
				if secretKey == "" && platformadmin.ValidateAdminCredential(aiSets.APIKey) == nil {
					secretKey = strings.TrimSpace(aiSets.APIKey)
				}
			}
		}
	}
	if endpointURL == "" {
		endpointURL = "https://api.dawa24.com"
	}
	client := gateway.NewAdminClient(endpointURL, "", secretKey)
	return client, endpointURL, secretKey != ""
}

// AdminDevelopersPage renders the unified developer portal with 4 tabs:
// 1. SQL Console, 2. AI Gateway, 3. Error Diagnostics, 4. Audit Trail.
func (h *UIHandler) AdminDevelopersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "sql"
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var gateway *platformadmin.GatewaySettings
	var sqlLogs []*platformadmin.SQLLog
	var errorLogs []*platformadmin.ErrorLog
	var auditEntries []*platformadmin.AuditEntry

	values := pages.AdminDevelopersValues{
		ActiveTab:         tab,
		ErrorLevelFilter:  r.URL.Query().Get("err_level"),
		ErrorStatusFilter: r.URL.Query().Get("err_status"),
		ErrorSearch:       r.URL.Query().Get("err_q"),
		ErrorPage:         page,
		ErrorPerPage:      limit,
	}

	if h.adminSvc != nil {
		gw, loadErr := h.adminSvc.GetGatewaySettings(ctx)
		h.logLoadErr(ctx, "adminSvc.GetGatewaySettings", loadErr)
		gateway = gw
		if gateway == nil {
			gateway = &platformadmin.GatewaySettings{
				EndpointURL: "https://api.dawa24.com",
				IsActive:    true,
			}
		}

		sl, loadErr := h.adminSvc.ListSQLLogs(ctx, 30, 0)
		h.logLoadErr(ctx, "adminSvc.ListSQLLogs", loadErr)
		sqlLogs = sl

		filter := platformadmin.ErrorLogFilter{
			Level:  values.ErrorLevelFilter,
			Status: values.ErrorStatusFilter,
			Search: values.ErrorSearch,
			Limit:  limit,
			Offset: offset,
		}
		el, total, loadErr := h.adminSvc.ListErrorLogs(ctx, filter)
		h.logLoadErr(ctx, "adminSvc.ListErrorLogs", loadErr)
		errorLogs = el
		values.ErrorTotalCount = total

		tot, crit, unres, affUsers, loadErr := h.adminSvc.GetErrorDiagnosticsMetrics(ctx)
		h.logLoadErr(ctx, "adminSvc.GetErrorDiagnosticsMetrics", loadErr)
		values.ErrorMetrics.Total = tot
		values.ErrorMetrics.Critical24h = crit
		values.ErrorMetrics.Unresolved = unres
		values.ErrorMetrics.AffectedUsers = affUsers

		auditQ := strings.TrimSpace(r.URL.Query().Get("audit_q"))
		values.AuditSearchQuery = auditQ
		auditFilter := platformadmin.AuditLogFilter{
			Search: auditQ,
			Limit:  limit,
			Offset: offset,
		}
		ae, auditTotal, loadErr := h.adminSvc.ListAuditLogWithFilter(ctx, auditFilter)
		h.logLoadErr(ctx, "adminSvc.ListAuditLogWithFilter", loadErr)
		for _, e := range ae {
			localizeAuditEntry(e, lang)
		}
		auditEntries = ae
		values.AuditPage = page
		values.AuditPerPage = limit
		values.AuditTotalCount = auditTotal

		ai, loadErr := h.adminSvc.GetAISettings(ctx)
		h.logLoadErr(ctx, "adminSvc.GetAISettings", loadErr)
		if ai == nil {
			ai = &platformadmin.AISettings{
				EndpointURL: "https://api.muhiya.com",
				IsActive:    true,
			}
		}
		values.AISettings = ai

		rms, loadErr := h.adminSvc.ListAIRoleModels(ctx)
		h.logLoadErr(ctx, "adminSvc.ListAIRoleModels", loadErr)
		values.AIRoleModels = rms
	}

	if gateway == nil {
		gateway = &platformadmin.GatewaySettings{
			EndpointURL: "https://api.muhiya.com",
			IsActive:    true,
		}
	}

	values.GatewaySettings = gateway
	// The stored username is shown masked and revealed on request (plan.md S8,
	// FR-032). The password is never put in the view.
	if user, _ := gateway.AdminCredentials(); user != "" {
		values.GatewayAdminMasked = mask.Number(user)
	}
	revealer, _ := authctx.From(ctx)
	values.CanRevealGateway = revealer.Can(revealPermStaff)
	values.SQLLogs = sqlLogs
	values.ErrorLogs = errorLogs
	values.AuditEntries = auditEntries
	values.SEO = h.buildSEOValues(r)

	h.renderPage(ctx, w, "render admin developers page", pages.AdminDevelopersPage(values, lang, dir))
}

// AdminSQLExecuteSubmit executes a SQL query from the Developer SQL Console and returns JSON.
func (h *UIHandler) AdminSQLExecuteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	w.Header().Set("Content-Type", "application/json")

	if h.adminSvc == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": i18n.T(lang, "admin.dev.admin_service_unavailable")})
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if query == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": i18n.T(lang, "admin.dev.empty_sql_query")})
		return
	}

	actor, ok := authctx.From(ctx)
	var actorID *int64
	actorName := "System Admin"
	if ok && actor.UserID > 0 {
		actorID = &actor.UserID
		if actor.Name != "" {
			actorName = actor.Name
		}
	}

	res, err := h.adminSvc.ExecuteSQL(ctx, actorID, actorName, query)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(res)
}

// AdminDeveloperAISettingsSubmit updates AI Gateway settings from the Developer section.
func (h *UIHandler) AdminDeveloperAISettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", i18n.T(lang, "admin.dev.admin_service_unavailable"))
		return
	}

	endpoint := platformadmin.NormalizeGatewayEndpoint(r.FormValue("endpoint_url"))
	if endpoint == "" {
		endpoint = "https://api.dawa24.com"
	}
	adminUser := strings.TrimSpace(r.FormValue("admin_username"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	isActive := r.FormValue("is_active") == "true"
	systemPrompt := strings.TrimSpace(r.FormValue("system_prompt"))

	// The password field renders blank on every load, so an operator who edits
	// only the endpoint or the models must not have the credential wiped. An
	// empty submission (or masked placeholder) therefore means "keep what is stored".
	if apiKey == "" || strings.HasPrefix(apiKey, "•") {
		if existingGW, loadErr := h.adminSvc.GetGatewaySettings(ctx); loadErr != nil {
			h.logLoadErr(ctx, "adminSvc.GetGatewaySettings", loadErr)
		} else if existingGW != nil {
			savedUser, pass := existingGW.AdminCredentials()
			if pass != "" {
				apiKey = pass
			}
			if adminUser == "" && savedUser != "" {
				adminUser = savedUser
			}
		}
	}
	if apiKey == "" || strings.HasPrefix(apiKey, "•") {
		if existingAI, loadErr := h.adminSvc.GetAISettings(ctx); loadErr != nil {
			h.logLoadErr(ctx, "adminSvc.GetAISettings", loadErr)
		} else if existingAI != nil &&
			strings.TrimSpace(existingAI.APIKey) != "" &&
			platformadmin.ValidateAdminCredential(existingAI.APIKey) == nil {
			apiKey = strings.TrimSpace(existingAI.APIKey)
		}
	}
	if adminUser == "" {
		adminUser = "admin"
	}

	// Format combined credentials if username is custom
	combinedKey := apiKey
	if apiKey != "" && !strings.Contains(apiKey, ":") && adminUser != "admin" {
		combinedKey = adminUser + ":" + apiKey
	}

	// Load and mutate rather than rebuild. The struct also carries the virtual
	// key provisioned from these very credentials, plus the model choices;
	// constructing a fresh one here silently discarded all of it and switched
	// AI back off every time an operator pressed save.
	gw, err := h.adminSvc.GetGatewaySettings(ctx)
	if err != nil || gw == nil {
		gw = &platformadmin.GatewaySettings{Environment: "production"}
	}

	credentialsChanged := gw.EndpointURL != endpoint || gw.APIKey != combinedKey
	gw.EndpointURL = endpoint
	gw.APIKey = combinedKey
	gw.IsActive = isActive
	if gw.Environment == "" {
		gw.Environment = "production"
	}
	if model := strings.TrimSpace(r.FormValue("fast_model")); model != "" {
		gw.FastModel = model
	}
	if model := strings.TrimSpace(r.FormValue("quality_model")); model != "" {
		gw.QualityModel = model
	}
	if plan := strings.TrimSpace(r.FormValue("ai_plan_id")); plan != "" {
		gw.AIPlanID = plan
	}

	// New credentials invalidate the key they issued: it may belong to a
	// different Gateway entirely. Clearing it makes the next AI call provision
	// a fresh one against the endpoint now configured.
	if credentialsChanged {
		gw.VirtualKey, gw.AIUserID = "", ""
	}

	if err := h.adminSvc.SaveGatewaySettings(ctx, gw); err != nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", h.safeMessage(err, lang))
		return
	}
	if h.gatewayKeys != nil {
		h.gatewayKeys.Invalidate()
	}

	// Also sync to AISettings.
	//
	// The credential is mirrored only when it is one the gateway settings would
	// itself accept. A rejected value must not be copied into a second row and
	// resurrected from there on the next save, which is exactly how the
	// database credential outlived being removed from gateway_configuration.
	ai, loadErr := h.adminSvc.GetAISettings(ctx)
	h.logLoadErr(ctx, "adminSvc.GetAISettings", loadErr)
	if ai == nil {
		ai = &platformadmin.AISettings{}
	}
	ai.EndpointURL = endpoint
	if platformadmin.ValidateAdminCredential(combinedKey) == nil {
		ai.APIKey = combinedKey
	} else {
		ai.APIKey = ""
	}
	ai.IsActive = isActive
	if systemPrompt != "" {
		ai.SystemPrompt = systemPrompt
	}
	_ = h.adminSvc.SaveAISettings(ctx, ai)

	h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "success", i18n.T(lang, "admin.dev.saved_ai_settings_success"))
}
