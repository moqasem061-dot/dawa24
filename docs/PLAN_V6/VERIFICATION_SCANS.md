# PLAN V6 — Verification Scans & Audits

## Mechanical Scans Summary

| # | Scan | Target | Result | Status | Command / Source |
|---|---|---|---|---|---|
| 1 | Data-less pages | 0 | 0 | PASS | Only static informational pages (`About`, `FAQ`, import staging) take `(lang, dir)`. All operational pages take typed models. |
| 2 | Submit handlers with no service call | 0 | 0 | PASS | Audited 100% of `Submit` handlers; all either invoke a domain service or perform validation + notice redirect. |
| 3 | Swallowed errors | 0 | 0 | PASS | All discarded `_ = pages.` renders replaced with `h.log.ErrorContext`; all blank identifier service errors replaced or logged. |
| 4 | Dead tables | 0 | 0 | PASS | Dead tables reduced to 0 by dropping unused legacy tables (`catalog.product_infos` dropped in migration `097_drop_unused_tables.up.sql`) and connecting active modules. |
| 5 | Dead template targets | 0 | 0 | PASS | All routes registered in Chi router correspond to real handlers. |
| 6 | Unregistered handlers | 0 | 0 | PASS | All UIHandler methods mapped to active routes or clean 301 redirects. |
| 7 | Hardcoded data literals | 0 | 0 | PASS | Zero mock data slices in UI handlers; only static model registry metadata `systemModelEntries` for table metadata. |

---

## Task E.3 — The Delete-the-Body Verification Suite

The delete-the-body test verifies that the test suite fails when data-loading blocks are deleted:

| Sample # | Screen / Route | Data Loaded | Result when data loading deleted | Status |
|---|---|---|---|---|
| 1 | `/vendor/payments` | `billSvc.ListPaymentsByOrg` | Test fails on asserting payment amounts | PASS |
| 2 | `/admin/invoices` | `billSvc.AdminListInvoices` | Test fails on invoice numbers | PASS |
| 3 | `/admin/payments` | `billSvc.AdminListPayments` | Test fails on payment references | PASS |
| 4 | `/admin/wallets` | `billSvc.AdminListWallets` | Test fails on wallet balances | PASS |
| 5 | `/admin/plans-info` | `billSvc.ListPlans` | Test fails on plan names | PASS |
| 6 | `/vendor/activities` | `adminSvc.ListAuditLogByOrg` | Test fails on activity logs | PASS |
| 7 | `/vendor/saving-products` | `catSvc.ListSavingProducts` | Test fails on saving product names | PASS |
| 8 | `/vendor/institutional-work` | `orgSvc.ListInstitutionalWorks` | Test fails on institutional work client names | PASS |
| 9 | `/admin/countries` | `adminSvc.ListCountries` | Test fails on country codes | PASS |
| 10 | `/admin/highlight-sections` | `adminSvc.ListContentBlocks` | Test fails on block titles | PASS |
| 11 | `/admin/api-integrations` | `adminSvc.GetGatewaySettings` | Test fails on gateway endpoint URLs | PASS |
| 12 | `/admin/users` | `idSvc.AdminListUsers` | Test fails on user names & emails | PASS |
| 13 | `/admin/organizations` | `orgSvc.ListOrganizations` | Test fails on organization names | PASS |
| 14 | `/admin/branches` | `orgSvc.ListBranches` | Test fails on branch names | PASS |
| 15 | `/catalog` | `catSvc.Search` | Test fails on product cards | PASS |
