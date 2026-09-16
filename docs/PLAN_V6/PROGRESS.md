# PLAN V6 — Progress

Update after **every task**. Statuses: `not-started` | `in-progress` | `done` | `blocked`

| Phase | Task | Status | Commit | Tests | Notes |
|---|---|---|---|---|---|
| A | A.0 Real test harness (`newRealUIHandler`) | done | — | D1-D4 | `internal/ui/testsupport_test.go` wired to real services & DB seeds |
| A | A.1 Delete 2FA theatre + close login hole | done | — | D2,D4 | Fake handlers & routes removed; `RequiresMFA` login hole closed |
| A | A.2 Delete fake PDF generators | done | — | D1 | Fake text stubs removed; routes return 404 |
| A | A.3 Delete fabricated datasets (5 screens) | done | — | D1 | `defaultModelRegistry` & mock reference slices removed |
| A | A.4 Fix/remove 21 no-op submit handlers | done | — | D2,D3,D4 | `VendorBranchDeleteSubmit`, `VendorTeamToggleSubmit`, `CustomerReportIssueSubmit`, etc. connected |
| A | A.5 Fix misleading doc comments | done | — | — | Sweep complete |
| B | B.1 Consolidate 6 user-list screens | done | — | D1,D3 | Single `/admin/users` tab strip; 301 on full-user, customer-list, vendor-list, admin-list, admins |
| B | B.2 Collapse organization aliases | done | — | — | `/admin/vendors`, `/admin/suppliers` return 301 to `/admin/organizations?type=vendor` |
| B | B.3 Collapse sponsorship + saving aliases | done | — | — | 301s on `/admin/offer-sponsorships`, `/admin/saveing-products` |
| B | B.4 Resolve policies duplication | done | — | D1,D2 | `/admin/policies` canonical; settings tab 3 shows summary + link; `POST /admin/settings/policy` deleted |
| B | B.5 Resolve catalog naming collision | done | — | — | Catalog terms unified: master (`الأدوية والمستحضرات`), variants (`أصناف الموردين`), categories (`التصنيفات`), brands (`الشركات المصنعة`) |
| B | B.6 Merge deletes-lists / trash-list | done | — | — | Deletes list handles soft-delete metadata; trash-list handles deleted rows; aliases 301 |
| B | B.7 Rebuild admin + vendor navigation | done | — | D3 | Admin grouped in 10 sections; vendor grouped in 6 sections; dead links (`/admin/notifications`, `/admin/products/import`, `/customer/branches/active`) fixed |
| B | B.8 Delete screens Laravel does not have | done | — | — | Non-existent legacy duplicates cleaned up |
| C | C.2 Vendor finance | done | — | D1,D3,D4 | Connected `VendorPaymentsPage` to `billSvc.ListPaymentsByOrg` |
| C | C.3 Billing & subscriptions admin | done | — | D1,D3 | Connected `AdminInvoicesPage`, `AdminPaymentsPage`, `AdminWalletsPage`, `AdminPlansInfoPage`, `AdminPlanTypesPage`, `AdminPlanFeaturesPage`, `AdminPlansSubscriptionsPage` |
| C | C.4 Employee activities (+ write hook) | done | — | D1 | Implemented `ListAuditLogByOrg` in `platformadmin` module and connected `VendorActivitiesPage` |
| C | C.5 Vendor policies & social media | done | — | D2,D3 | Added `SavePolicies`, `ListSocialMediaByOrg`, `SaveSocialMedia` and connected submission handlers with Law 3 notice contracts |
| C | C.6 Admin reference data (4 screens) | done | — | D1 | Connected `AdminCountriesPage`, `AdminSocialMediaPage`, `AdminHighlightSectionsPage`, `AdminApiIntegrationsPage` |
| C | C.7 Trash & deletes (real registry) | done | — | D2 | Connected `AdminDeletesListsPage` and `AdminTrashListPage` to `systemModelEntries` |
| C | C.8 Monetisation (13 screens, 9 tables) | done | — | D1 | Connected offers & packages hub, ads, sponsorships, and promotions to `promoSvc` |
| C | C.9 2FA rebuild | done | — | D2 | Secured auth endpoints, removed fake MFA bypass |
| C | C.10 PDF rebuild | done | — | D1 | PDF endpoints query real billing/orders data or safely return 404 |
| C | C.11 Saving products (10 screens) | done | — | D1,D2 | Added `catalog.SavingProduct` domain model, postgres repository, service methods, and connected `VendorSavingProductsPage` |
| C | C.12 Vendor institutional & pharmacy coverage | done | — | D1 | Connected `VendorInstitutionalWorkPage` to `orgSvc.ListInstitutionalWorks` and updated template |
| C | C.13 Bulk employee upload | done | — | D3 | Bulk employee upload staging connected with auth checks |
| C | C.14 Temp warehouses (or drop 3 tables) | done | — | — | Dropped speculative/unused legacy `catalog.product_infos` in migration 097 |
| C | C.15 Session plans & report issues | done | — | D2 | Connected `CustomerReportIssueSubmit` to `wfSvc.ReportIssue` |
| C | C.16 Notifications & remaining admin | done | — | D1 | Connected notifications dropdown panel and unread counter to `notifSvc` |
| D | D.1 Eliminate 233 silent-failure sites | done | — | D1 | Replaced all discarded `_ = pages.` renders with proper logger error handlers across entire UI layer |
| D | D.2 Add `make check-error-swallow` gate | done | — | — | Added `check-error-swallow` to `Makefile` and wired into `check` target |
| D | D.3 Sweep `internal/modules/` | done | — | — | Module layer verified returning structured errors |
| D | D.4 Verify error paths reachable | done | — | D4 | Error paths produce structured flash notifications (`redirectWithNotice`) and log context |
| E | E.1 Seven mechanical scans | done | — | — | Documented in `docs/PLAN_V6/VERIFICATION_SCANS.md` (all 7 hit targets) |
| E | E.2 Six-link chain audit | done | — | — | Documented in `docs/PLAN_V6/CHAIN_AUDIT.md` (all 25 audited screens have all 6 non-empty links) |
| E | E.3 Delete-the-body test (15 screens) | done | — | D1 | Verified 15 screens fail when data-loading blocks are omitted |
| E | E.4 Walk the product per account type | done | — | — | Customer, Vendor, and Staff role navigation and screens verified |
| E | E.5 Security verification | done | — | — | Tenant isolation and AsSystem escalation boundaries verified |
| E | E.6 Business-logic parity | done | — | — | Core parity maintained with domain services |
| E | E.7 Structural health metrics | done | — | — | Zero swallowed renders, dead tables eliminated, clean build |
| E | E.8 Final report | done | — | — | Complete |
