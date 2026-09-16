# Capsule — the read-only business agent, as shipped

**Date:** 2026-09-02
**Supersedes:** §3.6 (confirmation architecture) and §3.7 (agentic order flow) of
`02_CAPSULE_AGENT_ARCHITECTURE.md`. Everything else in that document still
describes this system.

## The scope decision

Capsule reads. It does not write.

There is no tool that creates an order, edits one, cancels one, moves money, or
changes a subscription — and the design makes adding one visible rather than
easy. `assistant.Reader` (`internal/modules/assistant/access.go`) declares
seventeen methods and not one of them mutates; the tool registry has no write
path to hand a handler; and a test walks every declared tool name and fails on
anything that reads as a mutation.

What it is instead: an analyst with exactly the caller's own eyesight. It reads
orders, spend, sales, catalogue, stock, offers, wallet and subscription —
whatever that specific user could already open on their own dashboard — and
turns it into totals, comparisons and summaries.

## Two switches, and what each one controls

**The owner's switch.** Three new permissions in the RBAC catalogue:

| Key | Scope | Where the owner finds it |
|---|---|---|
| `pharmacy.assistant.use` | pharmacy | الأدوار والصلاحيات → الحساب والأمان |
| `vendor.assistant.use` | vendor | الأدوار والصلاحيات → الحساب والأمان |
| `platform.assistant.use` | admin | Roles → Tools & Services |

They are ordinary catalogue entries, so they appear in the role editor
automatically and are synced to `identity.permissions` at boot. Company owners
hold their entire scope and therefore always have it; an employee has it only
where the owner granted it. Without it there is no launcher in the shell
(`layouts/base.templ`) and every endpoint answers 403 (`requireAssistant`).

**The permission the screen already needs.** Holding the assistant grant widens
nothing. Every tool additionally requires the same key its equivalent dashboard
screen requires — `orders_list` needs `pharmacy.order.view`, `low_stock` needs
`vendor.inventory.view`, and so on. An employee who cannot open the orders page
cannot ask the assistant about orders either.

The consequence is worth stating plainly, because it is the property the client
asked for: **the assistant can never show an employee anything they could not
already see themselves, and an owner can withdraw the whole feature from a role
in one click.**

## How a request is authorized

```
browser ─► POST /api/v1/assistant/turns
             │
             ├─ RequireAuth + ResolveTenant + RequireApproved   (router)
             ├─ requireAssistant  → assistant.Allowed(actor)    (the owner's switch)
             └─ turn created, work detached from the request
                   │
                   ▼
             agent loop (max 4 tool rounds, 90s ceiling)
                   │
      model says: {"name":"spend_summary","arguments":"{...}"}
                   │
                   ▼
             tools.Dispatch — the only way in
               1  actor from request context      (never from arguments)
               2  assistant gate re-checked
               3  exact tool name                 (no fuzzy matching)
               4  dashboard scope must match
               5  actor holds the tool's permission
               6  strict JSON decode              (unknown fields refused)
               7  handles verified and bound to this caller
               8  per-tool timeout
               9  row and byte ceilings on the result
              10  decision written to assistant.tool_audit
                   │
                   ▼
             assistant.Reader → InReadTx → row-level security
```

Three independent layers have to fail before one tenant sees another's data: the
tool's permission check, the query's own `organization_id` predicate, and the
Postgres RLS policy that `InReadTx` activates. The last one is the important
one, because it does not depend on any of this code being right.

## The rules that make it hold

**No identity ever crosses the model.** No tool schema contains `org_id`,
`user_id`, `branch_id` or any synonym; a test walks the whole registry and fails
the build if one appears. Handlers take `authctx.Actor` from the request
context.

**Every id is an opaque, actor-bound handle.**
`internal/modules/assistant/handles` issues HMAC-signed references carrying the
row's kind and id together with the organisation and user they were issued to,
with a thirty-minute expiry. Verification checks the signature, the kind, the
expiry, *and* that the binding matches the live caller. A handle lifted from
another tenant's session has a perfectly valid signature and still fails.
Sequential ids produce unrelated tokens, so enumeration has nothing to walk.

**Untrusted content is fenced.** Attachment text and anything else the caller did
not type arrives inside `<<<UNTRUSTED_CONTENT source="…">>>`, never in the system
message, with the markers inside the content neutralised so a file cannot close
the block early. The attachment-reading pass runs with **no tools bound at all**,
so a PDF instructing the model to call an admin tool is talking to a model that
has none.

**Prompts are not the boundary.** The system prompts tell the model it is
read-only and may only see the caller's data, because that produces honest
answers. What makes it true is that authorization happens after the model
speaks, against the live session.

## Streaming that survives the browser

The reported failure — *"the answer stops when the chat window becomes smaller or
minimised"* — was a client-side SSE parser that reset the current event name on
every network chunk. A chunk boundary between `event: delta` and its `data:`
line silently dropped the token, and resizing changes the paint cadence, which
changes where the boundaries land.

Fixed structurally rather than patched:

- The browser uses `EventSource`, which parses frames correctly, reconnects by
  itself, and resends `Last-Event-ID`.
- Asking and reading are separate requests. `POST /turns` starts the work on a
  context detached from the request (`context.WithoutCancel`) and returns an id;
  `GET /turns/{id}/stream` replays from a sequence number and then tails.
- Chunks live in a numbered log — **Redis in production, process memory in
  development** — for an hour, so a reconnect resumes with no gap, on any
  replica.
- The answer is persisted on every exit path, including "the client went away".
  Previously that path discarded an answer the tenant had already been billed
  for.
- `: keep-alive` every 15s stops idle proxies closing the stream.
- `DELETE /turns/{id}` stops generation server-side, so an abandoned answer stops
  costing money.

## Retention

Conversations are deleted **six months after they were created**. The deadline is
a column with a partial index (`assistant.conversations.expires_at`), the sweep
is a daily job in the worker, and messages, turns and attachments go with it by
cascade. Uploads never sent with a question are removed after 24 hours, objects
included.

The drawer states it: *«تُحذف هذه المحادثة تلقائياً في …»*, with the actual date
once the conversation exists, beside *«قراءة فقط — لا ينفّذ أي إجراء»*.

## Bugs fixed on the way

| | What it was |
|---|---|
| **Cross-user leak** | `/messages` took `conversation_id` from the body and loaded its history with no ownership check. Any approved user could read another tenant's conversation through the model, and have their turns written into it. Now `GetOwnedConversation` filters on organisation, user *and* agent role in SQL. |
| **Attachments never worked** | The capability probe decoded a nested `capabilities` object; `/v1/models` publishes flat `supports_*` fields. The decode succeeded, produced an all-false set, cached it as a success, and every attachment was refused as `no_capable_model` with nothing logged. |
| **Attachment bytes in three wrong places** | A base64 data URL in a never-evicted process map, *and* in `messages.attachments` JSONB, and not in object storage. A 10 MB PDF cost ~13 MB of heap for the process lifetime plus ~13 MB per conversation. |
| **Transcription billed to the platform** | It authenticated with the platform key, was invisible on the tenant's usage screen, and escaped the plan window the Gateway does enforce. It now uses the tenant key and is recorded. |
| **Transcription model hardcoded** | Pinned to a model the Gateway ships `inactive`, and undiscoverable because `/v1/models` hides transcription rows. Now discovered from the admin catalogue, cached 15 minutes, cheapest active model that accepts the upload. |
| **History was backwards** | `ORDER BY id ASC LIMIT 20` fed the model a long conversation's *opening* and dropped everything since. Now newest-first under a token budget derived from the model's real context window. |
| **Five malformed error bodies** | An i18n migration wrote the call expression into a raw string, so "file too large" and "rate limited" reached the browser as invalid JSON and showed nothing. |
| **Cross-tenant digest cache** | Keyed on content hash alone, process-global. |
| **Unbounded rate limiter** | Never evicted a user key. |
| **Admin agent could not exist** | The RLS policy on `assistant.conversations` admits `is_system() OR (organization_id IS NOT NULL AND …)`. Platform staff have no organisation, so both branches were false and the INSERT was refused. Staff bookkeeping now runs through `ownCtx`, scoped by an explicit `user_id` predicate. |

## What the agents can read

Same model for all three (`assistant.primary`); the difference is the persona,
the vocabulary and the tool set.

**Pharmacy** — `orders_list`, `order_details`, `spend_summary`,
`top_purchased_products`, `market_search`, plus the shared `branches_list`,
`wallet_summary`, `subscription_status`.

**Vendor** — `supply_orders_list`, `supply_order_details`, `sales_summary`,
`top_sold_products`, `my_products`, `low_stock`, `my_offers`, plus the shared
three.

**Admin** — `organizations_search`, `platform_overview`, `ai_usage_summary`.
Deliberately coarse: registration records and counts, never one tenant's order
book. Each is gated on the specific admin permission its screen needs, held by
*that* admin — being staff is not a permission.

Aggregates are computed in Postgres, never by the model. A model asked to sum
forty decimal figures gets it right most of the time, and "most of the time" is
not a property a pharmacy's monthly spend can have.

## Test coverage

`internal/modules/assistant/tools/*_test.go` scripts tool calls the way a
compromised model would emit them, against an instrumented reader, so "no query
ran" is checked rather than assumed:

gate required · gate alone grants nothing · cross-role tools refused both ways ·
schemas scoped · schemas follow individual grants · foreign handle refused · raw
ids refused · own handle works · no identity arguments in any schema · smuggled
arguments refused · out-of-range values refused · page size capped · unknown
tools refused · refusals leak nothing · every decision audited · no tool name
reads as a mutation.

Plus `handles` (forgery, expiry, kind, binding, key separation), `stream`
(replay, resume, timeout, concurrent readers), `agent` (role routing, distinct
prompts, read-only declaration), `context` (fence cannot be escaped), the
gateway wire-shape regression, and UI assertions that the drawer streams with
EventSource and states its retention.

## Operational notes

- **Redis** is used for the turn buffer when `REDIS_URL` is set; without it the
  buffer is process memory and a turn cannot be resumed from another replica.
  Production (Elest.io) has it; local development does not, and needs no
  configuration.
- **Handle signing** derives from `SESSION_SECRET`. An empty secret yields a
  random per-process key: handles then stop working across a restart, which
  nothing depends on, and are never forgeable.
- **`GATEWAY_MODEL_ASSISTANT_TRANSCRIBE`** pins a transcription model; empty
  means "choose the cheapest active one that accepts this audio".
- Migration **160** adds `agent_role`/`expires_at`, `assistant.turns`,
  `assistant.attachments` and `assistant.tool_audit`; **162** adds the
  `tool_audit` row-level security policy. All four tables carry FORCE RLS.

  162 is a separate file for a reason worth recording. The policy was first
  written into 160 *after* 160 had already been applied to production, so its
  checksum no longer matched and the runner refused to migrate at all — with the
  schema already current, `migrate-status` reporting zero pending, and everything
  that runs after migrating (the permission-catalogue sync in particular)
  silently skipped. The only visible symptom was
  `service "migrate" didn't complete successfully: exit 1`.

  `go run ./cmd/migratecheck` now reports that drift before rehearsing any SQL,
  so the next occurrence costs a second instead of a release.
