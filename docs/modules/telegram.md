# telegram

Telegram as a second interface to **Capsule** (the assistant) and to the
**in-app notification feed**. Not a second assistant, not a second permission
system.

- Code: `internal/modules/telegram` (service, bot, outbox), `…/postgres`,
  `…/http` (the bridge n8n calls), wiring in `cmd/server/telegram_wiring.go`,
  settings card in `internal/ui/settings_telegram_handlers.go` +
  `internal/ui/pages/settings_telegram.templ`.
- The channel-neutral flow (actor rebuild, gates, confirm path, `/whoami`
  `/org` `/notify`, notification eligibility, outbox SQL, bearer check) lives
  in `internal/modules/chatbridge`, shared with [whatsapp](whatsapp.md).
- Schema: `telegram` (migration 213). Every table is `FORCE ROW LEVEL SECURITY`
  with a system-only policy; the repository scopes each statement by user/link.
- n8n workflows (instance `n8n-u74003.vm.elestio.app`):
  - `Dawa24 Telegram · Capsule Assistant` (`2TPiekUTjEBRtGqR`)
  - `Dawa24 Telegram · Notification Delivery` (`80YRnwq3utnuINDQ`)

## Who owns what

| Concern | Owner |
|---|---|
| Who the Telegram user is | Dawa24: `telegram.links`, created only through browser confirmation |
| Role, permissions, منشأة, branch, account status | Dawa24: `rbac.Resolver`, re-read on **every** message and every notification |
| Whether the assistant may be used | Dawa24: `assistant.Allowed` (the same gate as `/api/v1/assistant/*`) plus the approved-منشأة rule |
| Tools, data access, audit, conversations | Dawa24: `assistant.Service.Ask` → `RunTurn`, identical to the drawer |
| Who receives a notification | Dawa24: existing fan-out into `notifications.logs`, re-checked at delivery |
| Receiving updates and sending messages | n8n |

n8n never decides identity or eligibility, and holds no Dawa24 data beyond the
message in flight.

## Linking (no password in Telegram)

1. Signed in, the user opens **Settings → تيليجرام → ربط تيليجرام**. Dawa24
   issues a one-time code: 32 random bytes, stored as SHA-256 only, valid 10
   minutes, a new code retires older ones, 5 codes per 10 minutes.
2. The deep link `t.me/<bot>?start=<code>` sends `/start <code>`. Dawa24 spends
   the code once and records a **pending** link. The bot reveals nothing about
   whose account it was.
3. The settings card (polling every 4s with `X-Dawa-Background`, so it does not
   defeat idle logout) shows the Telegram account that opened the code. The
   user confirms or rejects. **Nothing is answered or delivered until then.**
   This is what makes a leaked or shoulder-surfed link harmless.
4. A Telegram account can speak for one Dawa24 user; a user has one confirmed
   Telegram account (confirming a new one revokes the old). `/unlink` in the bot
   or the settings card revokes and drops queued messages.

Only private chats are served. Group, supergroup and channel updates are ignored
entirely, because an answer is a pharmacy's or supplier's trading position.

## A message

`POST /api/v1/integrations/telegram/updates` (Bearer `TELEGRAM_BRIDGE_TOKEN`)
with the raw Telegram update. Dawa24:

1. de-duplicates by `update_id`;
2. finds the live link for `from.id` (must equal `chat.id`);
3. rebuilds the actor with `rbac.Resolver` for the link's active منشأة
   (`authctx.FromGrant`, the same overlay the browser session uses), refusing a
   suspended account or an ended membership;
4. applies the approved-منشأة rule, `assistant.Allowed`, and the per-user question
   limit shared with the browser drawer;
5. takes a one-question-at-a-time lock (`links.busy_until`);
6. calls `assistant.Service.Ask` under `database.WithTenant` + the actor, never
   under the system context;
7. renders the Markdown answer to Telegram HTML (escape first, balanced tags,
   http(s) links only, split under 4096 UTF-16 units) and returns the messages
   for n8n to send.

Branch: a member bound to a branch gets `actor.BranchID` from the resolver, and
Capsule's branch-scoped tools already honour it. The web "buying branch"
selector is a checkout preference and is not used by the assistant on any
interface.

## Buttons and files (Capsule v2)

An answer can carry proposals and exports (`Answer.Proposals`, `Answer.Files`):

- A proposal becomes a message with ✅ تأكيد / ✖️ إلغاء inline buttons,
  `callback_data` = `act:c:<uuid>` or `act:x:<uuid>`. Preview text is escaped,
  including the plain-text fallback for an oversized card.
- A press arrives as `callback_query`. `handleCallback` requires a private chat
  whose id equals the presser. It then checks the live link, `resolveActor`,
  approval and tenant context, and calls `assistant.Decide`, which runs the full
  confirm-time re-check (`05_CAPSULE_V2.md`). Every press from a private chat is answered with
  `answer_callback`, so the button stops spinning.
- An export becomes `document: {url, filename}`, with the URL
  `BASE_URL/api/v1/integrations/telegram/exports/<token>`. n8n fetches it with
  the bridge bearer. The bridge serves it only while the export's owner has an
  active link.

Commands: `/whoami`, `/org` (list and switch among live memberships; staff also
get `0` for the platform scope), `/new`, `/notify`, `/unlink`, `/help`.

## Notifications

`POST …/outbox/claim` (n8n, every 20s):

1. Candidates: `notifications.logs` in-app rows for users with an **active**
   link, created after confirmation and within 2 hours, with no decision yet. No
   cursor — a late-committing row is still picked up.
2. Each gets one `telegram.deliveries` row recording the decision: `queued`, or
   `dropped` with `muted`, `offers_preference_off`, `account_inactive`,
   `membership_ended` or `permission_revoked`. Eligibility is the organisation
   fan-out's own rule (owner, platform staff, or holder of the permission),
   evaluated against the notification's منشأة (or the chat's active one when the
   row has none).
3. Due rows are leased (`FOR UPDATE SKIP LOCKED`, 2 minutes).

`POST …/outbox/report`: `ok` → sent; blocked/deactivated/chat not found → failed
and the link becomes `blocked` until the user writes again; `retry after N` →
requeued after N s; unparseable → failed; anything else → exponential backoff,
5 attempts.

Categories (`/notify` and the settings card) come from the notification's
`required_permission` resource segment (`order`, `wallet`, `delivery`, `offer`,
`organization`, …). The account-wide `notification_topics.offers = false` also
stops offers here.

## Configuration

| Env | Purpose |
|---|---|
| `TELEGRAM_BOT_USERNAME` | Bot name without `@`, for deep links |
| `TELEGRAM_BRIDGE_TOKEN` | ≥32 chars; n8n's Bearer secret. `openssl rand -hex 32` |

Both or neither; half-configured refuses to start. With neither, no bridge
routes are mounted and the settings tab is hidden.

n8n: a **Bearer Auth** credential holding `TELEGRAM_BRIDGE_TOKEN` (the
workflows use "Bearer Auth account"). Set the HTTP URLs to the deployed domain,
including the exports prefix in the Capsule workflow's **Fetch Export** node,
then activate both workflows. Deactivate any other workflow
with a Telegram Trigger on the same bot — Telegram allows one webhook per bot.

## Invariants and traps

- **Never** give the bot an answer path that skips `resolveActor` → `Allowed` →
  `Ask`. `Ask` re-applies the gate itself; `TestAskRefusesCallersTheBrowserWouldRefuse`.
- Authority changes must bump `identity.rbac_version` or the resolver serves a
  stale grant for up to 2 minutes, and Telegram has no session to revoke. User
  status, platform role, account deletion, member toggle/edit/removal bump it as
  of this change; role edits already did.
- A member whose role row is missing or soft-deleted now resolves to no
  permissions (was: a scan error, which made the browser fall back to the
  session's stale permission copy).
- `/api/v1/integrations/telegram/` is in `httpx.longRunningPrefixes`: a question
  waits for the full turn (≤150s). n8n's timeout is 180s; keep its retries off.

## Tests

- Unit: `internal/modules/telegram` (bot flows, gates, linking, outbox,
  rendering, buttons and files in `actions_test.go`), `…/http` (bridge auth), `authctx` (`FromGrant`), `assistant`
  (`Ask` gate).
- Postgres (`TEST_DATABASE_URL` only — they write fixtures):
  `internal/modules/telegram/postgres` and `cmd/server` `TestTelegramEndToEnd`,
  which drives the bridge over HTTP against the real resolver, assistant service,
  tool registry and audit trail with only the model scripted.
