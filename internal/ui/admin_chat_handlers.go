package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminChatHistoryPage renders audit list of all AI Assistant conversations with stats, search and pagination.
func (h *UIHandler) AdminChatHistoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	query := r.URL.Query()
	q := query.Get("q")
	page := pagination.PageNumber(r)
	perPage := pagination.RowsPerPage(r)
	offset := (page - 1) * perPage

	filter := assistant.AdminConversationFilter{
		Search:            q,
		UserQuery:         strings.TrimSpace(query.Get("user_q")),
		OrganizationQuery: strings.TrimSpace(query.Get("org_q")),
		DateFrom:          parseFilterDate(query.Get("date_from")),
		DateTo:            exclusiveEndOfDay(query.Get("date_to")),
		FlaggedOnly:       query.Get("flagged") == "1",
		Limit:             perPage,
		Offset:            offset,
	}

	var aiConvs []*assistant.ConversationSummary
	var totalCount int
	var stats *assistant.AssistantStats

	if h.assistantRepo != nil {
		var err error
		aiConvs, totalCount, err = h.assistantRepo.ListAllConversationsFiltered(database.AsSystem(ctx), filter)
		if err != nil {
			h.log.ErrorContext(ctx, "admin chat history: list conversations failed", "error", err)
		}
		stats, err = h.assistantRepo.GetAssistantStats(database.AsSystem(ctx))
		if err != nil {
			h.log.ErrorContext(ctx, "admin chat history: get stats failed", "error", err)
		}
	}

	data := pages.AdminChatHistoryData{
		SearchQuery: q,
		UserQuery:   filter.UserQuery,
		OrgQuery:    filter.OrganizationQuery,
		DateFrom:    strings.TrimSpace(query.Get("date_from")),
		DateTo:      strings.TrimSpace(query.Get("date_to")),
		FlaggedOnly: filter.FlaggedOnly,
		Page:        page,
		PerPage:     perPage,
		TotalCount:  totalCount,
		Stats:       stats,
		AIConvs:     aiConvs,
	}

	h.renderPage(ctx, w, "render admin chat history", pages.AdminChatHistoryPage(data, lang, dir))
}

// AdminAIChatDetailPage renders full messages transcript for an AI assistant conversation.
func (h *UIHandler) AdminAIChatDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	convID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || convID <= 0 {
		http.Redirect(w, r, "/admin/chat/history", http.StatusSeeOther)
		return
	}

	var summary *assistant.ConversationSummary
	var msgs []*assistant.Message
	if h.assistantRepo != nil {
		summary, _ = h.assistantRepo.GetConversationSummary(database.AsSystem(ctx), convID)
		msgs, _ = h.assistantRepo.ListMessages(database.AsSystem(ctx), convID, 200)
	}

	if summary == nil {
		http.Redirect(w, r, "/admin/chat/history", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render admin ai chat detail", pages.AdminAIChatDetailPage(summary, msgs, lang, dir))
}
