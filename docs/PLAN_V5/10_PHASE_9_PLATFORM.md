# PHASE 9 — Platform Capabilities

**Depends on:** Phases 0–8.
**Tasks:** 8.

Cross-cutting capabilities Laravel has and Go does not. Several are security
features that are half-built — the most dangerous kind.

---

## TASK 9.1 — Two-factor authentication

**Current state:** `identity.user_mfa` exists. `AdminResetMFA` exists and is
routed. There is **no enrollment screen and no login challenge**. An admin can
reset a second factor that no user can ever set.

### Inspect
```bash
cat F:/Dawa\ 24/Laravel/app/Services/Google2FAService.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Auth/TwoFactorChallenge.php
grep -rn "2fa\|two_factor\|google2fa" F:/Dawa\ 24/Laravel/routes/ F:/Dawa\ 24/Laravel/app/Http/Middleware/
```

### Build
| Route | Purpose | Audience |
|---|---|---|
| `/settings/security/2fa` | enrollment: QR code, secret, verify a code to activate | shared |
| `POST /settings/security/2fa/enable` | verify + activate | shared |
| `POST /settings/security/2fa/disable` | requires a current code or password | shared |
| `POST /settings/security/2fa/recovery` | regenerate recovery codes | shared |
| `/auth/2fa-challenge` | the login challenge | public (post-password, pre-session) |
| `POST /auth/2fa-challenge` | verify | public |

### Requirements
- TOTP (RFC 6238), 30s window, ±1 step tolerance
- Secret stored **encrypted at rest** — check what `internal/platform` provides
  for encryption; if nothing, add it and document the key source
- QR code generated server-side; never send the secret to a third-party QR service
- Recovery codes: single-use, hashed at rest, shown exactly once
- **Rate-limit the challenge**: lock after N failures, matching Laravel's N
- The login flow must not issue a full session until the challenge passes —
  inspect `identity.NewSessionStore` and add a pending-MFA session state
- Admin reset (already built) must invalidate all sessions (it does — verify)

### Tests
- T1: TOTP verification, including clock skew and replay rejection
- T6: enroll → logout → login → challenge → success
- T6b: wrong code N times → locked
- T20: a user with MFA enabled cannot obtain a session by password alone

---

## TASK 9.2 — Sessions & device tracking

**Current state:** `identity.session_plans` exists and is read-only.
`POST /settings/security/plan/{id}` exists with no admin side and no request
queue. `/what-in` advertises "تتبع الأجهزة المتعددة (Multi-session & Device UUID)".

Missing tables (audit §6.1): `user_sessions`, `user_session_histories`,
`session_plan_requests`.

### Inspect
```bash
cat F:/Dawa\ 24/Laravel/app/Services/SessionService.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Admin/SessionPlans/{Index,Create,Edit,Show,RequestsIndex,RequestsShow}.php
sed -n "/CREATE TABLE \`user_sessions\`/,/ENGINE=/p"        F:/Dawa\ 24/u924222867_Testv5.sql
sed -n "/CREATE TABLE \`session_plan_requests\`/,/ENGINE=/p" F:/Dawa\ 24/u924222867_Testv5.sql
```

### Build
- `identity.user_sessions`: device UUID, name, type, platform, browser, IP,
  user agent, country, city, `logged_in_at`, `last_activity_at`, `logged_out_at`
- `identity.user_session_histories`: the archive
- `identity.session_plan_requests`: request → admin approval → seat granted
- `/settings/security` shows the active session list with device details and a
  per-session revoke (the revoke endpoint already exists)
- Concurrent-session cap enforced from the user's session plan; behaviour on
  exceeding it must match Laravel (refuse or evict — same question as Phase 2
  Task 2.1.5; **use the same answer**)
- Admin: `/admin/session-plan` + create/edit/show + `/requests` + `/requests/{id}`

### Tests
- T1: cap enforcement
- T3: a user cannot revoke another user's session
- T6: sign in on two devices → both listed → revoke one → that session 401s on its next request

---

## TASK 9.3 — Arabic PDF invoices

`/what-in` grants "Arabic PDF Invoice Generation" to **all three** roles.
Laravel: `GET /pdf/{orderId}` + `resources/views/pdfs/`. Go: absent.

### Inspect
```bash
sed -n '277,331p' F:/Dawa\ 24/Laravel/routes/web.php
ls F:/Dawa\ 24/Laravel/resources/views/pdfs/
cat F:/Dawa\ 24/Laravel/app/Models/Invoice.php
```

### Build
- `GET /invoices/{id}/pdf` — shared audience, authorised to the owning org only
- `GET /orders/{id}/pdf` if Laravel generates from the order
- Arabic RTL PDF with a font that renders Arabic correctly (embed the font;
  do not rely on a system font)
- Contents must match Laravel's template: header, org details, customer details,
  line items, totals, tax, terms
- Money via `money.Amount`, exact (T8)

**Library choice:** pick one that handles RTL and embedded fonts. Verify Arabic
shaping (letters must join) with a visual check on a real invoice before
declaring this done — a PDF with disconnected Arabic letters is a failure.

### Tests
- T3: org B cannot fetch org A's invoice PDF
- T6: the PDF generates and contains the expected totals
- T21: manual visual verification of Arabic shaping, recorded in `PROGRESS.md`

---

## TASK 9.4 — AI providers registry

**Current state:** the gateway exists. `ai_providers` table is absent, so there
is no per-provider configuration, no health state, and no fallback cascade.

Laravel has six provider services and an `ai_providers` table with
`is_active`, `is_working`, `last_error`, `model_name`, `context_length`,
`price_per_1k`, `base_url`, `config_key`, `config_value`, `meta`.

### Build
- `db/migrations/NNN_ai_providers.up.sql` — the table, in the `platform_admin`
  schema (the gateway is platform infrastructure)
- **Rule R2 still applies**: provider *names* live in the database and in
  `internal/platform/gateway/` only. No module may read a provider name.
  `make check-provider-isolation` must stay green — verify it does not just grep
  for literals, since names now come from the DB.
- Cascade: ordered provider list per capability; on failure, mark `is_working =
  false`, record `last_error`, try the next; circuit-break with a cooldown
- `/admin/developers` (AI tab) already exists — extend it to manage providers:
  add/edit/enable/disable, test a provider, view last error, reorder the cascade
- **Never render an API key back to the browser.** Masked + replace only.

### Tests
- T7: every capability still works with all providers disabled (the deterministic fallback)
- T7b: provider 1 fails → provider 2 is used → `is_working` and `last_error` are recorded
- `make check-provider-isolation` green

---

## TASK 9.5 — Notifications & email

`notifications.logs`, `notifications.templates`, `notifications.admin_notifications`
exist. Laravel has `organization_notifications`, `admin_notifications`, and
email delivery, plus `Employee/VendorNotificationBell` and
`Admin/NotificationBell`.

### Build
- Verify Go's notification bell partials (`/notifications/dropdown`,
  `/notifications/unread-badge`) match Laravel's behaviour and counts
- **Email delivery**: determine whether Go sends any email at all. `/what-in`
  says "إشعارات فورية عبر البريد الإلكتروني واللوحة". If there is no mailer,
  add one to `internal/platform/`, with templates in `notifications.templates`,
  bilingual, RTL-safe HTML.
- Notification triggers must match Laravel's — read the Laravel notification
  classes and the observers to enumerate every event that notifies someone.
  **List them in `docs/modules/notifications.md`** and implement each.
- Admin: send a notification to a segment, if Laravel supports it.

---

## TASK 9.6 — Subscription lifecycle

Missing tables (audit §6.1): `plan_types`, `subscription_histories`,
`subscription_users`, `user_plan_histories`.

`billing.subscriptions` has no history. Add the tables, wire them to the
subscription service so every state change is recorded, and surface the history
on `/admin/plans/subscriptions` (Phase 5 Task 5.10) and on the user's own
billing screen.

---

## TASK 9.7 — Maintenance mode & system resources

Laravel middleware: `checkMaintenance`, `system.resource`, `checkUserStatus`,
`checkVerification`, `checkEmployeeStatus`, `check_admins_control`,
`first_admin_login`, `redirectIfVerified`, `lang`, `visitor`, `role_organization`.

**Audit which of these Go has.** For each missing one, decide: is it needed, or
is it covered by an existing gate? Record the mapping in
`docs/modules/platform.md`.

Specifically:
- `checkMaintenance` — a global maintenance toggle with a staff bypass
- `system.resource` — per-resource availability (Phase 5 Task 5.9d)
- `checkVerification` — email/phone verification gate. **Does Go verify email at
  all?** If not, that is a missing capability, not just a missing middleware.
- `check_admins_control` — inspect; it appears to let an admin impersonate or
  restrict a customer/employee. If it is impersonation, it needs a full audit
  trail and its own permission.

---

## TASK 9.8 — Report issues & contact

`workflow.report_issues` is a dead table. Laravel has `report_issues` and a
flow. Build the customer/vendor "report an issue" action and the admin queue.
Verify `/admin/messages` (contact-us) is wired and matches
`ContactUsIndex`/`ContactUsShow`.

---

## PHASE 9 COMPLETION GATE

```bash
make check && make check-provider-isolation && go test ./... -race
```

- [ ] A user can enable 2FA and is challenged at login
- [ ] Active sessions are listed with device details and can be revoked
- [ ] Arabic invoice PDFs render with correctly-joined Arabic (visually verified)
- [ ] The AI cascade falls over between providers and still works with all disabled
- [ ] Email is sent for every event Laravel sends email for
- [ ] No secret or API key is ever rendered to a browser
- [ ] Every Laravel middleware is either implemented or explicitly mapped to an existing gate
- [ ] `workflow.report_issues` is no longer dead
- [ ] `PROGRESS.md` updated for 9.1–9.8
