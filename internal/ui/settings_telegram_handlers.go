package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/telegram"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/chatbridge"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
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
		if until, parseErr := strconv.ParseInt(r.URL.Query().Get("until"), 10, 64); parseErr == nil && err == nil &&
			link == nil && time.Now().Before(time.Unix(until, 0)) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	h.renderTelegramCard(w, r, data)
}

// telegramLinkEvent is the browser event that carries the bot link back to the
// page. static/js/telegram-connect.js listens for it.
const telegramLinkEvent = "telegram-link"

// SettingsTelegramLinkSubmit issues a one-time link code.
//
// The browser opens a blank tab inside the click, because a tab opened after
// the answer arrives is blocked. This response names the link in an HX-Trigger
// event so the page can point that tab at the bot straight away. An empty URL
// means there is nothing to open, and the page closes the tab.
func (h *UIHandler) SettingsTelegramLinkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	lang := langOf(r)
	data := pages.TelegramSettingsData{Enabled: h.telegramEnabled()}
	link := ""
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
			link = code.DeepLink
		}
	}
	setTelegramLinkTrigger(w, link)
	h.renderTelegramCard(w, r, data)
}

// setTelegramLinkTrigger announces the link to the page. The JSON is encoded,
// not formatted, so no character in the link can break the header.
func setTelegramLinkTrigger(w http.ResponseWriter, link string) {
	// A map of strings always encodes; the error path is unreachable.
	raw, _ := json.Marshal(map[string]map[string]string{telegramLinkEvent: {"url": link}})
	w.Header().Set("HX-Trigger", string(raw))
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
