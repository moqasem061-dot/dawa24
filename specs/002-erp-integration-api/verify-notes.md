# ERP integration verification notes

V1 result: yes, client.go:611
V2 result: all foreign keys to catalog.match_runs are CASCADE or SET NULL, no child delete is needed

T005 initial check (resolved using the installed binary): The prescribed HOST-GO command `go run github.com/a-h/templ/cmd/templ version` fails because go.sum lacks entries for templ command dependencies, including github.com/a-h/parse and golang.org/x/tools. No working V3 command was verified; guidance is required before changing dependencies or using another command.
V3 result: ~/go/bin/templ.exe generate (version v0.3.1020; matches go.mod and generated file headers). The go run form fails because go.sum lacks the templ command dependencies.
V4 result: all found

V5 unverified (owner or Orgasoft must confirm)
- Sample 3 is the import acknowledgement (research D-18).
- Orgasoft wants `variantId` as purchase-order item identity (D-21).
- `taxes` in sample 3 is an amount while `tax: 14` in the product samples is a percent.
- `discount` and `tax` are percents and `tax` is stored, not applied.
- Quota means a cap per buying pharmacy branch.
- One external code is one sellable item per tenant.
- The optional fields `unit`, `manufacturer` and `batchNumber` are not in any Orgasoft sample and are ignored when absent.

T011 initial blocker (resolved by SB-6): The specified sandbox script connects tests as PostgreSQL user `dawa24`, which is a superuser (`pg_roles.rolsuper = true`). The prescribed example `test/integration/match_ai_attempts_isolation_test.go` uses `getTestDB`, which skips isolation tests for a superuser (`test/integration/rls_test.go:20-43`). Consequently the required SB run cannot fail RED for the missing `erp` schema using that structure, and a superuser cannot prove RLS isolation. Owner direction is needed on the test connection/role before implementing T011.

T013 initial blocker (resolved by owner): `data-model.md` specifies an index on `erp.sync_batches (organization_id, created_at DESC)` but its sync_batches column list has no `created_at` column (it specifies `accepted_at` instead). T013 requires the table and indexes exactly as documented. The partial unapplied migration contains the specified index and is incomplete; guidance is needed on whether to add `created_at` or change the index to `accepted_at`.

T013 resolution: sync_batches uses accepted_at for the organization index, list ordering and retention; sync_items retention uses its batch accepted_at or processed_at for processed items. No created_at column is added.

T019 initial blocker (resolved by owner): T018 correctly introduced integration test references to the not-yet-existing `database.AuditEntry.ActorKind` and `ActorKeyID` fields. The T019 Done check requires running `go test ./test/integration/` before T020 adds those fields, but Go compiles the entire package regardless of `-run`; the T018 check confirms compilation fails on both fields. T019 cannot pass as ordered without changing task scope/order or weakening the test. Owner direction is needed.

T019/T020 order swap: create migration 256, implement T020 first, then run T019 checks and rerun T018 GREEN as directed by owner.

T022 initial blocker (resolved by owner): The required `go test ./internal/platform/config/ ./internal/platform/queue/ -count=1` fails in three existing production configuration tests: `TestProductionUserConfig`, `TestProductionServerRequiresRLSByDefault`, and `TestCLIDoesNotRequireRLS`. Their fixtures do not set the newly required ERP_KEY_PEPPER; the task explicitly limits edits to `erp.go` and `config.go`. Queue tests passed. Direction is needed to update the existing test fixtures or otherwise resolve this conflict without weakening the production requirement.

Design change: ERP_API_ENABLED defaults to false. Disabled production and CLI require no ERP settings; enabled production requires a pepper of at least 32 bytes. Disabled means no ERP routes or erpsync worker. Deploy with the API off, then set ERP_KEY_PEPPER and ERP_API_ENABLED=true when ready.

T051 initial blocker (resolved by owner): The task requires `UpdateKeyLimits` itself to reject a non-owner, but its specified T052 signature receives only `actorUserID`, and the T040 repository interface has no owner lookup (only `ActiveMember`, which cannot establish ownership); T041 explicitly provides no ports. The UI later checks `actor.IsOwner`, but that cannot make the required service-level test pass. An ownership signal or lookup must be specified before writing the test and implementation; this is a security decision.

T051 resolution: rotate and rate-limit changes receive an explicit actorIsOwner from the signed-in actor; false returns ERP_VALIDATION with actor field and no mutation or audit. Tenant-scoped key lookup remains mandatory; staff revocation is separate.

T053 initial blocker (resolved by owner): The task requires tests proving that both unknown-prefix and wrong-secret authentication compute an HMAC. `HashSecret` and `VerifyHash` are direct package functions with no injectable observer, while T053 only authorizes a test file and T054 specifies a fixed `Authenticate` signature. Identical response assertions alone cannot prove HMAC execution, and timing assertions would be unreliable. Direction is needed for a testable HMAC observation mechanism; this affects the authentication timing security check.

T053 resolution: constructor-initialized private service HMAC field, wrapped only by same-package tests; exactly one real HMAC on every authentication path, constant-time comparison with dummy hash for unknown prefixes, and uniform authentication failure response.

T059 initial blocker (resolved by owner): T059 requires a per-IP counter of failed authentication attempts, but T061 requires calling the counter before `svc.Authenticate`, and T060 requires `AllowKey`, which increments the Redis counter on every call. That order counts successful authentication attempts as failures and can rate-limit legitimate traffic. Resolving the order or limiter behavior changes security and rate-limiting behavior, so owner direction is required before writing the middleware tests.

T059 resolution: read-only ERP-specific per-IP failure check before authentication; increment only on failure, never on blocked or successful requests. Blocked requests do not extend the window. Redis errors log and fail open; use the trusted client IP helper.

T059 seam blocker (resolved by owner): The required middleware test in `internal/modules/erp/http/auth_test.go` must prove a blocked request leaves the HMAC counter unchanged, but the owner-authorized `Service.hashSecret` seam is unexported in package `erp` and only same-package tests may replace it. The HTTP package cannot access it; importing the HTTP package from an `erp` same-package test creates an import cycle. Exposing a setter or seam would violate the explicit security restriction. Direction is needed on how to prove no authentication occurs without exposing the HMAC seam.

T059 test seam resolution: middleware tests count injected authenticate calls; the ERP package test proves one HMAC per call. Add a real-router real-service authentication and failure-limiter test in T066. The HMAC seam stays private.

T061 initial blocker (resolved by owner): The required `Handler.auth` cannot compile because `Handler` is first defined by T065. The RED order rule does not resolve this as a simple swap: T065 must mount `h.health` and the embedded spec from T063/T064, while T063 itself declares a method on the not-yet-defined `Handler` and requires a build. In addition T065 says `/health` is unauthenticated, while T062 requires key-specific health data for a valid key. Completing these out of order requires changing task scope and deciding authentication behavior. Owner direction is required.

T061–T065 resolution: T060a defines the handler and constructor only, before middleware. ERP health requires normal authentication; OpenAPI spec remains public per security: []. T066 tests real service health before and after revocation. Platform health is unchanged.

T062/T063 initial blocker (resolved by owner): The required health response includes organization bilingual name, warehouse name, key expiry and integration status. `erp.Principal` lacks those values, and `erp.Service`/`Repository` expose no method to load the health data; T063 authorizes only `http/health.go`. The HTTP package cannot access the service's private repository, and a warehouse is tenant-scoped. A service/repository contract and tenant-scoped query are needed to test and return real values; adding them changes task scope and the data-access contract, so owner direction is required.

T062/T063 resolution: add HealthData to ERP service and repository, using only principal IDs, InReadTx/WithTenant and explicit organization/key filters; no secrets or AsSystem. Add cross-tenant and request-forgery tests.

T066 initial blocker (resolved by owner): The requested real-server-router, real-service test must exercise a valid database-backed key and Redis failure limiter. `newRouter` production wiring uses the real database/cache handles, but HOST-GO explicitly unsets DATABASE_URL, TEST_DATABASE_URL and TEST_REDIS_URL, and the audit Postgres and Redis containers publish no host ports. T066/T069 require host `go test ./cmd/server/ -run Router`; a real wired test cannot connect there without a skip or changing the test execution/dependency setup. Owner direction is needed; no skip or fake-only substitute has been added.

T066 resolution: real router test runs with SB-6 and Redis in the sandbox; it may skip only when no database URL is set and must visibly PASS without skipping in the sandbox. Host router tests remain database-free.

T066 ordering blocker (resolved by owner): The new sandbox real-wiring test ran (no skip) and failed because a valid `/erp/v1/health` request returned 404. The actual route is only mounted by T067 and T069, which are later tasks. T066 now requires a visible PASS before advancing, and the RED order rule only permits swaps for compile failures. Owner direction is needed to run T067–T069 before the T066 PASS check; T066 remains unchecked.

T066 order resolution: complete T067–T069 first, then rerun the real-wiring sandbox test and mark T066 only after a visible PASS.

T050 resolution (pre-existing): `internal/modules/erp/fake_keys_test.go` already satisfies T050 — `type fakeRepo struct` with in-memory maps for keys and integrations, a `[]AuditRecord` slice, an org-status hook (`orgStatus`/`orgType` fields), and the compile-time assertion `var _ Repository = (*fakeRepo)(nil)`. It is used by `keys_service_test.go`, `calllog_test.go` and `authenticate_test.go`, so no second fake was written. Done check `gofmt -l internal/modules/erp/fake_keys_test.go` prints nothing (EXIT:0).

T070 resolution (pre-existing): `internal/ui/erp_keys_test.go` was already written and covers the T070 Do list (owner-only create with spoofed `is_owner`/`actorIsOwner` fields refused with no key row; full key once in the create response and prefix-only on reload; rotate/revoke/limits refuse non-owners even with the spoofed fields; limits save 700 and refuse 9 and 6001; unknown scope rejected; staff page lists both organizations and revokes; the pharmacy page under `/customer/integrations` shows only its own keys). Done checks: `gofmt -l internal/ui/erp_keys_test.go` prints nothing; `go test ./internal/ui/ -run Erp -count=1` failed RED for the right reason (ErpKeyCreate/ErpKeysPage/ErpKeyRotate/ErpKeyRevoke/ErpKeyLimits/ErpAdminKeysPage/ErpAdminKeyRevoke/parseCreateKeyForm undefined). Modal rendering (`components.Modal`) is enforced by `make check-modal-handwritten` in the T073/T079 Done checks rather than a Go unit test in this file.

T071 note: T071's Done (`go build ./internal/ui/`) needed validation message strings that did not exist yet, so six `erp.keys.*` strings were added to `internal/shared/i18n/catalog_erp_ui.go` (name_length, rate_range, scope_invalid, warehouse_invalid, branch_invalid, buying_user_invalid, grace_range, provider_invalid — the establisished erp.keys.* UI catalog file). Later additions for the pages: status_active, status_revoked, revoked_done, limits_saved, paused, resumed.

T072/T073/T074 note (rule 9): templ files edited (`erp_keys.templ`, `erp_key_modals.templ`, `admin_erp_keys.templ`); `~/go/bin/templ.exe generate` (v0.3.1020, matches go.mod) was run and the `_templ.go` outputs will be committed with the slice-gate commit (rule 2 commits only at the markers). No CSS or homepage template changed, so no homepage hash refresh — recorded here as "no hash change". `make` is not on the Git Bash PATH, so T073's `make check-modal-handwritten` was run as its literal equivalent command from Makefile lines 326–333: `grep -rn '<dialog' internal/ui/pages/*.templ | wc -l` prints 0 (ceiling 0). One comment in `erp_key_modals.templ` originally contained the literal `<dialog` and the ratchet grep counts comments too; the comment was reworded, not the check.

T075/T076 note: the Done check `go test ./internal/ui/ -run Erp -count=1` covers both tasks (one suite, same package) and passes: RUN=4 PASS=4 SKIP=0 (TestErpCreateOwnerAndOnce, TestErpOwnerMutationsAndLimits, TestErpFormRejectsUnknownScope, TestErpStaffAndPharmacyPages). The printf vet check flags direct `i18n.T` calls that pass arguments or a non-constant key; the established local-alias idiom from `internal/modules/erp/http/respond.go` (`translate := i18n.T`) is used for those call sites — no production behaviour change. S1 UI limitation recorded: the keys page always offers Pause (the integration's pause state is not read back — S1 has no tenant read method for it; posting `pause=false` resumes via `ErpIntegrationPause`, whose contract is the posted value). Ownership is read only from `authctx.Actor.IsOwner`; the parser never touches ownership fields.

T075/T076 order note (rule 12 reverse): T075's Done check compiles and runs the shared `internal/ui` Erp suite, which also exercises T076's two handlers; both were implemented before the single GREEN run, matching the test file's needs.

T080 gate notes:
- TestHandlerHealth (`internal/modules/erp/http/handler_test.go`) was flaky, not leaking: the secret is base64url and may contain `_`, but the test split the key on `_` so `parts[3]` became a short fragment; when the random secret contained an early `_`, the "logs must not contain the fragment" assertion could hit unrelated text (it failed once inside the combined `-run` run, passed alone and with the same `-run` pattern afterwards). Fixed by taking the secret from `erp.ParseKey` and asserting the absence of the whole secret and the whole bad key in the response and logs — strictly stronger than the fragment check. (The identical-looking splits in `authenticate_test.go` and the router integration test were later converted the same way — see the SEC-005 entry below.)
- The templates originally looked labels up with concatenated keys (`"erp.keys.status_"+status`, `"erp.keys."+scope`, ternary keys) for pause/resume; `go vet` (printf) rejects non-constant keys and the i18n scan test flagged the truncated literal `"erp.keys.status_"`. Replaced with literal-key Go helpers in `erp_keys.templ` (`erpScopeLabelText`, `erpKeyStatusText`, `erpPauseResumeText`) used by all three templates; `go vet ./internal/ui/...` and the i18n scan now pass. No messages changed.
- Slice-gate results on the final code: `go build ./...` ok; `go vet` on the changed packages ok; focused host suites ok (platform{database,config,httpx}, i18n, rbac, erp/..., cmd/server with the -run pattern; `./internal/ui/ -run Erp`; `./test/ -run 'TestModulesDoNotImportEachOther|TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated'`); SB-0: 6.0 GB free on C: (above the 3 GB floor); SB-1: Docker already up, both audit containers up ~9 h (no restart, so no SB-4 was needed — migrations 255/256 verified present: erp schema has 9 tables, platform.audit_log has actor_kind/actor_key_id); SB-6 on the final code: TestTenantIsolation_ERPIntegrations, TestTenantIsolation_ERPApiKeys, TestTenantIsolation_ERPCalls, TestERPKeysRepo, TestAuditActorKeyRoundTrip — 5 RUN, 5 PASS, 0 FAIL, 0 SKIP with -v against the non-superuser `dawa24_app` role. `make` is not on the Git Bash PATH; check-file-size and check-modal-handwritten were run as the literal equivalent commands from the Makefile (0 files over 400 lines excluding `_templ.go`; 0 raw `<dialog` in pages). `go test ./internal/ui/ -run Erp -count=1 -v`: 4 RUN / 4 PASS / 0 SKIP.
- `templ.exe generate` also rewrote `internal/ui/pages/vendor_team_modals_templ.go`: it only upgrades the error metadata `FileName` from `.` to the real template path (a regeneration artifact of the installed templ v0.3.1020). Functional behaviour unchanged; committed as generated output.

Owner-directed review fixes (SEC-001..SEC-007, from the read-only review of 601a8c7..17c1405):
- SEC-001: `ListKeysAsStaff` now joins `erp.integrations` and `org.organizations` and fills `StaffKeyRow.Integration` + `OrganizationName` (field changed from `string` to `map[string]string` — the shape of `HealthInfo.OrganizationName` — because `org.organizations.name` is a bilingual JSONB object). The admin page renders the name per page language with a fallback helper. Proven by the new real-database test `TestERPKeysStaffListShowsOrgNameAndProvider` (two organizations through the Postgres repository, asserting real names and providers; connectTestDB, ran in the sandbox without skip).
- SEC-002: `AllowKey`, `FailedAuthCount` and `IncrementAuthFailure` now prepend the limiter prefix (`l.prefixed`), so the ERP counters live entirely inside `dawa24:erp:ratelimit:…`; `TestKeyMethodsUseOwnNamespace` asserts the touched Redis keys and the namespace boundary.
- SEC-003: `ERP_API_ENABLED=true` with a pepper under 32 bytes now fails startup in staging and production (only `APP_ENV=dev` keeps the committed development fallback); disabled deployments still need no pepper. `env.IsDev()` added. `TestERPConfig` gained staging-fail, staging-disabled and staging-long-pepper cases.
- SEC-004: `/erp/v1/openapi.yaml` stays public but is throttled by its own per-IP counter (`dawa24:erp:docs:<ip>`, 30/min, fails open); `docsGate` was dead wiring for the ERP surface and is deleted from `mountERP` (it remains in use by `buildUIHandler` for the website's own docs gating).
- SEC-005: the two remaining `strings.Split(full,"_")` secret mutations (`cmd/server/erp_router_integration_test.go`, `internal/modules/erp/authenticate_test.go`) now use `erp.ParseKey` and mutate the whole secret, like the already-fixed `handler_test.go`.
- SEC-006: with the ERP disabled (`SetERP(nil)`, i.e. `ERP_API_ENABLED=false`) the three integration UI registrars register nothing, and `rbac.NavSuppressions` (written once at startup) hides the `integrations` sidebar entries in every audience. `TestErpUIRoutesFollowDisabledFlag` proves route presence with the service and 404 without; `TestNavSuppressionsHideIntegrationsWhenERPIsDisabled` proves the sidebar behaviour. NavSuppressions is a deployment switch, not an authorization control.
- SEC-007: `writeError` logs the underlying error and the request id; the comment records why no key, hash or pepper can reach that line (`erp.Error.Error()` is the stable code alone; database errors are catalog messages without bound parameters; the bearer value never enters an error string in the erp module).
- `newTestRouter` (ui_test) now injects a minimal ERP fake (`sidebarErpRepo`), so `TestEverySidebarItemIsRevealedAndGatedByTheSamePermission` exercises the registered integration routes in both the holder and the without-it directions.
- Full-suite protocol note: the whole-repo `go test -p 1 ./...` in the sandbox uses the sandbox's default DSN (superuser `dawa24`). Passing `DATABASE_URL`/`TEST_DATABASE_URL` as `dawa24_app` for the WHOLE suite is not what pre-ERP full runs did: ~20 suites insert fixtures as the connection user and rely on the superuser to bypass RLS, so that override fails them with SQLSTATE 42501 — observed and rerun correctly. The dedicated SB-6 run as `dawa24_app` remains the authoritative RLS evidence (isolation tests skip for a superuser by design; rule 7 mandates the separate SB-6 path). The whole-repo runs used `timeout 2400`; focused SB-5 runs keep 400.

S2 CopyFrom deviation (recorded per rule "plan bug"): the plan's InsertBatch specifies tx.CopyFrom for erp.sync_items, but PostgreSQL refuses COPY FROM on any table with RLS enabled (SQLSTATE 0A000, feature not supported). The tenant-scoped transaction makes RLS mandatory, so CopyFrom cannot work on any tenant-owned table in this platform. The equivalent batched INSERT through unnest(...) replaces it, same rows, same transaction, same org-scoped columns. Recorded here rather than asking because the owner's SEC review already confirmed the RLS design; the plan's step was written before the RLS-vs-COPY interaction was tested.

T138 fixes (S2 flow, sandbox as dawa24_app):
- Stock writes ran with organization_id 0 (`inventoryStockWriter` set `OrganizationID: 0`), so RLS rejected every stock insert and each valid row failed with ERP_ITEM_WRITE_FAILED. Fixed: `erp.StockWrite` now carries `OrganizationID` (mirrors `inventory.Stock`), `writeResolved` fills it from the chunk, and the adapter passes it through. The adapter's catalog and stock failure reasons are now logged (`logItemWriteFailure`) instead of discarded.
- Superseded duplicate rows overwrote their external code with `ERP_ITEM_SUPERSEDED` (`sync_intake.go`); the earlier row now keeps its code and gets `ERP_ITEM_SUPERSEDED` only in its stored error code.
- Test fixture order fix (`TestERPSyncFlow_TwoBatchesInOrder`): the test accepted batch B before batch A, then asserted A had the lower id and that B waits for A. The two accept calls are swapped so acceptance order matches the test's stated intent (research D-09). No assertion changed.
- Removed the temporary `DBG` t.Logf lines from `test/integration/erp_sync_flow_test.go`.
V8a result: accept 1.08 s, process 33.4 s (TestERPSyncFlow_Scale50000, sandbox as dawa24_app, 50,000 items, 20 chunks, one worker; SC-002 window is 5 s and 7 min)

Rule 11 (naming slip): T104's Done and Do name `internal/modules/erp/postgres/variants_lookup.go`, but the file is `internal/modules/erp/postgres/sync_variants_lookup.go` (the same prefix as the other S2 SQL files). Done was checked against the real path; the task text was not changed.

T131 (S2 wiring): `cmd/server/erp_wiring.go` now fills `erp.Ports` with the existing catalog, inventory and matching services through `internal/composition/erpports` (catalog cache and matching wired as `routes.go` does). The `Enqueuer` is `erpEnqueuer`: it builds `queue.NewInsertOnly` on first use under a mutex and does not cache a failed build. Done: `go build ./cmd/server/` and `go test ./cmd/server/ -run Router -count=1` pass on the host.

Checkpoint (S3 in progress, not committed): S2 committed as 896dc33. Done after it: T142 and T143 (cursor, `internal/modules/erp/http/cursor.go`, both host tests pass). Open S3 work: T140, T141, T144 to T155. Not started: S4 to S8 and the polish phase. The S2 gate tick (T139) is in the working tree.

S3 design notes (order feeds, written before the S3 code):
- Purchase lines carry `externalProductCode` only when a link exists for the key's integration; E7 takes no integration id, so purchase lines return null (plan allows null; variantId identifies the line).
- `ListSalesShipmentsAsSystem` and `GetSalesShipmentAsSystem` run the header, customer and branch read in one E2 `AsSystem` statement guarded by `s.organization_id = $1`; the lines read is a second `WithTenant(supplier)` statement. The customer code read (`org.user_organizations`) has no RLS policy, so `resolveCustomerCodes` runs inside that transaction without AsSystem.
- Orders list services receive `limit+1` from the repository and trim; the next cursor is the last returned id (`s.id` for sales, `o.id` for purchase).
- `importState` is computed in SQL from `erp.order_imports` (no row = new, same acked status = imported, otherwise changed).

T154 first sandbox run (S3), recorded per the rule to log before changing code:
- `TestERPOrders_Paging5000`: `paged 5020 original orders, want 5000`. Test bug: the orders inserted mid-paging share the prefix (`<prefix>x<i>-`), so the prefix check counted them as originals. Fixed in the test: an original has only digits after the prefix. Product code unchanged.
- `TestERPOrders_NewAckImportedChanged`: `after re-ack state = "changed", want imported`. Test bug: the re-ack reused the first call's `AckedStatus` ("pending"), while the service always acknowledges the shipment's current status. Fixed in the test: the re-ack carries the current status ("processing"). Product code unchanged.
- Product bug found by the first run: `import_state` had no alias in the E2 and purchase header statements (`column q.import_state does not exist`, SQLSTATE 42703). Fixed with `AS import_state` in `postgres/orders_sales.go` and `postgres/orders_purchase.go`.

S3 deviations and observations (slice gate T155):
- DEVIATION (T145 field list): `OrderImport` gained `OrganizationID`. RLS needs the real tenant on every write, and the listed fields had none, so the repository could not write. The service fills it from the principal.
- Purchase lines carry `externalProductCode` only when the repository finds a link (the purchase path has no integration link for a supplier's variant, so it is null). `variantId` identifies the line (research D-21).
- `order_imports` joins: sales by `(organization, shipment)`, purchase by `(organization, order)`; the integration id filters product links on sales lines and the purchase list. Each organization has one integration per provider, so the lateral join returns one import row.
- `org.user_organizations` has no RLS (migration 138 never enables it), so `resolveCustomerCodes` runs inside the E2 transaction without `AsSystem`.
- `from`/`to` filter the order's `created_at`; `createdAt`/`updatedAt` on sales rows are the order's timestamps.
- Test names: the HTTP order tests are named `TestOrders*` so that the T151 and T155 `-run Orders` checks execute them (a naming change only, no assertion changed).
- Ratchet observation (not an S3 change): `TestDeadcodeRatchet` reports 14 against a ceiling of 12 on this branch. The extra entries are S2 code already in HEAD (`NewItemRow.messageJSON`, `anyKeys`, `gatewayOffMatcher.Prepare`, `prepareOnlyMatcher.Prepare`, `syncFixture.linkedVariantID`). Not fixed here; it needs an owner decision on the S2 helpers or the ceiling.
- Observation for the owner (S2 behaviour, not changed): `GET /products/sync/{syncId}/items` returns a plain decimal id as `nextCursor` (`encodeItemCursor`), while the orders feeds use the `v1:` cursor from `contracts/openapi.yaml`. Aligning them would change an API response, so it is left for a decision.
- Gate: `go build ./...` ok; `go vet` on `internal/modules/erp/...` and `test/integration` ok; `go test ./internal/modules/erp/... -count=1` ok; `gofmt -l` on changed files prints nothing; `go test ./test/ -run TestModulesDoNotImportEachOther` ok; SB-6 `TestERPOrders_*` (6) and `TestTenantIsolation_ERPOrderImports` PASS with -v and no skip (sandbox as dawa24_app). File-size check skipped by owner override.

S4 (T156 onward):
- T156 RED: `test/integration/erp_decide_test.go` written (gofmt clean). `go test ./test/integration/ -run ERPDecide` fails to build with undefined `erp.ShipmentDecision`, `erp.ShipmentDecisionResult`, `erp.Ports.Decider`, `Service.Decide`, `UIHandler.ErpDecideShipment` (the intended RED). The tests use the real `ui.NewUIHandler` as the decider (website rules, wallet charges and history), with `commerce`, `billing`, `org`, `catalog`, `inventory` and `notifications` built from their Postgres repositories; identity, ingest, promo, admin, chat, workflow, hr and attachments are nil.
V6 before: ok. `go test ./internal/ui/ -run 'Vendor.*Order|Shipment' -count=1 -v`: 8 `--- PASS` lines (subtests included), 0 FAIL, 1 SKIP.
- Design concern (S4, to decide before T163): trap 15 and research D (order decisions write `WriteAudit` in the same transaction as the change) cannot hold with the planned port. `ShipmentDecision` carries no key id, and the change commits inside `commerce.TransitionShipmentStatusWithWallet`'s own transaction, which accepts no hook. An erp-owned audit row would therefore be written after the change, in a second transaction.

T158 / T158a / T158b done:
V6 after: ok, same counts as V6 before (8 PASS lines, 0 FAIL, 1 SKIP) after the split.
- T158 detail: `transitionVendorShipment` is now a wrapper that resolves `vendorName` with `resolveOrgName` before the call, where the old body resolved it after a successful transition. The lookup is read-only and nothing else moved; behaviour is unchanged.
- T158a RED: `internal/ui/customer_checkout_wallet_shortfall_test.go` (`package ui`, the typed error is unexported) fails to build with `undefined: walletShortfallError`.
- T158b GREEN: `walletShortfallError` (value type, `Available`, `Required`, `msg`) returned at the shortfall branch with the same message text. `go test ./internal/ui/ -run 'WalletShortfall|Wallet|Checkout' -count=1` passes.

S4 stop (owner decision needed before T159 to T163):
1. AUDIT ATOMICITY (security). Trap 15 and research D say the order decision's `erp` audit row is written in the same transaction as the change. The planned port cannot do that: `ShipmentDecision` carries no key id, and the status change commits inside `commerce.TransitionShipmentStatusWithWallet`'s own transaction, which takes no hook. An audit row written by `erp` after the decider returns is a second transaction, so a failed audit write leaves an unaudited change. Options: (a) accept the second transaction and log the failure; (b) add an optional audit hook to `commerce.TransitionShipmentStatusWithWallet` that runs inside its transaction (touches the commerce module); (c) something else. Stopped here; T163 cannot be written without this choice.
2. BUYER NOTICE VS HISTORY NOTE. `transitionVendorShipmentBy` takes one `notes` argument, which is both the status-history note and the text that `notifyOrderStatusChanged` puts into the buyer's reason. The plan needs history `ERP: <reason>` and buyer reason `<reason>` only. The fix is one more argument on the private helper (the buyer notice text), which keeps website behaviour unchanged. Not applied yet; it needs the owner's OK because it changes the T158 signature.

S4 owner decisions (2026-11-08, answering the T161/T163 blockers):
- Audit atomicity (T163): add an optional in-transaction hook to commerce's shipment transition (e.g. a `func(ctx context.Context, tx pgx.Tx) error` option/argument run inside commerce's own transaction before commit). The erp module passes a hook that writes the order-decision audit row, so audit and state change commit or roll back together (trap 15, research D). Commerce never imports erp; the composition root adapts. Existing callers pass nil and keep their behaviour.
- Buyer notice vs history note (T161): add a separate buyer-note argument to `transitionVendorShipmentBy`; history stores "ERP: <reason>", the buyer notice carries only the reason. Existing callers pass the same value for both, so their behaviour is unchanged.
S4 implementation (T159 to T170), recorded after the gate:
- Owner decision 1 (T163) implemented: `commerce.ShipmentTxHook` (`func(ctx, pgx.Tx) error`) is a new last parameter of `TransitionShipmentStatusWithWallet` and of the repository's `UpdateShipmentStatusWithWallet`. It runs after the status, history and wallet settlement, inside the transition's transaction, and an error rolls all of it back. Existing callers pass nil (`admin_order_actions.go`, the wallet tests). Rollback is proven by `TestCommerceShipmentHook_FailureRollsBackStatus`; a committing hook by `TestCommerceShipmentHook_SuccessCommitsWithChange`. The erp audit writer is `erp.AuditTxHook` (`internal/modules/erp/audit_tx.go`).
- Owner decision 2 (T161) implemented: `transitionVendorShipmentBy` takes `notes, buyerNote`. The website wrapper passes the same text for both. The history note is `"ERP: " + reason`; the buyer notice gets the reason alone. Decision for owner: with no reason the history note is the literal `"ERP: "` (the spec text), not empty.
- Deviation (T160, T162): `erp.ShipmentDecision` gained `Audit AuditRecord`. The service builds the record and the UI decider turns it into the in-transaction hook. Without the field the service test cannot assert the audit row, and the service does not own the transaction.
- Deviation (T161): `ErpDecideShipment` sets `database.WithTenant(ctx, d.OrganizationID)` first. The ERP request carries no tenant and `commerce.GetShipment` requires one. The website path is unchanged.
- RED note (T159, T162, T163): the test files were written before the code they test. The RED run was not executed as a separate step; the first run was after the code existed. Recorded here rather than hidden.
- Test fixture fixes (not product code). Each was found by logging the underlying error before changing anything:
  (a) `seedDecideFixture` passed warehouse 0 and branch 0 to `seedERPKey`, which violates `api_keys_warehouse_id_fkey`. It now creates a branch and a warehouse as `erp_sync_flow_test.go` does.
  (b) The test principal omitted `OrgType`. Production principals always carry it, and the decide contract is supplier-only, so it failed `ERP_CAPABILITY_REQUIRED`. `OrgType: "vendor"` added.
  (c) The decide test's commerce repository lacked `WithWalletRefunder` and `WithWalletPayer`, which `cmd/server/routes_ui_builder.go` wires. A wallet charge failed "wallet payments are not configured" and surfaced as `ERP_UNAVAILABLE`. Wired as production does.
- Trailing `nil` hook argument added to six existing `TransitionShipmentStatusWithWallet` call sites (signature change only, no assertion changed).
- Line endings: `internal/modules/commerce/fulfilment.go` and `api_cancel.go` were CRLF in the working copy; `gofmt -l` flagged them. Normalised to LF (`.gitattributes` stores LF; `git diff --stat` shows only the real change).
- Wiring: `lateERPDecider` (`cmd/server/erp_late.go`) held in `deps.erpDecide` (created in `routes.go` beside `deps.orders`), set in `erpPorts`, bound after `buildUIHandler`. A call before the bind returns `ERP_UNAVAILABLE`.
- SB-6 ERPDecide and CommerceShipmentHook (sandbox, `dawa24_app`, -v, no skip): PASS=9 FAIL=0 SKIP=0 (7 ERPDecide, 2 hook).
- V6 after (re-run at the gate): `go test ./internal/ui/ -run 'Vendor.*Order|Shipment' -count=1 -v`: 8 PASS, 0 FAIL, 1 SKIP, identical to V6 before.
- Gate: `go build ./...` ok; `go vet` on commerce, erp, internal/ui, cmd/server and test/integration ok; gofmt clean on the changed files; `go test ./internal/ui/ -run 'ErpDecide|Vendor.*Order|Shipment'` ok; `go test ./internal/modules/erp/... -run Decide` ok; `go test ./internal/modules/commerce/...` ok; `go test ./test/ -run 'TestDeadcodeRatchet|TestModulesDoNotImportEachOther|TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated'` ok; `go test ./cmd/server/ -run Router` ok. File-size check skipped (owner override).

S5 (T171 onward):
- T171 RED: `test/integration/erp_customer_code_test.go` written (gofmt clean). `go vet ./test/integration/` fails with `svc.SuppliersUsingERP undefined`, the intended RED. The package stays uncompilable until T174 and T175 land.
- T172 done: migration 257 `org.idx_user_orgs_vendor_number` (non-unique, `lower(organization_number)`, `organization_number <> ''`, Arabic `COMMENT ON INDEX`) and its down file. SB-2, SB-3 and SB-4 applied it with no error; `pg_indexes` shows the index. `go test ./test/ -run 'TestRepositorySQLMatchesMigrations|TestWriteSQLMatchesMigrations' -count=1` passes.
- T173 RED: `internal/modules/erp/customer_code_test.go` written (gofmt clean). `go test ./internal/modules/erp/ -run CustomerCode -count=1` fails with `repo.setERP undefined`, `repo.addLink undefined` and `svc.SuppliersUsingERP undefined`, the intended RED.

S5 BLOCKER (stop at T174 and T175; owner decision needed):
- What is needed: `SuppliersUsingERP` must know whether a supplier has an active integration with an active, unexpired key. Those rows are in `erp.integrations` and `erp.api_keys`, and both use `platform.tenant_visible(organization_id)` (`db/migrations/255_erp_schema.up.sql:18` and `:61`; function at `001_foundation.up.sql:73-80`).
- Why it fails: the checkout (T183 to T188) and the guard called by the customer writers (T177, T181) run under the buyer's tenant. Under that tenant the supplier's rows are invisible, so the query returns no rows for every supplier. The existing `TestTenantIsolation_ERPIntegrations` and `TestTenantIsolation_ERPApiKeys` assert this same behaviour (another organization reads zero rows). This is the T175 stop condition.
- Why it cannot be done inside the rules: trap 1 and the named exceptions (plan.md E1 to E7) allow no further system read. Rule 17 says an AsSystem read needs owner approval.
- Effect if left alone: the checkout modal never appears, and `GuardCustomerCode` cannot tell an ERP supplier from a plain one.
- Options:
  (a) Recommended: add named exception E8 `SupplierERPStatusAsSystem(ctx, vendorOrgIDs []int64) (map[int64]bool, error)`. It is read-only, returns one boolean per supplier and no key data, and uses the same predicates as the key authentication (`authenticate.go:33`). Add E8 to plan.md's exceptions table, with a test run as the non-superuser role.
  (b) A status column maintained on `org.organizations`. This is a schema change and a cross-module write, so it needs owner approval.
  (c) Run the guard for every supplier. This changes SC-009 behaviour and is not recommended.

Design finding for the owner (affects T177, T181 and T182):
- The guard takes `pg_advisory_xact_lock` in its own transaction and releases it when it returns. The link write (`CreateUserOrgLink`/`UpdateUserOrgLink`) then runs in a later transaction. Two customers can pass the guard at the same time and both write the same code, so the lock only serializes the check. A fix needs the check and the write in one transaction.

Observation (S3 behaviour, not changed):
- `customerConflictSQL` (`postgres/customer_code.go`) compares `trim(organization_number)` exactly, so it is case-sensitive. The guard and migration 257 compare `lower()`. A feed can therefore show `C-1` and `c-1` as two mapped customers while the guard treats them as one code. Making them consistent changes the API output (`code_conflict` for case variants), so it needs owner approval.

Stopped: T174 not started (no code written). T175 is blocked. Nothing committed for S5. Uncommitted: the S5 test files, migration 257 and the tasks.md checkboxes for T171 to T173. Resume at T174 after the owner picks an option for SuppliersUsingERP.

S5 owner decisions (2026-11-08, approved "Yes, all three"):
- D1 SuppliersUsingERP: option (a). Add named exception E8 `SupplierERPStatusAsSystem(ctx, vendorOrgIDs []int64) (map[int64]bool, error)`. It is read-only, takes the supplier IDs only, and returns one boolean per supplier with no key data. It uses the same predicates as `authenticate.go:33`: the integration is active and the key is active, not revoked and unexpired. Add E8 to plan.md's exceptions table, and prove it with a test that runs as `dawa24_app` (not a superuser).
- D2 Guard race: the customer-code check and the link write run in one transaction. The advisory lock is held across both, so two concurrent saves of the same code cannot both pass.
- D3 Case: customer codes compare case-insensitively everywhere (`lower(trim(...))`). This covers the guard, migration 257's index and `customerConflictSQL`, so the order feeds report `code_conflict` for case variants. Add a test for `C-1` against `c-1`, and record the behaviour change in `docs/modules/erp.md`.

S5 implementation (T174 to T191), run 2:
- Owner decisions applied: D1 `SupplierERPStatusAsSystem` is E8 (plan.md table; `asystem_test.go` regex widened to E1 to E8); D2 guard and write share one transaction under `pg_advisory_xact_lock` (`postgres/customer_code.go` `GuardCustomerCode`, `database.JoinTx` in `internal/platform/database/join_tx.go`); D3 codes compare `lower(trim(...))` in the guard, `customerConflictSQL` and migration 257.
- Behaviour change (D3, noted in docs/modules/erp.md): the order feed reports `code_conflict` for `C-1` against `c-1`.
- Tests added beyond the tasks: `TestERPCustomerCode_ConcurrentSavesOneWins`, `_WriteSharesGuardTransaction`, `_CaseVariantsConflict`, `_SuppliersUsingERPUnderBuyerTenant` in `test/integration/erp_customer_code_race_test.go`.
- Deviations to review:
  1. `JoinTx` is a new opt-in platform helper. The org link write (`CreateUserOrganization`) opens its own `InTx`, so without it the write cannot share the guard's transaction. Only the customer-code guard uses it.
  2. RED order: T177, T180, T183 and T185 were written after their code in this run, so no RED run was captured for them. Their GREEN results are below.
  3. T183 tests `erpMissingCodeFailure`, the helper `planCheckout` calls. `planCheckout` itself needs a live commerce service.
  4. Pre-existing vet finding from S2 (`internal/ui/pages/erp_review.templ`, commit 896dc33): a dynamic key passed to `i18n.T`. Fixed with `erpReasonLabel` in `pages/erp_review_labels.go`, same lookup order. The `pages` package is in this slice's vet set.
  5. Sandbox SB-3: the removal step (`find ... -exec rm -rf` in the volume) was refused by the safety check. I ran only the copy part (`tar -x` over the volume, no deletion). Stale-file check: no non-vendor files differ from the tree; three spot checks matched. The owner should run the clean refresh when convenient.
  6. `make` is not installed in this shell. Equivalents run: Arabic-literal scan on the changed Go files (none); no raw `<dialog>` in new templates. `check-file-size` skipped (owner override).
- Gate results: `go build ./...` ok; `go vet` on changed packages ok; `gofmt -l` clean. `go test ./internal/shared/i18n/ ./internal/modules/erp/... ./internal/modules/org/ ./internal/platform/database/ -count=1` ok. `go test ./internal/ui/ -run 'Erp|Checkout'` ok. `go test ./test/ -run 'TestDeadcodeRatchet|TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated|TestModulesDoNotImportEachOther|TestRepositorySQLMatchesMigrations|TestWriteSQLMatchesMigrations'` ok (deadcode ratchet passes).
- SB-6 (sandbox, `dawa24_app`, `-p 1 -v -run ERPCustomerCode`): PASS=10 FAIL=0 SKIP=0. Note: `TestERPCustomerCode_ConcurrentSavesOneWins` took about 10 s, the first test in the run; the result is correct. Not investigated.

S6 (T192 to T207), run 2: NOT COMMITTED (blocked at T198 to T200; see BLOCKER below).

Done:
- T192 `TestOpenAPIMatchesRoutes` (internal/modules/erp/http/spec_test.go): mounted routes and the contract agree in both directions. Two documented operations are listed as not yet mounted (`GET /offers`, `POST /orders/purchase`). Pass.
- T193 contract copies byte-identical (`cmp` clean). The served spec no longer names repo paths (`error-codes.md`, `orgasoft-samples.md`, `fields.go`) and no longer says "unverified". Wording only; no field, route or code changed.
- T194 (adapted): `cmd/cli/erp_postman_test.go` pins the committed collection to the contract: `TestPostmanCollectionShape` and `TestPostmanCollectionCoversContract` (every contract operation except GET /openapi.yaml has a request). Pass.
- T195 to T197, owner direction: the collection is generated by the owner-reviewed Python generator, copied to `docs/erp-api/oas2postman.py` (tasks.md's artifact folder). The Go generator was not written, so `erp-postman` is not dispatched in main.go. The output has the four bilingual folders the owner approved (01 Start here, 02 Products, 03 Sales orders, 04 Purchase orders), not the six of T195. The output matches the copy handed to Orgasoft except `_postman_id`. Output names were renamed to the tasks.md names (`dawa24-erp.postman_collection.json` and `dawa24-erp.postman_environment.json`). The environment is "Dawa24 ERP - test server" with placeholders only, as the generator defines it.
- T201 to T203 `docs/erp-api/README.md`: Arabic first, then English; per-endpoint sections; field aliases; the 58-row error table generated from `error-codes.md`.
- T204 `docs/modules/erp.md`: entities, customer codes, E1 to E8, JoinTx, V5 list, Postman/OpenAPI.
- T205 `AGENTS.md`: `erp` added to the schema line.
- T206 quickstart section 4 and the section 5 table now cite the collection's real folder and request names. Steps 9, 11 and 12 reuse an earlier request with a different query or body.

BLOCKER (T198 to T200, erp-demo; owner decision needed):
- What: the `erp-demo` command, its tests `TestErpDemoRefusesProd` and `TestErpDemoCountFlag`, and the sandbox seed run.
- Why stopped: the task seeds a catalog variant (SKU 100245), a warehouse, a branch, an integration, a key and a pending link. That needs writes through catalog, inventory and org services from cmd/cli, a larger change than the rest of S6. It is also a scope question, so I did not start it.
- Options:
  (a) Build it as written. About 250 lines in cmd/cli plus tests and a sandbox run.
  (b) Drop T198 to T200 from S6. The journey does not need the seed: step 2's first send creates SKU 100245, and the checkout modal creates the pending link (S5 `CheckoutErpCodeSubmit`). Keep the website key path in quickstart section 3. Update quickstart section 5 step 2 to say "created by the first send".
  (c) Move erp-demo to S7.
- Recommendation: (b). It removes a seed path that writes through three modules, and the journey still works without it.
- Gate effect: S6 gate (T207) is not run, and S6 is not committed. The working tree holds the S6 changes for review. Nothing pushed.

S6 owner decision (2026-11-08): option (b). T198 to T200 (`erp-demo`) are dropped. Keys come from the website (quickstart section 3). The first product send creates SKU 100245. The checkout modal creates the pending customer link. The V8 load test makes its 10 keys in the website. T195 to T197 are done in adapted form: the owner-reviewed Python generator lives at docs/erp-api/oas2postman.py, with Go tests in cmd/cli/erp_postman_test.go. Opus also removed the three "unverified" field notes left in the served spec, keeping both copies identical.

S7 (T208 to T230), run 3: built and committed at the slice gate.

Sandbox (this run): SB-0 `df -h /c` 8.4 GB free. SB-1 containers up. SB-2 Redis save and stop-writes settings applied. SB-3 official refresh ran with exit 0 (the removal step was not refused this time). SB-4 migrate applied 258 `258_order_number_sequence`. SB-6 `ALTER ROLE dawa24_app` applied once, no Postgres restart since.

Gate results:
- `go build ./...` ok; `go vet` on `./internal/modules/commerce/... ./internal/modules/erp/... ./cmd/server/ ./cmd/cli/` ok; `gofmt -l` on the touched packages prints nothing.
- Focused: `go test ./internal/modules/commerce/... ./internal/modules/erp/... ./cmd/server/ ./cmd/cli/ -run 'OrderNumber|Purchase|ErpPurchasePlacer|OpenAPIMatchesRoutes|Postman|Router|Offers' -count=1` ok. Full `go test ./internal/modules/erp/... -count=1` ok (after the citation fix below, `TestAsSystemOnlyInNamedFunctions` passes).
- Structural: `go test ./test/ -run 'TestDeadcodeRatchet|TestModulesDoNotImportEachOther|TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated|TestRepositorySQLMatchesMigrations|TestWriteSQLMatchesMigrations|TestAsSystem'` ok.
- SB-6 (dawa24_app, -v, no skips): `TestOrderNumberSequenceConcurrent`, `TestERPPO_CreateAndRepeat100`, `_WrongPriceRefused`, `_NoActiveOffer`, `_SupplierNotLinked`, `_WalletRefused`, `_CrashRepair`, `_SelfSupply`, `TestTenantIsolation_ERPIdempotency`: PASS=9 FAIL=0 SKIP=0.
- `go test ./cmd/cli/ -run Postman` ok. Contract copies `cmp` identical.

RED record and order deviations:
- T211 (allocator unit tests): RED captured, build failed on the missing `nextOrderNumber`. GREEN after T212.
- T220 (purchase service tests): RED captured, build failed on the missing `CreatePurchaseOrder` and `Offers`. GREEN after T221.
- T225 (handler tests): RED captured, 404 and 405 because the routes were not mounted. GREEN after T226.
- T208 (`erp_po_test.go`): written before the ERP purchase code, but its RED run was not captured in a separate step; its first run was the GREEN run above.
- T209 (`commerce_order_number_test.go`): written after the allocator (T212 to T214) in this run, so RED was not captured. The test asserts that `commerce.order_number_seq` advanced by at least orders-1 during the run, which the in-process counter cannot satisfy. Recorded as an order deviation (rule 12 and rule 5).
- T222 (placer tests): the test file and the placer were written together, so RED was not captured. GREEN run above.

Deviations from the names in the task text (all additive, same behaviour as the task describes):
1. `VariantOfferRow` carries `SupplierName`, `Active` (live variant, approved vendor) and `InStock`. `Offer` carries `UnitPrice`. The purchase rules need the variant's status and the supplier name, which the task's field list did not hold.
2. `IdemState` carries `Created` (the caller owns a new row) and `RequestSHA`. A different body under the same reference is `ERP_IDEMPOTENCY_CONFLICT`; without the stored fingerprint the service cannot tell.
3. `FinishIdempotency` takes the `AuditRecord` as its last argument, so the audit row and the finished row commit in one transaction (trap 15). An empty action skips the audit (used by the stale repair, which creates no new order).
4. `PurchaseRequest` carries `BranchID` (the key's receiving branch, needed by the quote) and `BuyerOrgType` (the checkout's customer type). `PurchaseResult` carries `Row PurchaseRow` instead of raw `Response`. The stored response is the JSON of the internal `PurchaseRow`; a replay decodes it and renders it with the same `purchaseJSON` as GET, so the reply shape is one code path.
5. The fake named `fakePurchase` in the task clashed with the existing row type in `fake_orders_test.go`; it is `fakePurchaseRepo`.
6. `Repository` embeds `PurchaseRepository`; `PurchasePlacer` is a port in `ports.go`; the adapter `erpPurchasePlacer` is in `cmd/server/erp_purchase_placer.go` and is bound late (`lateERPPurchasePlacer`, the S4 pattern), because billing and commerce are built after the ERP routes.
7. The price is checked twice: the service compares the sent `unitPrice` with the offer price (the E6 row, `offerUnitPrice`, the website's effective price), so a wrong price writes no order even in an integration test with a placer double; the placer compares again with the website quote and maps a checkout price change to `ERP_PO_PRICE_MISMATCH`.
8. The integration tests use a placer double that writes the order row from `commerce.order_number_seq`. The real checkout is exercised by `TestOrderNumberSequenceConcurrent` (`CheckoutAPI` with an open availability probe and an identity quote) and by the placer unit tests.
9. `stale repair` deletes the stale row when no order carries the marker, then places again. A refusal (any `*Error` below 500) deletes the row; an internal error keeps it in progress, which is the crash case.
10. The marker lookup uses `starts_with(notes, marker)` rather than `notes LIKE '[ERP:%'`, so a reference containing `%` or `_` cannot match another order.
11. `TestPostmanCollectionShape` expected two "Coming soon" requests in the purchase folder (S6 comment: "until S7"). S7 mounts both, so it now expects zero. The generator dropped the marks, per the owner's instruction.
12. Postman error examples: `docs/erp-api/oas2postman.py` has real examples for create purchase order (price, supplier link, no offer, refused reason, idempotency conflict, in progress) and for offers (scope, barcode). Regenerated and copied to the committed names.
13. `TestAsSystemOnlyInNamedFunctions` rejected the `VariantOfferAsSystem` comment because the "plan.md E6" citation wrapped onto two lines. Fixed the comment wording; no behaviour change.

Migration 258 (`db/migrations/258_order_number_sequence.up.sql`, `.down.sql`): sequence start at MAX(numeric order number)+1000, grant to `dawa24_app` in the role block, Arabic comment. Expand-only; rollback to the old image is safe.

Not done in this run: SB-5 for T230 is covered by the SB-6 run above (the stricter form). A comment-only edit to `offers.go` came after the SB-6 run and was checked on the host only.
