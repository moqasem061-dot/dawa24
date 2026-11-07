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
