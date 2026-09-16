# Capsule v2 — from a read-only helper to a production assistant

Status: phases 1–4 BUILT 2026-09-13, see `05_CAPSULE_V2.md`. Open: the phase 0
fixture/corpus growth and bake-off, and phase 5. Supersedes the read-only
decision in `03_READ_ONLY_AGENT.md` (decision Q7).

One assistant, two doors. The web drawer and the Telegram bot both call
`assistant.Service.Ask`, so every item below lands on both at once. Telegram only
adds transport: files and confirmation buttons.

---

## 1. Why it feels weak today (measured from the code)

| # | Limit | Where | Effect |
|---|-------|-------|--------|
| 1 | **25 rows per call**, 24 KB per result | `access.go:38` `PageLimit`, `tools/registry.go` `maxResultBytes` | "How many orders last year / total spend / top 50 items" is computed by the model from 25 rows, which is wrong, or it stops early. |
| 2 | **~80 fixed projections** | `access_stage3.go`, `tools/tools_*.go` | A question that does not match one of them gets no answer. Several projections overlap, so a mid-size model often picks the wrong one. |
| 3 | **Tool calls run one after another**, 6 rounds, 16 calls, 90 s | `service_init.go`, `service_methods.go` loop | Multi-part questions run out of budget. Independent reads wait on each other. |
| 4 | **Primary model is `gemma-4-31b-it`** | `gateway/roles.go` | Weak at choosing among 60–80 tools and at long multi-step reasoning. This is the biggest single factor in answer quality. |
| 5 | **The prompt causes the slop** | `prompt.go` | Rule ٢ forbids ever saying data is unavailable, which pushes the model to invent. Rule ٤ demands "executive analysis, percentages, elegant tables" for every answer, which makes it verbose. |
| 6 | **No actions at all** | by design, `03_READ_ONLY_AGENT.md` | It cannot do anything the user can do. |
| 7 | **Services don't check ownership** | e.g. `commerce.RespondToQuote(ctx, quoteID, …)`, `SetShipmentTracking(ctx, id, …)` | Ownership is enforced in HTTP handlers (`commerce/http/order_auth.go`). **Action tools cannot safely call these services directly.** This is the main engineering cost of adding actions. |
| 8 | **RLS is inert** (app runs as superuser) | memory `dawa24-store-rls-is-inert` | Tenant safety depends entirely on each query's own WHERE clause, so the model must never write SQL. |

Scale of the action surface: about 720 mutating routes (674 POST, 22 PUT,
23 DELETE, 1 PATCH). Wrapping all of them is not the goal. Phase 3 picks from
this inventory.

---

## 2. Target architecture

```
                 web drawer ─┐                 ┌─ Telegram (n8n)
                             ▼                 ▼
                    assistant.Service.Ask  (one loop, one prompt set)
                             │
          ┌──────────────────┼─────────────────────────┬──────────────────┐
          ▼                  ▼                         ▼                  ▼
   A. Data engine      B. Domain tools          C. Action layer     D. Platform guide
   describe_data       availability, coverage,  propose_action  →   how-to, statuses,
   query_data          quota, checkout rules    pending action  →   rules, screen links
   get_record          (call the real services) user confirms   →
   export_data                                  command executes
          │                  │                         │
          └──────── tools.Registry.Dispatch (the 10-step gate, unchanged) ────────┘
                             │
                 read-only tx + low-privilege DB role (new)
```

### A. Governed data engine: full tables, no text-to-SQL

This replaces most of the 80 projections with **four general tools** over a
server-side **dataset catalogue**.

A **dataset** is a Go declaration, not SQL from the model:

```go
Dataset{
  Name: "purchase_orders", Scopes: {Pharmacy},
  Permission: "pharmacy.order.view",            // same key as the screen
  Base: `commerce.orders o`,
  Tenant: func(a Actor) (sql string, args) { "o.customer_org_id = $1", a.OrgID }, // mandatory
  Fields: { "number", "status", "created_at", "total", "supplier_name", "branch_name", ... },
  Filters: { status: enum, created_at: range, supplier: handle, branch: handle, search: text },
  Metrics: { count, sum(total), avg(total) },
  GroupBy: { status, supplier_name, month(created_at), branch_name },
  Sensitive: { "cost_price": "vendor.pricing.view" },   // field-level permission
  Screen: "/customer/orders/{id}",              // clickable references
}
```

The tools:

- **`describe_data`** lists the datasets, fields, filters and metrics this actor
  may use. It is generated from the catalogue, so it is always accurate.
- **`query_data(dataset, filters, group_by, metrics, sort, limit, cursor)`**
  - The server compiles a parameterized query from whitelisted parts only. The
    tenant predicate is always ANDed in and can't be removed.
  - **Aggregates run in SQL over the whole table**, which fixes limit #1: "total
    spend in 2025 by supplier" is exact no matter how many rows there are.
  - Row mode returns up to **200 rows** in a compact columnar encoding (header
    once, then value arrays). That fits roughly 5–8 times more rows per KB than
    today's JSON objects. It pages by cursor.
- **`get_record(handle)`** returns full detail for one record with its related
  rows (order → lines, shipments, status history, invoice).
- **`export_data(...)`** takes the same arguments as `query_data`, with no row
  cap (bounded by a statement timeout and 100k rows).
  - It writes CSV or XLSX to attachments. The web shows a download; Telegram
    sends it as a document.
  - This is the honest answer to "give me everything".

Keep existing tools only where they encode **business logic a query can't
express**: availability, coverage, quota, checkout rules, reorder suggestions,
saving products. They must call the same commerce functions checkout uses, so
the assistant and checkout can never disagree.

Target: **about 20 tools visible per scope** instead of 60–80. Fewer, stronger
tools give a model much better routing.

**Security for the data engine:**

1. **Low-privilege Postgres role for assistant reads.** `InReadTx` runs
   `SET LOCAL ROLE dawa24_assistant_ro`, which has SELECT on exposed tables and
   views only. This is defense in depth over the WHERE clause, since RLS is
   inert.
2. **Hard limits:** `statement_timeout` 5 s (15 s for exports), maximum 3 group-bys,
   maximum 6 metrics, and every filter value bound as a parameter.
3. **Never exposed, enforced by a test:** password hashes, session and API tokens,
   bridge secrets, bank and card data, national IDs. A test scans every dataset
   against this denylist.
4. **Cross-tenant test per dataset.** Two orgs are seeded. Every dataset is run
   with every filter, group and metric combination as org A, and any org-B row
   fails the build.
5. **Permission parity test.** Each dataset's permission must equal the
   permission on the dashboard route that lists the same data, read from the
   route table used by `route_guard_audit_test.go`.

### B. Action layer: the assistant acts only through a confirmed command

**Rule: the model never executes anything.** It can only *propose*. A human
click executes, and at that moment the server re-checks everything.

1. **Actor-aware commands, shared with the UI.** For each action, move the
   ownership check out of the HTTP handler into a command in the module:
   ```go
   type Command[In any] struct {
     Name, Scope, Permission string          // Permission == the route's guard
     Risk     Low | High
     Authorize func(ctx, actor, In) error    // ownership, state, quota — moved from handler
     Preview   func(ctx, actor, In) (Preview, error)   // "Accept order #2298 · 14 items · 3,250 ج.م"
     Execute   func(ctx, actor, In, idemKey) (Outcome, error)
   }
   ```
   The existing handler is rewritten to call the same command. That leaves one
   implementation, so a web click and an assistant confirmation are the same
   code path. It also closes limit #7 for the web.
2. **`propose_action(name, args)`** checks the gate, scope, permission and strict
   arguments, then runs `Authorize` and `Preview`. It stores a
   `assistant.pending_actions` row bound to user, org, conversation and channel,
   with the argument snapshot, a hash of the preview, a 10-minute expiry and
   single use. It returns the preview.
3. **Confirmation UI:**
   - Web: a card with **تأكيد / إلغاء**.
   - Telegram: inline keyboard buttons. `callback_data` carries only the opaque
     pending id.
4. **Confirm** runs as a new request, not inside the model loop:
   - Re-resolve the live grant, which covers roles changed and member removed.
   - Re-run `Authorize`, which covers state changed and stock gone.
   - Verify the preview hash is unchanged. Execute with an idempotency key, audit
     with source `assistant.web` or `assistant.telegram`, then feed the outcome
     back to the conversation.
5. **Second gate, owner-controlled:** new permissions `pharmacy.assistant.act`,
   `vendor.assistant.act` and `platform.assistant.act`. An employee who may accept
   orders on the web does not automatically get to do it through Capsule; the
   owner decides.
6. **Risk tiers:**
   - **Low** (cart edits, favourites, mark read, draft RFQ): one confirm.
   - **High** (checkout or submit order, accept or reject orders, price and stock
     changes, respond to RFQs with prices, cancel): confirm, plus a preview that
     shows exact money and quantities.
   - **Never through the assistant:** passwords, payment methods, withdrawals and
     payouts, role or permission grants, account or org deletion, platform-owner
     actions. These remain web-only with the existing flows.
7. **Prompt injection is contained by design.** A poisoned product description
   can at most make the model *propose* something. The user sees the exact
   preview and has to press confirm.

**First wave (about 25 commands, highest value, chosen from the route inventory):**

- **Pharmacy:** add, update and remove cart line; reorder a previous order into
  the cart; submit cart as order (High); cancel pending order; create purchase
  request or RFQ; accept or reject a negotiation; favourites; mark notifications
  read.
- **Vendor:** accept, reject or prepare an incoming order; set shipment tracking;
  respond to RFQ or purchase request with price (High); update variant stock or
  price (High); activate or pause an offer.
- **Admin:** approve or reject an organization (High); resolve a support ticket.

Each command in a later wave is added only with its parity test.

### C. Agent runtime

- **Model:**
  - Run a bake-off on the eval suite (§E) and move `assistant.primary` to the
    best tool-calling model the gateway offers. It is already configurable via
    `GATEWAY_MODEL_ASSISTANT_PRIMARY` or DB role settings, so no code change is
    needed.
  - Keep a cheaper fallback model for when the primary is unavailable.
  - Choose by score, cost and latency, not by name.
- **Parallel tools:** independent calls in one round run concurrently, bounded to
  4 at once. Results are still appended in call order.
- **Budgets:**
  - 10 rounds, 30 calls, 150 s.
  - Exports run as background jobs (River) and post the file when ready, so they
    are not bound by the turn deadline.
- **Context hygiene:** after the model has answered from a large tool result,
  later rounds and turns keep a short summary plus the handle, not the full
  payload. This makes long conversations cheaper and sharper.
- **Per-org daily token budget** with a clear message when it is reached, so a
  stronger model does not become an unbounded bill.

### D. Voice: direct and natural

Rewrite `prompt.go` into one short shared core plus a small playbook per scope.
The rules:

1. **Answer first.** The first line is the answer (number, status, yes or no),
   then only the detail that supports it.
2. **Every number comes from a tool result.** If the data isn't available or
   isn't permitted, say so in one sentence and offer the nearest thing you *can*
   show. This replaces rule ٢, which is the main cause of invented answers.
3. **Tables only for 3 or more comparable rows.** No section headings, preamble,
   closing summary, "I hope this helps", or restating the question.
4. **Match the user's language and register** (Egyptian Arabic in, Egyptian
   Arabic out; English in, English out).
5. **Ask a clarifying question only when the answer would genuinely differ**
   (for example, which branch for a purchase). Otherwise pick the obvious reading
   and state it in half a line.
6. **For actions,** state what will happen and let the confirmation card do the
   rest. Don't narrate tool calls.

Formatting per channel: the web renders markdown. Telegram already uses the HTML
subset in `telegram/render.go`, with shorter replies, no wide tables (tables
become "label: value" lines), and files as documents.

Style checks run in evals: a length budget per question class, a banned-phrase
list, and "the first sentence contains the answer".

### E. Platform guide

Add a `platform_guide(topic)` tool over versioned markdown in
`docs/assistant_guide/`: order lifecycle and statuses, quota rules, coverage and
delivery windows, subscriptions, RFQ flow, returns, glossary, and "where is X"
with screen links. This lets "how do I…" and "why is my order stuck" be answered
from the platform's real rules instead of guesses. The `describe_data` output
doubles as the data dictionary.

---

## 3. Evaluation and safety gates (built first, used at every phase)

- **Fixture database:** a deterministic seed with 2 pharmacies (one multi-branch),
  2 vendors, admin staff and employees with narrowed roles, and about 18 months of
  orders, invoices, RFQs, stock and offers. The seed runs in the scratch-DB
  pattern already used for Telegram tests.
- **Eval corpus:** grow the current 189 cases (`evals/corpus_*.jsonl`) to about
  600, each with a graded expectation:
  - exact numbers checked against SQL ground truth
  - correct tool and dataset
  - correct refusal for missing permissions
  - cross-tenant probing ("show me pharmacy X's orders")
  - prompt injection planted in product names and notes
  - action proposals with correct arguments and previews
  - style checks
- **Release gate per phase:**
  - numeric accuracy ≥ 95%
  - zero cross-tenant or permission leaks (hard fail)
  - zero actions executed without confirmation (hard fail)
  - p95 latency and cost per turn within budget
- **Observability:** per-tool failure, denial and latency; model cost per org;
  eval score over time on an admin screen, built from the existing
  `assistant_tool_audit` data.

---

## 4. Phases

Each phase ships on its own, runs on web and Telegram at once, and must pass the
gate in §3.

| Phase | Scope | Exit criteria |
|-------|-------|---------------|
| **0. Baseline** | Fixture seed, grown eval corpus, harness scoring today's Capsule; model bake-off; DECISIONS entry reversing read-only | Baseline scores recorded; primary model chosen |
| **1. Data engine** | Dataset catalogue with the top ~30 datasets across the 3 scopes; `describe_data`, `query_data`, `get_record`, `export_data`; low-privilege DB role; denylist, cross-tenant and permission-parity tests | Aggregate questions exact on full tables; exports work on web and Telegram; zero leaks |
| **2. Runtime and voice** | New prompt, parallel tools, new budgets, context hygiene, per-channel formatting, platform guide; retire projections now covered by datasets (only after eval parity) | Accuracy and style gates beat the baseline; visible tools ≤ ~20 per scope |
| **3. Actions, web** | Command abstraction; extract the first ~25 commands from their handlers (handlers refactored to use them); `assistant.pending_actions`; `*.assistant.act` permissions in the role editor; confirmation card | Parity tests per command; confirm-time re-authorization tests; injection cannot execute |
| **4. Actions and files on Telegram** | Inline-keyboard confirm; `callback_query` bridge endpoint; `sendDocument` for exports; n8n workflow update | End-to-end test: propose on Telegram → confirm → audited execution; revoked role between propose and confirm → refused |
| **5. Breadth and hardening** | Remaining datasets and commands by usage data; per-org budgets; admin observability screen; load test | Staged rollout: internal orgs → selected customers → everyone |

---

## 5. Decisions for the client (with recommendations)

1. **Model and cost:** a stronger primary model costs several times more per turn
   than gemma. *Recommendation:* choose by the Phase 0 bake-off, with per-org
   daily budgets.
2. **Actions from Telegram:** *Recommendation:* allow Low and High tier on both
   channels with confirmation. "Submit order" on Telegram also shows the full
   total and supplier split before the button.
3. **Who may act:** *Recommendation:* the separate `*.assistant.act` permission,
   on by default only for owners.
4. **Never-list:** confirm the §B.6 list (passwords, payments and withdrawals,
   roles, deletion) stays web-only.
