package ui

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/modules/whatsapp"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The WhatsApp card on the settings page, the counterpart of the Telegram
// card: every action is the signed-in user acting on their own link, and
// linking grants nothing by itself.

// SetWhatsApp installs the WhatsApp bridge. Nil leaves the card reporting the
// integration as not enabled.
func (h *UIHandler) SetWhatsApp(svc *whatsapp.Service) { h.whatsapp = svc }

func (h *UIHandler) whatsappEnabled() bool { return h.whatsapp != nil && h.whatsapp.Enabled() }

// SettingsWhatsAppCard renders the card. With ?until= it is the poll issued
// while a fresh link waits to be opened: it answers 204 (nothing to swap)
// until the link is opened or has expired.
func (h *UIHandler) SettingsWhatsAppCard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	lang := langOf(r)
	data := pages.WhatsAppSettingsData{Enabled: h.whatsappEnabled()}
	if data.Enabled {
		link, err := h.whatsapp.CurrentLink(ctx, actor.UserID)
		if err != nil {
			h.log.ErrorContext(ctx, "whatsapp settings: load link", "error", err)
			data.Notice, data.NoticeKind = i18n.T(lang, "settings.tg_error"), "danger"
		}
		data.Link = link
		if until, err := strconv.ParseInt(r.URL.Query().Get("until"), 10, 64); err == nil &&
			link == nil && time.Now().Before(time.Unix(until, 0)) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	h.renderWhatsAppCard(w, r, data)
}

// SettingsWhatsAppLinkSubmit issues a one-time link code.
func (h *UIHandler) SettingsWhatsAppLinkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	lang := langOf(r)
	data := pages.WhatsAppSettingsData{Enabled: h.whatsappEnabled()}
	if data.Enabled {
		code, err := h.whatsapp.StartLink(ctx, actor)
		switch {
		case errors.Is(err, whatsapp.ErrTooManyCodes):
			data.Notice, data.NoticeKind = i18n.T(lang, "settings.tg_too_many"), "danger"
		case err != nil:
			h.log.ErrorContext(ctx, "whatsapp settings: start link", "error", err, "user_id", actor.UserID)
			data.Notice, data.NoticeKind = i18n.T(lang, "settings.tg_error"), "danger"
		default:
			data.DeepLink, data.ExpiresAt = code.DeepLink, code.ExpiresAt
		}
	}
	h.renderWhatsAppCard(w, r, data)
}

// SettingsWhatsAppConfirmSubmit confirms the user's own pending link.
func (h *UIHandler) SettingsWhatsAppConfirmSubmit(w http.ResponseWriter, r *http.Request) {
	h.whatsappAction(w, r, "settings.wa_linked_ok", func(actor authctx.Actor) error {
		_, err := h.whatsapp.ConfirmLink(r.Context(), actor, r.PostFormValue("link"))
		return err
	})
}

// SettingsWhatsAppUnlinkSubmit revokes the user's link, or rejects a pending
// one they do not recognise.
func (h *UIHandler) SettingsWhatsAppUnlinkSubmit(w http.ResponseWriter, r *http.Request) {
	h.whatsappAction(w, r, "settings.wa_unlinked_ok", func(actor authctx.Actor) error {
		return h.whatsapp.Unlink(r.Context(), actor)
	})
}

// SettingsWhatsAppNotifySubmit saves which categories reach WhatsApp. The form
// posts the categories that are ON; everything else is muted.
func (h *UIHandler) SettingsWhatsAppNotifySubmit(w http.ResponseWriter, r *http.Request) {
	h.whatsappAction(w, r, "settings.tg_saved", func(actor authctx.Actor) error {
		if err := r.ParseForm(); err != nil {
			return err
		}
		on := map[string]bool{}
		for _, v := range r.PostForm["on"] {
			on[v] = true
		}
		var muted []chatbridge.Category
		for _, c := range chatbridge.Categories {
			if !on[string(c)] {
				muted = append(muted, c)
			}
		}
		return h.whatsapp.SetMuted(r.Context(), actor, muted)
	})
}

func (h *UIHandler) whatsappAction(w http.ResponseWriter, r *http.Request, okKey string, act func(authctx.Actor) error) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	lang := langOf(r)
	data := pages.WhatsAppSettingsData{Enabled: h.whatsappEnabled()}
	if data.Enabled {
		err := act(actor)
		switch {
		case errors.Is(err, whatsapp.ErrNoPendingLink):
			data.Notice, data.NoticeKind = i18n.T(lang, "settings.tg_confirm_expired"), "danger"
		case err != nil:
			h.log.ErrorContext(ctx, "whatsapp settings action", "error", err, "user_id", actor.UserID)
			data.Notice, data.NoticeKind = i18n.T(lang, "settings.tg_error"), "danger"
		default:
			data.Notice, data.NoticeKind = i18n.TKey(lang, okKey), "success"
		}
		if data.Link, err = h.whatsapp.CurrentLink(ctx, actor.UserID); err != nil {
			h.log.ErrorContext(ctx, "whatsapp settings: reload link", "error", err)
		}
	}
	h.renderWhatsAppCard(w, r, data)
}

func (h *UIHandler) renderWhatsAppCard(w http.ResponseWriter, r *http.Request, data pages.WhatsAppSettingsData) {
	w.Header().Set("Cache-Control", "no-store")
	h.renderPage(r.Context(), w, "render whatsapp settings card", pages.WhatsAppSettingsCard(data, langOf(r)))
}
