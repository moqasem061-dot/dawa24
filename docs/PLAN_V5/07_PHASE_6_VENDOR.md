# PHASE 6 — Vendor Dashboard Completion

**Depends on:** Phase 0 (coverage), Phase 1 (institutional), Phase 2 (compare),
Phase 4 (ingest), Phase 5 (shared admin CRUD components).
**Tasks:** 12.

Laravel's vendor sidebar has **32 entries**; Go's has 17. This phase closes the
15-entry gap and fixes the partial ones.

The authoritative sidebar with positions is in `00_MASTER.md` §0.7.3.
**Reproduce that order.**

---

## TASK 6.1 — Vendor warehouses (position 14)

| Route | Laravel |
|---|---|
| `/vendor/warehouses` | `Employee/Warehouses` |
| `/vendor/warehouses/{id}` | `Employee/WarehouseDetails` |

`inventory.warehouses` and `inventory.warehouse_transfers` exist.
`/vendor/transfers` exists; the warehouse list itself does not.

Requirements: list with capacity/utilisation, create/edit/delete, per-warehouse
stock view, link to transfers. Reproduce Laravel's columns and actions.

---

## TASK 6.2 — Choose available products (position 9)

| Route | Laravel |
|---|---|
| `/vendor/catalog/select` | `Employee/VendorProducts` (`/employee/choose-products`) |

"تحديد المستحضرات المتاحة بالمستودع" per `/what-in`. The vendor picks, from the
master catalog, which products they carry. This is upstream of
`/vendor/products` (their priced list).

**Inspect the relationship** between `Employee/VendorProducts` (choose),
`VendorProductsList` (`/employee/products`), and `VendorProductShow`. Determine
what table records the selection — `catalog.product_variants`? a join table? —
and record it.

Requirements: searchable master-catalog browser with category/brand filters,
bulk select, "add selected to my catalog", already-selected indication,
pagination matching Laravel's page size.

---

## TASK 6.3 — Vendor saving products (position 12)

| Route | Laravel |
|---|---|
| `/vendor/saving-products` | `Employee/SavingProducts` |
| `/vendor/saving-products/import` | `Employee/SavingProductsImport` |
| `/vendor/saving-products/{id}` | `Employee/SavingProductShow` |

Table reinstated in Phase 0 Task 0.6.1. Import goes through Phase 4's pipeline.
Keep the `/saveing-products` misspelling as a 301 alias.

---

## TASK 6.4 — Vendor payments (position 23)

| Route | Laravel |
|---|---|
| `/vendor/payments` | `Employee/EmployeePayments` |

`billing.payments` and `billing.payment_histories` exist (`payment_histories` is
one of the 21 dead tables). Build the list with Laravel's filters and status
badges. Money via `money.Amount`, T8 exact assertions.

---

## TASK 6.5 — Vendor earnings

| Route | Laravel |
|---|---|
| `/vendor/earnings/order` | `Employee/EarningsOrder` |
| `/vendor/earnings/offers` | `Employee/EarningsOffers` |

Revenue reporting. **The calculation is a business rule** — read the components
and reproduce it exactly, to the minor unit. Same calculation as the admin
earnings screens (Phase 5 Task 5.10); implement it **once** in the service layer
and call it from both.

---

## TASK 6.6 — Vendor activities (position 31)

| Route | Laravel |
|---|---|
| `/vendor/activities` | `Employee/Activities` |

Depends on `org.employee_activities` (added in Phase 5 Task 5.3) and the
observer equivalent. The vendor sees their own organization's activity only —
tenant-scoped, **not** `AsSystem`. Write the T3 test.

---

## TASK 6.7 — Vendor policies & social media (positions 28, 29)

| Route | Laravel | Table |
|---|---|---|
| `/vendor/policies` | `Employee/Policies` | `org.organization_policies` |
| `/vendor/social-media` | `Employee/SocialMedia` | `org.organization_social_media` |

Both tables exist. Policies: shipping, return, payment, cancellation — read the
Laravel component for the exact policy types and whether they are free text or
structured. These surface on the public supplier profile, so verify
`/suppliers/{id}` renders them after this task.

---

## TASK 6.8 — Bulk employee upload (position 6)

| Route | Laravel |
|---|---|
| `/vendor/team/import` | `Employee/FirstTimeUploadUsers` |
| `/vendor/team/fast-add` | `Employee/EmployeeFastAdd` |

Goes through Phase 4's pipeline. **Security-sensitive**: bulk-creating user
accounts. Requirements:
- each created user gets a real identity record through the identity service —
  never a direct insert
- no plaintext passwords in the sheet; generate and send an invite
- duplicate email handling defined and tested
- the importing vendor cannot create users in another organization (T3)
- rate/size cap

---

## TASK 6.9 — Vendor employee detail screens

Laravel has more than Go: `/employee/users/{id}`, `/{id}/info`,
`/users/create`, `/users/edit/{id}`. Go has `/settings/employees` with
create/delete/toggle only.

Add: employee detail, edit, per-employee activity, branch assignment
(the branch-manager assignment already works after AUDIT_V3's `AsSystem` fix —
**verify it, do not rebuild it**).

---

## TASK 6.10 — Vendor institutional work

| Route | Laravel |
|---|---|
| `/vendor/institutional-work` | `Employee/EmployeeInstitutional` |

Service methods land in Phase 1 Task 1.1.7. This task builds the vendor screen:
which institutional works the organization belongs to, which employees hold
which works, and the request/assign flow.

---

## TASK 6.11 — Pharmacy coverage (position 8)

| Route | Laravel |
|---|---|
| `/vendor/pharmacy-coverage` | `Employee/PharmacyEmployeeWeeklyCoverage` |
| `/vendor/pharmacy-coverage/{id}` | `Employee/PharmacyEmployeeShowWeeklyCoverage` |

**Inspect what this is** — it is coverage viewed from the pharmacy side, listed
under the vendor panel. Likely: which pharmacies fall inside this vendor's
coverage on a given day. Read the component before building; do not guess.

---

## TASK 6.12 — Offer orders (position 21)

| Route | Laravel |
|---|---|
| `/vendor/orders/offers` | `Employee/OfferOrders` |
| `/vendor/orders/offers/{id}` | `Employee/ShowOfferOrder` |

Laravel separates general orders from offer-based orders. Go collapsed the two
order systems (audit §3.3). **Determine whether a separate screen is still
meaningful** given the collapsed model — if `commerce.orders.offer_id IS NOT NULL`
is the whole distinction, this is a filtered view of `/vendor/orders`, and
should be implemented as such with the same Arabic label. Record the decision.

---

## PHASE 6 COMPLETION GATE

```bash
make check && go test ./... -race
```

- [ ] All 32 vendor sidebar entries present, in Laravel's order, with Laravel's Arabic labels
- [ ] `inventory.warehouses`, `billing.payment_histories`, `org.branch_institutional_works` are no longer dead tables
- [ ] The earnings calculation is implemented once and shared with admin
- [ ] Bulk employee upload cannot create users outside the vendor's org (T3)
- [ ] Every vendor screen is tenant-scoped, not `AsSystem` (audit each new query)
- [ ] Dead-target scan = 0
- [ ] `PROGRESS.md` updated for 6.1–6.12
