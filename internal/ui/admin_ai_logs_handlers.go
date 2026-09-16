package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/aiusage"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminAILogsPage renders the system-wide AI consumption logs and audit dashboard for administrators.
func (h *UIHandler) AdminAILogsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	orgIDStr := strings.TrimSpace(r.URL.Query().Get("org_id"))
	orgID, _ := strconv.ParseInt(orgIDStr, 10, 64)
	feature := strings.TrimSpace(r.URL.Query().Get("feature"))
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	dateFrom := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateTo := strings.TrimSpace(r.URL.Query().Get("date_to"))

	var since, until time.Time
	if dateFrom != "" {
		if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
			since = t
		}
	}
	if dateTo != "" {
		if t, err := time.Parse("2006-01-02", dateTo); err == nil {
			until = t.Add(24 * time.Hour).Add(-time.Nanosecond)
		}
	}

	// 1. Load active organizations for dropdown filter and name resolution
	var orgs []*org.Organization
	orgMap := make(map[int64]*org.Organization)
	if h.orgSvc != nil {
		if list, err := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 1000, 0); err == nil {
			orgs = list
			for _, o := range list {
				orgMap[o.ID] = o
			}
		}
	}

	// 2. Fetch AI usage ledger events
	var entries []aiusage.Entry
	var totalCount int
	var summary aiusage.Summary

	if h.aiUsage != nil {
		var err error
		entries, totalCount, err = h.aiUsage.List(sysCtx, aiusage.Filter{
			OrganizationID: orgID,
			Since:          since,
			Until:          until,
			Feature:        feature,
			Model:          model,
			Status:         status,
			Search:         query,
			Limit:          limit,
			Offset:         offset,
		})
		if err != nil {
			h.log.WarnContext(ctx, "could not query admin ai logs", "error", err)
		}

		summary, err = h.aiUsage.Summarize(sysCtx, orgID, since)
		if err != nil {
			h.log.WarnContext(ctx, "could not summarize admin ai usage", "error", err)
		}
	}

	// 3. Resolve user details for entries
	userMap := make(map[int64]*identity.User)
	if h.idSvc != nil {
		for _, e := range entries {
			if e.UserID > 0 {
				if _, ok := userMap[e.UserID]; !ok {
					if u, err := h.idSvc.AdminGetUser(sysCtx, e.UserID); err == nil && u != nil {
						userMap[e.UserID] = u
					}
				}
			}
		}
	}

	// 4. Map logs with enriched metadata
	items := make([]pages.AdminAILogItem, 0, len(entries))
	for _, e := range entries {
		var orgName string
		orgType := "customer"
		if o, ok := orgMap[e.OrganizationID]; ok && o != nil {
			orgType = string(o.Type)
			if o.LegalName != "" {
				orgName = o.LegalName
			} else if !o.TradeName.IsEmpty() {
				orgName = o.TradeName.Get(i18n.Lang(lang))
			}
		}
		if orgName == "" {
			if e.OrganizationID > 0 {
				orgName = fmt.Sprintf("منشأة #%d", e.OrganizationID)
			} else {
				orgName = "النظام العام"
			}
		}

		var userName, userEmail string
		if e.UserID > 0 {
			if u, ok := userMap[e.UserID]; ok && u != nil {
				userName = u.Name.Get(i18n.Lang(lang))
				if userName == "" {
					userName = u.Name.Get("ar")
				}
				if userName == "" {
					userName = u.Name.Get("en")
				}
				userEmail = u.Email
			}
		}

		featName, _ := mapGatewayCapabilityToName(e.Capability, e.Feature, orgType == "vendor", lang)
		statusLabel := aiStatusLabel(e.Status, e.FromCache, e.Fallback, lang)

		items = append(items, pages.AdminAILogItem{
			ID:             e.ID,
			OrganizationID: e.OrganizationID,
			OrgName:        orgName,
			OrgType:        orgType,
			UserID:         e.UserID,
			UserName:       userName,
			UserEmail:      userEmail,
			Capability:     e.Capability,
			Feature:        e.Feature,
			FeatureName:    featName,
			Model:          e.Model,
			RequestID:      e.RequestID,
			InputTokens:    e.InputTokens,
			OutputTokens:   e.OutputTokens,
			TotalTokens:    e.TotalTokens(),
			CostUSD:        e.CostUSD(),
			CostKnown:      e.CostKnown,
			DurationMS:     e.DurationMS,
			Status:         e.Status,
			StatusLabel:    statusLabel,
			FinishReason:   e.FinishReason,
			ErrorMessage:   e.ErrorMessage,
			FromCache:      e.FromCache,
			Fallback:       e.Fallback,
			CreatedAt:      e.CreatedAt,
		})
	}

	var successRate float64
	if summary.Requests > 0 {
		successRate = (float64(summary.Succeeded) / float64(summary.Requests)) * 100
	}

	data := pages.AdminAILogsData{
		Logs:          items,
		Organizations: orgs,
		TotalRequests: summary.Requests,
		TotalTokens:   summary.TotalTokens(),
		TotalCostUSD:  summary.CostUSD(),
		SuccessCount:  summary.Succeeded,
		FailedCount:   summary.Failed,
		CachedCount:   summary.Cached,
		FallbackCount: summary.FellBack,
		SuccessRate:   successRate,
		FilterOrgID:   orgID,
		FilterFeature: feature,
		FilterModel:   model,
		FilterStatus:  status,
		FilterSearch:  query,
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		Page:          page,
		PerPage:       limit,
		TotalCount:    totalCount,
	}

	h.renderPage(ctx, w, "render admin ai logs", pages.AdminAILogsPage(data, lang, dir))
}
