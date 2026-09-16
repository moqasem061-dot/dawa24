package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// MessagesPage renders the two-pane messaging surface.
func (h *UIHandler) MessagesPage(w http.ResponseWriter, r *http.Request) {
	h.renderMessages(w, r, 0)
}

// MessagesConversationPage renders a specific thread inside the two-pane.
func (h *UIHandler) MessagesConversationPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/messages", http.StatusSeeOther)
		return
	}
	h.renderMessages(w, r, id)
}

func (h *UIHandler) renderMessages(w http.ResponseWriter, r *http.Request, activeID int64) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/messages", http.StatusSeeOther)
		return
	}

	data := pages.MessagesData{CurrentOrgID: actor.OrganizationID}
	if h.chatSvc != nil && actor.OrganizationID > 0 {
		convs, _ := h.chatSvc.ListConversations(ctx, actor.OrganizationID, 50, 0)
		for _, c := range convs {
			data.Items = append(data.Items, pages.ChatListItem{
				Conversation: c,
				OtherName:    h.otherPartyName(ctx, c, actor.OrganizationID),
			})
			if activeID != 0 && c.ID == activeID {
				data.Active = c
				data.ActiveOther = h.otherPartyName(ctx, c, actor.OrganizationID)
			}
		}
		if data.Active != nil {
			data.Thread, _ = h.chatSvc.GetThread(ctx, data.Active.ID, 100)
			_ = h.chatSvc.MarkRead(ctx, data.Active.ID, actor.OrganizationID)
		}
	}

	h.renderPage(ctx, w, "render messages page", pages.MessagesPage(lang, dir, data))
}

// otherPartyName resolves the display name of the conversation counterparty.
func (h *UIHandler) otherPartyName(ctx context.Context, c *chat.Conversation, myOrgID int64) i18n.Text {
	other := c.CounterpartyOrgID
	if c.OrganizationID != myOrgID {
		other = c.OrganizationID
	}
	if h.orgSvc != nil {
		if o, err := h.orgSvc.GetOrganization(ctx, other); err == nil && o != nil {
			return o.TradeName
		}
	}
	return i18n.New(fmt.Sprintf(i18n.TDefault("w4_ui.d_27"), c.ID), fmt.Sprintf("Conversation %d", c.ID))
}

// MessagesSendSubmit appends a message and returns to the thread.
func (h *UIHandler) MessagesSendSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/messages", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	body := r.PostFormValue("body")
	if h.chatSvc != nil && id > 0 && body != "" {
		var orgPtr *int64
		if actor.OrganizationID > 0 {
			orgPtr = &actor.OrganizationID
		}
		_, _ = h.chatSvc.SendMessage(ctx, id, actor.UserID, orgPtr, body)
	}
	http.Redirect(w, r, fmt.Sprintf("/messages/%d", id), http.StatusSeeOther)
}

// SupplierMessageSubmit opens (or continues) a conversation with a supplier.
func (h *UIHandler) SupplierMessageSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect="+r.Referer(), http.StatusSeeOther)
		return
	}
	if actor.OrganizationID <= 0 {
		h.redirectWithNotice(w, r, "/suppliers", "error", i18n.T(lang, "chat.org_required_to_message"))
		return
	}

	supplierID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || h.chatSvc == nil {
		h.redirectWithNotice(w, r, "/suppliers", "error", i18n.T(lang, "chat.cannot_start_conversation"))
		return
	}
	// A conversation between a company and itself has no second party, and
	// StartConversation would create one with the buyer and the supplier on
	// the same row.
	if ownedByBuyer(buyerOrgID(ctx), supplierID) {
		h.redirectWithNotice(w, r, "/suppliers", "error", i18n.T(lang, "err.own_organization_supply"))
		return
	}

	c, err := h.chatSvc.StartConversation(ctx, actor.OrganizationID, supplierID, actor.UserID, i18n.New(i18n.TDefault("w4_ui.s_77_77"), "Inquiry"), chat.ContextGeneral, nil)
	if err != nil {
		h.redirectWithNotice(w, r, "/suppliers/"+strconv.FormatInt(supplierID, 10), "error", h.safeMessage(err, lang))
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/messages/%d", c.ID), http.StatusSeeOther)
}
