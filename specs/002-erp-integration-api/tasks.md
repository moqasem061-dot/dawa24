# Tasks: ERP Integration API

**Input**: `specs/002-erp-integration-api/` (plan.md, spec.md, data-model.md, research.md, quickstart.md, contracts/)
**Reader**: a cheaper model with no memory of this project, doing one task at a time, in order.

**Slice mapping**: plan.md names the slices S1 to S8 in the delivery order below, so the numbers match. Phase 1 is Setup, Phase 2 is Foundational, Phases 3 to 10 are S1 to S8, Phase 11 is Polish.

| Phase | Plan slice | Story | Content |
|---|---|---|---|
| 3 | S1 | US1 | keys and the machine principal |
| 4 | S2 | US2 | product sync |
| 5 | S3 | US3 | order feeds and acknowledgement |
| 6 | S4 | US4 | confirm and reject |
| 7 | S5 | US5 | customer codes and the modal |
| 8 | S6 | US8 | OpenAPI, Postman, docs |
| 9 | S7 | US6 | purchase orders |
| 10 | S8 | US7 | Reveal |

Format: `- [x] T001 [P] [US1] Action in exact/file/path`. A task added after the first numbering carries a letter suffix (`T124a`) and sits in the position where it must run; no number was changed. `[P]` means the files differ and nothing unfinished is needed. "Traps: n" are the numbers in the "Traps" list of plan.md. Two plan.md details that tasks changed on purpose, both to stay under 400 lines: extra files `authenticate.go`, `orders_decide.go`, `decide_handlers.go`, `repository_purchase.go`, `catalog_erp_ui.go`, `reveal_kinds.go`, `erp_purchase_placer.go`, and test files split per slice (`erp_isolation_sync_test.go`, `erp_orders_paging_test.go`, `erp_decide_test.go`, `erp_customer_code_test.go`, `erp_sync_repeat_test.go`).

## Rules for every task

1. Read `AGENTS.md` first. Do not read `.env`. No production, ever. Do not touch containers named `muhiya-*`.
2. Never stage `test/visual_baselines/*.png`. No task may push, merge, or deploy.
3. C: disk is low: run `df -h /c` before heavy runs; under 3 GB free, stop and report. Delete nothing to make room. Only one test run at a time.
4. If a task is blocked, stop, write the reason in `specs/002-erp-integration-api/verify-notes.md`, and do not guess. Do not change a task's scope. Mark the checkbox only after its Done check passes.
5. Tests first: a test task is done when the file is gofmt-clean and the named run fails for the stated reason (RED). The implementation task then makes it pass (GREEN). Never weaken or delete a test to get a pass.
6. **HOST-GO**: every `go ...` command runs on the host as `cd /f/dawa24 && mkdir -p tmp/gotmp && GOTMPDIR='F:\dawa24\tmp\gotmp' env -u DATABASE_URL -u TEST_DATABASE_URL -u TEST_REDIS_URL go <args>`. Never pass `/c/...` paths to Windows programs. Never print whole logs.
7. **SB** (database tests; a Done check starting with `SB` means SB-3, then SB-5 with that command, plus SB-4 when a migration changed or Postgres restarted). Source: `C:\Users\mydwa\security-audit-skill\dawa24-store\run-5\batch-brief.md`. Read `~/security-audit-skill/dawa24-store/run-5/sandbox/sb.sh` once first.
   - SB-0: `df -h /c`.
   - SB-1: `docker desktop start` if Docker is down; `docker start dawa24audit-pg dawa24audit-redis`. If Docker hangs, stop and report; do not restart it.
   - SB-2: `docker exec dawa24audit-redis redis-cli config set save ""` and `docker exec dawa24audit-redis redis-cli config set stop-writes-on-bgsave-error no`.
   - SB-3 (refresh sandbox source, new files included): `cd /f/dawa24 && git ls-files -co --exclude-standard -z | tar --ignore-failed-read --null -T - -cf - | MSYS_NO_PATHCONV=1 docker run --rm -i --network none -v dawa24audit-src:/src golang:1.27.2 sh -c 'cd /src && find . -mindepth 1 -maxdepth 1 ! -name vendor -exec rm -rf {} + && tar -x && chmod -R a+rX .'`
   - SB-4 (Postgres is on tmpfs, migrate after every start): `bash ~/security-audit-skill/dawa24-store/run-5/sandbox/sb.sh run sh -c 'go build -o /tmp/cli ./cmd/cli && /tmp/cli migrate'`
   - SB-5: `bash ~/security-audit-skill/dawa24-store/run-5/sandbox/sb.sh run sh -c 'timeout 400 go test -count=1 <args> 2>&1'`. For a clean DB: `docker restart dawa24audit-pg`, then SB-4.
   - SB-6, for any test whose name starts with `TestTenantIsolation` or that uses `getTestDB`: run `docker exec dawa24audit-pg psql -U dawa24 -d dawa24_store -c "ALTER ROLE dawa24_app PASSWORD 'dawa24_app_change_me'"`, then `bash ~/security-audit-skill/dawa24-store/run-5/sandbox/sb.sh run sh -c 'DATABASE_URL="postgres://dawa24_app:dawa24_app_change_me@dawa24audit-pg:5432/dawa24_store?sslmode=disable" timeout 400 go test -count=1 <args> 2>&1'`. Run ALTER ROLE again after every Postgres restart. This password is for the throwaway sandbox only. Isolation tests must actually run; a skip is not a pass. Insert fixtures in `db.InTx(database.AsSystem(ctx), ...)` and read as the other tenant.
8. **Slice gate** (last task of each slice): `go build ./...`; `go vet <changed packages>`; the slice's focused tests; `gofmt -l` on changed files prints nothing; `make check-file-size`; then commit locally: `git add -A -- . ':(exclude)test/visual_baselines'`, confirm the staged list holds no `.env`, `.exe` or `.log` file, run `~/go/bin/gitleaks.exe protect --staged --no-banner --redact`, then `git commit` with the stated message ending in the line `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`. Never push, merge, or deploy.
9. If a task edits a `.templ` file: run the V3 command, commit the `_templ.go` output, then refresh the homepage hash only if CSS or the homepage template changed (`node "$(cygpath -m ~/security-audit-skill/dawa24-store/run-5/sandbox/homehash.cjs)" .`); otherwise record "no hash change" in `verify-notes.md`.
10. No `float64` money, tenant queries inside `db.InTx`/`db.InReadTx`, every message bilingual, no module imports another module, new migrations backward-compatible only. Do not touch `/api/v1/integrations/`, `MODULE_API_ENABLED`, secrets, or git history. Do not read or log API keys.

11. **Small document mismatch rule**: if two spec documents disagree only on a name or a column, and the data-model.md column list is clear, follow the column list, fix the wrong line in the document, record it in verify-notes.md, and continue without stopping. This covers only naming and doc slips. Still stop and ask for anything that changes behaviour, security, tenant isolation, money, an API response, a migration's meaning, or a task's scope, or if adding something new would be needed to continue.

12. **RED order rule**: when a test or Done check fails only because a later task in the same slice has not been done, do that task first and rerun both Done checks. Record the swap in verify-notes.md and continue. If the missing work is in no task, or the swap would change behaviour or a migration, stop and ask.

13. Any test or Postman flow needing the ERP API sets `ERP_API_ENABLED=true` explicitly. Deploy first with the API off, then set `ERP_KEY_PEPPER` and `ERP_API_ENABLED=true` when ready.

14. **Ownership signal security**: `ActorIsOwner` and `actorIsOwner` must be read only from the signed-in actor, `authctx.Actor.IsOwner`, in the handler. They must never come from a form, query, header, or JSON field.

15. **Test seam rule**: an unexported function field on a struct, set in its constructor, is allowed without asking when a test needs to observe behaviour. It must not change production behaviour, and it must not be a global, exported, or configurable from outside. Record it in verify-notes.md and continue. Security checks must never be bypassable through it.

16. **Layer test rule**: to test a layer, inject the next layer down through an interface it already depends on. Do not export private seams for tests. This needs no question. Each security path also needs one test through the real wiring.

17. **Read method rule**: you may add a read-only service and repository method inside the erp module without asking when a task needs it. It must use the key’s tenant context, explicit organization filters, no AsSystem, no new table or column, no writes, and it must return only fields the contract lists. Add a test and record it in verify-notes.md. Anything needing AsSystem, a schema change, a write, or new API fields still stops and asks.

18. **Real wiring rule**: any test needing a real database, Redis, or real server wiring runs in the sandbox with SB-6, and its Done check requires a visible PASS with -v. Host runs may skip it. A sandbox skip fails the task. This needs no question.

## Phase 1: Setup (verify first, no application code)

Purpose: settle the unverified items in plan.md before any code depends on them. Every result is written to `specs/002-erp-integration-api/verify-notes.md`. If a check fails, stop, record it there, and do not start the slice that needs it.

- [x] T001 Confirm branch and clean tree
  - Do: run `git branch --show-current` and `git status --short`. Nothing under `internal/`, `cmd/`, `db/`, `test/` (except `test/visual_baselines`) may be modified.
  - Done: branch prints `codex/audit-batches` and the status list holds only `.specify/feature.json`, the four `test/visual_baselines/*.png`, and `specs/002-erp-integration-api/`.

- [x] T002 Confirm the sandbox and disk
  - Do: run SB-0, SB-1, SB-2 and SB-4 from "Rules for every task". Read `sb.sh` once first.
  - Done: `bash ~/security-audit-skill/dawa24-store/run-5/sandbox/sb.sh run sh -c 'go version'` prints a Go version, and `df -h /c` shows at least 3 GB free.

- [x] T003 V1 verify River insert-only client in `specs/002-erp-integration-api/verify-notes.md`
  - Do: create the file with a heading. Find River with `go list -m -f '{{.Dir}}' github.com/riverqueue/river`; read the `Config` validation in `client.go`. Confirm `river.NewClient` accepts a config with `Queues` nil and `Workers` nil (an insert-only client). Write one line `V1 result: yes|no, <file>:<line>`.
  - Use: `queue.New` at `internal/platform/queue/queue.go` for how the existing client is built.
  - Traps: 20. If `no`: stop; S2 task "NewInsertOnly" is blocked until the owner decides.
  - Done: `grep -n "^V1 result:" specs/002-erp-integration-api/verify-notes.md` prints one line.

- [x] T004 V2 verify the foreign keys to catalog.match_runs in `specs/002-erp-integration-api/verify-notes.md`
  - Do: run `grep -rn 'REFERENCES catalog.match_runs' db/migrations` and record `V2 result: all foreign keys to catalog.match_runs are CASCADE or SET NULL, no child delete is needed` (verified when this plan was written: the table is created in 232; the references are in 234 to 238, `ON DELETE SET NULL`, and in 240 and 242, `ON DELETE CASCADE`). If any hit is neither, record `V2 result: <file>:<line> has no cascade` and the S2 cleanup task T118 must delete that child table first.
  - Traps: none. Reason: research D-14 deletes ERP `catalog.match_runs` rows after 30 days.
  - Done: `grep -n "^V2 result:" specs/002-erp-integration-api/verify-notes.md` prints one line.

- [x] T005 V3 verify the templ generate form in `specs/002-erp-integration-api/verify-notes.md`
  - Do: run `~/go/bin/templ.exe version` from the repo root. Also run `grep -n templ Makefile`. Record the exact command that works, as `V3 result: <command>`.
  - Traps: 22.
  - Done: `grep -n "^V3 result:" specs/002-erp-integration-api/verify-notes.md` prints one line.

- [x] T006 V4 verify every existing symbol that plan.md cites in `specs/002-erp-integration-api/verify-notes.md`
  - Do: run these greps and record found or missing for each: `func (s \*Service) BulkWriteVariants` in `internal/modules/catalog/vendor_import.go`; `BulkWriteStocks` in `internal/modules/inventory/vendor_import.go`; `Prepare` in `internal/modules/catalog/matching/service.go`; `Remember` in `internal/modules/catalog/matching/knowledge_service.go`; `TransitionShipmentStatusWithWallet` in `internal/modules/commerce/fulfilment.go`; `prepareWalletShipmentCharge` in `internal/ui/customer_checkout_wallet.go`; `CheckoutAPI` in `internal/modules/commerce/api_checkout.go`; `func.*quote` in `cmd/server/api_checkout_quote.go`; `QueryRowUnscoped` and `WithTenant` in `internal/platform/database/query_helpers.go`; `EnqueueTx` and `StartStalledJobReaper` in `internal/platform/queue/queue.go`; `lateOrderAnnouncer` in `cmd/server/order_announce_wiring.go`; `RegisterApprovedSharedRoutes` in `internal/ui/handlers.go`.
  - Done: `grep -n "^V4 result:" specs/002-erp-integration-api/verify-notes.md` prints one line that lists any missing symbol or says `all found`.

- [x] T007 V5 record the Orgasoft unverified items in `specs/002-erp-integration-api/verify-notes.md`
  - Do: copy these as a list headed `V5 unverified (owner or Orgasoft must confirm)`: sample 3 is the import acknowledgement (research D-18); Orgasoft wants `variantId` as purchase-order item identity (D-21); `taxes` in sample 3 is an amount while `tax: 14` in the product samples is a percent; `discount` and `tax` are percents and `tax` is stored, not applied; quota means a cap per buying pharmacy branch; one external code is one sellable item per tenant; the optional fields `unit`, `manufacturer` and `batchNumber` are not in any Orgasoft sample and are ignored when absent.
  - Done: `grep -c "unverified" specs/002-erp-integration-api/verify-notes.md` prints 1 or more.

## Phase 2: Foundational (blocks more than one slice)

Only the Orgasoft sample files qualify: S2 (products), S3 (acknowledgement) and S6 (Postman) all read them. Nothing else blocks every slice; S8 needs T026 to T029 from S1 (the i18n files and the rbac test file) and nothing else.

- [x] T008 [P] Add the camelCase product sample as `internal/modules/erp/testdata/orgasoft_products_camel.json`
  - Do: copy the JSON under "Product sync, camelCase" in `specs/002-erp-integration-api/contracts/orgasoft-samples.md` byte for byte (field names `externalProductCode`, `arName`, `stock`, `publicPrice`, `discount`, `tax`, `isQuota`, `quotaQuantity`, `barcode`, `expiryDate`).
  - Done: `node -e "JSON.parse(require('fs').readFileSync('internal/modules/erp/testdata/orgasoft_products_camel.json','utf8'))"` prints nothing and exits 0.

- [x] T009 [P] Add the PascalCase product sample as `internal/modules/erp/testdata/orgasoft_products_pascal.json`
  - Do: copy the JSON under "Product sync, PascalCase (11 Mar 2026)" byte for byte (`ExternalProductCode` value `ORGA-100`, no `ExpiryDate`).
  - Done: `node -e "JSON.parse(require('fs').readFileSync('internal/modules/erp/testdata/orgasoft_products_pascal.json','utf8'))"` exits 0.

- [x] T010 [P] Add the order acknowledgement sample as `internal/modules/erp/testdata/orgasoft_order_ack_sample3.json`
  - Do: copy the array under "Order data (11 Mar 2026)" byte for byte (`order_number` value `{{orderNumber}}`, `orga_orderID` value `ORG-9921`, one item with `sku` `MED-001`, `taxes` 1.50).
  - Done: `node -e "JSON.parse(require('fs').readFileSync('internal/modules/erp/testdata/orgasoft_order_ack_sample3.json','utf8'))"` exits 0.

## Phase 3: S1 Keys and the machine principal [US1]

Goal: owners and staff create, rotate and revoke keys; `GET /erp/v1/health` works. Independent test: create a key, call health, revoke, call health again and get 401 `ERP_KEY_REVOKED`.

- [x] T011 [US1] Write the S1 tenant isolation tests in `test/integration/erp_isolation_test.go`
  - Do: `TestTenantIsolation_ERPIntegrations`, `TestTenantIsolation_ERPApiKeys`, `TestTenantIsolation_ERPCalls`: insert a row for organization A with raw SQL, open a transaction as organization B, assert 0 rows. Use time-based fixture ids. Copy the structure of `test/integration/match_ai_attempts_isolation_test.go`.
  - Traps: 19.
  - Done: `gofmt -l test/integration/erp_isolation_test.go` prints nothing; `SB-6 go test -p 1 ./test/integration/ -run TestTenantIsolation_ERP -count=1` fails (RED) because schema `erp` does not exist.

- [x] T012 [US1] Write migration 255 part 1 in `db/migrations/255_erp_schema.up.sql`
  - Do: `BEGIN;`, `CREATE SCHEMA erp;`, then tables `erp.integrations` and `erp.api_keys` exactly as in `data-model.md` (columns, CHECKs, unique indexes, RLS block, touch triggers, one Arabic `COMMENT ON TABLE` and one `COMMENT ON COLUMN` per column using the Arabic text in `data-model.md`; for a blank cell use the standard comment list in `data-model.md`). Do not write `COMMIT;` yet.
  - Traps: 21, 30 (each `CREATE TABLE` ends with `);` alone on its line).
  - Done: `grep -c "CREATE TABLE erp" db/migrations/255_erp_schema.up.sql` prints 2.

- [x] T013 [US1] Write migration 255 part 2 in `db/migrations/255_erp_schema.up.sql`
  - Do: append `erp.product_links`, `erp.sync_batches`, `erp.sync_items` exactly as in `data-model.md`, same RLS, trigger and comment rules.
  - Traps: 30.
  - Done: `grep -c "CREATE TABLE erp" db/migrations/255_erp_schema.up.sql` prints 5.

- [x] T014 [US1] Write migration 255 part 3 in `db/migrations/255_erp_schema.up.sql`
  - Do: append `erp.review_items`, `erp.order_imports`, `erp.idempotency_records`, same rules.
  - Traps: 30.
  - Done: `grep -c "CREATE TABLE erp" db/migrations/255_erp_schema.up.sql` prints 8.

- [x] T015 [US1] Write migration 255 part 4 in `db/migrations/255_erp_schema.up.sql`
  - Do: append `erp.api_calls` (organization_id nullable, no update trigger, standard policy), then the `dawa24_app` grant `DO $$ ... $$;` block copied from `data-model.md`, then `COMMIT;`.
  - Traps: 21, 30.
  - Done: `grep -c "CREATE TABLE erp" db/migrations/255_erp_schema.up.sql` prints 9 and the last non-blank line is `COMMIT;`.

- [x] T016 [US1] Write `db/migrations/255_erp_schema.down.sql` and apply 255
  - Do: `BEGIN; DROP SCHEMA IF EXISTS erp CASCADE; COMMIT;`. Then run SB-3 and SB-4.
  - Done: `SB-6 go test -p 1 ./test/integration/ -run TestTenantIsolation_ERP -count=1` passes (GREEN), and `go test ./test/ -run 'TestRepositorySQLMatchesMigrations|TestWriteSQLMatchesMigrations' -count=1` passes.

- [x] T017 [US1] Write the audit actor default test in `internal/platform/database/audit_actor_test.go`
  - Do: table test `TestAuditActorKindDefault` calling an unexported helper `auditActorKind(e AuditEntry) string`: empty returns `user`, `api_key` and `system` pass through.
  - Traps: 15.
  - Done: `gofmt -l internal/platform/database/audit_actor_test.go` prints nothing; `go test ./internal/platform/database/ -run AuditActorKind -count=1` fails to compile (RED).

- [x] T018 [US1] Write the audit round-trip test in `test/integration/erp_audit_actor_test.go`
  - Do: `TestAuditActorKeyRoundTrip`: inside `db.InTx` call `database.WriteAudit` with `ActorKind: "api_key"`, `ActorKeyID` set, then read the row from `platform.audit_log` and assert both columns. A second entry with no `ActorKind` reads back `user` and NULL.
  - Traps: 15.
  - Done: `gofmt -l test/integration/erp_audit_actor_test.go` prints nothing; the test fails (RED) because the fields do not exist.

- [x] T020 [US1] Add `ActorKind` and `ActorKeyID` in `internal/platform/database/audit.go`
  - Do: add `ActorKind string` and `ActorKeyID *int64` to `AuditEntry`; add `auditActorKind`; extend the INSERT with `actor_kind, actor_key_id`.
  - Use: existing `WriteAudit` in the same file.
  - Traps: 15.
  - Done: `go test ./internal/platform/database/ -count=1` and `SB go test -p 1 ./test/integration/ -run TestAuditActorKeyRoundTrip -count=1` both pass.

- [x] T019 [US1] Write migration 256 in `db/migrations/256_audit_actor_kind.up.sql` and `.down.sql`
  - Do: use the SQL in `data-model.md` "Migration 256" with `BEGIN;`/`COMMIT;` and the two Arabic column comments. Down: drop the index, the constraint and both columns.
  - Traps: 15.
  - Done: SB-3 and SB-4 apply 256 with no error, then `SB-6 go test -p 1 ./test/integration/ -run TestTenantIsolation_ERP -count=1` still passes.

- [x] T021 [US1] Write the ERP config test in `internal/platform/config/erp_test.go`
  - Do: cases: defaults (`Enabled` false, `DefaultRate` 600, `MaxBatchItems` 50000); `ERP_KEY_PEPPER` shorter than 32 bytes with `APP_ENV=prod` and enabled fails; the same in `dev` loads with a fixed development pepper; disabled in prod needs no pepper; `WORKER_ERPSYNC` defaults 3 in `Worker.Queues["erpsync"]`. Copy how existing config tests build the env.
  - Done: `gofmt -l internal/platform/config/erp_test.go` prints nothing; `go test ./internal/platform/config/ -run ERP -count=1` fails (RED).

- [x] T022 [US1] Add ERP config in `internal/platform/config/erp.go` and `config.go`
  - Do: `type ERP struct{Enabled bool; KeyPepper string; DefaultRate, MaxBatchItems int}`; `loadERP(env, fail)`; keys `ERP_API_ENABLED` (default false), `ERP_KEY_PEPPER`, `ERP_DEFAULT_RATE_PER_MINUTE`, `ERP_MAX_BATCH_ITEMS`; add field `ERP` to the config struct and call `loadERP`; add `"erpsync": getInt("WORKER_ERPSYNC", 3)` to `Worker.Queues`. Add a comment on `DefaultRate`: `DefaultRate default 600 duplicates erp.DefaultRatePerMinute; keep both, TestERPDefaultRateMatchesLimit in cmd/server/erp_wiring_test.go asserts they match` (`platform/config` cannot import `erp`); the same comment on `MaxBatchItems` (50000, `erp.MaxBatchItems`).
  - Traps: 4.
  - Done: `go test ./internal/platform/config/ ./internal/platform/queue/ -count=1`.

- [x] T023 [US1] Write the `AllowKey` test in `internal/platform/httpx/ratelimit_allowkey_test.go`
  - Do: `TestAllowKey`: limit 3 per window gives true,true,true,false; count and retryAfter returned; a nil Redis client returns `allowed=true`; a Redis error returns `allowed=true`. Copy the setup of the existing tests in `internal/platform/httpx/ratelimit*_test.go`.
  - Done: `gofmt -l internal/platform/httpx/ratelimit_allowkey_test.go` prints nothing; `go test ./internal/platform/httpx/ -run AllowKey -count=1` fails to compile (RED).

- [x] T024 [US1] Add `AllowKey` in `internal/platform/httpx/ratelimit.go`
  - Do: `func (l *Limiter) AllowKey(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, count int64, retryAfter time.Duration, err error)` that calls the private `allow` at line 149 and returns its ttl as `retryAfter`. Do not change `allow`.
  - Done: `go test ./internal/platform/httpx/ -run AllowKey -count=1`.

- [x] T025 [US1] Write the i18n key tests in `internal/shared/i18n/catalog_erp_test.go`
  - Do: two tests. `TestErpErrorKeys` asserts, for the keys `erp.err.auth_missing`, `erp.err.rate_limited`, `erp.err.po_price_mismatch`, `erp.err.reveal_forbidden`: `i18n.T(i18n.AR, key)` and `i18n.T(i18n.EN, key)` are not empty, differ from the key, and differ from each other. `TestErpUIKeys` asserts the same for `erp.keys.title` and `erp.keys.change_limit`.
  - Done: `gofmt -l internal/shared/i18n/catalog_erp_test.go` prints nothing; `go test ./internal/shared/i18n/ -run 'TestErpErrorKeys|TestErpUIKeys' -count=1` fails (RED).

- [x] T026 [US1] Write error messages part 1 in `internal/shared/i18n/catalog_erp.go`
  - Do: `func loadErpKeys(e ...)` (copy the signature style of the other `load*` functions in `internal/shared/i18n/`). Add keys `erp.err.<code lower case without ERP_>` with the Arabic and English text from `contracts/error-codes.md` for the sections "Authentication and access", "Request shape", "Product sync (request level)", "Product sync (item level)".
  - Traps: 12.
  - Done: `gofmt -l internal/shared/i18n/catalog_erp.go` prints nothing and `go build ./internal/shared/i18n/`.

- [x] T027 [US1] Write error messages part 2 in `internal/shared/i18n/catalog_erp.go` and call it from `catalog.go`
  - Do: add the sections "Orders", "Purchase orders", "Internal codes" (keys `erp.err.reveal_not_revealable`, `erp.err.reveal_forbidden`, `erp.err.reveal_not_found`, `erp.err.code_taken`). Register `loadErpKeys` in `loadCatalogDefaults` in `catalog.go`.
  - Traps: 12.
  - Done: `go test ./internal/shared/i18n/ -run TestErpErrorKeys -count=1` passes; `make check-hardcoded-arabic` prints ok.

- [x] T028 [US1] Add key-page strings in `internal/shared/i18n/catalog_erp_ui.go`
  - Do: `loadErpUIKeys(e ...)` called at the end of `loadErpKeys`. Add `erp.keys.*` bilingual strings for the pages: `title`, `new_key`, `provider`, `name`, `warehouse`, `branch`, `buying_user`, `scopes`, `rate_limit`, `expires`, `prefix`, `last_used`, `status`, `rotate`, `revoke`, `grace_hours`, `pause`, `resume`, `shown_once`, `copy`, `empty`, `owner_only`, `revoke_reason`, `change_limit`, plus one label per scope name. Arabic first.
  - Traps: 12.
  - Done: `go test ./internal/shared/i18n/ -run 'TestErpErrorKeys|TestErpUIKeys' -count=1`, then `go test ./internal/shared/i18n/ -count=1` and `make check-hardcoded-arabic`.

- [x] T029 [US1] Write the permission tests in `internal/platform/rbac/erp_perms_test.go`
  - Do: three tests. `TestErpPermsVendor`: keys `vendor.integration.view` and `vendor.integration.manage` exist, `manage` implies `view`, nav entry `integrations` exists for vendor. `TestErpPermsPharmacy`: `pharmacy.integration.view` and `pharmacy.integration.manage` the same way, nav entry for pharmacy. `TestErpPermsAdmin`: `platform.integration.view` and `platform.integration.revoke` exist, nav entry for admin. Copy how existing rbac tests read the catalog.
  - Done: `gofmt -l internal/platform/rbac/erp_perms_test.go` prints nothing; `go test ./internal/platform/rbac/ -run 'TestErpPerms' -count=1` fails (RED).

- [x] T030 [P] [US1] Add vendor permissions and nav in `internal/platform/rbac/catalog_vendor.go` and `nav_vendor.go`
  - Do: add `vendor.integration.view` and `vendor.integration.manage` in the style of `vendor.quota.view` (`catalog_vendor.go:117`); `manage` implies `view`. Add nav entry `integrations` near line 109 of `nav_vendor.go`, pointing to `/vendor/integrations`.
  - Traps: 29.
  - Done: `go test ./internal/platform/rbac/ -run TestErpPermsVendor -count=1`.

- [x] T031 [P] [US1] Add pharmacy permissions and nav in `internal/platform/rbac/catalog_pharmacy.go` and `nav_pharmacy.go`
  - Do: add `pharmacy.integration.view` and `pharmacy.integration.manage` the same way; nav entry `integrations` to `/customer/integrations`.
  - Traps: 29.
  - Done: `go test ./internal/platform/rbac/ -run TestErpPermsPharmacy -count=1`.

- [x] T032 [P] [US1] Add admin permissions and nav in `internal/platform/rbac/catalog_admin.go` and `nav_admin.go`
  - Do: add `platform.integration.view` and `platform.integration.revoke`; nav entry `integrations` to `/admin/integrations`.
  - Traps: 29.
  - Done: `go test ./internal/platform/rbac/ ./test/ -run 'TestErpPermsAdmin|TestEveryRouteGateNamesADeclaredPermission' -count=1`.

- [x] T033 [P] [US1] Add limits in `internal/modules/erp/limits.go`
  - Do: `package erp`; one constant each, exact names and values from the plan.md "Limits" list: `KeyPrefixLen=8`, `DefaultRatePerMinute=600`, `MinRatePerMinute=10`, `MaxRatePerMinute=6000`, `SyncBatchesPerMinute=6`, `AuthFailuresPerIPPerMinute=30`, `MaxBatchItems=50000`, `MaxBodyBytes=32<<20`, `MaxInflatedBytes=128<<20`, `SyncChunkSize=2500`, `SyncSnoozeSeconds=15`, `PageDefault=100`, `PageMax=500`, `SyncItemsPageDefault=1000`, `SyncItemsPageMax=5000`, `AckMax=500`, `PurchaseLinesMax=200`, `OffersMax=50`, `RotateGraceDefault=24h`, `RotateGraceMax=72h`, `LastUsedTouchEvery=1m`, `IdempotencyStaleAfter=2m`, `RetentionAPICalls=30d`, `RetentionSyncItems=7d`, `RetentionSyncBatches=90d`, `RetentionReview=180d`, `RetentionMatchRuns=30d`, `CleanupBatchRows=10000`. Durations are `time.Duration` (days as `n*24*time.Hour`).
  - Traps: 3, 11.
  - Done: `go build ./internal/modules/erp/`.

- [x] T034 [P] [US1] Add scopes in `internal/modules/erp/scopes.go`
  - Do: `ScopeProductsSync = "erp.products.sync"`, `ScopeOrdersRead = "erp.orders.read"`, `ScopeOrdersAck = "erp.orders.ack"`, `ScopeOrdersDecide = "erp.orders.decide"`, `ScopePurchaseCreate = "erp.po.create"`, `func AllScopes() []string`, `func DefaultScopes(vendor bool) []string` (vendor: first four; customer: read, ack, po.create), `func ValidScope(s string) bool`. These three function names are chosen by this task list; plan.md gives none.
  - Done: `go build ./internal/modules/erp/`.

- [x] T035 [US1] Write the error-code test in `internal/modules/erp/errors_test.go`
  - Do: `TestAllCodesHaveBilingualMessages`: for each code in `AllCodes`, `i18n.T(i18n.AR, "erp.err."+strings.ToLower(strings.TrimPrefix(code,"ERP_")))` and the EN one are non-empty and not equal to the key. `TestAllCodesCount`: `len(AllCodes) == 58` (57 codes in the tables plus `ERP_CODE_TAKEN`; recount against `contracts/error-codes.md` and stop if it differs). `TestNewErrorFields`.
  - Done: `gofmt -l internal/modules/erp/errors_test.go` prints nothing; `go test ./internal/modules/erp/ -run Codes -count=1` fails to compile (RED).

- [x] T036 [US1] Add errors in `internal/modules/erp/errors.go`
  - Do: `type Error struct{Code string; Status int; Details map[string]any}`, `func (e *Error) Error() string`, `func NewError(code string, status int, details map[string]any) *Error`, `var AllCodes []string` listing all 58 `ERP_*` codes from `contracts/error-codes.md` (no REVEAL codes), `var ErrWaitForEarlierBatch = errors.New(...)`.
  - Traps: 12.
  - Done: `go test ./internal/modules/erp/ -run 'Codes|NewError' -count=1`.

- [x] T037 [US1] Add domain types in `internal/modules/erp/domain.go`
  - Do: `Principal{OrganizationID, IntegrationID, KeyID int64; Provider string; Scopes []string; WarehouseID, BranchID int64; BuyingUserID *int64; MaxBatchItems, RatePerMinute int; OrgType string; KeyPrefix string}` with `func (p Principal) HasScope(s string) bool`; `Integration`, `APIKey` (all columns of `erp.api_keys` except `key_hash`), `CreateKeyInput`, `CreatedKey{Key APIKey; FullKey string}`, `CallRecord` (fields of `erp.api_calls`), `KeyAuthRow` (key columns plus `key_hash`, integration status). Money-free; no float.
  - Done: `go build ./internal/modules/erp/` and `go test ./internal/modules/erp/ -run Principal -count=1` (write one `TestPrincipalHasScope` table test in `domain_test.go` first).

- [x] T038 [US1] Write the key tests in `internal/modules/erp/keys_test.go`
  - Do: format regex `^d24_(live|test)_[a-z0-9]{8}_[A-Za-z0-9_-]{43}$`; round trip `GenerateKey` then `ParseKey` then `VerifyHash`; wrong secret fails; 1,000 keys have unique prefixes; `ParseKey` rejects short, wrong tag and extra segments; `HashSecret` is 64 hex characters and depends on the pepper.
  - Done: `gofmt -l internal/modules/erp/keys_test.go` prints nothing; `go test ./internal/modules/erp/ -run Key -count=1` fails (RED).

- [x] T039 [US1] Add key functions in `internal/modules/erp/keys.go`
  - Do: `GenerateKey(tag string, pepper []byte) (full, prefix, hash string, err error)`, `ParseKey(raw string) (prefix, secret string, ok bool)`, `HashSecret(pepper []byte, secret string) string` (hex HMAC-SHA256), `VerifyHash(pepper []byte, secret, hash string) bool` using `hmac.Equal`. Prefix: 8 chars from `[a-z0-9]` via `crypto/rand`; secret: 32 random bytes, base64url without padding.
  - Traps: 24.
  - Done: `go test ./internal/modules/erp/ -run Key -count=1`.

- [x] T040 [P] [US1] Add the repository interface in `internal/modules/erp/repository.go`
  - Do: `type Repository interface` with these methods (names chosen by this list): `FindKeyByPrefixAsSystem(ctx, prefix string) (KeyAuthRow, bool, error)`; `OrgStatusType(ctx, orgID int64) (status, orgType string, err error)`; `TouchLastUsed(ctx, orgID, keyID int64, ip string) error`; `InsertCallsAsSystem(ctx, []CallRecord) error`; `EnsureIntegration(ctx, orgID int64, provider string, createdBy int64, audit AuditRecord) (Integration, error)`; `InsertKey(ctx, orgID int64, k NewKey, audit AuditRecord) (APIKey, error)`; `GetKey(ctx, orgID, keyID int64) (APIKey, error)` (returns `ErrNotFound` for a missing id); `ListKeys(ctx, orgID int64) ([]APIKey, error)`; `SetKeyExpiry(ctx, orgID, keyID int64, at time.Time, audit AuditRecord) error`; `SetKeyRateLimit(ctx, orgID, keyID int64, ratePerMinute int, audit AuditRecord) error`; `RevokeKey(ctx, orgID, keyID int64, reason string, by int64, audit AuditRecord) error`; `SetIntegrationStatus(ctx, orgID, integrationID int64, status string, audit AuditRecord) error`; `WarehouseInOrg`, `BranchInOrg`, `ActiveMember(ctx, orgID, id int64) (bool, error)`; `ListKeysAsStaff(ctx) ([]StaffKeyRow, error)`; `RevokeKeyAsStaff(ctx, keyID int64, reason string, by int64, audit AuditRecord) error`. Define `AuditRecord{Action, EntityType, EntityID string; Before, After any; ActorUserID int64; ActorKeyID *int64; ActorKind string}`, `NewKey`, `StaffKeyRow` in the same file; `var ErrNotFound`.
  - Done: `go build ./internal/modules/erp/`.

- [x] T041 [P] [US1] Add ports in `internal/modules/erp/ports.go`
  - Do: `type Ports struct{}` with a comment that each later slice adds its ports here. Do not add interfaces nothing uses yet.
  - Traps: 23.
  - Done: `go build ./internal/modules/erp/`.

- [x] T042 [US1] Add the service shell in `internal/modules/erp/service.go`
  - Do: `type Config struct{Pepper []byte; Tag string; DefaultRate, MaxBatchItems int; Provider string}` (the caller sets `Provider` to `provider.DefaultProvider`, created by the next task; never type the provider name), `type Service struct`, `func NewService(repo Repository, ports Ports, cfg Config, log *slog.Logger) *Service`. Keep under 100 lines.
  - Done: `go build ./internal/modules/erp/`.

- [x] T042a [US1] Add the provider name constant in `internal/modules/erp/provider/registry.go`
  - Do: create the file with `package provider`, `const DefaultProvider = "orgasoft"` and `func Names() []string { return []string{DefaultProvider} }`. Task T093 extends this file with the registry map and `Get`; this is the only non-test, non-doc place outside `provider/orgasoft` that may contain the string `orgasoft`. Every other place uses `provider.DefaultProvider` or `provider.Names()`.
  - Traps: 28.
  - Done: `gofmt -l internal/modules/erp/provider/registry.go` prints nothing and `go build ./internal/modules/erp/provider/`.

- [x] T043 [US1] Write the AsSystem guard test in `internal/modules/erp/postgres/asystem_test.go`
  - Do: `TestAsSystemOnlyInNamedFunctions`: use `go/parser` on the package directory; fail when `database.AsSystem(` is called inside a function whose name does not end in `AsSystem` or `AsStaff`. Each allowed function must have a comment containing one of `plan.md E1` to `plan.md E7`.
  - Traps: 1.
  - Done: `gofmt -l internal/modules/erp/postgres/asystem_test.go` prints nothing; `go test ./internal/modules/erp/postgres/ -run AsSystem -count=1` passes on an empty package (GREEN is expected; it guards the next tasks).

- [x] T044 [US1] Write the key repository integration test in `test/integration/erp_keys_test.go`
  - Do: `TestERPKeysRepo` against `postgres.New(db)`: `EnsureIntegration` twice returns the same row; `InsertKey` then `FindKeyByPrefixAsSystem` finds it and an unknown prefix returns not found; `TouchLastUsed` twice within a minute changes `last_used_at` once; `RevokeKey`; `SetKeyRateLimit` changes `rate_limit_per_minute` and leaves one audit row; rotation sets old `expires_at`, both keys authenticate until then; `ListKeys(orgA)` never returns org B keys; `ListKeysAsStaff` returns both; every mutation left one `platform.audit_log` row with `actor_kind`.
  - Traps: 1, 15, 19.
  - Done: `gofmt -l test/integration/erp_keys_test.go` prints nothing; the test fails to compile (RED).

- [x] T045 [US1] Add the Postgres repository base in `internal/modules/erp/postgres/repository.go`
  - Do: `type Repository struct{ db *database.DB }`, and `func New(db *database.DB) *Repository` (this task creates the file with `New` only; the compile-time check `var _ erp.Repository = (*Repository)(nil)` is added by T118).
  - Use: `internal/platform/database/query_helpers.go` for `InTx`, `InReadTx`, `WithTenant`.
  - Traps: 1.
  - Done: `go build ./internal/modules/erp/postgres/`.

- [x] T046 [US1] Add the audit writer in `internal/modules/erp/postgres/audit.go`
  - Do: unexported `writeAudit(ctx, tx pgx.Tx, orgID int64, a erp.AuditRecord) error` calling `database.WriteAudit` with `ActorKind` defaulting to `user` and `ActorKeyID` copied.
  - Traps: 15.
  - Done: `go build ./internal/modules/erp/postgres/`.

- [x] T047 [P] [US1] Add integration SQL in `internal/modules/erp/postgres/integrations.go`
  - Do: `EnsureIntegration` (find active row for `(org, provider)` or insert, audit in the same `InTx`), `SetIntegrationStatus`, `OrgStatusType` via `db.QueryRowUnscoped` on `org.organizations` (read status and type only), `WarehouseInOrg`, `BranchInOrg`, `ActiveMember` (active `org.members` row).
  - Traps: 1, 15.
  - Done: `go build ./internal/modules/erp/postgres/`.

- [x] T048 [P] [US1] Add key SQL in `internal/modules/erp/postgres/keys.go`
  - Do: `InsertKey`, `GetKey`, `ListKeys`, `SetKeyExpiry`, `SetKeyRateLimit`, `RevokeKey` (all `WithTenant`, audit in the same `InTx`); `FindKeyByPrefixAsSystem` (comment `plan.md E1`, `WHERE key_prefix = $1`, joins the integration status, returns one row); `TouchLastUsed` (update only when `last_used_at IS NULL OR last_used_at < now() - interval '1 minute'`); `ListKeysAsStaff`, `RevokeKeyAsStaff` (comment `plan.md E5`).
  - Traps: 1, 15, 24.
  - Done: `go test ./internal/modules/erp/postgres/ -count=1` and `SB go test -p 1 ./test/integration/ -run TestERPKeysRepo -count=1`.

- [x] T049 [P] [US1] Add call-log SQL in `internal/modules/erp/postgres/calls.go`
  - Do: `InsertCallsAsSystem` (comment `plan.md E4`, insert only) inside `db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error { _, err := tx.CopyFrom(ctx, pgx.Identifier{"erp","api_calls"}, cols, pgx.CopyFromRows(rows)); return err })`; never `db.Pool()`.
  - Traps: 1, 24.
  - Done: `go build ./internal/modules/erp/postgres/` and `go test ./internal/modules/erp/postgres/ -count=1`.

- [ ] T050 [US1] Add the S1 fake repository in `internal/modules/erp/fake_keys_test.go`
  - Do: `type fakeRepo struct` implementing `erp.Repository` in memory (maps for keys and integrations, a slice of `AuditRecord`, a hook to set org status). Used by the next tests.
  - Done: `gofmt -l internal/modules/erp/fake_keys_test.go` prints nothing.

- [x] T051 [US1] Write the key service tests in `internal/modules/erp/keys_service_test.go`
  - Do: only an owner may create (when `CreateKeyInput.ActorIsOwner` is false the call returns `ERP_VALIDATION` with `details.fields` naming `actor`, because no error code names this case); warehouse or branch of another organization refused with `ERP_VALIDATION`; buying user not an active member refused with `ERP_VALIDATION` and `details.fields=["buyingUserId"]` (`ERP_PO_BUYING_USER_MISSING` is for purchase orders only, T220); `CreateKey` returns the full key once and the stored record has only prefix and hash; rotation: new key active, old `expires_at = now + grace`, grace above 72 h refused; `RevokeKey` for org A on a key of org B returns the same error as a missing id (`ERP_NOT_FOUND`, US1.3); every mutation recorded one `AuditRecord`; `ListKeys` shows only own keys, `ListKeysAsStaff` shows all; default scopes follow `DefaultScopes`; `UpdateKeyLimits` stores a value from `MinRatePerMinute` to `MaxRatePerMinute`, refuses 9 and 6001 with `ERP_VALIDATION` and `details.fields=["ratePerMinute"]`, refuses a non-owner, refuses a key of another organization with `ERP_NOT_FOUND`, and records one `AuditRecord`; a non-owner rotate is refused with no mutation or audit; an owner of org B changing org A’s key gets the same error as a missing key; after `RotateKey` the new key has the old key's scopes, warehouse, branch, buying user, rate limit and max batch items.
  - Done: `gofmt -l internal/modules/erp/keys_service_test.go` prints nothing; `go test ./internal/modules/erp/ -run KeyService -count=1` fails to compile (RED).

- [x] T052 [US1] Add the key service in `internal/modules/erp/keys_service.go`
  - Do: methods on `*Service`: `CreateKey(ctx, CreateKeyInput) (CreatedKey, error)`, `RotateKey(ctx, orgID, keyID int64, grace time.Duration, actorUserID int64, actorIsOwner bool) (CreatedKey, error)`, `RevokeKey(ctx, orgID, keyID int64, reason string, actorUserID int64) error`, `UpdateKeyLimits(ctx, orgID, keyID int64, ratePerMinute int, actorUserID int64, actorIsOwner bool) error` (owner only, range check, calls `SetKeyRateLimit`), `PauseIntegration(ctx, orgID int64, pause bool, actorUserID int64) error`, `ListKeys(ctx, orgID int64) ([]APIKey, error)`, `ListKeysAsStaff(ctx) ([]StaffKeyRow, error)`, `RevokeKeyAsStaff(ctx, keyID int64, reason string, actorUserID int64) error`. For rotate and limits, reject `actorIsOwner=false` with `ERP_VALIDATION` and `details.fields` naming `actor`, with no change or audit. Use `GenerateKey` from `keys.go` with tag `Config.Tag`. `RotateKey` copies scopes, warehouse, branch, buying user, rate limit and max batch items from the old key.
  - Traps: 1, 15, 24.
  - Done: `go test ./internal/modules/erp/ -run KeyService -count=1`.

- [x] T053 [US1] Write the authentication tests in `internal/modules/erp/authenticate_test.go`
  - Do: wrap the real service HMAC function with a counter and assert exactly one call for unknown prefix, wrong secret, revoked, expired, and valid keys. Unknown prefix and wrong secret return identical `ERP_AUTH_INVALID` errors to the caller; all authentication failures use the same error and message. Test paused/closed integration, unapproved organization, approval restored, principal fields, and successful last-used touch.
  - Done: `gofmt -l internal/modules/erp/authenticate_test.go` prints nothing; `go test ./internal/modules/erp/ -run Authenticate -count=1` fails to compile (RED).

- [x] T054 [US1] Add `Authenticate` in `internal/modules/erp/authenticate.go`
  - Do: `func (s *Service) Authenticate(ctx context.Context, raw, ip string) (Principal, error)`. Constructor initializes an unexported service HMAC function using the real pepper; call it exactly once on every path and compare using constant-time comparison, including a fixed dummy hash for unknown prefix. Check key, integration and organization status after hashing. All failures return the same error and message. No global, exported setter, or configuration switch.
  - Traps: 1, 24.
  - Done: `go test ./internal/modules/erp/ -run Authenticate -count=1`.

- [x] T055 [US1] Write the call logger test in `internal/modules/erp/calllog_test.go`
  - Do: `Record` x 200 flushes without waiting; fewer rows flush within 1 s (use a short injected interval); a full 4,096 channel drops and counts; `Close(ctx)` flushes the rest; a `CallRecord` never has an `Authorization` value. Use a fake `InsertCallsAsSystem`.
  - Done: `gofmt -l internal/modules/erp/calllog_test.go` prints nothing; `go test ./internal/modules/erp/ -run CallLog -count=1` fails to compile (RED).

- [x] T056 [US1] Add the call logger in `internal/modules/erp/calllog.go`
  - Do: `CallLogger` with `NewCallLogger(repo Repository, log *slog.Logger)`, `Record(CallRecord)` into a 4,096-slot channel, a goroutine that flushes every 1 s or 200 rows, `Dropped() int64`, `Close(ctx)`. Start the goroutine from `Run(ctx)` so `cmd/server` can start it with `safe.Go`.
  - Traps: 24.
  - Done: `go test ./internal/modules/erp/ -run CallLog -count=1`.

- [x] T057 [US1] Write the envelope tests in `internal/modules/erp/http/respond_test.go`
  - Do: `writeError` outputs `{"status":"error","requestId","error":{"code","message":{"ar","en"},"details"}}` with non-empty `ar` and `en` for every code in `erp.AllCodes`; `writeOK` outputs `{"status":"ok","requestId","data"}`; every placeholder in the table of T058 is filled from the named `details` key, and a missing key leaves the placeholder text unfilled and does not panic; response header `X-Request-ID` equals `requestId`; request id comes from `observability.RequestIDFrom`.
  - Traps: 9, 12.
  - Done: `gofmt -l internal/modules/erp/http/respond_test.go` prints nothing; `go test ./internal/modules/erp/http/ -run Respond -count=1` fails (RED).

- [x] T058 [US1] Add the envelope writer in `internal/modules/erp/http/respond.go`
  - Do: `writeOK(w, r, status int, data any)`, `writeError(w, r, err error)` (non-`*erp.Error` becomes `ERP_INTERNAL` 500 and is logged without the body), message built with `i18n.T(i18n.AR, ...)` and `i18n.T(i18n.EN, ...)`. Do not use `httpx.Error`. Set the `X-Request-ID` header to the request id explicitly if it is absent (`w.Header().Set`). Placeholders are filled from `details` by this table: `ERP_RATE_LIMITED` uses `details.retryAfterSeconds`; `ERP_BODY_TOO_LARGE` uses `details.limitBytes/1048576`; `ERP_BATCH_TOO_LARGE` uses `details.limit`; `ERP_PO_ITEM_NOT_FOUND`, `ERP_PO_NO_ACTIVE_OFFER`, `ERP_PO_QUANTITY_INVALID` and `ERP_PO_SUPPLIER_NOT_LINKED` use `details.line`; `ERP_ORDER_STATE_CONFLICT` uses `details.currentStatus`; `ERP_PO_REFUSED` uses `details.reason`. A missing key leaves the placeholder text unfilled and must not panic.
  - Traps: 9, 12.
  - Done: `go test ./internal/modules/erp/http/ -run Respond -count=1`.

- [x] T059 [US1] Write the middleware tests in `internal/modules/erp/http/auth_test.go`
  - Do: missing header `ERP_AUTH_MISSING`; bad format fails normally; 100 successful calls do not consume the per-IP failure limit; failures up to the limit return the normal error and the next attempt returns 429 without invoking an injected authenticate function (the existing erp test establishes one HMAC per Authenticate call); a valid key from a blocked IP remains blocked until expiry; failed ERP attempts do not affect the website login limiter; Redis failure does not block valid keys; per-key rate headers and `Retry-After`; successful principal and tenant context ignore caller-supplied organization; call log uses route pattern; query key ignored. Use the trusted client IP helper. Tests that need the API explicitly enable it.
  - Done: `gofmt -l internal/modules/erp/http/auth_test.go` prints nothing; `go test ./internal/modules/erp/http/ -run Auth -count=1` fails (RED).

- [x] T060 [US1] Add rate-limit helpers in `internal/modules/erp/http/ratelimit.go` and read-only failure check and failure-only increment next to `AllowKey` in `internal/platform/httpx/ratelimit.go`
  - Do: separate read-only IP failure count check and increment used only after failed authentication, using an ERP-specific namespace distinct from per-key and website LimitByIP keys; check at or above `erp.AuthFailuresPerIPPerMinute` without increment and return retry TTL. Keep the existing one-minute window. Blocked requests do not extend it. Redis errors are logged and fail open. `checkKeyLimit` uses `AllowKey` and the principal rate; set limit headers. Do not change `AllowKey`.
  - Done: `go build ./internal/modules/erp/http/`.

- [x] T060a [US1] Add the handler shell in `internal/modules/erp/http/handler.go`
  - Do: define only `Handler` and `NewHandler(svc *erp.Service, cfg erp.Config, lim *httpx.Limiter, log *slog.Logger)` with the dependencies later tasks use; no routes yet.
  - Done: `go build ./internal/modules/erp/...`.

- [x] T061 [US1] Add the auth middleware in `internal/modules/erp/http/auth.go`
  - Do: use the platform trusted client IP helper. Check the ERP-specific failure counter read-only first; if blocked return 429 and `Retry-After` without authenticating or incrementing. Authenticate; on failure increment the counter and return the normal error, on success do not increment or reset it. Log Redis failures and continue authentication; per-key limiting counts every authenticated request. Set principal and `database.WithTenant` context, ignoring caller-supplied tenant. Middleware depends on a small authentication interface or function; production passes the real Service.Authenticate and has no alternate production path. After the handler record the call using the route pattern; failed authentication records no organization.
  - Traps: 1, 24.
  - Done: `go test ./internal/modules/erp/http/ -run Auth -count=1`.

- [x] T062 [US1] Write the handler tests in `internal/modules/erp/http/handler_test.go`
  - Do: `GET /health` with a valid key returns 200 and asserts `data.organization{id,name{ar,en},type}`, `data.integration{provider,status}`, `data.key{prefix,scopes,warehouse{id,name},branch{id},rateLimitPerMinute,expiresAt}`, `data.apiVersion`, `data.serverTime`; `GET /openapi.yaml` works without a key; an unknown route under the prefix returns the bilingual `ERP_NOT_FOUND`; captured logs and every response body for a bad key contain neither the key nor the secret part (grep the buffer).
  - Traps: 24.
  - Done: `gofmt -l internal/modules/erp/http/handler_test.go` prints nothing; `go test ./internal/modules/erp/http/ -run Handler -count=1` fails (RED).

- [x] T063 [P] [US1] Add the health route in `internal/modules/erp/http/health.go`
  - Do: `func (h *Handler) health(w, r)` returning exactly the `HealthResponse` schema and example in `openapi.yaml`; `warehouse.name` comes from `inventory.warehouses.name`. Call `HealthData` with the authenticated principal; never accept organization or key IDs from the request. It reads under `WithTenant(principal.OrganizationID)` and explicit organization/key filters without AsSystem.
  - Done: `go build ./internal/modules/erp/http/`.

- [x] T064 [P] [US1] Add the embedded spec in `internal/modules/erp/http/spec.go` and `spec/openapi.yaml`
  - Do: copy `specs/002-erp-integration-api/contracts/openapi.yaml` to `internal/modules/erp/http/spec/openapi.yaml`; `spec.go` embeds it with `go:embed` and serves `GET /openapi.yaml` as `application/yaml`.
  - Done: `diff -q specs/002-erp-integration-api/contracts/openapi.yaml internal/modules/erp/http/spec/openapi.yaml` prints nothing, and `go build ./internal/modules/erp/http/`.

- [x] T065 [US1] Add the handler root in `internal/modules/erp/http/handler.go`
  - Do: `func (h *Handler) Routes() chi.Router` mounts `/health` inside the authenticated group and `/openapi.yaml` without auth, as specified by OpenAPI; unknown paths return `ERP_NOT_FOUND`. The constructor already exists from T060a.
  - Done: `go test ./internal/modules/erp/http/ -count=1`.

- [x] T066 [US1] Write the real server router test in `cmd/server/erp_router_integration_test.go`
  - Do: use the production router and real ERP service with sandbox Postgres and Redis; set `ERP_API_ENABLED=true` and a test pepper of at least 32 bytes in the test. Seed unique organization, integration and key through `db.InTx(database.AsSystem(ctx), ...)`. Assert valid health 200, revoked key refused, wrong key refused then failure limit 429, and disabled API has no health route while the website responds. Skip only when no database URL is set; sandbox skip fails.
  - Done: `SB-6 go test -p 1 ./cmd/server/ -run TestRouterERPRealWiring -count=1 -v` visibly PASSes without a skip.

- [x] T067 [US1] Add `mountERP` in `cmd/server/erp_wiring.go`
  - Do: `mountERP(r, cfg, log, deps, db, idSvc, docsGate) *erp.Service` (plan.md S1 step 15 names this parameter list; take the types from the neighbouring `mount*` calls in `cmd/server/routes.go`; return without mounting any ERP routes when disabled). Builds `postgres.New(db)`, `erp.NewService` with `Config.Provider` set to `provider.DefaultProvider` (never typed), creates `lim := httpx.NewLazyLimiter(func() *redis.Client { return deps.CacheHandle().Redis() }, "dawa24:erp:ratelimit:")` inside `mountERP` (pattern of `cmd/server/routes.go:78`) and passes it to `erpHttp.NewHandler`, starts `CallLogger.Run` with `safe.Go`, mounts at `/erp/v1`. Disabled mode mounts no `/erp/v1` route.
  - Traps: 9, 10, 20.
  - Done: `go build ./cmd/server/`.

- [x] T068 [US1] Add `erpSvc` and `SetERP` in `internal/ui/handlers.go`
  - Do: field `erpSvc *erp.Service` on the UI handler struct and `func (h *UIHandler) SetERP(s *erp.Service)`. The `ui` package may import `erp`.
  - Traps: 10.
  - Done: `go build ./internal/ui/`.

- [x] T069 [US1] Call `mountERP` in `cmd/server/routes.go`
  - Do: test that disabled mode mounts no `/erp/v1` route and leaves website, n8n and existing routes unchanged; call it in `mountModuleRoutes` right after `mountModuleRoutesAPI` (about line 74) and before `buildUIHandler`; after the UI handler exists call `uiHandler.SetERP(erpSvc)`. `SetERP` exists because the previous task added it.
  - Done: `go build ./... && go test ./cmd/server/ -run Router -count=1`.

- [ ] T070 [US1] Write the key page tests in `internal/ui/erp_keys_test.go`
  - Do: names start with `TestErp`: create requires `actor.IsOwner` (non-owner gets the notice, no key row); the full key appears in the create response once and a reload shows only `key_prefix`; rotate and revoke require owner; the admin page lists every organization's keys and revoke works for staff; a pharmacy owner sees only its own keys under `/customer/integrations`; the form rejects unknown scopes; the limits form saves a value from 10 to 6000 for an owner, refuses 9 and 6001, and refuses a non-owner even when a posted `is_owner` or `actorIsOwner` field is set; the modals are rendered through `components.Modal`.
  - Done: `gofmt -l internal/ui/erp_keys_test.go` prints nothing; `go test ./internal/ui/ -run Erp -count=1` fails (RED).

- [ ] T071 [US1] Add the key form parser in `internal/ui/erp_keys_forms.go`
  - Do: `parseCreateKeyForm(r *http.Request, vendor bool) (erp.CreateKeyInput, []formError)`, `parseRotateForm`, `parseRevokeForm`, `parseLimitsForm`; the provider is chosen from `provider.Names()` and defaults to `provider.DefaultProvider` (never typed); scopes checked with `erp.ValidScope`; defaults from `erp.DefaultScopes(vendor)`; name length 1 to 80; rate between `MinRatePerMinute` and `MaxRatePerMinute`; messages from `erp.keys.*` keys.
  - Done: `go build ./internal/ui/`.

- [ ] T072 [P] [US1] Add the key list page in `internal/ui/pages/erp_keys.templ`
  - Do: RTL-first page using `layouts.VendorShell` for suppliers and `layouts.ShellFor` for pharmacies (one templ func per shell, same body component). Table columns: name, prefix, scopes, last used, expires, status, change limit, rotate and revoke. Use `components` that already exist. Run templ generate (form from V3).
  - Traps: 22.
  - Done: `go build ./internal/ui/pages/` and the `_templ.go` file exists.

- [ ] T073 [P] [US1] Add the key modals in `internal/ui/pages/erp_key_modals.templ`
  - Do: create form modal, show-once key modal, rotate modal, revoke modal, change-limit modal (one number input, 10 to 6000), all built with `components.Modal`. Run templ generate.
  - Traps: 22.
  - Done: `go build ./internal/ui/pages/` and `make check-modal-handwritten`.

- [ ] T074 [P] [US1] Add the staff page in `internal/ui/pages/admin_erp_keys.templ`
  - Do: `layouts.AdminShell` page listing every key with organization name, prefix, status, last used and a Revoke button. Run templ generate.
  - Traps: 22.
  - Done: `go build ./internal/ui/pages/`.

- [ ] T075 [US1] Add the key handlers in `internal/ui/erp_keys_handlers.go`
  - Do: `ErpKeysPage`, `ErpKeyCreate`, `ErpKeyRotate`, `ErpKeyRevoke`, `ErpKeyLimits` (calls `UpdateKeyLimits`), `ErpIntegrationPause` for both vendor and customer audiences (an audience flag picks permission keys and shell). Owner check `actor.IsOwner`; pass this signed-in actor value as `actorIsOwner` to rotate and limits and as `ActorIsOwner` to create, never read ownership from submitted fields. The full key is returned only in the create and rotate responses.
  - Use: `h.erpSvc` methods from `keys_service.go`.
  - Traps: 24, 29.
  - Done: `go test ./internal/ui/ -run Erp -count=1` (vendor and pharmacy cases pass).

- [ ] T076 [US1] Add the staff handlers in `internal/ui/erp_admin_handlers.go`
  - Do: `ErpAdminKeysPage`, `ErpAdminKeyRevoke` calling `ListKeysAsStaff` and `RevokeKeyAsStaff`.
  - Traps: 1, 29.
  - Done: `go test ./internal/ui/ -run Erp -count=1`.

- [ ] T077 [P] [US1] Register vendor routes in `internal/ui/vendor_routes_erp.go`
  - Do: do not edit `internal/ui/vendor_routes.go` (384 lines) beyond one call line. Create `internal/ui/vendor_routes_erp.go` with `func (h *UIHandler) registerVendorERPRoutes(g chi.Router)` using the parameter name `g` (not `r`, so the `reProtectedPath` regex in `test/route_audience_test.go` ignores it) and declaring each group as `g.Group(func(gg chi.Router) { gg.Use(authctx.RequireTenantPagePermission(...)); gg.Get(...) })`. Routes: `GET /vendor/integrations`, `POST /vendor/integrations/keys`, `POST /vendor/integrations/keys/{id}/rotate`, `POST /vendor/integrations/keys/{id}/revoke`, `POST /vendor/integrations/keys/{id}/limits`, `POST /vendor/integrations/pause`, each with a gate naming `vendor.integration.view` or `vendor.integration.manage` (`limits` uses `vendor.integration.manage`). Call it as the last statement of `registerVendorRoutes` in `vendor_routes.go` (the registrar that `RegisterVendorRoutes` in `handlers.go` calls). Run `TestUIRoutesAreAudienceGated`.
  - Traps: 11, 29.
  - Done: `go test ./test/ -run 'TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1` and `make check-file-size`.

- [ ] T078 [P] [US1] Register pharmacy routes in `internal/ui/customer_routes.go`
  - Do: the same six routes (including `POST /customer/integrations/keys/{id}/limits`, gate `pharmacy.integration.manage`) under `/customer/integrations` with `pharmacy.integration.view`/`manage`.
  - Traps: 29.
  - Done: `go test ./test/ -run 'TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1`.

- [ ] T079 [P] [US1] Register admin routes in `internal/ui/admin_routes_platform.go`
  - Do: `GET /admin/integrations` (`platform.integration.view`) and `POST /admin/integrations/keys/{id}/revoke` (`platform.integration.revoke`).
  - Traps: 29.
  - Done: `go test ./internal/ui/ -run Erp -count=1 && go test ./test/ -run 'TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated|TestModulesDoNotImportEachOther' -count=1` and `make check-modal-handwritten check-hardcoded-arabic`.

- [ ] T080 [US1] S1 slice gate
  - Do: run the slice gate from "Rules for every task". Changed packages: `./internal/platform/database/ ./internal/platform/config/ ./internal/platform/httpx/ ./internal/shared/i18n/ ./internal/platform/rbac/ ./internal/modules/erp/... ./cmd/server/ ./internal/ui/...`. Focused tests: `go test ./internal/platform/database/ ./internal/platform/config/ ./internal/platform/httpx/ ./internal/shared/i18n/ ./internal/platform/rbac/ ./internal/modules/erp/... ./cmd/server/ -run 'Audit|ERP|AllowKey|Erp|Key|Authenticate|CallLog|Respond|Auth|Handler|Router|AsSystem|Codes|Principal' -count=1`; `go test ./internal/ui/ -run Erp -count=1`; `SB-6 go test -p 1 ./test/integration/ -run 'TestTenantIsolation_ERP|TestERPKeysRepo|TestAuditActorKeyRoundTrip' -count=1`; `go test ./test/ -run 'TestModulesDoNotImportEachOther|TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1`.
  - Commit message: `feat(erp): S1 keys and machine principal`. No push, merge or deploy.
  - Done: `git log -1 --format=%s` prints the commit message and `git status --short` lists no file other than the four baselines and the spec folder.

## Phase 4: S2 Product sync [US2]

Goal: the ERP sends batches, reads per-item results; suppliers resolve review items. Depends on S1. Independent test: a batch with a new, a known and an invalid item reports each correctly; repeating it creates no duplicates.

- [ ] T081 [US2] Write the queue args test in `internal/platform/queue/queue_erp_test.go`
  - Do: `ERPSyncChunkArgs{BatchID, OrganizationID int64; ChunkNo int}` has Kind `erp.sync_chunk`, `InsertOpts` queue `erpsync` and `MaxAttempts 3`; `ERPCleanupArgs{}` has Kind `erp.cleanup`, queue `maintenance`, `MaxAttempts 1`. If V1 said yes, also `TestNewInsertOnly`: building the client from a `database.DB` returns no error and has no workers (use the same DB setup as the existing queue tests).
  - Traps: 4, 5, 20.
  - Done: `gofmt -l internal/platform/queue/queue_erp_test.go` prints nothing; `go test ./internal/platform/queue/ -run ERP -count=1` fails to compile (RED).

- [ ] T082 [US2] Add the job args in `internal/platform/queue/jobs.go`
  - Do: the two Args types with `Kind()` and `InsertOpts()` in the style of the existing args in the file. Do not use `river.UniqueOpts`.
  - Traps: 4, 5.
  - Done: `go test ./internal/platform/queue/ -count=1` (includes `TestEveryInsertedQueueIsConfigured`).

- [ ] T083 [US2] Add `NewInsertOnly` in `internal/platform/queue/queue.go`
  - Do: `func NewInsertOnly(db *database.DB, log *slog.Logger) (*Client, error)`: a River client with no `Queues` and no `Workers`; reuse the existing `Client` type and `EnqueueTx`. Do not start it.
  - Traps: 20.
  - Done: `go test ./internal/platform/queue/ -count=1`.

- [ ] T084 [US2] Write the quota writer tests in `internal/modules/catalog/variant_quota_test.go`
  - Do: `TestSetVariantQuotaLimits`: values pass through `NormalizeQuotaLimit`; a nil pointer clears to NULL; an empty map is a no-op with no repository call; a repository without the optional interface returns a clear error. Copy the fake-repo style of the catalog tests next to `vendor_import.go`.
  - Done: `gofmt -l internal/modules/catalog/variant_quota_test.go` prints nothing; `go test ./internal/modules/catalog/ -run Quota -count=1` fails (RED).

- [ ] T085 [US2] Add the quota writer in `internal/modules/catalog/variant_quota.go`
  - Do: `type QuotaLimitWriter interface{ SetVariantQuotaLimits(ctx context.Context, orgID int64, limits map[int64]*int) error }` and `func (s *Service) SetVariantQuotaLimits(...)` type-asserting the repository, same pattern as `VariantWriter` in `vendor_import.go:90`.
  - Done: `go test ./internal/modules/catalog/ -run Quota -count=1`.

- [ ] T086 [US2] Add the quota SQL in `internal/modules/catalog/postgres/variant_quota.go`
  - Do: one `UPDATE catalog.product_variants SET quota_limit = u.q FROM unnest($2::bigint[], $3::int[]) AS u(id, q) WHERE organization_id = $1 AND product_variants.id = u.id` inside `db.InTx`. Confirm the real column name first with `grep -rn "quota_limit" db/migrations | head -3` and use it.
  - Done: `go build ./internal/modules/catalog/...` (the SQL itself is proved by the S2 flow test).

- [ ] T087 [P] [US2] Document the quota writer in `docs/modules/catalog.md`
  - Do: add a short section: `SetVariantQuotaLimits` is called by the ERP sync, writes `quota_limit` only, null clears.
  - Done: `grep -n "SetVariantQuotaLimits" docs/modules/catalog.md` prints a line.

- [ ] T088 [US2] Write the provider tests in `internal/modules/erp/provider/orgasoft/orgasoft_test.go`
  - Do: `DecodeProductBatch` on the two fixtures `../../testdata/orgasoft_products_camel.json` and `orgasoft_products_pascal.json`: code `4001` and `ORGA-100`, name from `arName`/`ArName`, price `1305.46` and `250` as `json.Number`, `quotaQuantity` 0 with `isQuota` false accepted, absent expiry accepted; inline cases: price as a JSON string `"62.50"`, `Items` PascalCase wrapper, unknown fields ignored, numeric code as a number and as a string, `clientBatchId`. `DecodeAck` on `orgasoft_order_ack_sample3.json` gives one entry with order number `{{orderNumber}}` and reference `ORG-9921` (and ignores `items`), and on the canonical `{"orders":[{"orderNumber":"10542","erpReference":"ORGA-77123"}]}`. Money fields must be `json.Number`, never `float64`.
  - Traps: 13, 28.
  - Done: `gofmt -l` on the file prints nothing; `go test ./internal/modules/erp/provider/... -count=1` fails to compile (RED).

- [ ] T089 [US2] Add the provider contract in `internal/modules/erp/provider/provider.go`
  - Do: `type Provider interface{ Name() string; DecodeProductBatch(body []byte) (Batch, error); DecodeAck(body []byte) ([]AckEntry, error) }`; `Batch{ClientBatchID string; Items []RawItem}`; `RawItem` as a typed struct, all numbers as `*json.Number`, internal names equal to the wire names: `Code, ArName, Barcode, Unit, Manufacturer, Batch, Expiry string; PublicPrice, Discount, Tax, Stock, QuotaQty *json.Number; IsQuota *bool` (there is no English name field: Orgasoft sends none; `Unit`, `Manufacturer` and `Batch`, wire name `batchNumber`, are optional, not in any Orgasoft sample, unverified, and ignored when absent); `AckEntry{OrderNumber, ErpReference string}`.
  - Traps: 13.
  - Done: `go build ./internal/modules/erp/provider/`.

- [ ] T090 [US2] Add the alias table in `internal/modules/erp/provider/orgasoft/fields.go`
  - Do: `var productAliases = map[string][]string{...}` keyed by the canonical wire name: `externalProductCode`, `arName`, `publicPrice`, `stock`, `discount`, `tax`, `isQuota`, `quotaQuantity`, `barcode`, `expiryDate`, and the optional `unit`, `manufacturer`, `batchNumber` (not in Orgasoft's samples, unverified); `func foldKey(k string) string` lower-cases the first letter so PascalCase equals camelCase. This is the only place to add spellings.
  - Traps: 28.
  - Done: `go build ./internal/modules/erp/provider/orgasoft/`.

- [ ] T091 [US2] Add the batch decoder in `internal/modules/erp/provider/orgasoft/orgasoft.go`
  - Do: `New() *Orgasoft`, `Name()` returns `orgasoft`, `DecodeProductBatch`: `json.Decoder` with `UseNumber()`; decode `items`/`Items` as `[]map[string]json.RawMessage`; fold each key; fill `RawItem` through `productAliases`; accept strings or numbers for code and money; ignore unknown fields.
  - Traps: 13.
  - Done: `go test ./internal/modules/erp/provider/... -run 'Products|Batch' -count=1`.

- [ ] T092 [US2] Add the ack decoder in `internal/modules/erp/provider/orgasoft/ack.go`
  - Do: `DecodeAck`: accept the canonical `orders[]` body, a single object with `order_number` and `orga_orderID`, and an array of those objects (sample 3); ignore `items`.
  - Done: `go test ./internal/modules/erp/provider/orgasoft/ -run Ack -count=1`.

- [ ] T093 [US2] Add the registry in `internal/modules/erp/provider/registry.go`, `internal/modules/erp/provider/all/all.go`, and `internal/modules/erp/providers.go`
  - Do: extend `provider/registry.go` (created by T042a, which keeps `const DefaultProvider = "orgasoft"`) with `func Register(p Provider)`, which panics on a duplicate name, plus `func Get(name string) (Provider, bool)`, and make `func Names() []string` return the sorted registered names. `registry.go` must NOT import `provider/orgasoft`, because that is an import cycle. In `provider/orgasoft` add `func init() { provider.Register(New()) }`. Create `provider/all/all.go` with `package all` and a blank import of `provider/orgasoft`. Create `internal/modules/erp/providers.go` with `package erp` and a blank import of `provider/all`. A future ERP adds one blank import in `all.go`.
  - Traps: 28.
  - Done: `go test ./internal/modules/erp/provider/... -count=1`.

- [ ] T094 [US2] Write the provider seam test in `internal/modules/erp/seam_test.go`
  - Do: `TestNoOrgasoftOutsideProvider`: walk `internal/modules/erp` (not `provider/orgasoft`, not `_test.go`, not `docs`) and fail when a `.go` file contains `orgasoft` except `provider/registry.go`, `provider/all/all.go` (the registry line and `DefaultProvider`). `TestNeverCallsEnhancement`: no `.go` file in `internal/modules/erp` contains `StartEnhancement` or imports `gateway`.
  - Traps: 18, 28.
  - Done: `go test ./internal/modules/erp/ -run 'NoOrgasoft|NeverCallsEnhancement' -count=1`.

- [ ] T095 [US2] Write the item tests in `internal/modules/erp/item_test.go`
  - Do: one table row per item rule in `contracts/error-codes.md` "Product sync (item level)": code required, name required, price missing and negative, `12.345` gives `ERP_ITEM_PRICE_PRECISION`, stock missing, `-1` gives `ERP_ITEM_STOCK_NEGATIVE`, stock `1.5`, discount `100.01`, tax `-1`, `isQuota true` with quantity 0, expiry `31/12/2027`, barcode `ABC`, barcode of 33 digits. Valid rows: both samples (quotaQuantity 0 with isQuota false is valid and gives `QuotaQty` nil and `IsQuota` false), `"2028-01-10"`, `"2027-09-30T00:00:00"` (date part kept), numeric text code. `Hash`: same item and warehouse and branch gives the same hash; a change in stock, price, warehouse or branch changes it. Prices as exact strings, for example `"1305.46"`.
  - Traps: 13.
  - Done: `gofmt -l internal/modules/erp/item_test.go` prints nothing; `go test ./internal/modules/erp/ -run Item -count=1` fails to compile (RED).

- [ ] T096 [US2] Add item normalisation in `internal/modules/erp/item.go`
  - Do: `type Item struct{ Code, ArName, Barcode, Unit, Manufacturer, Batch string; PublicPrice money.Amount; Discount, Tax *money.Amount; Stock int; Expiry *time.Time; IsQuota *bool; QuotaQty *int }` (no English name); `type ItemError struct{Code string; Details map[string]any}`; `func NormalizeItem(raw provider.RawItem) (Item, *ItemError)`; `func (i Item) Hash(warehouseID, branchID int64) string` (sha256 of a fixed-order text form). Money through `money.Parse`; percents compared with `money.FromMajor(100)`; no `float`.
  - Use: `internal/shared/money`.
  - Traps: 13.
  - Done: `go test ./internal/modules/erp/ -run Item -count=1`.

- [ ] T097 [US2] Write the S2 isolation tests in `test/integration/erp_isolation_sync_test.go`
  - Do: `TestTenantIsolation_ERPProductLinks`, `TestTenantIsolation_ERPSyncBatches`, `TestTenantIsolation_ERPSyncItems`, `TestTenantIsolation_ERPReviewItems` (separate file from the S1 one to keep both under 400 lines).
  - Traps: 19.
  - Done: `gofmt -l test/integration/erp_isolation_sync_test.go` prints nothing; `SB-6 go test -p 1 ./test/integration/ -run 'TestTenantIsolation_ERP(ProductLinks|SyncBatches|SyncItems|ReviewItems)' -count=1` passes (the tables already exist from 255).

- [ ] T098 [US2] Write the sync flow tests in `test/integration/erp_sync_flow_test.go`
  - Do: `TestERPSyncFlow_Basic`: seed a vendor organization, warehouse, branch, key principal and one existing variant with SKU `4001`; call `AcceptBatch` then `ProcessChunk` directly (no River) with three items from the two fixtures plus one invalid price `12.345`; assert created, updated (known), failed `ERP_ITEM_PRICE_PRECISION`, stock row for the key's warehouse; absent product untouched; stock 0 writes quantity 0; duplicate code in one batch: last wins, earlier row `unchanged` `ERP_ITEM_SUPERSEDED`. Real adapters from `internal/composition/erpports` with the matcher stubbed to return `needs_review` for unknown items.
  - Traps: 6, 16, 18.
  - Done: `gofmt -l test/integration/erp_sync_flow_test.go` prints nothing; compile fails (RED) until the S2 wiring tasks exist.

- [ ] T099 [US2] Write the sync repeat and order tests in `test/integration/erp_sync_repeat_test.go`
  - Do: `TestERPSyncFlow_Repeat100`: the same batch 100 times creates 0 new variants after the first run and 0 duplicate links (SC-004); `TestERPSyncFlow_TwoBatchesInOrder`: batch 2 chunk returns `ErrWaitForEarlierBatch` while batch 1 is `queued` or `processing`, then succeeds after batch 1 completes; `TestERPSyncFlow_SameBatchTwoChunks` with 2501 items completes with `chunks_done = chunk_count = 2`.
  - Traps: 5, 25.
  - Done: `gofmt -l test/integration/erp_sync_repeat_test.go` prints nothing; compile fails (RED) until the S2 wiring tasks exist.

- [ ] T099a [US2] Write the SC-002 scale test in `test/integration/erp_sync_scale_test.go`
  - Do: `TestERPSyncFlow_Scale50000`: build one batch of 50,000 valid items in memory (numeric codes `900000` to `949999`, price `10.50`, stock 5); call `AcceptBatch` and fail when it takes 5 s or more; assert `chunk_count = 20` and 20 `ERPSyncChunkArgs` enqueued; call `ProcessChunk` for chunk 0 to 19 one after the other (the matcher stub returns `accepted` with one seeded product id; real catalog and inventory adapters from `internal/composition/erpports`) and fail when the whole processing takes 7 minutes or more; assert the batch is `completed`, no item is `pending`, and `created_count + needs_review_count + failed_count = 50000`. One worker is stricter than the three workers of production. Log both durations with `t.Logf`. Time-based fixture ids.
  - Traps: 3, 5, 19.
  - Done: `gofmt -l test/integration/erp_sync_scale_test.go` prints nothing; compile fails (RED) until the S2 wiring tasks exist.

- [ ] T100 [US2] Add the sync repository interface in `internal/modules/erp/repository_sync.go`
  - Do: `type SyncRepository interface` with: `InsertBatch(ctx, orgID int64, b NewBatch, items []NewItemRow) (SyncBatch, error)` (one tx: batch row, `CopyFrom` items, optional `Enqueue` callback `func(ctx, tx pgx.Tx) error`); `FindBatchByIdempotency(ctx, orgID, integrationID int64, key string) (SyncBatch, bool, error)`; `GetBatchByPublicID(ctx, orgID int64, publicID string) (SyncBatch, error)`; `GetBatchByID(ctx, orgID, batchID int64) (SyncBatch, error)`; `ListSyncItems(ctx, orgID, batchID, afterID int64, state string, limit int) ([]SyncItemRow, error)` (empty `state` means all); `EarlierBatchActive(ctx, orgID, integrationID, batchID int64) (bool, error)`; `MarkBatchProcessing(ctx, orgID, batchID int64) error`; `LoadPendingItems(ctx, orgID, batchID int64, chunkNo int) ([]SyncItemRow, error)`; `LoadLinksByCode(ctx, orgID, integrationID int64, codes []string) (map[string]ProductLink, error)`; `FindVariantsBySKU(ctx, orgID int64, codes []string) (map[string][]int64, error)`; `FindVariantsByBarcode(ctx, orgID int64, barcodes []string) (map[string][]int64, error)`; `WriteResults(ctx, orgID int64, res []ItemResult, links []LinkWrite, reviews []ReviewWrite) error`; `FinishChunk(ctx, orgID, batchID int64) (done bool, err error)`; `ListReview`, `GetReview`, `ResolveReviewRow`, `DismissReviewRow`; `PurgeExpiredAsSystem(ctx, cut Cutoffs) error`. Add the types in the same file (field names mirror the table columns in camel-to-Pascal case): `NewBatch{IntegrationID, APIKeyID, WarehouseID, BranchID int64; ClientBatchID, IdempotencyKey, BodySHA256 string; ItemCount, ChunkCount int}`; `NewItemRow{Position, ChunkNo int; ExternalCode string; Payload []byte; State, Code string; Message map[string]string}` (`Message` keys `ar` and `en`); `SyncBatch` = every column of `erp.sync_batches` plus `Provider string` read from `erp.integrations`; `SyncItemRow` = every column of `erp.sync_items`; `ItemResult{ItemID int64; State, Code string; Message map[string]string; VariantID *int64}`; `LinkWrite{IntegrationID, VariantID int64; ProductID *int64; ExternalCode, LinkSource, LastHash string; TaxPercent *money.Amount}`; `ReviewWrite{IntegrationID, BatchID int64; ExternalCode, ReasonCode string; Payload, Candidates []byte}`; `ProductLink` = every column of `erp.product_links`; `ReviewItem` = every column of `erp.review_items`; `Page{After int64; Limit int}`. Embed `SyncRepository` in `Repository` (edit `repository.go`).
  - Done: `go build ./internal/modules/erp/`.

- [ ] T101 [P] [US2] Add batch SQL in `internal/modules/erp/postgres/sync.go`
  - Do: `InsertBatch` (inside `db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error { ... tx.CopyFrom(ctx, pgx.Identifier{"erp","sync_items"}, cols, pgx.CopyFromRows(rows)) ... })`, never `db.Pool()`; calls the enqueue callback inside the same tx), `FindBatchByIdempotency`, `GetBatchByPublicID`, `GetBatchByID` (joined to `erp.integrations` for `provider`), `EarlierBatchActive` (query from research D-09), `MarkBatchProcessing` (`UPDATE ... WHERE state='queued'`), `FinishChunk` (`chunks_done = chunks_done + 1 ... RETURNING`, the chunk that reaches `chunk_count` sets `completed`, `finished_at` and the counters), plus product link reads and writes.
  - Traps: 1, 5.
  - Done: `gofmt -l internal/modules/erp/postgres/sync.go` prints nothing (`go vet` is not required: the package compiles as a whole at T118).

- [ ] T102 [P] [US2] Add item SQL in `internal/modules/erp/postgres/sync_items.go`
  - Do: `ListSyncItems` (keyset by `sync_items.id`), `LoadPendingItems`, `WriteResults` (retried chunk skips rows that are not `pending`; clears `payload` when processed; writes links with `last_hash`, `tax_percent`, `link_source`); `ListSyncItems` filters on `state` when it is not empty.
  - Traps: 1, 25.
  - Done: `gofmt -l internal/modules/erp/postgres/sync_items.go` prints nothing (`go vet` is not required).

- [ ] T103 [P] [US2] Add review SQL in `internal/modules/erp/postgres/review.go`
  - Do: upsert one open `erp.review_items` row per `(integration, external_code)` (second sync updates `payload`, `candidates`, `batch_id`), `ListReview`, `GetReview`, `ResolveReviewRow`, `DismissReviewRow`.
  - Traps: 1.
  - Done: `gofmt -l internal/modules/erp/postgres/review.go` prints nothing (`go vet` is not required).

- [ ] T104 [P] [US2] Add variant lookup SQL in `internal/modules/erp/postgres/variants_lookup.go`
  - Do: `FindVariantsBySKU`, `FindVariantsByBarcode`: read-only SQL on `catalog.product_variants`, live variants only (`deleted_at IS NULL`), `WithTenant`, using `unnest($2::text[])`.
  - Traps: 1.
  - Done: `gofmt -l internal/modules/erp/postgres/variants_lookup.go` prints nothing (`go vet` is not required).

- [ ] T105 [US2] Add the S2 fakes in `internal/modules/erp/fake_sync_test.go`
  - Do: in-memory `SyncRepository`; fake `VariantWriter`, `StockWriter`, `Matcher` (returns configured decisions), `MatchMemory` (records `Remember` calls), `Enqueuer` (records args). Add the `Ports` fields they fill (next task) as the contract. Embed the sync fake in `fakeRepo` (`fake_keys_test.go`): `type fakeRepo struct { ...; *fakeSync }`, and make `newFakeRepo()` allocate it, so `fakeRepo` still satisfies `erp.Repository`.
  - Done: `gofmt -l internal/modules/erp/fake_sync_test.go internal/modules/erp/fake_keys_test.go` prints nothing; `go vet ./internal/modules/erp/` passes once T106 has added the types the fakes use.

- [ ] T106 [US2] Add the S2 ports in `internal/modules/erp/ports.go`
  - Do: add `VariantWriter`, `StockWriter`, `Matcher`, `MatchMemory`, `Enqueuer` exactly as in plan.md "Ports" with these types, whose fields mirror the catalog and inventory rows: `VariantWrite{Ref int; VariantID, ProductID, BranchID int64; Name map[string]string; SKU, Barcode, Unit, BatchNumber string; Price, Discount money.Amount; ExpiryDate *time.Time}` (`VariantID` 0 means insert; the adapter sets the catalog status to active for inserts; names mirror `catalog.ProductVariant`); `VariantWriteResult{Inserted, Updated int; Failures map[int]string; IDs map[int]int64; InsertedRefs map[int]bool}` (mirrors `catalog.VariantWriteResult`, `Failures` keyed by `Ref`); `StockWrite{Ref int; WarehouseID, ProductID, VariantID int64; Quantity int; HasQuantity bool}` (mirrors `inventory.StockWriteRow` and its `Stock`); `KnowledgeWrite{SourceNamespace, QuestionSignature, Kind string; TargetProductID *int64; TargetGranularity, PolicyVersion, NormalizerVersion, DisplayText string}` (identical to `matching.KnowledgeWrite`); fields on `Ports`. `erp` may import `productmatch` and `matchflow` from `internal/shared`. No `OrderDecider` or `PurchasePlacer` yet.
  - Traps: 10, 18, 23.
  - Done: `go build ./internal/modules/erp/` and `go vet ./internal/modules/erp/` (this proves the `fakeRepo` embedding of T105).

- [ ] T107 [US2] Write the intake tests in `internal/modules/erp/sync_intake_test.go`
  - Do: invalid items stored as `failed` and counted in `rejectedAtIntake`; duplicate codes: last wins, earlier `unchanged` with `ERP_ITEM_SUPERSEDED`; empty batch `ERP_BATCH_EMPTY`; above `p.MaxBatchItems` gives `ERP_BATCH_TOO_LARGE` with `limit` and `received`; 2,500 items make 1 chunk and 2,501 make 2, one `ERPSyncChunkArgs` per chunk enqueued in the same transaction; same `Idempotency-Key` and body hash returns the first accepted result and enqueues nothing; same key with a different hash gives `ERP_IDEMPOTENCY_CONFLICT`; a vendor scope check `ERP_SCOPE_MISSING` and org type check `ERP_CAPABILITY_REQUIRED`.
  - Done: `gofmt -l internal/modules/erp/sync_intake_test.go` prints nothing; `go test ./internal/modules/erp/ -run Intake -count=1` fails (RED).

- [ ] T108 [US2] Add intake in `internal/modules/erp/sync_intake.go`
  - Do: `func (s *Service) AcceptBatch(ctx context.Context, p Principal, body []byte, idemKey string) (Accepted, error)` as described in plan.md S2 step 6. Uses the provider from `provider.Get(p.Provider)`. Chunk number is `position / SyncChunkSize`. `Accepted{SyncID string; State string; ItemCount, RejectedAtIntake int}`. Never calls `StartEnhancement`.
  - Traps: 3, 5, 6, 25.
  - Done: `go test ./internal/modules/erp/ -run Intake -count=1`.

- [ ] T109 [US2] Write the chunk processing tests in `internal/modules/erp/sync_process_test.go`
  - Do: `ErrWaitForEarlierBatch` while an earlier batch is active; hash equal to `last_hash` gives `unchanged` and no writer call; SKU tier links without calling the matcher; barcode tier; two variants with one barcode gives review `ERP_ITEM_BARCODE_CONFLICT`; one `Matcher.Prepare` call per chunk for all remaining rows with `Scope{SourceNamespace:"supplier:<org>", Workflow: WorkflowVendor, EntryPoint:"erp_sync"}` and `ConsumerRef:"erp.sync:<batchID>:<chunkNo>"`; `accepted`/`confirmed` creates the link and calls `Remember` with `Kind: "supplier_code"` (tiers 1 and 2 never call `Remember`); other decisions create a review item and `needs_review`; soft-deleted linked variant gives review `ERP_ITEM_DELETED_LINK`; one `VariantWriter.Write` and one `StockWriter.Replace` per chunk with `HasQuantity: true` and the key's warehouse; quota set `SetQuotaLimits`; a failing item writes `ERP_ITEM_WRITE_FAILED` and the rest continue; the last chunk sets `completed`; a retried chunk does not rewrite finished rows; after the key is revoked or rotated following acceptance, `ProcessChunk` still completes the batch because the chunk context comes from the batch row, not from the key; `RetireVariantsExcept` is never called.
  - Traps: 5, 6, 17, 25.
  - Done: `gofmt -l internal/modules/erp/sync_process_test.go` prints nothing; `go test ./internal/modules/erp/ -run Process -count=1` fails (RED).

- [ ] T110 [US2] Write the gateway-off tests in `internal/modules/erp/sync_ai_off_test.go`
  - Do: `TestSyncWithGatewayOff`: 20 items, matcher stub returns `needs_review`, no gateway object exists anywhere in the test; assert the batch completes, all 20 items have a terminal state, linked items update, unknown ones are `needs_review` with a review row. `TestSyncMatcherErrorKeepsReview`: matcher returns an error, items become `needs_review` with `ERP_ITEM_MATCHER_UNAVAILABLE`. `TestSyncNeverCallsEnhancement`: the `Matcher` interface has exactly the method `Prepare` (compile-time assignment of a struct with only `Prepare`) and `erp` contains no `StartEnhancement` (read the files).
  - Traps: 18.
  - Done: `gofmt -l internal/modules/erp/sync_ai_off_test.go` prints nothing; `go test ./internal/modules/erp/ -run 'GatewayOff|NeverCallsEnhancement|MatcherError' -count=1` fails (RED).

- [ ] T111 [US2] Add item resolution in `internal/modules/erp/sync_resolve.go`
  - Do: `type chunkCtx struct{ OrganizationID, IntegrationID, WarehouseID, BranchID int64; Provider string }` and `resolveItems(ctx, cc chunkCtx, batchID, chunkNo, items []pendingItem) (resolved, error)` implementing steps (c) to (f) of plan.md S2 step 7: links by code in one query, `unchanged` by hash, tiers SKU then barcode, then one `Matcher.Prepare` call. Use `productmatch.Row`, `productmatch.QuestionSignature`, `productmatch.ExtractRowFacts` (see V4).
  - Traps: 17, 18.
  - Done: `go build ./internal/modules/erp/`.

- [ ] T112 [US2] Add the write step in `internal/modules/erp/sync_write.go`
  - Do: `writeResolved(ctx, cc chunkCtx, resolved) (writeOutcome, error)` for steps (g) and (h): one `VariantWriter.Write` (new variants `StatusActive`, `BranchID` from `cc`, `Unit` from the item when sent, and name `{ar: ArName, en: ArName}`), then `SetQuotaLimits`, then one `StockWriter.Replace`; write links, results and `Remember`. A per-row failure in the stock map becomes that item's `ERP_ITEM_WRITE_FAILED`.
  - Traps: 6, 8, 13, 16, 17.
  - Done: `go build ./internal/modules/erp/`.

- [ ] T113 [US2] Add chunk orchestration in `internal/modules/erp/sync_process.go`
  - Do: `func (s *Service) ProcessChunk(ctx context.Context, batchID int64, chunkNo int) error` doing step (a) wait check, (b) mark processing once, calling `resolveItems` and `writeResolved`, then step (i) `FinishChunk`. Takes the organization from `database.TenantFrom(ctx)`. It loads the batch with `GetBatchByID(ctx, orgID, batchID)` and builds `chunkCtx{OrganizationID, IntegrationID, WarehouseID, BranchID int64; Provider string}` from `erp.sync_batches` (`warehouse_id`, `branch_id`) joined to `erp.integrations.provider`; it passes `chunkCtx`, not `Principal`, to `resolveItems` and `writeResolved`, and never re-reads the key.
  - Traps: 3, 5, 25.
  - Done: `go test ./internal/modules/erp/ -run 'Process|GatewayOff|NeverCallsEnhancement|MatcherError|Intake' -count=1`.

- [ ] T114 [US2] Write the review service tests in `internal/modules/erp/sync_review_test.go`
  - Do: `ListReview` pages open items; `ResolveReview` creates the variant through the writer, links it, calls `Remember` with `Kind: "human_confirm"`, marks `resolved`, writes one audit row; `DismissReview` marks `dismissed`; resolving another organization's item returns `ERP_NOT_FOUND`.
  - Done: `gofmt -l internal/modules/erp/sync_review_test.go` prints nothing; `go test ./internal/modules/erp/ -run Review -count=1` fails (RED).

- [ ] T115 [US2] Add the review service in `internal/modules/erp/sync_review.go`
  - Do: `ListReview(ctx, orgID int64, page Page) ([]ReviewItem, error)`, `ResolveReview(ctx, orgID, id, productID, actorUserID int64) error`, `DismissReview(ctx, orgID, id, actorUserID int64) error`.
  - Traps: 15, 17.
  - Done: `go test ./internal/modules/erp/ -run Review -count=1`.

- [ ] T116 [US2] Write the cleanup tests in `internal/modules/erp/cleanup_test.go`
  - Do: `Cleanup` calls `PurgeExpiredAsSystem` once with cut-offs: api_calls 30 d, sync_items 7 d, sync_batches 90 d, resolved or dismissed review items 180 d, ERP `match_runs` 30 d, batch size `CleanupBatchRows`; `order_imports` and `idempotency_records` are never purged.
  - Done: `gofmt -l internal/modules/erp/cleanup_test.go` prints nothing; `go test ./internal/modules/erp/ -run Cleanup -count=1` fails (RED).

- [ ] T117 [US2] Add the cleanup service in `internal/modules/erp/cleanup.go`
  - Do: `type Cutoffs struct{ APICalls, SyncItems, SyncBatches, Review, MatchRuns time.Time; BatchRows int }`, `func (s *Service) Cleanup(ctx context.Context, now time.Time) error`.
  - Done: `go test ./internal/modules/erp/ -run Cleanup -count=1`.

- [ ] T118 [US2] Add cleanup SQL in `internal/modules/erp/postgres/cleanup.go`
  - Do: `PurgeExpiredAsSystem` (comment `plan.md E3`): fixed table list, loops `DELETE ... WHERE id IN (SELECT id ... LIMIT 10000)` until none remain. For ERP `catalog.match_runs` delete only from `catalog.match_runs WHERE consumer_ref LIKE 'erp.sync:%' AND created_at < $cut`; children cascade or null out (V2). Add `var _ erp.Repository = (*Repository)(nil)` at the end of `postgres/repository.go`; it must compile now.
  - Traps: 1.
  - Done: `go build ./internal/modules/erp/postgres/` (including the `var _ erp.Repository` line), `go test ./internal/modules/erp/postgres/ -count=1` and `go build ./...`.

- [ ] T119 [US2] Write the worker tests in `internal/modules/erp/jobs/sync_chunk_worker_test.go`
  - Do: the worker sets `database.WithTenant(ctx, args.OrganizationID)` before `ProcessChunk`; `ErrWaitForEarlierBatch` returns `river.JobSnooze(time.Duration(erp.SyncSnoozeSeconds) * time.Second)`; other errors pass through; cleanup worker calls `Cleanup`.
  - Done: `gofmt -l internal/modules/erp/jobs/sync_chunk_worker_test.go` prints nothing; `go test ./internal/modules/erp/jobs/ -count=1` fails to compile (RED).

- [ ] T120 [P] [US2] Add the chunk worker in `internal/modules/erp/jobs/sync_chunk_worker.go`
  - Do: `NewSyncChunkWorker(svc *erp.Service, log *slog.Logger)`, `Work(ctx, job *river.Job[queue.ERPSyncChunkArgs]) error`. Import the args from `internal/platform/queue` as the other `internal/modules/*/jobs/` workers do. Map `ErrWaitForEarlierBatch` to `river.JobSnooze(time.Duration(erp.SyncSnoozeSeconds) * time.Second)`; never type `15`.
  - Traps: 3, 5.
  - Done: `go test ./internal/modules/erp/jobs/ -run Sync -count=1`.

- [ ] T121 [P] [US2] Add the cleanup worker in `internal/modules/erp/jobs/cleanup_worker.go`
  - Do: `NewCleanupWorker(svc *erp.Service, log *slog.Logger)`, `Work(ctx, job *river.Job[queue.ERPCleanupArgs]) error`.
  - Done: `go test ./internal/modules/erp/jobs/ -count=1`.

- [ ] T122 [P] [US2] Add the catalog adapter in `internal/composition/erpports/catalog.go`
  - Do: `NewVariantWriter(svc *catalog.Service) erp.VariantWriter`; `Write` converts `erp.VariantWrite` to `catalog.VariantWriteRow` and calls `BulkWriteVariants`; `SetQuotaLimits` calls `SetVariantQuotaLimits`. Field conversion only.
  - Traps: 10.
  - Done: `go build ./internal/composition/...`.

- [ ] T123 [P] [US2] Add the inventory adapter in `internal/composition/erpports/inventory.go`
  - Do: `NewStockWriter(svc *inventory.Service) erp.StockWriter`; `Replace` calls `BulkWriteStocks(ctx, inventory.StockReplace, rows)` and returns the failure map keyed by `Ref`.
  - Traps: 10.
  - Done: `go build ./internal/composition/...`.

- [ ] T124 [P] [US2] Add the matching adapter in `internal/composition/erpports/matching.go`
  - Do: `NewMatcher(svc matching.Service) erp.Matcher` (`Prepare`) and `NewMatchMemory(svc matching.Service) erp.MatchMemory` (`Remember`, where `matching.Service` is an interface at `internal/modules/catalog/matching/service.go:22`, so no pointer; converting `erp.KnowledgeWrite` to `matching.KnowledgeWrite` at `internal/modules/catalog/matching/domain.go:104` field by field).
  - Traps: 10, 17.
  - Done: `go build ./internal/composition/... && go test ./test/ -run TestModulesDoNotImportEachOther -count=1`.

- [ ] T124a [US2] Record ADR-ERP-1 in `docs/adr/decisions.md`
  - Do: append `ADR-ERP-1` in the layout of the existing records (heading `## ADR-ERP-1 - Shared ERP port adapters`, line `**Status:** Accepted` with today's date from `date +%F`, then Context, Decision, Consequences). Context: the server and the worker both need the same four ports (catalog writer, stock writer, matcher, match memory). Decision: `internal/composition/erpports`, imported only from `cmd/`. Consequences: it is not covered by `TestModulesDoNotImportEachOther`; keep field conversion only. Do not edit `AGENTS.md` here (T205 only adds `erp` to the schema list).
  - Done: `grep -c "ADR-ERP-1" docs/adr/decisions.md` prints 1 or more.

- [ ] T125 [US2] Add the worker wiring in `cmd/worker/erp.go`
  - Do: `registerERPWorkers(workers *river.Workers, db *database.DB, matchSvc matching.Service, log *slog.Logger)` building the repository, the adapters, `erp.NewService` and registering both workers. Build the catalog and inventory services the same way `cmd/worker/matching.go` builds its services.
  - Done: `go build ./cmd/worker/`.

- [ ] T126 [US2] Wire the worker in `cmd/worker/main.go` and `cmd/worker/matching.go`
  - Do: change `registerMatchingWorkers` to also return its `matching.Service` (an interface value, no pointer); only when `ERP.Enabled` is true call `registerERPWorkers` and start the erpsync queue; add a worker test asserting disabled mode does not register or start erpsync and enabled mode does; add `river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour), ... ERPCleanupArgs ...)` to the returned periodic list.
  - Traps: 4.
  - Done: `go build ./cmd/worker/ && go test ./cmd/worker/ ./internal/platform/queue/ -count=1`.

- [ ] T127 [US2] Write the body tests in `internal/modules/erp/http/body_test.go`
  - Do: body over 32 MiB gives 413 `ERP_BODY_TOO_LARGE`; gzip body with `Content-Encoding: gzip` is inflated; inflated size over 128 MiB is refused with the same code; `Content-Type` other than `application/json` gives 415; broken JSON gives 400 `ERP_BAD_JSON` with `position`.
  - Done: `gofmt -l internal/modules/erp/http/body_test.go` prints nothing; `go test ./internal/modules/erp/http/ -run Body -count=1` fails (RED).

- [ ] T128 [US2] Add the body reader in `internal/modules/erp/http/body.go`
  - Do: `readBody(w, r) ([]byte, error)` with `http.MaxBytesReader(w, r.Body, erp.MaxBodyBytes)`, gzip with a `MaxInflatedBytes` limit reader, content type check.
  - Done: `go test ./internal/modules/erp/http/ -run Body -count=1`.

- [ ] T129 [US2] Write the sync handler tests in `internal/modules/erp/http/sync_handlers_test.go`
  - Do: `POST /products/sync` returns 202 with `syncId`, `state`, `itemCount`, `rejectedAtIntake`, `statusUrl`; the 7th call in a minute with one key gets 429 (limit `SyncBatchesPerMinute`); `GET /products/sync/{syncId}` returns state, counts and chunk progress; another organization's `syncId` gives the same `ERP_SYNC_NOT_FOUND` as a missing id; `GET .../items` pages by `sync_items.id` (default 1000, max 5000), items carry `code`, `state`, bilingual `message`; `?state=failed` returns only failed items; scope `erp.products.sync` and vendor type enforced; every error body has `ar` and `en`.
  - Done: `gofmt -l internal/modules/erp/http/sync_handlers_test.go` prints nothing; `go test ./internal/modules/erp/http/ -run Sync -count=1` fails (RED).

- [ ] T130 [US2] Add the sync handlers in `internal/modules/erp/http/sync_handlers.go`
  - Do: `postSync`, `getSync`, `getSyncItems`; add the three routes to `handler.go` in the same task (two files). Shapes follow `openapi.yaml` paths `/products/sync`, `/products/sync/{syncId}`, `/products/sync/{syncId}/items`. Item messages are built from the stored `code` with `i18n.T`. `getSyncItems` reads `state`, `cursor`, `limit`; `limit` default 1000, max 5000.
  - Traps: 9, 24.
  - Done: `go test ./internal/modules/erp/http/ -count=1`.

- [ ] T131 [US2] Add the lazy enqueuer and S2 ports in `cmd/server/erp_wiring.go`
  - Do: `erpEnqueuer` that creates `queue.NewInsertOnly(db, log)` on first use (the database handle is not connected at mount time) and implements `erp.Enqueuer`; fill `erp.Ports` with the adapters from `internal/composition/erpports` (matching service for the server comes from the same place `cmd/server` builds it today; find it with `grep -rn "matching.New" cmd/server | head`).
  - Traps: 20.
  - Done: `go build ./cmd/server/ && go test ./cmd/server/ -run Router -count=1`.

- [ ] T132 [US2] Write the review permission test (edit `internal/platform/rbac/erp_perms_test.go`)
  - Do: add `vendor.integration.review` to the expected keys of `TestErpPermsVendor`; `view` is not implied by `review` and `manage` does not imply `review`.
  - Done: `go test ./internal/platform/rbac/ -run TestErpPermsVendor -count=1` fails (RED).

- [ ] T133 [US2] Add `vendor.integration.review` in `internal/platform/rbac/catalog_vendor.go`
  - Do: add the key next to the other integration keys.
  - Traps: 29.
  - Done: `go test ./internal/platform/rbac/ -run TestErpPermsVendor -count=1`.

- [ ] T134 [US2] Write the review page tests in `internal/ui/erp_review_test.go`
  - Do: names start with `TestErpReview`: the list shows open items with candidates; resolve with a candidate product id calls `ResolveReview`; dismiss works; both need `vendor.integration.review`; another organization's item id gives not found; search uses the existing `/vendor/catalog/search-json` (no new search endpoint).
  - Done: `gofmt -l internal/ui/erp_review_test.go` prints nothing; `go test ./internal/ui/ -run ErpReview -count=1` fails (RED).

- [ ] T135 [US2] Add the review page in `internal/ui/pages/erp_review.templ`
  - Do: `layouts.VendorShell` page listing open review items (code, name, price, reason, candidates) with resolve and dismiss forms. Run templ generate.
  - Traps: 22.
  - Done: `go build ./internal/ui/pages/`.

- [ ] T136 [US2] Add the review handlers in `internal/ui/erp_review_handlers.go`
  - Do: `ErpReviewPage`, `ErpReviewResolve`, `ErpReviewDismiss`, using `h.erpSvc`.
  - Traps: 29.
  - Done: `go test ./internal/ui/ -run ErpReview -count=1`.

- [ ] T137 [US2] Register the review routes in `internal/ui/vendor_routes_erp.go`
  - Do: do not edit `internal/ui/vendor_routes.go`. In `registerVendorERPRoutes(g chi.Router)` (created in T077, parameter name `g`, groups declared as `g.Group(func(gg chi.Router) {...})`) add `GET /vendor/integrations/review`, `POST /vendor/integrations/review/{id}/resolve`, `POST /vendor/integrations/review/{id}/dismiss`, gated by `vendor.integration.review`. Run `TestUIRoutesAreAudienceGated`. Add `erp.review.*` strings to `internal/shared/i18n/catalog_erp_ui.go` in the same task (two files) before building.
  - Traps: 11, 12, 29.
  - Done: `go test ./internal/ui/ -run 'Erp' -count=1 && go test ./test/ -run 'TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1 && make check-hardcoded-arabic check-file-size`.

- [ ] T138 [US2] Run the S2 flow tests
  - Do: run SB-3 and SB-4, then the flow tests. If one fails, fix the code of the earlier S2 task that owns the behavior; do not edit the test unless the test contradicts plan.md, and then record the reason in `verify-notes.md`.
  - Done: `SB-6 go test -p 1 ./test/integration/ -run 'ERPSyncFlow|TestTenantIsolation_ERP' -skip 'Scale50000' -count=1` passes.

- [ ] T138a [US2] Run the SC-002 scale test
  - Do: run SB-3, then `docker restart dawa24audit-pg` and SB-4 (a clean database), then the command below on its own. The SB timeout of 400 s is stricter than the 7-minute window: if the run is killed by it, write `V8a result: SC-002 not met, killed at 400 s` in `verify-notes.md` and stop the slice.
  - Done: `SB go test -p 1 ./test/integration/ -run TestERPSyncFlow_Scale50000 -count=1 -v` passes; copy the two logged durations into `verify-notes.md` as `V8a result: accept <seconds> s, process <seconds> s`.

- [ ] T139 [US2] S2 slice gate
  - Do: slice gate. Changed packages: `./internal/platform/queue/ ./internal/modules/catalog/... ./internal/modules/erp/... ./internal/composition/... ./cmd/worker/ ./cmd/server/ ./internal/platform/rbac/ ./internal/ui/ ./internal/shared/i18n/`. Focused tests: `go test ./internal/platform/queue/ ./internal/modules/catalog/ ./internal/modules/erp/... ./internal/platform/rbac/ -run 'ERP|Quota|Item|Intake|Process|GatewayOff|Review|Cleanup|NoOrgasoft|NeverCallsEnhancement|Erp|Body|Sync|Orgasoft' -count=1`; `go test ./internal/ui/ -run ErpReview -count=1`; the SB flow command of T138 (with `-skip 'Scale50000'`); `go test ./test/ -run 'TestModulesDoNotImportEachOther|TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1`; `grep -n "^V8a result:" specs/002-erp-integration-api/verify-notes.md` prints a line.
  - Commit message: `feat(erp): S2 product sync`. No push, merge or deploy.
  - Done: `git log -1 --format=%s` prints the commit message.

## Phase 5: S3 Order feeds, tracking and acknowledgement [US3]

Goal: the ERP reads sales and purchase orders, sees what is new, and acknowledges imports. Depends on S1 (S2 only for `externalProductCode`). Independent test: place orders, fetch, ack some, and the next `state=new` fetch returns only the rest.

- [ ] T140 [US3] Write the order visibility and state tests in `test/integration/erp_orders_test.go`
  - Do: `TestERPOrders_SupplierSeesOnlyOwnShipment` (supplier A key never sees supplier B lines or prices in a two-supplier order, US3.1); `TestERPOrders_BuyerKeySeesOnlyOwnOrders` (each order carries its shipments and lines, read through plan.md E7); `TestERPOrders_NewAckImportedChanged` (new, then ack gives `imported`, a shipment status change gives `changed`, re-ack with the same reference clears it); `TestERPOrders_ExternalProductCodeInLines`; `TestTenantIsolation_ERPOrderImports`. Time-based ids.
  - Traps: 1, 2, 19, 26.
  - Done: `gofmt -l test/integration/erp_orders_test.go` prints nothing; compile fails (RED).

- [ ] T141 [US3] Write the paging test in `test/integration/erp_orders_paging_test.go`
  - Do: `TestERPOrders_Paging5000`: 5,000 orders paged 100 at a time with 20 more inserted between pages; no id repeats, none skipped from the original set (SC-007). `TestERPOrders_PagingWithNewState`: the cycle with `state=new` and ack between pages loses nothing.
  - Traps: 27.
  - Done: `gofmt -l test/integration/erp_orders_paging_test.go` prints nothing; compile fails (RED).

- [ ] T142 [US3] Write the cursor tests in `internal/modules/erp/http/cursor_test.go`
  - Do: `EncodeCursor(id)` is base64url of `v1:<id>` without padding; `DecodeCursor` round trips; empty string means start; garbage, wrong version, negative and non-numeric ids give `ERP_CURSOR_INVALID` (400).
  - Done: `gofmt -l internal/modules/erp/http/cursor_test.go` prints nothing; `go test ./internal/modules/erp/http/ -run Cursor -count=1` fails (RED).

- [ ] T143 [US3] Add the cursor in `internal/modules/erp/http/cursor.go`
  - Do: `EncodeCursor(id int64) string`, `DecodeCursor(s string) (int64, error)`.
  - Traps: 27.
  - Done: `go test ./internal/modules/erp/http/ -run Cursor -count=1`.

- [ ] T144 [US3] Write the order service tests in `internal/modules/erp/orders_service_test.go`
  - Do: state mapping `new`/`imported`/`changed`/`all`; `Ack` results: a new reference gives `result=acknowledged`; the same reference again gives `result=already_acknowledged` and still refreshes `acked_status`, `acked_at` and `ack_count`; a different reference gives `result=conflict` with `code=ERP_ACK_REFERENCE_CONFLICT` and `existingReference`, and changes nothing; an unknown order gives `not_found`; more than `AckMax` (500) entries gives `ERP_VALIDATION`; sample 3: replace the literal `{{orderNumber}}` in the text of `orgasoft_order_ack_sample3.json` with a real seeded order number before decoding it through the provider, then assert the result is `acknowledged` with reference `ORG-9921`; also assert that the unreplaced fixture decodes to `OrderNumber == "{{orderNumber}}"` and yields `not_found`; the `from` and `to` filters return only orders created inside the range (FR-019); sales calls need `erp.orders.read` and vendor type (`ERP_CAPABILITY_REQUIRED`), purchase calls work for vendor and customer organizations; page size default 100, max 500; a missing order gives `ERP_ORDER_NOT_FOUND` identical for another organization's order.
  - Done: `gofmt -l internal/modules/erp/orders_service_test.go` prints nothing; `go test ./internal/modules/erp/ -run Orders -count=1` fails (RED).

- [ ] T145 [US3] Add the order repository interface in `internal/modules/erp/repository_orders.go`
  - Do: `ListSalesShipmentsAsSystem(ctx, orgID, integrationID int64, f OrderFilter) ([]SalesRow, error)`, `GetSalesShipmentAsSystem(ctx, orgID int64, orderNumber string) (SalesRow, error)`, `ListPurchaseOrders(ctx, buyerOrgID, integrationID int64, f OrderFilter) ([]PurchaseRow, error)`, `GetPurchaseOrder(ctx, buyerOrgID int64, orderNumber string) (PurchaseRow, error)`, `ListPurchaseShipmentsAsSystem(ctx, buyerOrgID int64, orderIDs []int64) ([]PurchaseShipment, error)` (plan.md E7), `UpsertOrderImport(ctx, imp OrderImport, audit AuditRecord) (AckOutcome, error)`; types (field names mirror the openapi schemas and the table columns in camel-to-Pascal case): `OrderFilter{After int64; Limit int; State string; From, To *time.Time}`; `OrderLine{LineID int64; VariantPublicID, ExternalProductCode, Barcode string; Name map[string]string; Quantity int; UnitPrice, DiscountAmount, TotalPrice money.Amount}`; `CustomerInfo{ErpCode, UnmappedReason string; Mapped bool; Name, BranchName map[string]string; Address, Phone string}`; `SalesRow{OrderID, ShipmentID int64; OrderNumber, ShipmentNumber, Status, ImportState, PaymentMethod, PaymentStatus, Notes, ErpReference string; CreatedAt, UpdatedAt time.Time; AckedAt *time.Time; Subtotal, ShippingFee, Total money.Amount; Customer CustomerInfo; Lines []OrderLine}`; `PurchaseShipment{OrderID int64; ShipmentNumber, Status string; SupplierName map[string]string; Subtotal, ShippingFee money.Amount; Lines []OrderLine}`; `PurchaseRow{OrderID int64; OrderNumber, Status, ImportState, ExternalReference, ErpReference string; CreatedAt, UpdatedAt time.Time; AckedAt *time.Time; Total money.Amount; Shipments []PurchaseShipment}`; `OrderImport{IntegrationID, OrderID, APIKeyID int64; ShipmentID *int64; Direction, OrderNumber, ErpReference, AckedStatus string}`; `AckOutcome{Result, Code, ExistingReference string}` (`Result` is `acknowledged`, `already_acknowledged` or `conflict`). Embed in `Repository` (edit `repository.go`). Create `fake_orders_test.go` with `type fakeOrders struct` implementing these methods in memory and embed it in `fakeRepo` (`fake_keys_test.go`): `type fakeRepo struct { ...; *fakeSync; *fakeOrders }`, so `fakeRepo` still satisfies `erp.Repository`.
  - Done: `go build ./internal/modules/erp/` and `gofmt -l internal/modules/erp/fake_orders_test.go` prints nothing; `go vet ./internal/modules/erp/` passes once T150 exists (the T144 tests need it).

- [ ] T146 [US3] Add the customer code SQL in `internal/modules/erp/postgres/customer_code.go`
  - Do: const `customerCodeSQL` (the only copy): `organization_number` of the `org.user_organizations` link where `vendor_org_id` is the supplier and (`customer_org_id = order.organization_id` or `customer_org_id IS NULL AND user_id = order.customer_id`); statuses `pending` and `approved`; prefer the row with `customer_org_id` set, then newest `updated_at`; several different customers sharing one code gives unmapped reason `code_conflict`; no link gives `no_link`; a link found but with an `organization_number` that is empty after trim gives `mapped=false`, `unmappedReason=empty_code` (checked before `code_conflict`). Function `resolveCustomerCodes(ctx, tx, supplierOrgID int64, orders []orderRef) (map[int64]CustomerCode, error)`.
  - Use: research D-20.
  - Done: `gofmt -l internal/modules/erp/postgres/customer_code.go` prints nothing (`go vet` is not required: the package compiles as a whole at T149).

- [ ] T147 [P] [US3] Add sales order SQL in `internal/modules/erp/postgres/orders_sales.go`
  - Do: `ListSalesShipmentsAsSystem` (comment `plan.md E2`): every statement has `s.organization_id = $1`, `s.id > $afterID`, join `commerce.orders` header, left join `erp.order_imports` for the state filter, `ORDER BY s.id LIMIT n+1`. Lines are loaded with a second statement under `WithTenant` on `commerce.order_lines WHERE shipment_id = ANY($1)` joined to `erp.product_links` for `externalProductCode`. `GetSalesShipmentAsSystem` with the same filter. Customer fields, read inside the same E2 statement with the guard `s.organization_id = $1`: `name` from `org.organizations` of `commerce.orders.organization_id` (unscoped table); `branchName`, `address` and `phone` from `org.branches` of `commerce.orders.branch_id` (the buyer's branch; `order_shipments.branch_id` is the supplier's own fulfilling branch and must not be used), with the join condition `b.organization_id = o.organization_id`; when the order has no branch use the buyer organization's own `address` and `phone` from `org.organizations`. Confirm the column names first with `grep -n 'CREATE TABLE org.branches' -A15 db/migrations/003_organizations.up.sql`. Mark the call `sensitive_read` (T152).
  - Traps: 1, 2, 26, 27.
  - Done: `gofmt -l internal/modules/erp/postgres/orders_sales.go` prints nothing (`go vet` is not required).

- [ ] T148 [P] [US3] Add purchase order SQL in `internal/modules/erp/postgres/orders_purchase.go`
  - Do: `ListPurchaseOrders`, `GetPurchaseOrder`: headers via `WithTenant(buyer)` on `commerce.orders WHERE organization_id = $1`, cursor over `orders.id`; shipments and lines via `ListPurchaseShipmentsAsSystem(ctx, buyerOrgID int64, orderIDs []int64)` (comment `plan.md E7`), grouped by supplier shipment, where every statement joins `commerce.orders o ON o.id = s.order_id` with `o.organization_id = $1`. No other `AsSystem`.
  - Traps: 1, 27.
  - Done: `gofmt -l internal/modules/erp/postgres/orders_purchase.go` prints nothing (`go vet` is not required).

- [ ] T149 [P] [US3] Add import tracking SQL in `internal/modules/erp/postgres/imports.go`
  - Do: `UpsertOrderImport`: first ack inserts and returns `Result=acknowledged`; same reference updates `acked_status`, `acked_at`, `ack_count` and returns `Result=already_acknowledged`; different reference returns `Result=conflict`, `Code=ERP_ACK_REFERENCE_CONFLICT` and `ExistingReference`, and changes nothing; audit row in the same `InTx`.
  - Traps: 1, 25.
  - Done: `go build ./internal/modules/erp/postgres/` must pass including the `var _ erp.Repository` line.

- [ ] T150 [US3] Add the order service in `internal/modules/erp/orders_service.go`
  - Do: `ListSales`, `GetSales`, `ListPurchase`, `GetPurchase` (each `(ctx, p Principal, ...)`), `Ack(ctx, p Principal, direction string, entries []provider.AckEntry) ([]AckResult, error)`. Scope and capability checks first. A cursor is `after` id in `OrderFilter`. `AckResult{OrderNumber, Result, Code string; ExistingReference string}` is the per-order output of `Ack`.
  - Traps: 1, 2, 26, 27.
  - Done: `go test ./internal/modules/erp/ -run Orders -count=1` and `go vet ./internal/modules/erp/`.

- [ ] T151 [US3] Write the order handler tests in `internal/modules/erp/http/orders_handlers_test.go`
  - Do: routes `GET /orders/sales`, `GET /orders/sales/{orderNumber}`, `POST /orders/sales/ack`, `GET /orders/purchase`, `GET /orders/purchase/{orderNumber}`, `POST /orders/purchase/ack` per `openapi.yaml`; response carries `customer.erpCode`, or `customer.mapped=false` with `unmappedReason`; `importState`; `nextCursor`; bad cursor `ERP_CURSOR_INVALID`; a sales read marks the call record `SensitiveRead=true`; scope errors name `requiredScope`.
  - Done: `gofmt -l internal/modules/erp/http/orders_handlers_test.go` prints nothing; `go test ./internal/modules/erp/http/ -run Orders -count=1` fails (RED).

- [ ] T152 [US3] Add the order handlers in `internal/modules/erp/http/orders_handlers.go` and mark sensitive reads in `auth.go`
  - Do: handlers for the six routes; query parameters `cursor`, `limit`, `state`, `from`, `to`. In `auth.go` put a `*callState` in the request context and add `markSensitiveRead(ctx)`; the post-handler recorder copies it into `CallRecord.SensitiveRead`.
  - Traps: 9, 24.
  - Done: `go test ./internal/modules/erp/http/ -run Orders -count=1`.

- [ ] T153 [US3] Mount the order routes in `internal/modules/erp/http/handler.go`
  - Do: register the six routes in the auth-wrapped group.
  - Done: `go test ./internal/modules/erp/http/ -count=1 && go build ./cmd/server/`.

- [ ] T154 [US3] Run the S3 integration tests
  - Do: SB-3, SB-4, then run. Fix the owning S3 task when a test fails; do not edit a test unless it contradicts plan.md (record why in `verify-notes.md`).
  - Done: `SB-6 go test -p 1 ./test/integration/ -run 'ERPOrders|TestTenantIsolation_ERPOrderImports' -count=1` passes.

- [ ] T155 [US3] S3 slice gate
  - Do: slice gate. Changed packages: `./internal/modules/erp/... ./cmd/server/`. Focused tests: `go test ./internal/modules/erp/... -run 'Orders|Cursor|Ack' -count=1`; the SB command above; `go test ./test/ -run TestModulesDoNotImportEachOther -count=1`.
  - Commit message: `feat(erp): S3 order feeds and acknowledgement`. No push, merge or deploy.
  - Done: `git log -1 --format=%s` prints the commit message.

## Phase 6: S4 Confirm and reject [US4]

Goal: the ERP confirms or rejects pending orders through the website rules. Depends on S3. Independent test: confirm one pending order and reject another; the website shows what a supplier action would show.

- [ ] T156 [US4] Write the decide integration tests in `test/integration/erp_decide_test.go`
  - Do: `TestERPDecide_ConfirmCash`; `TestERPDecide_ConfirmWalletCharges` (wallet charged once; with short funds the shipment stays pending and the call gets `ERP_WALLET_INSUFFICIENT` with `requiredAmount` and `availableAmount`); `TestERPDecide_RejectRefunds`; `TestERPDecide_RepeatConfirm` (`already_confirmed`, wallet charged once); `TestERPDecide_SameHistoryAsWebsite` (status history rows equal to a website confirm); audit row `erp.order.confirm` has `actor_kind = 'api_key'`. Exact money strings.
  - Traps: 8, 15, 25.
  - Done: `gofmt -l test/integration/erp_decide_test.go` prints nothing; compile fails (RED).

- [ ] T157 [US4] Record the vendor order test baseline in `specs/002-erp-integration-api/verify-notes.md`
  - Do: run `go test ./internal/ui/ -run 'Vendor.*Order|Shipment' -count=1` and write `V6 before: <ok/fail and counts>`.
  - Done: `grep -n "^V6 before:" specs/002-erp-integration-api/verify-notes.md` prints a line.

- [ ] T158 [US4] Split the shipment transition in `internal/ui/vendor_order_handlers.go`
  - Do: at about line 250 split `transitionVendorShipment` into `transitionVendorShipmentBy(ctx, orgID int64, isStaff bool, changedBy *int64, shipmentID int64, to commerce.OrderStatus, notes, carrier, tracking, vendorName string) error` holding the existing body, and keep `transitionVendorShipment` as a wrapper passing `&actor.UserID`. No behaviour change.
  - Traps: 7, 8.
  - Done: `go test ./internal/ui/ -run 'Vendor.*Order|Shipment' -count=1` gives the same result as V6; record `V6 after:` in `verify-notes.md`.

- [ ] T158a [US4] Write the wallet shortfall error test in `internal/ui/customer_checkout_wallet_shortfall_test.go`
  - Do: `TestWalletShortfallError`: copy the wallet fake setup of `internal/ui/customer_checkout_wallet_test.go`; call `prepareWalletCharge` with an available balance below the goods amount and assert `errors.As(err, &walletShortfallError{})` succeeds, `Available` and `Required` equal the exact `money.Amount` values, and `err.Error()` equals the current message text; with enough balance the error is nil; a missing billing service returns a plain error that is not a `walletShortfallError`.
  - Traps: 8.
  - Done: `gofmt -l internal/ui/customer_checkout_wallet_shortfall_test.go` prints nothing; `go test ./internal/ui/ -run WalletShortfall -count=1` fails to compile (RED).

- [ ] T158b [US4] Return a typed shortfall error from `prepareWalletCharge` in `internal/ui/customer_checkout_wallet.go`
  - Do: `prepareWalletCharge` returns plain `fmt.Errorf` strings today, so nothing can be extracted from them. Add `type walletShortfallError struct{ Available, Required money.Amount; msg string }` with `Error() string` returning exactly the current text, and return it at the shortfall branch (`wallet.AvailableBalance.Minor() < goodsAmount.Minor()`, about line 124), which is the only shortfall branch. Existing callers keep working because it is an `error`. Change no other branch and no message text.
  - Traps: 8.
  - Done: `go test ./internal/ui/ -run 'WalletShortfall|Wallet|Checkout' -count=1`.

- [ ] T159 [US4] Write the decision method tests in `internal/ui/erp_order_decision_test.go`
  - Do: names start with `TestErpDecide`: pending to confirmed; pending to rejected maps to `StatusCancelled` with status-history notes `"ERP: " + reason`, while the buyer notification receives only `reason` (without the prefix) when a reason was sent and the default text when not; non-pending gives `ERP_ORDER_STATE_CONFLICT` with `currentStatus`; a repeat gives the `already_*` result; `IsNegotiation` with `NegotiationStatus=="pending"` gives `ERP_ORDER_NEGOTIATION_PENDING`; a `walletShortfallError` gives `ERP_WALLET_INSUFFICIENT` with `requiredAmount` and `availableAmount`, any other wallet error gives `ERP_UNAVAILABLE`; `changedBy` is nil; the buyer notification is sent.
  - Done: `gofmt -l internal/ui/erp_order_decision_test.go` prints nothing; `go test ./internal/ui/ -run ErpDecide -count=1` fails (RED).

- [ ] T160 [US4] Add the decision ports in `internal/modules/erp/ports.go`
  - Do: add `OrderDecider`, `ShipmentDecision{ShipmentID, OrganizationID int64; Status string; Reason string}`, `ShipmentDecisionResult{Result string; OrderStatus string}` as in plan.md "Ports"; field `Decider OrderDecider` on `Ports`.
  - Traps: 23.
  - Done: `go build ./internal/modules/erp/`.

- [ ] T161 [US4] Add the website-rule decision method in `internal/ui/erp_order_decision.go`
  - Do: `func (h *UIHandler) ErpDecideShipment(ctx context.Context, d erp.ShipmentDecision) (erp.ShipmentDecisionResult, error)`: load shipment and order as `transitionVendorShipment` does, apply the refusals above, call `transitionVendorShipmentBy` with `changedBy=nil`. Wallet pre-check and buyer notice come from `prepareWalletShipmentCharge` and `notifyOrderStatusChanged` through the same path. `errors.As(err, &walletShortfallError)` becomes `ERP_WALLET_INSUFFICIENT` with `details.requiredAmount` and `details.availableAmount` as `money.Amount.String()`; any other wallet error becomes `ERP_UNAVAILABLE`. The status-history note is `ERP: <reason>`; the buyer notification receives only `<reason>` (without the prefix) when a reason was sent, and the default text when not.
  - Traps: 7, 8.
  - Done: `go test ./internal/ui/ -run ErpDecide -count=1`.

- [ ] T162 [US4] Write the decide service tests in `internal/modules/erp/orders_decide_test.go`
  - Do: `confirmed` and `rejected` reach the decider; `delivered`, `shipped`, `completed`, `out_for_delivery`, `in_transit` give 403 `ERP_DELIVERY_NOT_ALLOWED`; any other value gives 422 `ERP_STATUS_NOT_ALLOWED` with `allowed`; scope `erp.orders.decide` required; the shipment is resolved by `orderNumber` and the principal's organization only; an audit record `erp.order.confirm` or `erp.order.reject` with `ActorKind: "api_key"` and the key id is written with the change.
  - Done: `gofmt -l internal/modules/erp/orders_decide_test.go` prints nothing; `go test ./internal/modules/erp/ -run Decide -count=1` fails (RED).

- [ ] T163 [US4] Add `Decide` in `internal/modules/erp/orders_decide.go`
  - Do: `func (s *Service) Decide(ctx context.Context, p Principal, orderNumber, status, reason string) (DecideResult, error)` with `DecideResult{OrderNumber, Result, OrderStatus string}`. Own file because `orders_service.go` would pass 350 lines.
  - Traps: 7, 15, 25.
  - Done: `go test ./internal/modules/erp/ -run Decide -count=1`.

- [ ] T164 [US4] Add the decide handler in `internal/modules/erp/http/decide_handlers.go`
  - Do: `POST /orders/sales/{orderNumber}/status` body `{"status","reason"?}`; response `{orderNumber, result, orderStatus}`; mount it in `handler.go` in the same task (two files). Handler test first: add `TestDecideHandler` cases to a new `decide_handlers_test.go` before writing the handler.
  - Done: `go test ./internal/modules/erp/http/ -run Decide -count=1`.

- [ ] T165 [US4] Add the late decider in `cmd/server/erp_late.go`
  - Do: `lateERPDecider` implementing `erp.OrderDecider` with `bind(fn)`; calls before binding return `ERP_UNAVAILABLE`. Pattern: `lateOrderAnnouncer` in `cmd/server/order_announce_wiring.go`.
  - Done: `go build ./cmd/server/`.

- [ ] T166 [US4] Hold the decider in `cmd/server/erp_ports.go` and `deps.go`
  - Do: create `erp_ports.go` with `func newERPDecideDeps() *lateERPDecider`; add the field `erpDecide *lateERPDecider` to the deps struct in `deps.go`. Setting `erp.Ports.Decider` in `erp_wiring.go` is the next task.
  - Done: `go build ./cmd/server/`.

- [ ] T167 [US4] Set the decider port in `cmd/server/erp_wiring.go`
  - Do: set `erp.Ports.Decider` to `deps.erpDecide`.
  - Done: `go build ./cmd/server/`.

- [ ] T168 [US4] Bind the decider in `cmd/server/routes.go`
  - Do: after `buildUIHandler` call `deps.erpDecide.bind(uiHandler.ErpDecideShipment)`.
  - Done: `go build ./... && go test ./cmd/server/ -run Router -count=1`.

- [ ] T169 [US4] Run the S4 integration tests
  - Do: SB-3, SB-4, then run. Fix the owning S4 task when a test fails.
  - Done: `SB go test -p 1 ./test/integration/ -run ERPDecide -count=1` passes.

- [ ] T170 [US4] S4 slice gate
  - Do: slice gate. Changed packages: `./internal/ui/ ./internal/modules/erp/... ./cmd/server/`. Focused tests: `go test ./internal/ui/ -run 'ErpDecide|Vendor.*Order|Shipment' -count=1`; `go test ./internal/modules/erp/... -run 'Decide' -count=1`; the SB command above.
  - Commit message: `feat(erp): S4 confirm and reject`. No push, merge or deploy.
  - Done: `git log -1 --format=%s` prints the commit message.

## Phase 7: S5 Customer codes and the quick-entry modal [US5]

Goal: the link-page organization number acts as the ERP customer code, unique per ERP supplier, with a quick-entry modal in checkout. Depends on S3. Independent test: order as a user with no code, enter it in the modal, finish the order, and the API shows `erpCode`.

- [ ] T171 [US5] Write the customer code integration tests in `test/integration/erp_customer_code_test.go`
  - Do: `TestERPCustomerCode_OrderWithCode` (`erpCode` returned); `TestERPCustomerCode_NoLink` (`mapped=false`, `unmappedReason="no_link"`); `TestERPCustomerCode_EmptyCode` (a link exists with `organization_number=''`: `mapped=false`, `unmappedReason="empty_code"`); `TestERPCustomerCode_Conflict` (code shared by two customers: `code_conflict`); `TestERPCustomerCode_SupplierWithoutERP` (no modal, plan unchanged, SC-009); `TestERPCustomerCode_GuardTakenAndLock` (second customer with the same code refused `ERP_CODE_TAKEN`).
  - Traps: 19.
  - Done: `gofmt -l test/integration/erp_customer_code_test.go` prints nothing; compile fails (RED).

- [ ] T172 [US5] Write migration 257 in `db/migrations/257_user_org_erp_code_index.up.sql` and `.down.sql`
  - Do: the index from `data-model.md` "Migration 257" with `BEGIN;`/`COMMIT;`; down `DROP INDEX IF EXISTS org.idx_user_orgs_vendor_number`. Not unique. Add an Arabic `COMMENT ON INDEX`.
  - Done: SB-3 and SB-4 apply 257 with no error; `go test ./test/ -run 'TestRepositorySQLMatchesMigrations|TestWriteSQLMatchesMigrations' -count=1` passes.

- [ ] T173 [US5] Write the customer code service tests in `internal/modules/erp/customer_code_test.go`
  - Do: `SuppliersUsingERP` true only for an active integration with an active, unexpired key; `MissingCodes(buyerOrgID, buyerUserID, vendorOrgIDs)` returns only ERP suppliers with no code for this buyer; `GuardCustomerCode` returns `ERP_CODE_TAKEN` (409) when another customer holds the code (case-insensitive) among `pending`/`approved` links, accepts the same customer, honours `excludeLinkID`, and is a no-op for a supplier without ERP.
  - Done: `gofmt -l internal/modules/erp/customer_code_test.go` prints nothing; `go test ./internal/modules/erp/ -run CustomerCode -count=1` fails (RED).

- [ ] T174 [US5] Add customer code logic in `internal/modules/erp/customer_code.go`
  - Do: `(*Service).SuppliersUsingERP(ctx, vendorOrgIDs []int64) (map[int64]bool, error)`, `MissingCodes(ctx, buyerOrgID, buyerUserID int64, vendorOrgIDs []int64) ([]int64, error)`, `GuardCustomerCode(ctx, vendorOrgID int64, customerOrgID *int64, userID int64, code string, excludeLinkID int64) error`; a `CodeRepository` interface declared in this file and embedded in `Repository`. Embed a code fake in `fakeRepo`: create `fake_code_test.go` with `type fakeCode struct` implementing the three methods in memory, and change `fakeRepo` to `type fakeRepo struct { ...; *fakeSync; *fakeOrders; *fakeCode }` so it still satisfies `erp.Repository`.
  - Traps: 1.
  - Done: `go test ./internal/modules/erp/ -run CustomerCode -count=1` and `go vet ./internal/modules/erp/`.

- [ ] T175 [US5] Add guard SQL in `internal/modules/erp/postgres/customer_code.go`
  - Do: add `SuppliersUsingERP`, `MissingCodes`, `GuardCustomerCode` SQL: take `pg_advisory_xact_lock(hashtextextended('erp-code:'||$vendor,0))` then check other customers' `lower(organization_number)` among `pending`/`approved` links. `org.user_organizations` has no RLS (research D-20): read it under `WithTenant` like the other queries. Do not add an `AsSystem` function; if a query returns no rows because of the tenant context, stop and record it in `verify-notes.md`.
  - Traps: 1.
  - Done: `go build ./internal/modules/erp/postgres/` (must pass including the `var _ erp.Repository` line) `&& go test ./internal/modules/erp/postgres/ -count=1`.

- [ ] T176 [US5] Add `erp.checkout.*` and `customer.userorg.number_taken` strings in `internal/shared/i18n/catalog_erp_ui.go`
  - Do: bilingual strings for the modal title, explanation, input label, submit, cancel, and the notice `customer.userorg.number_taken`.
  - Traps: 12.
  - Done: `go test ./internal/shared/i18n/ -count=1 && make check-hardcoded-arabic`.

- [ ] T177 [US5] Write the checkout code handler tests in `internal/ui/erp_customer_code_test.go`
  - Do: names start with `TestErpCustomerCode`: `CheckoutErpCodeSubmit` creates a `pending` link through `orgSvc.CreateUserOrgLink(sysCtx, userID, &buyerOrg, vendorOrgID, code, org.UserOrgStatusPending)` when none exists and calls `UpdateUserOrgLink` when one does; a taken code redirects with `customer.userorg.number_taken` and changes nothing; success redirects to `/checkout`; the cart is untouched; the helper `erpGuardCustomerCode` is a no-op when `erpSvc` is nil.
  - Done: `gofmt -l internal/ui/erp_customer_code_test.go` prints nothing; `go test ./internal/ui/ -run ErpCustomerCode -count=1` fails (RED).

- [ ] T178 [US5] Add the checkout code handler in `internal/ui/erp_customer_code.go`
  - Do: `func (h *UIHandler) erpGuardCustomerCode(...) error` and `CheckoutErpCodeSubmit` (`POST /checkout/erp-customer-code`, reads `vendor_org_id` and `organization_number`).
  - Done: `go test ./internal/ui/ -run ErpCustomerCode -count=1`.

- [ ] T179 [US5] Register the route in `internal/ui/buying_routes.go`
  - Do: register `POST /checkout/erp-customer-code` beside `g.Post("/checkout", ...)` (about line 101), with the same gate as `/checkout`.
  - Traps: 29.
  - Done: `go test ./test/ -run 'TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1`.

- [ ] T180 [US5] Write tests for the four number writers in `internal/ui/erp_code_writers_test.go`
  - Do: names start with `TestErpCodeWriter`: one test per writer (customer create at `customer_user_org_handlers.go` line 166, customer update at 201, vendor create at `vendor_user_org_handlers.go` line 168, vendor update at 268): taken code redirects with `customer.userorg.number_taken` and no write; free code writes; a supplier without ERP is unchanged.
  - Done: `gofmt -l internal/ui/erp_code_writers_test.go` prints nothing; `go test ./internal/ui/ -run ErpCodeWriter -count=1` fails (RED).

- [ ] T181 [P] [US5] Guard the customer-side writers in `internal/ui/customer_user_org_handlers.go`
  - Do: call `h.erpGuardCustomerCode` before `CreateUserOrgLink` (line 166) and `UpdateUserOrgLink` (line 201); on `ERP_CODE_TAKEN` redirect with notice key `customer.userorg.number_taken`.
  - Done: `go test ./internal/ui/ -run 'ErpCodeWriter' -count=1` (customer cases pass).

- [ ] T182 [P] [US5] Guard the vendor-side writers in `internal/ui/vendor_user_org_handlers.go`
  - Do: the same guard before the writes at lines 168 and 268.
  - Done: `go test ./internal/ui/ -run 'ErpCodeWriter' -count=1` (all four pass).

- [ ] T183 [US5] Write the checkout plan test in `internal/ui/customer_checkout_core_erp_test.go`
  - Do: `TestErpPlanCheckout`: a supplier without integration leaves `planCheckout` unchanged; a supplier with an integration and no code returns `&checkoutFailure{Back: "/checkout?erp_code=1"}` with no message; a code present continues.
  - Done: `gofmt -l internal/ui/customer_checkout_core_erp_test.go` prints nothing; `go test ./internal/ui/ -run ErpPlanCheckout -count=1` fails (RED).

- [ ] T184 [US5] Add the missing-code check in `internal/ui/customer_checkout_core.go`
  - Do: in `planCheckout` (about line 54), after `prepareCheckoutItems`, call `resolveCheckoutVendors` first, collect vendor ids from `CheckoutLineItem.VendorOrgID`, call `h.erpSvc.MissingCodes`; if any, return the failure above.
  - Done: `go test ./internal/ui/ -run 'ErpPlanCheckout|Checkout' -count=1`.

- [ ] T185 [US5] Write the checkout page test in `internal/ui/customer_checkout_erp_page_test.go`
  - Do: `TestErpCheckoutPageModal`: the page handler recomputes `MissingCodes` itself, never trusts `?erp_code=1`, and renders `erp-code-modal` with one input per missing supplier posting to `/checkout/erp-customer-code`; with nothing missing no modal renders even when the query string asks for it.
  - Done: `gofmt -l internal/ui/customer_checkout_erp_page_test.go` prints nothing; `go test ./internal/ui/ -run ErpCheckoutPage -count=1` fails (RED).

- [ ] T186 [P] [US5] Add the modal component in `internal/ui/pages/checkout_erp_code_modal.templ`
  - Do: `ErpCodeModal(needs []ErpCodeNeed)` built with `components.Modal(ModalProps{ID:"erp-code-modal", Open:true, ...})`; `ErpCodeNeed{VendorOrgID int64; SupplierName string}`. Run templ generate.
  - Traps: 22.
  - Done: `go build ./internal/ui/pages/ && make check-modal-handwritten`.

- [ ] T187 [US5] Extend the checkout page in `internal/ui/pages/customer_checkout.templ`
  - Do: keep the `CustomerCheckout(...)` signature; add `CustomerCheckoutWithExtras(...same args..., extras CheckoutExtras)` with `CheckoutExtras{MissingERPCodes []ErpCodeNeed}`; `CustomerCheckout` calls it with the zero value. Run templ generate.
  - Traps: 22.
  - Done: `go build ./internal/ui/...`.

- [ ] T188 [US5] Pass the extras in `internal/ui/customer_checkout_handlers.go`
  - Do: at about line 141 call `MissingCodes` and `CustomerCheckoutWithExtras`.
  - Done: `go test ./internal/ui/ -run 'ErpCheckoutPage|Checkout' -count=1 && make check-modal-handwritten check-hardcoded-arabic`.

- [ ] T189 [P] [US5] Note the guard in `docs/modules/org.md`
  - Do: record: the `organization_number` on the link page is the ERP customer code, unique per ERP supplier through `GuardCustomerCode`, existing duplicates stay.
  - Done: `grep -n "GuardCustomerCode" docs/modules/org.md` prints a line.

- [ ] T190 [US5] Run the S5 integration tests
  - Do: SB-3, SB-4, then run. Fix the owning S5 task when a test fails.
  - Done: `SB go test -p 1 ./test/integration/ -run ERPCustomerCode -count=1` passes.

- [ ] T191 [US5] S5 slice gate
  - Do: slice gate. Changed packages: `./internal/ui/... ./internal/modules/erp/... ./internal/shared/i18n/`. Focused tests: `go test ./internal/ui/ -run 'Erp|Checkout' -count=1`; `go test ./internal/modules/erp/... -run CustomerCode -count=1`; the SB command above; `go test ./test/ -run 'TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1`; `make check-modal-handwritten check-hardcoded-arabic`.
  - Commit message: `feat(erp): S5 customer codes and checkout modal`. No push, merge or deploy.
  - Done: `git log -1 --format=%s` prints the commit message.

## Phase 8: S6 OpenAPI, Postman and docs [US8]

Goal: the owner imports a Postman collection and runs the whole journey from the documentation. Depends on the endpoints it describes; the generator reads the OpenAPI examples, so the collection already lists folders for S7 requests. Folder `04 Purchase` only passes after S7, so S7 ends with a regeneration task. Postman and docs work therefore grows: S6 builds the generator and first files; S7 re-runs it.

- [ ] T192 [US8] Write the route-versus-spec test in `internal/modules/erp/http/spec_test.go`
  - Do: `TestOpenAPIMatchesRoutes`: walk the chi router from `NewHandler(...).Routes()` and compare `METHOD path` with the paths in the embedded spec, both directions. Declare `var specRoutesNotYetMounted = []string{"GET /offers", "POST /orders/purchase"}` skipped on the spec side until S7 removes them.
  - Done: `gofmt -l internal/modules/erp/http/spec_test.go` prints nothing; `go test ./internal/modules/erp/http/ -run OpenAPIMatchesRoutes -count=1` runs (record pass or the drift it finds).

- [ ] T193 [US8] Bring the embedded spec in step with the code
  - Do: if the test above reports drift, fix `specs/002-erp-integration-api/contracts/openapi.yaml` to match the code only when the code follows plan.md; otherwise fix the code. Then `cp specs/002-erp-integration-api/contracts/openapi.yaml internal/modules/erp/http/spec/openapi.yaml`.
  - Done: `diff -q specs/002-erp-integration-api/contracts/openapi.yaml internal/modules/erp/http/spec/openapi.yaml` prints nothing and `go test ./internal/modules/erp/http/ -run OpenAPIMatchesRoutes -count=1` passes.

- [ ] T194 [US8] Write the Postman golden test in `cmd/cli/erp_postman_test.go`
  - Do: `TestPostmanCollectionIsCurrent`: regenerate in memory and compare with the committed `docs/erp-api/dawa24-erp.postman_collection.json` and `dawa24-erp.postman_environment.json`. `TestPostmanCollectionShape`: Collection v2.1 schema URL; folders `00 Health`, `01 Products`, `02 Sales orders`, `03 Decide`, `04 Purchase`, `05 Errors`; variables `baseUrl`, `apiKey`, `syncId`, `orderNumber`, `erpReference`, `externalReference` (`{{$guid}}`); every request has the header `Authorization: Bearer {{apiKey}}` and a `pm.test` script checking the status code and the envelope shape; sync saves `syncId`; the orders list saves `orderNumber`; folder 05 has one request per documented request-level code with a test that `error.code` equals it.
  - Done: `gofmt -l cmd/cli/erp_postman_test.go` prints nothing; `go test ./cmd/cli/ -run Postman -count=1` fails to compile (RED).

- [ ] T195 [US8] Add the collection builder in `cmd/cli/erp_postman_items.go`
  - Do: functions that build the folders and requests from the embedded spec examples (the Orgasoft samples are the product bodies: camelCase, PascalCase, invalid price `12.345`). Read the example bodies from the embedded OpenAPI YAML (`yaml.Unmarshal` of the spec with `gopkg.in/yaml.v3`, which `go.mod` lists as `// indirect`: run `go mod tidy` and touch only that marker), never from Go string literals, because `make check-hardcoded-arabic` scans `cmd`. Request names, exactly, per folder: `00 Health`: `Check key`; `01 Products`: `Send batch`, `Sync state`, `Item results`; `02 Sales orders`: `New orders`, `Acknowledge`, `Imported orders`; `03 Decide`: `Confirm`, `Confirm again`, `Deliver`; `04 Purchase`: `Find offers`, `Create purchase order`; `05 Errors`: one request per documented request-level code.
  - Done: `go build ./cmd/cli/` and `make check-hardcoded-arabic`.

- [ ] T196 [US8] Add the generator in `cmd/cli/erp_postman.go`
  - Do: `runERPPostman(args []string) int` (the process exit code) writes both files into `docs/erp-api/` with stable key order and two-space indent; `erpPostmanBytes() (collection, environment []byte, err error)` used by the test. The environment is named `Dawa24 ERP (local)` with `baseUrl` `http://localhost:8080` and an empty secret `apiKey`.
  - Done: `go test ./cmd/cli/ -run PostmanCollectionShape -count=1`.

- [ ] T197 [US8] Add the `erp-postman` dispatch in `main()` of `cmd/cli/main.go`
  - Do: `run()` loads the configuration and opens the database (10 attempts, 2 s apart) before its `switch`, so a case there would fail or hang without a database. Add `if len(os.Args) >= 2 && os.Args[1] == "erp-postman" { os.Exit(runERPPostman(os.Args[2:])) }` in `main()` next to the `match-bench` block, before `run()`. Also add the `erp-postman` and `erp-demo` lines to `usage()`.
  - Done: `go run ./cmd/cli erp-postman` under HOST-GO (`DATABASE_URL` unset) writes both files; `go test ./cmd/cli/ -run Postman -count=1` passes.

- [ ] T198 [US8] Write the demo command test in `cmd/cli/erp_demo_test.go`
  - Do: `TestErpDemoRefusesProd` (`cfg.Env == "prod"` returns an error and touches nothing); `TestErpDemoCountFlag` parses `-count N`.
  - Done: `gofmt -l cmd/cli/erp_demo_test.go` prints nothing; `go test ./cmd/cli/ -run ErpDemo -count=1` fails to compile (RED).

- [ ] T199 [US8] Add the demo command in `cmd/cli/erp_demo.go`
  - Do: refuses in prod; for the seeded `vendor@dawa24.test` organization ensures a warehouse and a branch, an integration (provider `provider.DefaultProvider`, never typed), and a key with all four vendor scopes; also creates one variant with SKU `100245` for the supplier (so quickstart step 2 has a known item) and one pending `org.user_organizations` link from `pharmacy@dawa24.test` to the supplier with an empty number via `orgSvc.CreateUserOrgLink` (quickstart step 6 needs it); prints the key once; `-count N` creates N keys. Warehouse and branch names are `i18n.TDefault("erp.demo.warehouse")` and `i18n.TDefault("erp.demo.branch")` (add both keys to `internal/shared/i18n/catalog_erp_ui.go`), never Arabic literals. Use `erp.Service.CreateKey` (never print a hash).
  - Traps: 24.
  - Done: `go test ./cmd/cli/ -run ErpDemo -count=1`.

- [ ] T200 [US8] Add the `erp-demo` case in `cmd/cli/main.go` and prove idempotence
  - Do: add case `erp-demo` in the `run()` switch (it needs the database and `cfg.Env`). Run SB-3 and SB-4, then export `SEED_PASSWORD=<12-char throwaway>` inside the sandbox command, run `cli seed-users`, then `erp-demo` twice; never print the password. The command: `bash ~/security-audit-skill/dawa24-store/run-5/sandbox/sb.sh run sh -c 'export SEED_PASSWORD=<12-char throwaway>; go build -o /tmp/cli ./cmd/cli && /tmp/cli seed-users && /tmp/cli erp-demo && /tmp/cli erp-demo'` (rule 7 defines only `SB go test`, so this task gives the exact command).
  - Done: the command prints one key per run, and the second run creates no second integration or warehouse (check with `select count(*) from erp.integrations` through `docker exec dawa24audit-pg psql`, user and database as in `batch-brief.md`).

- [ ] T201 [P] [US8] Write the Arabic part of `docs/erp-api/README.md`
  - Do: Arabic section first: how to get a key, first call, order of the Postman folders, the journey of `quickstart.md` section 5 (13 steps), polling pattern (start each cycle without a cursor and filter `state=new`), retries and idempotency, rate limits.
  - Done: `grep -c "^## " docs/erp-api/README.md` prints 5 or more.

- [ ] T202 [US8] Write the English part of `docs/erp-api/README.md`
  - Do: append the same content in English under a clear heading, plus a per-endpoint section for every path in `openapi.yaml` (purpose, fields, one example, errors, limits).
  - Done: `for p in /health /products/sync /orders/sales /orders/sales/ack /orders/purchase /offers; do grep -c "$p" docs/erp-api/README.md; done` prints no 0.

- [ ] T203 [US8] Add the field alias table and error list to `docs/erp-api/README.md`
  - Do: append the alias table from `provider/orgasoft/fields.go`, the Orgasoft unverified items (copy the V5 list), and a table of every code in `contracts/error-codes.md`.
  - Done: `grep -c "ERP_ITEM_PRICE_PRECISION" docs/erp-api/README.md` prints 1 or more.

- [ ] T204 [P] [US8] Write `docs/modules/erp.md`
  - Do: entities, invariants, named exceptions E1 to E7 with function names, behaviour changes (order numbers in S7, customer code guard, quota writer), and the list of unverified assumptions (V5).
  - Done: `grep -c "E1\|E7" docs/modules/erp.md` prints 2 or more.

- [ ] T205 [P] [US8] Add `erp` to the schema list in `AGENTS.md`
  - Do: add `erp` to the line "Postgres schemas mirror module names:". Change nothing else.
  - Done: `grep -n "ai\`, \`erp\`\|erp" AGENTS.md | head -3` shows the schema line containing `erp`.

- [ ] T206 [US8] Check the docs against the quickstart
  - Do: read `specs/002-erp-integration-api/quickstart.md` section 5 and tick each of the 13 steps against the README and the collection; fix any step that has no matching request or text.
  - Done: `for s in "Check key" "Send batch" "Sync state" "Item results" "Acknowledge" "Confirm" "Deliver" "Find offers"; do grep -c "$s" docs/erp-api/dawa24-erp.postman_collection.json; done` prints no 0.

- [ ] T207 [US8] S6 slice gate
  - Do: slice gate. Changed packages: `./cmd/cli/ ./internal/modules/erp/http/`. Focused tests: `go test ./cmd/cli/ ./internal/modules/erp/http/ -run 'Postman|ErpDemo|OpenAPIMatchesRoutes' -count=1`.
  - Commit message: `feat(erp): S6 OpenAPI, Postman and docs`. No push, merge or deploy.
  - Done: `git log -1 --format=%s` prints the commit message.

## Phase 9: S7 Purchase orders from the ERP [US6]

Goal: the ERP creates a purchase order for its own organization at Dawa24 prices with safe retries. Depends on S1. Carries the order-number fix. Independent test: create, repeat 100 times, one order.

- [ ] T208 [US6] Write the purchase order integration tests in `test/integration/erp_po_test.go`
  - Do: `TestERPPO_CreateAndRepeat100` (one order, same number, HTTP 200 on replays, SC-004); `TestERPPO_WrongPriceRefused` (`ERP_PO_PRICE_MISMATCH` with `sentPrice` and `correctPrice`, no order row); `TestERPPO_NoActiveOffer`; `TestERPPO_SupplierNotLinked` (`ERP_PO_SUPPLIER_NOT_LINKED`, and no `org.user_organizations`, customer or account row was created, FR-031); `TestERPPO_WalletRefused` (`ERP_PO_PAYMENT_NOT_SUPPORTED`); `TestERPPO_CrashRepair` (an `in_progress` row older than 2 minutes with an order carrying the note marker `[ERP:orgasoft:<ref>]` is repaired and returns that order); `TestERPPO_SelfSupply`; `TestTenantIsolation_ERPIdempotency`.
  - Traps: 14, 19, 25.
  - Done: `gofmt -l test/integration/erp_po_test.go` prints nothing; compile fails (RED).

- [ ] T209 [US6] Write the order number concurrency test in `test/integration/commerce_order_number_test.go`
  - Do: `TestOrderNumberSequenceConcurrent`: two `commerce.Service` instances, 200 concurrent checkouts, 200 distinct order numbers, no unique violation.
  - Traps: 14.
  - Done: `gofmt -l test/integration/commerce_order_number_test.go` prints nothing; the run fails (RED) with duplicate numbers or a compile error.

- [ ] T210 [US6] Write migration 258 in `db/migrations/258_order_number_sequence.up.sql` and `.down.sql`
  - Do: the SQL from `data-model.md` "Migration 258" with `BEGIN;`/`COMMIT;`, an Arabic `COMMENT ON SEQUENCE`, and the grant to `dawa24_app` inside the `IF EXISTS` role block. Down: `DROP SEQUENCE IF EXISTS commerce.order_number_seq`.
  - Done: SB-3 and SB-4 apply 258 with no error and the migrate output lists `258_order_number_sequence`.

- [ ] T211 [US6] Write the allocator unit tests in `internal/modules/commerce/order_number_test.go`
  - Do: when the repository implements `OrderNumberAllocator`, `checkout` uses it; otherwise the old counter is used; an allocator error fails the checkout.
  - Done: `gofmt -l internal/modules/commerce/order_number_test.go` prints nothing; `go test ./internal/modules/commerce/ -run OrderNumber -count=1` fails (RED).

- [ ] T212 [US6] Add the allocator in `internal/modules/commerce/order_number.go`
  - Do: `type OrderNumberAllocator interface{ NextOrderNumber(ctx context.Context) (string, error) }`, and `func (s *Service) nextOrderNumber(ctx context.Context, now time.Time) (string, error)` in the same file: if `s.repo` implements `OrderNumberAllocator` return `NextOrderNumber(ctx)`, else return `GenerateOrderNumber(now, orderCounter.Add(1))`. `service.go` is 397 lines and the limit is 400, so the logic lives here.
  - Done: `go build ./internal/modules/commerce/`.

- [ ] T213 [US6] Implement the allocator in `internal/modules/commerce/postgres/order_number.go`
  - Do: `func (r *Repository) NextOrderNumber(ctx context.Context) (string, error)` running `SELECT nextval('commerce.order_number_seq')::text`.
  - Done: `go build ./internal/modules/commerce/...`.

- [ ] T214 [US6] Use the allocator in `internal/modules/commerce/service.go`
  - Do: in `checkout`, replace the two lines `seq := orderCounter.Add(1)` and `orderNumber := GenerateOrderNumber(now, seq)` (lines 155 and 156) with `orderNumber, err := s.nextOrderNumber(ctx, now)` followed by a 3-line `if err != nil { return nil, err }`. The net change is +2 lines (399 lines). Pattern for the optional interface: `shipmentWalletUpdater` at `api_cancel.go:107`.
  - Traps: 14.
  - Done: `make check-file-size`, `go test ./internal/modules/commerce/... -count=1` and `SB go test -p 1 ./test/integration/ -run TestOrderNumberSequenceConcurrent -count=1`.

- [ ] T215 [P] [US6] Note the order number change in `docs/modules/commerce.md`
  - Do: record the sequence, the optional interface, the formats of 9 or more digits never colliding, and that rollback to the old image is safe.
  - Done: `grep -n "order_number_seq" docs/modules/commerce.md` prints a line.

- [ ] T216 [US6] Add the purchase repository interface in `internal/modules/erp/repository_purchase.go`
  - Do: `ListOffersAsSystem(ctx, buyerOrgID int64, barcode string, limit int) ([]Offer, error)` (plan.md E6); `BeginIdempotency(ctx, orgID, integrationID int64, ref, requestSHA string) (IdemState, error)`; `FinishIdempotency(ctx, orgID, integrationID int64, ref string, orderID int64, orderNumber string, response []byte) error`; `DeleteIdempotency(ctx, orgID, integrationID int64, ref string) error`; `FindOrderByNoteMarker(ctx, buyerOrgID int64, marker string) (orderID int64, orderNumber string, found bool, err error)`; `SupplierLinked(ctx, buyerOrgID, supplierOrgID int64) (bool, error)`; `VariantOfferAsSystem(ctx, publicID string) (VariantOfferRow, error)` (plan.md E6). Types: `Offer{VariantID string; SupplierName, Name map[string]string; SupplierLinked bool; Barcode string; PublicPrice, DiscountPercent, UnitPrice money.Amount; InStock bool}`; `IdemState{Replay bool; InProgress bool; Stale bool; Response []byte}`; `VariantOfferRow{ID, OrganizationID int64; PublicID string; PublicPrice, DiscountPercent money.Amount}`; `CreatePurchaseRequest{ExternalReference, Notes string; Lines []PurchaseLine}`; `PurchaseLine{VariantID string; Quantity int; UnitPrice *money.Amount}`. Embed in `Repository`. New file because plan keeps purchase code out of `repository_orders.go`. Create `fake_purchase_test.go` with `type fakePurchase struct` implementing these methods in memory and change `fakeRepo` to `type fakeRepo struct { ...; *fakeSync; *fakeOrders; *fakeCode; *fakePurchase }` so it still satisfies `erp.Repository`.
  - Done: `go build ./internal/modules/erp/` and `go vet ./internal/modules/erp/`.

- [ ] T217 [P] [US6] Add offers SQL in `internal/modules/erp/postgres/offers.go`
  - Do: implement `ListOffersAsSystem` and `VariantOfferAsSystem` (resolve a variant `public_id` UUID to id, organization id and price fields). Read-only on `catalog.product_variants`: `status='active'`, `deleted_at IS NULL`, organization type vendor and approved, stock greater than 0, barcode match, price ascending, `LIMIT 50` (`OffersMax`), public fields only; `SupplierLinked` in the offer is an `EXISTS` on `org.user_organizations` for the buyer. These two functions are plan.md E6: wrap the query in `database.AsSystem(ctx)` and put the comment `plan.md E6` on each.
  - Traps: 1.
  - Done: `gofmt -l internal/modules/erp/postgres/offers.go` prints nothing (`go vet` is not required: the package compiles as a whole at T218).

- [ ] T218 [P] [US6] Add idempotency SQL in `internal/modules/erp/postgres/idempotency.go`
  - Do: insert `in_progress`, get, finish (`done` with `response`), delete; `FindOrderByNoteMarker` on `commerce.orders.notes LIKE '[ERP:%'` under `WithTenant(buyer)`; `SupplierLinked(ctx, buyerOrgID, supplierOrgID)`: `SELECT 1 FROM org.user_organizations WHERE vendor_org_id=$2 AND customer_org_id=$1 AND status IN ('pending','approved') AND deleted_at IS NULL` (no RLS on that table, research D-20).
  - Traps: 1, 25.
  - Done: `go build ./internal/modules/erp/postgres/` must pass including the `var _ erp.Repository` line.

- [ ] T219 [US6] Add the purchase ports in `internal/modules/erp/ports.go`
  - Do: add `PurchasePlacer`, `PurchaseRequest{BuyerOrgID, BuyingUserID int64; Lines []PurchaseLine; Notes string}`, `PlacedOrder{OrderID int64; OrderNumber string; Total money.Amount}`, field `Placer` on `Ports`.
  - Traps: 23.
  - Done: `go build ./internal/modules/erp/`.

- [ ] T220 [US6] Write the purchase service tests in `internal/modules/erp/purchase_service_test.go`
  - Do: every purchase code in `contracts/error-codes.md` "Purchase orders" has a row: reference required, buying user missing or not an active member, item not found (line number in `details`), no active offer, quantity zero or negative or fractional, price mismatch with `lines[{line,sentPrice,correctPrice}]` as exact strings like `"62.50"`, supplier not linked (no account is created), self supply, refused by rules (`reason`), in progress, payment not supported; more than `PurchaseLinesMax` (200) lines gives `ERP_VALIDATION`; a replay returns the stored response with HTTP 200 and calls `Place` once; a refusal deletes the idempotency row; a stale `in_progress` row is repaired through the marker; audit `erp.po.create` with `ActorKind: "api_key"`; `Offers` returns at most 50.
  - Traps: 13, 25.
  - Done: `gofmt -l internal/modules/erp/purchase_service_test.go` prints nothing; `go test ./internal/modules/erp/ -run Purchase -count=1` fails (RED).

- [ ] T221 [US6] Add the purchase service in `internal/modules/erp/purchase_service.go`
  - Do: `Offers(ctx, p Principal, barcode string) ([]Offer, error)` and `CreatePurchaseOrder(ctx, p Principal, req CreatePurchaseRequest) (PurchaseResult, error)` as in plan.md S7 step 4, with `PurchaseResult{OrderID int64; OrderNumber string; Replay bool; Response []byte}`. The order note starts with `[ERP:<provider>:<externalReference>]`. Never creates accounts or links.
  - Traps: 13, 25.
  - Done: `go test ./internal/modules/erp/ -run Purchase -count=1`.

- [ ] T222 [US6] Write the placer tests in `cmd/server/erp_purchase_placer_test.go`
  - Do: `TestErpPurchasePlacer`: a `variantId` UUID resolves to a variant id; a quote is made with `apiQuoteResolver.quote`; a differing `unitPrice` returns `ERP_PO_PRICE_MISMATCH` with the quote's amounts; an `apperr` code from `CheckoutAPI` becomes `ERP_PO_REFUSED` with `details.reason`; a wallet error becomes `ERP_PO_PAYMENT_NOT_SUPPORTED`.
  - Done: `gofmt -l cmd/server/erp_purchase_placer_test.go` prints nothing; `go test ./cmd/server/ -run ErpPurchasePlacer -count=1` fails (RED).

- [ ] T223 [US6] Add the placer in `cmd/server/erp_purchase_placer.go`
  - Do: `erpPurchasePlacer` builds a dedicated `commerce.Service` exactly like `routes.go:217-236` (wallet refunder and payer, docs gate, `OnOrderPlaced(deps.orders.announce)`, availability probe, `apiQuoteResolver` with `apiBuyerChoices`); `Place` as in plan.md S7 step 5. Own file because `erp_ports.go` would pass 350 lines.
  - Traps: 9, 14.
  - Done: `go test ./cmd/server/ -run ErpPurchasePlacer -count=1`.

- [ ] T224 [US6] Hold the placer in `cmd/server/erp_wiring.go`
  - Do: set `erp.Ports.Placer` to the placer after the commerce dependencies exist.
  - Done: `go build ./... && go test ./cmd/server/ -run Router -count=1`.

- [ ] T225 [US6] Write the purchase handler tests in `internal/modules/erp/http/purchase_handlers_test.go`
  - Do: `GET /offers?barcode=` (scope `erp.po.create`), `POST /orders/purchase` returns 201 new and 200 replay; body `externalReference`, `lines[{variantId, quantity, unitPrice?}]`, `notes?` per `openapi.yaml`; error bodies bilingual.
  - Done: `gofmt -l internal/modules/erp/http/purchase_handlers_test.go` prints nothing; `go test ./internal/modules/erp/http/ -run Purchase -count=1` fails (RED).

- [ ] T226 [US6] Add the purchase handlers in `internal/modules/erp/http/purchase_handlers.go` and mount them
  - Do: `getOffers`, `postPurchase`; mount both in `handler.go` (two files).
  - Done: `go test ./internal/modules/erp/http/ -count=1`.

- [ ] T227 [US6] Empty the not-yet-mounted list in `internal/modules/erp/http/spec_test.go`
  - Do: set `specRoutesNotYetMounted` to an empty slice.
  - Done: `go test ./internal/modules/erp/http/ -run OpenAPIMatchesRoutes -count=1`.

- [ ] T228 [US6] Regenerate the Postman files after S7
  - Do: run `go run ./cmd/cli erp-postman` so the two JSON files in `docs/erp-api/` include the purchase requests.
  - Done: `go test ./cmd/cli/ -run PostmanCollectionIsCurrent -count=1`.

- [ ] T229 [US6] Run the S7 integration tests
  - Do: SB-3, SB-4, then run. Fix the owning S7 task when a test fails.
  - Done: `SB-6 go test -p 1 ./test/integration/ -run 'ERPPO|TestTenantIsolation_ERPIdempotency|TestOrderNumberSequenceConcurrent' -count=1` passes.

- [ ] T230 [US6] S7 slice gate
  - Do: slice gate. Changed packages: `./internal/modules/commerce/... ./internal/modules/erp/... ./cmd/server/ ./cmd/cli/`. Focused tests: `go test ./internal/modules/commerce/... ./internal/modules/erp/... ./cmd/server/ ./cmd/cli/ -run 'OrderNumber|Purchase|ErpPurchasePlacer|OpenAPIMatchesRoutes|Postman|Router' -count=1`; the SB command above.
  - Commit message: `feat(erp): S7 purchase orders and order number sequence`. No push, merge or deploy.
  - Done: `git log -1 --format=%s` prints the commit message.

## Phase 10: S8 Reveal [US7]

Goal: sensitive values are masked on screen and revealed one at a time with an audit row. Needs only T026 to T029 of S1 (the i18n files and the rbac test file); independent of S2 to S7. Independent test: open a masked field, reveal it as a permitted user, see the audit row; an unpermitted user is refused and logged.

- [ ] T231 [P] [US7] Write the mask tests in `internal/shared/mask/mask_test.go`
  - Do: table tests for `Phone`, `Email`, `Number`: keep the last 3 characters; values of 3 characters or fewer fully masked; empty stays empty; email keeps the domain; Arabic-Indic digits handled.
  - Done: `gofmt -l internal/shared/mask/mask_test.go` prints nothing; `go test ./internal/shared/mask/ -count=1` fails to compile (RED).

- [ ] T232 [US7] Add the mask functions in `internal/shared/mask/mask.go`
  - Do: `Phone(s string) string`, `Email(s string) string`, `Number(s string) string`, pure; `shared` imports nothing from this repository.
  - Done: `go test ./internal/shared/mask/ -count=1`.

- [ ] T233 [US7] Write the reveal permission tests (edit `internal/platform/rbac/erp_perms_test.go`)
  - Do: add `platform.reveal.sensitive` (admin catalog) to `TestErpPermsAdmin` and `vendor.reveal.customer_contact` (vendor catalog) to `TestErpPermsVendor`.
  - Done: `go test ./internal/platform/rbac/ -run 'TestErpPermsAdmin|TestErpPermsVendor' -count=1` fails (RED).

- [ ] T234 [P] [US7] Add `platform.reveal.sensitive` in `internal/platform/rbac/catalog_admin.go`
  - Do: add the permission key.
  - Traps: 29.
  - Done: `go test ./internal/platform/rbac/ -run TestErpPermsAdmin -count=1`.

- [ ] T235 [P] [US7] Add `vendor.reveal.customer_contact` in `internal/platform/rbac/catalog_vendor.go`
  - Do: add the permission key.
  - Traps: 29.
  - Done: `go test ./internal/platform/rbac/ -run TestErpPermsVendor -count=1`.

- [ ] T236 [US7] Add reveal button strings in `internal/shared/i18n/catalog_erp_ui.go`
  - Do: `reveal.button`, `reveal.hide`, `reveal.masked_hint` bilingual (the three `erp.err.reveal_*` messages already exist from S1).
  - Traps: 12.
  - Done: `go test ./internal/shared/i18n/ -count=1 && make check-hardcoded-arabic`.

- [ ] T237 [US7] Write the reveal handler tests in `internal/ui/reveal_handlers_test.go`
  - Do: names start with `TestReveal`: each allowlisted kind (`org.tax_number`, `org.commercial_register`, `org.pharmacist_license`, `org.phone`, `org.email` admin only, `branch.phone` supplier only for a branch on an order shipped by the caller's organization, `gateway.admin_username` admin) grants with the right permission and returns an HTML fragment holding the value; wrong permission gives `REVEAL_FORBIDDEN`; unknown kind, gateway password, virtual key, AI key and ERP key give `REVEAL_NOT_REVEALABLE`; missing record gives `REVEAL_NOT_FOUND`; each outcome writes one audit row `reveal.grant` or `reveal.deny` with entity kind and id and no value, in its own transaction (a deny row survives); a request without CSRF is refused.
  - Done: `gofmt -l internal/ui/reveal_handlers_test.go` prints nothing; `go test ./internal/ui/ -run Reveal -count=1` fails (RED).

- [ ] T238 [P] [US7] Add the reveal allowlist in `internal/ui/reveal_kinds.go`
  - Do: `type revealKind struct{ Permission string; Load func(ctx context.Context, h *UIHandler, actor authctx.Actor, id int64) (string, error); Audience string }` and `var revealKinds = map[string]revealKind{...}` for the seven kinds. Load each value through the service call the existing page already uses (find it with `grep -rn "tax_number\|commercial_register" internal/ui/*.go | head`), not new SQL in `ui`. Separate file because `reveal_handlers.go` would pass 350 lines.
  - Done: `go build ./internal/ui/`.

- [ ] T239 [US7] Add the reveal handler in `internal/ui/reveal_handlers.go`
  - Do: `func (h *UIHandler) Reveal(w, r)` for `POST /reveal` (fields `kind`, `id`): allowlist lookup, permission check, load, audit row, HTML fragment response for an htmx swap.
  - Traps: 24.
  - Done: `go test ./internal/ui/ -run Reveal -count=1`.

- [ ] T240 [US7] Register `POST /reveal` in `internal/ui/handlers.go`
  - Do: register it inside `RegisterApprovedSharedRoutes` (about line 287) with the gate naming the reveal permissions and CSRF protection.
  - Traps: 29.
  - Done: `go test ./test/ -run 'TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1`.

- [ ] T241 [US7] Add the reveal field component in `internal/ui/components/reveal_field.templ`
  - Do: `RevealField(kind string, id int64, maskedValue string, canReveal bool)`: renders only the masked value; when `canReveal`, a button posting to `/reveal` with htmx and swapping the span. Run templ generate.
  - Traps: 22.
  - Done: `go build ./internal/ui/components/`. (`make check-unused-components` fails until a page uses the component; it runs in T245 and T246.)

- [ ] T242 [US7] Write the page source tests in `internal/ui/reveal_pages_test.go`
  - Do: `TestRevealPagesHideValues`: render the admin organization detail, the vendor order detail and the admin developers page with known sensitive values and assert with `strings.Contains` that the clear value is absent from the HTML (SC-010), the masked form is present, the buyer branch phone has no `tel:` link until revealed, and no gateway password, virtual key or AI provider key appears in any of the three pages. `TestRevealEveryGrantAuditsOnce`: n reveals produce n audit rows.
  - Done: `gofmt -l internal/ui/reveal_pages_test.go` prints nothing; `go test ./internal/ui/ -run RevealPages -count=1` fails (RED).

- [ ] T243 [P] [US7] Use Reveal in `internal/ui/pages/admin_org_detail.templ`
  - Do: at lines about 196 to 215 replace commercial register, tax number, pharmacist license, phone and email with `RevealField`; the view struct and its handler pass the masked value (`mask.Number`, `mask.Phone`, `mask.Email`), never the clear one. Run templ generate. Touched: the templ file and the view builder file that fills it (find with `grep -rn "AdminOrgDetail" internal/ui/*.go | head -3`).
  - Traps: 22.
  - Done: `go test ./internal/ui/ -run RevealPages -count=1` (admin case passes).

- [ ] T244 [P] [US7] Use Reveal in `internal/ui/pages/vendor_order_detail.templ`
  - Do: at lines 105 to 108 mask the buyer branch phone with `RevealField` kind `branch.phone`, remove the `tel:` link until revealed; the handler passes the masked value. Run templ generate. Touched: the templ file and the vendor order detail handler file.
  - Traps: 22.
  - Done: `go test ./internal/ui/ -run RevealPages -count=1` (vendor case passes).

- [ ] T245 [P] [US7] Use Reveal in `internal/ui/pages/admin_developers_ai.templ`
  - Do: the gateway administrator username on this page is an editable input (`name="admin_username"`, lines 152 to 155), not a display field. Keep the `admin_username` input empty with its placeholder, and render `RevealField(kind=gateway.admin_username)` beside it as a read-only display of the stored value; the submit still posts only the typed value. If no value is stored, show nothing. The password stays unrendered as it is today. Run templ generate. Touched: the templ file and its handler file.
  - Traps: 22.
  - Done: `go test ./internal/ui/ -run Reveal -count=1` and `make check-unused-components`.

- [ ] T246 [US7] S8 slice gate
  - Do: slice gate. Changed packages: `./internal/shared/mask/ ./internal/platform/rbac/ ./internal/ui/... ./internal/shared/i18n/`. Focused tests: `go test ./internal/shared/mask/ ./internal/platform/rbac/ -count=1`; `go test ./internal/ui/ -run Reveal -count=1`; `go test ./test/ -run 'TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1`; `make check-hardcoded-arabic check-unused-components`.
  - Commit message: `feat(erp): S8 reveal for sensitive values`. No push, merge or deploy.
  - Done: `git log -1 --format=%s` prints the commit message.

## Phase 11: Polish

- [ ] T247 [P] Add the load script `test/load/erp_sync.js`
  - Do: k6 script from `quickstart.md` section 9: variables `BASE`, `KEY`, `KEYS` (comma list), `ITEMS` (default 50000); posts one batch, checks 202 within 5 s, polls the sync status until `completed`. Do not run it.
  - Done: `node --check test/load/erp_sync.js` prints nothing.

- [ ] T247a Add the SC-003 website-latency scenario to `test/load/erp_sync.js`
  - Do: add a scenario `web` that logs in with `EMAIL` and `PASSWORD` and requests the pages of `test/load/k6_pages.js` (copy its `pages` list and the login in its `setup`) with 20 virtual users for 8 minutes, tagged `scenario:web`. Add a sync scenario that runs only when `SYNC=1` (default `0`): each key in `KEYS` posts one batch of `ITEMS` items every 7 minutes (`constant-arrival-rate`, `rate` equal to the number of keys, `timeUnit` `7m`). In the end summary print the p95 of `http_req_duration{scenario:web}` as a line `WEB_P95_MS <number>`. Do not run it here.
  - Done: `node --check test/load/erp_sync.js` prints nothing and `grep -c "WEB_P95_MS" test/load/erp_sync.js` prints 1 or more.

- [ ] T247b Run the load script once and record SC-002 and SC-003 in `specs/002-erp-integration-api/verify-notes.md`
  - Do: run `k6 version`. If it fails, write `V8 result: not run, k6 is not installed; owner runs quickstart section 9` and stop; install nothing. Otherwise start the server and worker in the sandbox (quickstart sections 1 and 2, `APP_ENV` not `prod`), create 10 keys with `erp-demo -count 10` (keep them in a shell variable; never print them), run `k6 run test/load/erp_sync.js -e BASE=<url> -e KEYS=<the keys> -e ITEMS=50000 -e SYNC=1 -e EMAIL=<seed email> -e PASSWORD=<seed password>` once, then the same command with `-e SYNC=0`. Record the 202 time, the completion time, and `WEB_P95_MS` of both runs. Pass rule: 202 in under 5 s, completed in under 7 minutes, and `WEB_P95_MS` with sync at most 1.2 times the value without sync. Append: `SC-003 modelled with 10 keys of one organization, not 50 tenants; the owner confirms with real tenants before launch`.
  - Done: `grep -n "^V8 result:" specs/002-erp-integration-api/verify-notes.md` prints one line.

- [ ] T248 [P] Run the static gates for the whole feature
  - Do: `make check-file-size check-hardcoded-arabic check-provider-isolation check-modal-handwritten check-deadcode check-unused-components`. Fix only findings in files this feature added or changed; record any other finding in `verify-notes.md` and do not touch it.
  - Done: the command exits 0 or `verify-notes.md` lists every remaining finding.

- [ ] T249 Run the structural tests for the whole feature
  - Do: `go test ./test/ -run 'TestModulesDoNotImportEachOther|TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1` and `go test ./internal/platform/queue/ -count=1`.
  - Done: both pass.

- [ ] T250 Run the whole ERP isolation and flow suite once
  - Do: SB-3, SB-4, then one run.
  - Done: `SB-6 go test -p 1 ./test/integration/ -run 'ERP|TestTenantIsolation_ERP|TestOrderNumberSequenceConcurrent|TestAuditActorKeyRoundTrip' -skip 'Scale50000' -count=1` passes (the scale test has its own run, T138a).

- [ ] T251 Confirm no secret or key is stored or logged
  - Do: `grep -rn "Authorization" internal/modules/erp --include=*.go | grep -v _test.go` and read each hit; confirm none writes the header value to a log, a database row or a response.
  - Traps: 24.
  - Done: the review is written as `V7 result:` in `verify-notes.md`.

- [ ] T252 Write the owner handoff in `specs/002-erp-integration-api/verify-notes.md`
  - Do: add a section `Handoff` listing: migrations 255 to 258 and their order; config keys `ERP_API_ENABLED`, `ERP_KEY_PEPPER` (required in prod, 32 bytes or more), `ERP_DEFAULT_RATE_PER_MINUTE`, `ERP_MAX_BATCH_ITEMS`, `WORKER_ERPSYNC`; worker must run for sync; `ERP_API_ENABLED` defaults false and disabled means no ERP route or erpsync worker; deploy first with the API off, then set `ERP_KEY_PEPPER` and `ERP_API_ENABLED=true` when ready; tests and Postman flows that need the API explicitly enable it; the V5 unverified items; any `V` result that said no.
  - Done: `grep -c "Handoff" specs/002-erp-integration-api/verify-notes.md` prints 1.

## Dependencies and order

- Phase 1 (T001 to T007) first. T003 (V1) blocks T083 (`NewInsertOnly`) in S2; T004 (V2) confirms the S2 cleanup task T118 needs no child delete; T005 (V3) is the templ command for every `.templ` task.
- Phase 2 (T008 to T010) before S2 (provider and item tests), S3 (acknowledgement test) and S6 (Postman bodies).
- S1 (T011 to T080, plus T042a) blocks S2, S3, S7. S8 (T231 to T246) needs T026 to T029 from S1 (the i18n files and the rbac test file) and nothing else; it may run any time after those four tasks.
- S2 needs S1. S3 needs S1 (S2 only for `externalProductCode` in lines). S4 needs S3. S5 needs S3. S6 needs the endpoints it describes (S1 to S5; folder `04 Purchase` passes only after S7). S7 needs S1.
- Recommended order, as in plan.md: S1, S2, S3, S4, S5, S6, S7, S8, then Polish.
- Tasks with a letter suffix run right after the task whose number they carry, in this order: T042a (provider constant, before T067, T071 and T093), T060a (handler shell before T061), T099a (scale test, RED, before the S2 wiring), T124a (ADR, after the adapters), T138a (scale run, after T138), T158a then T158b (wallet shortfall error, before T161), T247a then T247b (load script scenario, then the run).
- `var _ erp.Repository = (*Repository)(nil)` is added by T118. From T145 the Postgres tasks T146 to T148, and T217, only need `gofmt` to pass; T149, T175 and T218 must `go build` the package with the assertion.
- Inside a slice: migration before code that uses it; tests before the code they prove; templ tasks before the handlers that call them; the slice gate last.
- Postman work grows: S6 builds the generator and the first committed files; the S7 task "Regenerate the Postman files after S7" re-runs it. Re-run it again after any later change to `openapi.yaml`.

## Counts

Total 260 tasks: 252 numbered T001 to T252 and 8 with a letter suffix (T042a, T099a, T124a, T138a, T158a, T158b, T247a, T247b).

| Phase | Tasks |
|---|---|
| 1 Setup (T001 to T007) | 7 |
| 2 Foundational (T008 to T010) | 3 |
| 3 S1 (T011 to T080) | 71 |
| 4 S2 (T081 to T139) | 62 |
| 5 S3 (T140 to T155) | 16 |
| 6 S4 (T156 to T170) | 17 |
| 7 S5 (T171 to T191) | 21 |
| 8 S6 (T192 to T207) | 16 |
| 9 S7 (T208 to T230) | 23 |
| 10 S8 (T231 to T246) | 16 |
| 11 Polish (T247 to T252) | 8 |

## Parallel examples

Tasks marked `[P]` touch different files and need nothing unfinished. Run them together only if one agent per task is available; otherwise do them in order.

- Foundational: T008, T009, T010 (three fixture files).
- S1: T030, T031, T032 (vendor, pharmacy, admin permissions); T033 and T034 (`limits.go`, `scopes.go`); T040 and T041 (`repository.go`, `ports.go`); T047, T048, T049 (postgres integrations, keys, calls SQL); T063, T064 (health route, embedded spec); T072, T073, T074 (three templ pages); T077, T078, T079 (three route files).
- S2: T101, T102, T103, T104 (postgres sync, items, review, lookup SQL; they need only `gofmt`, see above); T120 and T121 (two workers); T122, T123, T124 (three adapters).
- S3: T147, T148, T149 (sales, purchase, imports SQL).
- S5: T181 and T182 (guard in the customer and vendor handler files); T186 (modal templ); T189 (org.md note).
- S6: T201 (README Arabic part), T204 and T205 (`docs/modules/erp.md`, `AGENTS.md`).
- S7: T215 (commerce.md), T217 and T218 (offers and idempotency SQL).
- S8: T231 (mask test, may start with Phase 1 done), T234 and T235 (admin and vendor permissions), T238 (`reveal_kinds.go`), T243, T244, T245 (three page edits).
- Polish: T247 and T248 (load script, static gates).

## MVP scope

The first version the owner can test is **S1 + S2 + S3** (T001 to T155 plus Phase 2, including the suffix tasks in that range): a key from the website, product sync with per-item results and the review screen, and order feeds with acknowledgement. The owner can drive that with plain HTTP requests. The Postman collection (S6) is what makes owner testing comfortable; its generator task can be pulled forward right after S3 if the owner wants the collection for the MVP, at the cost that folders `03 Decide` and `04 Purchase` fail until S4 and S7. S4, S5, S7, S8 add on top without changing the MVP.

## Optional extension hook

`.specify/extensions.yml` registers `after_tasks`: `speckit.security-review.tasks` (optional, enabled, prompt "Would you like to verify that security boundaries are represented in the tasks?"). It is listed here and was not run.
