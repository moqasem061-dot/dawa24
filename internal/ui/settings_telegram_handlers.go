package ui

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/modules/telegram"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The Telegram card on the settings page.
//
// Every action here is the signed-in user acting on their own link: the user
// id comes from the authenticated actor, never from the form, and the service
// scopes each statement to it. Linking a Telegram account grants nothing by
// itself — what the bot may answer is decided per message by the same
// permission resolver as every page.

// SetTelegram installs the Telegram bridge. Nil leaves the card reporting the
// integration as not enabled.
func (h *UIHandler) SetTelegram(svc *telegram.Service) { h.telegram = svc }

func (h *UIHandler) telegramEnabled() bool { return h.telegram != nil && h.telegram.Enabled() }

// SettingsTelegramCard renders the card. With ?until= it is the poll issued
// while a fresh link waits to be opened: it answers 204 (nothing to swap)
// until the link is opened or has expired.
func (h *UIHandler) SettingsTelegramCard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	lang := langOf(r)
	data := pages.TelegramSettingsData{Enabled: h.telegramEnabled()}
	if data.Enabled {
		link, err := h.telegram.CurrentLink(ctx, actor.UserID)
		if err != nil {
			h.log.ErrorContext(ctx, "telegram settings: load link", "error", err)
			data.Notice, data.NoticeKind = i18n.T(lang, "settings.tg_error"), "danger"
		}
		data.Link = link
		if until, err := strconv.ParseInt(r.URL.Query().Get("until"), 10, 64); err == nil &&
			link == nil && time.Now().Before(time.Unix(until, 0)) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	h.renderTelegramCard(w, r, data)
}

// SettingsTelegramLinkSubmit issues a one-time link code.
func (h *UIHandler) SettingsTelegramLinkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	lang := langOf(r)
	data := pages.TelegramSettingsData{Enabled: h.telegramEnabled()}
	if data.Enabled {
		code, err := h.telegram.StartLink(ctx, actor)
		switch {
		case errors.Is(err, telegram.ErrTooManyCodes):
			data.Notice, data.NoticeKind = i18n.T(lang, "settings.tg_too_many"), "danger"
		case err != nil:
			h.log.ErrorContext(ctx, "telegram settings: start link", "error", err, "user_id", actor.UserID)
			data.Notice, data.NoticeKind = i18n.T(lang, "settings.tg_error"), "danger"
		default:
			data.DeepLink, data.ExpiresAt = code.DeepLink, code.ExpiresAt
		}
	}
	h.renderTelegramCard(w, r, data)
}

// SettingsTelegramConfirmSubmit confirms the user's own pending link.
func (h *UIHandler) SettingsTelegramConfirmSubmit(w http.ResponseWriter, r *http.Request) {
	h.telegramAction(w, r, "settings.tg_linked_ok", func(actor authctx.Actor) error {
		_, err := h.telegram.ConfirmLink(r.Context(), actor, r.PostFormValue("link"))
		return err
	})
}

// SettingsTelegramUnlinkSubmit revokes the user's link, or rejects a pending
// one they do not recognise.
func (h *UIHandler) SettingsTelegramUnlinkSubmit(w http.ResponseWriter, r *http.Request) {
	h.telegramAction(w, r, "settings.tg_unlinked_ok", func(actor authctx.Actor) error {
		return h.telegram.Unlink(r.Context(), actor)
	})
}

// SettingsTelegramNotifySubmit saves which categories reach Telegram. The form
// posts the categories that are ON; everything else is muted.
func (h *UIHandler) SettingsTelegramNotifySubmit(w http.ResponseWriter, r *http.Request) {
	h.telegramAction(w, r, "settings.tg_saved", func(actor authctx.Actor) error {
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
		return h.telegram.SetMuted(r.Context(), actor, muted)
	})
}

func (h *UIHandler) telegramAction(w http.ResponseWriter, r *http.Request, okKey string, act func(authctx.Actor) error) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	lang := langOf(r)
	data := pages.TelegramSettingsData{Enabled: h.telegramEnabled()}
	if data.Enabled {
		err := act(actor)
		switch {
		case errors.Is(err, telegram.ErrNoPendingLink):
			data.Notice, data.NoticeKind = i18n.T(lang, "settings.tg_confirm_expired"), "danger"
		case err != nil:
			h.log.ErrorContext(ctx, "telegram settings action", "error", err, "user_id", actor.UserID)
			data.Notice, data.NoticeKind = i18n.T(lang, "settings.tg_error"), "danger"
		default:
			data.Notice, data.NoticeKind = i18n.TKey(lang, okKey), "success"
		}
		if data.Link, err = h.telegram.CurrentLink(ctx, actor.UserID); err != nil {
			h.log.ErrorContext(ctx, "telegram settings: reload link", "error", err)
		}
	}
	h.renderTelegramCard(w, r, data)
}

func (h *UIHandler) renderTelegramCard(w http.ResponseWriter, r *http.Request, data pages.TelegramSettingsData) {
	w.Header().Set("Cache-Control", "no-store")
	h.renderPage(r.Context(), w, "render telegram settings card", pages.TelegramSettingsCard(data, langOf(r)))
}
