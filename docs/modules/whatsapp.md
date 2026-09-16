# whatsapp

WhatsApp as a further interface to **Capsule** and to the **in-app
notification feed**, with the same behaviour as [telegram](telegram.md). Not a
second assistant, not a second permission system.

- Code: `internal/modules/whatsapp` (service, bot, outbox, Cloud API payloads),
  `…/postgres`, `…/http` (the bridge n8n calls), wiring in
  `cmd/server/whatsapp_wiring.go`, settings card in
  `internal/ui/settings_whatsapp_handlers.go` +
  `internal/ui/pages/settings_whatsapp.templ`.
- Shared with Telegram: `internal/modules/chatbridge` — actor rebuild
  (`Core.Resolve`), approval, assistant gate, rate limit, one-question lock,
  confirm path (`Core.Ask`, `Core.Decide`), `/whoami` `/org` `/notify`,
  notification eligibility (`Core.DecideNotifications`), categories — plus
  `chatbridge/postgres` (codes, links, outbox lease/retry SQL) and
  `chatbridge/http` (bearer check). A security fix there applies to both
  channels; do not fork it into a channel package.
- Schema: `whatsapp` (migration 219), same shape as `telegram`, system-only RLS.
  219 also allows `channel = 'whatsapp'` on `assistant.pending_actions`.
- n8n workflows (instance `n8n-u74003.vm.elestio.app`), created inactive:
  - `Dawa24 WhatsApp · Capsule Assistant` (`HmXzZDz9tf8cTZ56`)
  - `Dawa24 WhatsApp · Notification Delivery` (`aLAByP7obEpSBGcC`)

## Differences from Telegram

| | Telegram | WhatsApp |
|---|---|---|
| Account id | `from.id` (int) | `wa_id`, the number without `+` (TEXT, `^[0-9]{6,20}$`) |
| Link code | `t.me/<bot>?start=<code>` | `wa.me/<number>?text=ربط Dawa24 <code>`; the user presses Send |
| De-duplication | `update_id` | message `id` (`wamid…`) in `whatsapp.processed_messages` |
| Groups | `chat.type != private` ignored | messages with `group_id` ignored |
| Formatting | HTML, escaped | `*bold*`, `` `code` ``, fenced blocks; nothing to escape |
| Buttons | inline keyboard, `callback_query` | interactive reply buttons (body ≤1024, title ≤20); a press is an `interactive.button_reply` message |
| Files | `sendDocument` | n8n fetches the export, the WhatsApp node uploads and sends it |
| Blocked | `my_chat_member` kicked | send errors 131026/131021/133010 → `blocked` until the user writes |
| Outbound | any time | free text only within **24 h** of the user's last message |

## The 24-hour window

`links.last_inbound_at` is set by every inbound message (and by the message
that carried the link code). A claim returns each delivery's payload:

- window open (last message < 23 h 30 m ago) → text;
- closed and `WHATSAPP_NOTIFICATION_TEMPLATE` set → that template, its single
  body parameter the flattened notification text (no newlines, tabs, `*`,
  runs of spaces; ≤900 characters);
- closed and no template → the delivery is dropped with `window_closed`.

If WhatsApp still answers 131047 (window closed), the report clears
`last_inbound_at` and requeues, so the next claim sends the template.

Template to create in WhatsApp Manager (category **Utility**, language `ar`):
`🔔 إشعار من Dawa24: {{1}}` — set its name in `WHATSAPP_NOTIFICATION_TEMPLATE`.

## Delivery reports

Codes are read from n8n's error text: 131026/131021/133010 → failed + link
blocked; 131047 → close window + retry; 130429/131056/131048/80007 → retry in
60 s; 100/131008/131009/1320xx template errors → failed, not retried; anything
else → exponential backoff, 5 attempts.

## Configuration

| Env | Purpose |
|---|---|
| `WHATSAPP_BUSINESS_NUMBER` | Business number, international, no `+` |
| `WHATSAPP_BRIDGE_TOKEN` | ≥32 chars; n8n's Bearer secret |
| `WHATSAPP_NOTIFICATION_TEMPLATE` | Optional approved template name |
| `WHATSAPP_TEMPLATE_LANGUAGE` | Template language, default `ar` |

Number and token both or neither. The n8n workflows use the existing
**Bearer Auth account** credential, so either set `WHATSAPP_BRIDGE_TOKEN` to the
`TELEGRAM_BRIDGE_TOKEN` value or give the WhatsApp workflows their own
credential.

n8n credentials: **WhatsApp Business Cloud** (`whatsAppApi`: permanent System
User token with `whatsapp_business_messaging`, WABA id) on every Graph node, and
**WhatsApp OAuth** (`whatsAppTriggerApi`: Meta App ID + secret) on the trigger.
Put the Phone Number ID in the notification workflow's Send Notification URL;
the Capsule workflow takes it from each webhook. Only one app webhook per
WhatsApp app — deactivate any other WhatsApp Trigger on the same app.

## Tests

- Unit: `internal/modules/whatsapp` (linking, gates, buttons, exports, `/org`,
  window/template choice, report classification, rendering), `…/http` (bearer).
- Postgres (`TEST_DATABASE_URL` only): `internal/modules/whatsapp/postgres`
  (link lifecycle, takeover refusal, window open/close, drop, block, revoke).
- Telegram's unit, Postgres and end-to-end tests cover the shared
  `chatbridge` flow.
