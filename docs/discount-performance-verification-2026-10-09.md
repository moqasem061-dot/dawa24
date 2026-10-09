# Discount and upload verification — 2026-10-09

## Changes

- Comparison and temporary warehouse spreadsheets read raw XLSX cell values. A stored `0.25` remains `0.25` percentage points even when Excel formats it as `25%`; no fraction-to-percent conversion is applied. Existing persistence supports two decimal places, so additional decimal places still round to that precision.
- Discount comparisons select the highest discount. Net price only resolves identical discounts. Hundredth-point comparisons and displayed/filter values preserve differences such as `12.34` versus `12.35`.
- Valid discount-only rows and literal `100` can participate in discount benchmarking. Price intelligence retains its own usable-price requirements. Invalid discounts do not become fabricated `100%` values.
- Preview reads a bounded raw sample instead of loading the entire sheet. Security scanning remains enabled. Detached comparison staging has a process-wide four-worker limit, and processing timeouts start after queue acquisition. Background failures cannot restore archived file metadata.
- Admin and vendor product linking searches skip irrelevant stock-priority work. Buyer catalog sponsorships are aggregated once and joined by target. Admin import configuration loads independent category/history data concurrently.
- Saving-product import readers receive snapshots under the store lock, avoiding races with worker progress and editable rows.

## Verification

- Race-enabled short tests passed for UI, comparison, sheet reading, and file security. Comparison race tests also passed after the final archived-file failure regression was added.
- Browser session-search and UI-stability suites: all 18 tests passed, including stale-response rejection and Arabic input/focus behavior.
- `go vet ./...`, module-boundary checks, and `git diff --check` passed.
- Regression fixtures cover real XLSX percentage styling, literal fractional values, Arabic numbers, invalid values, exact net prices, discount-first winners, display precision, archived-file protection, and concurrent import-session snapshots.
- Local single-iteration preview benchmark, 10,000 rows: full-sheet path 148 ms / 79.9 MB allocated; bounded raw sample 90 ms / 45.2 MB allocated. These are local reader measurements, not production upload or page response times.

## Remaining verification limits

- PostgreSQL buyer-offer integration test was skipped because `DATABASE_URL` is unset and the Docker engine is unavailable. Query latency and execution plans on production-sized data remain unverified.
- The full short suite encounters existing CSP failures at line 17 of `internal/ui/tutorials/driver.html`, `pharmacy.html`, and `vendor.html` (styles missing a nonce).
- Repository lint reports seven existing issues in assistant buying actions, HTTP middleware tests, billing admin payment queries, organization delivery/profile service, and document serving handlers. None of those files were changed by this work.
- A final repeated full-suite run stalled during linking and was interrupted; the repeated lint run timed out while loading packages. The full-suite CSP and seven lint findings above refer to the earlier completed runs. Targeted race tests and the final `go vet` run completed successfully.
- `make` is unavailable in this Windows environment; individual available checks were run instead.
- Existing imports previously saved with transformed percentages require reprocessing from their original files; the source values cannot be recovered reliably from already transformed database values.
- Changes are local and have not been deployed. No claim of complete production performance verification is made.

## Clean-code guard

- [row_extraction.go](F:/dawa24/internal/modules/compare/row_extraction.go) — removed the obsolete duplicate numeric parser after switching to shared literal-discount parsing.
- [admin_temp_warehouse_helpers.go](F:/dawa24/internal/ui/admin_temp_warehouse_helpers.go) — removed the unused permissive parser and its imports after switching warehouse staging to the same shared parser.

clean-code-guard: 2 fixed, 0 flagged for author
