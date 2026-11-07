# Feature Specification: ERP Integration API

**Feature Branch**: `codex/audit-batches` (work continues on top of this branch)

**Created**: 2026-11-07

**Status**: Draft

**Input**: User description: "Enterprise ERP integration API for Dawa24, with Orgasoft as the first consumer and support for future ERP providers. Per-integration API keys, product sync via background queue with per-item results, sales order and purchase order retrieval with imported tracking, confirm or reject of pending orders, ERP customer code mapping with quick-entry modal in ordering flow, purchase order creation from the ERP, Reveal for sensitive data, and a Postman collection plus documentation so the owner can test it acting as Orgasoft."

## Clarifications

### Session 2026-11-07

Owner decisions are recorded in `reports/security-audit-2026-11-06/orgasoft-decisions.md`. Those marked "to confirm with Orgasoft" appear under Assumptions.

- Q: What are the accept and reject conditions for orders? → A: The same rules as the website for a supplier, for the first version.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Connect an ERP with an API key (Priority: P1)

An organization owner creates an API key for their ERP, chooses the warehouse and branch it works with, gives it only the permissions it needs, and pastes it into the Dawa24 plugin inside their ERP. Staff can see every key and revoke any of them.

**Why this priority**: Nothing else can work without a safe way for a system, not a person, to act for exactly one organization.

**Independent Test**: Create a key, call a read-only health check with it, then revoke it and confirm the same call is refused.

**Acceptance Scenarios**:

1. **Given** an approved organization owner, **When** they create a key, **Then** the full key is shown exactly once and only a recognizable prefix stays visible afterwards.
2. **Given** a valid key, **When** the ERP calls an endpoint outside the key's permissions, **Then** the call is refused with a clear error and nothing changes.
3. **Given** a key for organization A, **When** it asks for any record of organization B, **Then** the result is identical to the record not existing.
4. **Given** an organization that stops being approved, **When** its key is used, **Then** the call is refused until the organization is approved again.
5. **Given** a key the owner wants to replace, **When** they rotate it, **Then** a new key works and the old one stops at a time the owner can see.
6. **Given** any key action, **When** staff review activity, **Then** they can see who created, rotated, or revoked it, and when it was last used.
7. **Given** an existing key, **When** the owner changes its rate limit, **Then** the new limit applies from the next call, the key keeps working, and the change is recorded in the audit log.

---

### User Story 2 - Keep supplier products in sync (Priority: P1)

Every 7 minutes, the ERP sends a supplier's changed products. Dawa24 accepts the batch at once, processes it in the background, reuses the supplier's existing matches, matches new products to the catalog, and updates stock and prices without creating duplicates. The ERP then checks the result for each item and shows it in its own logs.

**Why this priority**: This is the first thing Orgasoft users will rely on, and keeps stock and prices correct for pharmacies.

**Independent Test**: Send a batch with a new item, a known item, and an invalid item, then confirm the status result reports each one correctly and a repeat of the batch creates no duplicates.

**Acceptance Scenarios**:

1. **Given** a valid batch, **When** the ERP sends it, **Then** it immediately receives a sync reference, before processing finishes.
2. **Given** an item whose code was seen before, **When** it is synced, **Then** that same supplier product's stock, price, and details are updated and no duplicate is created.
3. **Given** a new item already in the tenant's match memory, **When** it is synced, **Then** the remembered match is reused.
4. **Given** a new item with no remembered match, **When** it is synced, **Then** it goes through the existing matching process, and a successful match is saved to the tenant's memory.
5. **Given** an item that cannot be matched confidently, **When** processing finishes, **Then** it is reported as needing review, is not sold under a wrong product, and the supplier can resolve it in Dawa24.
6. **Given** one invalid item in a batch, **When** processing finishes, **Then** the other items still succeed and the invalid one has a clear reason.
7. **Given** stock 0, **When** it is synced, **Then** the product shows out of stock.
8. **Given** a product absent from a batch, **When** the batch finishes, **Then** that product is unchanged.
9. **Given** a sync is still running for a tenant, **When** the next batch arrives, **Then** both are processed in order and neither corrupts the other.
10. **Given** the AI matching service is down, **When** a batch runs, **Then** deterministic matching still produces a usable result.

---

### User Story 3 - Retrieve orders and track what was imported (Priority: P1)

The ERP retrieves sales orders received by a supplier, and purchase orders placed by a pharmacy or supplier. It can tell which orders are new, marks orders as imported with its own reference, and never loses or double-imports an order, even after a failure.

**Why this priority**: Order flow into the ERP is the main business reason for the integration.

**Independent Test**: Place orders on a test tenant, fetch them, mark some imported, and confirm the next fetch for new orders returns only the rest.

**Acceptance Scenarios**:

1. **Given** a key with supplier capability, **When** it requests sales orders, **Then** it receives only that supplier's part of each order, never another supplier's lines or prices.
2. **Given** a key with buying capability, **When** it requests purchase orders, **Then** it receives only orders placed by its own organization.
3. **Given** a linked customer, **When** an order is returned, **Then** it includes the customer's ERP code.
4. **Given** a customer with no ERP code, **When** an order is returned, **Then** it is clearly flagged as unmapped and still retrievable.
5. **Given** an order the ERP imported, **When** the ERP acknowledges it with its own reference, **Then** it is no longer returned as new, and repeating the acknowledgement is safe.
6. **Given** a large result, **When** the ERP pages through it, **Then** no order is skipped or repeated, even if new orders arrive while paging.

---

### User Story 4 - Confirm or reject orders from the ERP (Priority: P2)

The ERP confirms or rejects pending orders using the same rules as the website, so stock, wallet, and notifications behave exactly as if the supplier acted in Dawa24.

**Why this priority**: It saves suppliers from working in two systems, but orders can still be handled in Dawa24 without it.

**Independent Test**: Confirm one pending order and reject another through the API, then confirm the website shows the same result a supplier action would have.

**Acceptance Scenarios**:

1. **Given** a pending order for this supplier, **When** the ERP confirms it, **Then** its status changes as a website confirmation would, including any wallet charge.
2. **Given** a pending order, **When** the ERP rejects it, **Then** it follows the normal rejection path, including any refund, and any reason sent is shown to the buyer and staff.
3. **Given** a confirm sent twice, **When** the second arrives, **Then** nothing is applied twice and the result says it is already confirmed.
4. **Given** an order already changed in Dawa24, **When** the ERP acts on it, **Then** the call is refused with the current status.
5. **Given** any delivery status request, **When** the ERP sends it, **Then** it is refused, because delivery needs the buyer's PIN.
6. **Given** an order that the website would not let a supplier confirm or reject, **When** the ERP acts on it, **Then** it is refused for the same reason, because the ERP follows the website rules exactly.

---

### User Story 5 - Map customers to ERP codes (Priority: P2)

Each supplier links a pharmacy or vendor to its customer code in their ERP. If a code is missing when someone orders, the person ordering is told, and can enter the code in a quick modal without leaving the order.

**Why this priority**: Orders without the customer's ERP code cannot be used in the ERP, so mapping must happen in the ordering flow, not afterwards.

**Independent Test**: Order as a user whose link has no code, enter it in the modal, finish the order, and confirm the code appears on the order in the API.

**Acceptance Scenarios**:

1. **Given** a supplier using an ERP integration and a customer with no code, **When** the customer orders from them, **Then** the flow explains the code is required and offers a quick-entry modal.
2. **Given** the modal, **When** a valid code is entered, **Then** the order continues without losing the cart.
3. **Given** a code already used by another customer of that supplier, **When** it is entered, **Then** it is refused with a clear message.
4. **Given** a supplier with no ERP integration, **When** a customer orders, **Then** the ordering flow is unchanged.
5. **Given** a saved code, **When** an order is sent to the ERP, **Then** the customer details come from the linked organization.

---

### User Story 6 - Create purchase orders from the ERP (Priority: P3)

A pharmacy or supplier using an ERP creates a purchase order for itself from inside the ERP. Dawa24 validates it, creates the order at Dawa24 prices, and returns its order number.

**Why this priority**: Useful, but it depends on the mapping and keys above, and carries the most risk.

**Independent Test**: Submit a purchase order for known items, confirm the returned order number, then repeat the same request and confirm no second order is created.

**Acceptance Scenarios**:

1. **Given** valid items and quantities, **When** the ERP submits a purchase order, **Then** an order is created for the key's organization and its number is returned.
2. **Given** the same request retried, **When** it arrives again, **Then** the original order is returned and no duplicate exists.
3. **Given** a price that differs from Dawa24's, **When** it is submitted, **Then** it is refused with the correct amount and no order is created.
4. **Given** an item with no active offer, **When** it is submitted, **Then** it is refused with a clear reason and no order is created.
5. **Given** a customer or supplier that is not linked, **When** it is submitted, **Then** it is refused with a clear error, and no account is created.

---

### User Story 7 - Protect sensitive data with Reveal (Priority: P3)

Sensitive values are masked by default. An authorized person presses Reveal to see one value, and every reveal is logged.

**Why this priority**: It reduces exposure on existing screens, and doesn't block the integration.

**Independent Test**: Open a masked field, reveal it as a permitted user, confirm the log entry, and confirm an unpermitted user cannot reveal it.

**Acceptance Scenarios**:

1. **Given** a screen showing customer contact details, tax and license numbers, or the gateway administrator username, **When** it loads, **Then** those values are masked and absent from the page source.
2. **Given** a permitted user, **When** they press Reveal, **Then** only that value is fetched and shown, and the action is logged.
3. **Given** an unpermitted user, **When** they request a value directly, **Then** it is refused and the attempt is logged.
4. **Given** an API key, a gateway password, a gateway virtual key, or an AI provider key, **When** anyone tries to reveal it, **Then** it cannot be shown, and the attempt is logged.

---

### User Story 8 - Test the API as Orgasoft (Priority: P2)

The owner imports a Postman collection, enters a test key, and runs the full journey as if they were Orgasoft, following plain documentation, with no developer help.

**Why this priority**: It is the owner's stated end goal, and how the integration is proved before Orgasoft starts building their plugin.

**Independent Test**: Follow the documentation on a test environment using only Postman, from first call to a synced product and an acknowledged order.

**Acceptance Scenarios**:

1. **Given** the collection, **When** the owner sets the base address and key, **Then** every request runs with no other setup.
2. **Given** the documentation, **When** a reader follows it, **Then** each endpoint explains its purpose, fields, examples, errors, and limits.
3. **Given** a failure case, **When** the owner runs the matching example, **Then** the real error matches the documentation.
4. **Given** the collection, **When** run in order, **Then** its built-in checks pass on a test environment.

---

### Edge Cases

- A key is revoked while its batch is still processing: the accepted batch finishes, and new calls are refused.
- Two ERP items share one barcode, or a barcode changes: the external code decides identity, and conflicts are reported for review.
- The same code appears twice in one batch: the last one wins, and the item result says so.
- Field names arrive in camelCase or PascalCase, as the two Orgasoft samples differ: both are accepted.
- A price has more than two decimals or an invalid format: the item is rejected with a reason, and it is never rounded silently.
- Negative stock: the item is rejected.
- An expiry date is missing: it is accepted, as in the second sample.
- A batch is larger than the allowed size: it is refused at once, with the limit in the message.
- The ERP sends too many requests: it is told to slow down and when to retry.
- A previously deleted supplier product's code is sent again: it is restored or reported, not silently duplicated.
- An existing supplier product was imported by file with no ERP code: the first sync links it by a confident match, and otherwise reports it for review.
- An order changes after being imported, such as a cancellation: it is flagged as changed so the ERP can pick it up.
- An organization has several integrations: each key has its own codes and tracking, so a future ERP never collides with Orgasoft.

## Requirements *(mandatory)*

### Functional Requirements

**Keys and access**

- **FR-001**: Owners MUST be able to create, name, rotate, and revoke API keys for their own organization. Staff MUST be able to see and revoke any key.
- **FR-002**: A key MUST belong to exactly one organization and one ERP provider, and carry its own permissions, warehouse, branch, and buying user.
- **FR-003**: Full key values MUST be shown once at creation and stored so that nobody, including staff, can read them back.
- **FR-004**: Keys MUST work only while the organization is approved, and the organization MUST come from the key, never from the request.
- **FR-005**: Each key MUST have a rate limit that the organization owner can change after the key is created, and exceeding it MUST tell the caller when to retry.
- **FR-006**: The API MUST use its own versioned address, separate from the existing website and internal APIs, and existing n8n integrations MUST keep working unchanged.
- **FR-007**: Every call MUST be logged with its key, action, result, and time. Reads of sensitive data MUST be logged too. Logs MUST never contain key values.

**Product sync**

- **FR-008**: The system MUST accept a batch of products and return a sync reference without waiting for processing.
- **FR-009**: The system MUST process batches in the background, per tenant, in order, with no impact on other tenants.
- **FR-010**: Items MUST be identified by the ERP's external code per integration, and a repeated item MUST update the same supplier product.
- **FR-011**: The system MUST reuse the tenant's match memory first, use the existing matching process for new items, and save successful matches.
- **FR-012**: Uncertain matches MUST go to supplier review, and MUST NOT be sold under a guessed product.
- **FR-013**: Matching MUST still work with the AI service down.
- **FR-014**: Sync MUST update stock, public price, discount percentage, quota, barcode, and expiry for the key's warehouse.
- **FR-015**: Sync MUST NOT take a product off sale because it was missing from a batch.
- **FR-016**: The status of a sync MUST report overall state and a per-item result: created, updated, unchanged, needs review, or failed with a reason.
- **FR-017**: Money values MUST be exact, and invalid values MUST be rejected rather than rounded.

**Orders**

- **FR-018**: The API MUST return sales orders limited to the supplier's own shipments, and purchase orders limited to the organization's own orders.
- **FR-019**: Results MUST support reliable paging and filtering by new, imported, changed, and date.
- **FR-020**: Each order MUST include the customer's ERP code, or a clear unmapped flag.
- **FR-021**: The ERP MUST be able to acknowledge import with its own reference, safely repeatable.
- **FR-022**: Orders changed after import MUST be identifiable.
- **FR-023**: The ERP MUST be able to confirm or reject pending orders through the same rules as the website, including wallet effects and notifications.
- **FR-024**: Rejection MUST accept an optional reason. Accept and reject conditions are the same as the website rules for a supplier. Extra business conditions may be added later.
- **FR-025**: The API MUST refuse delivered status and any status the key may not set.

**Customer mapping**

- **FR-026**: The organization number on the user-to-organization link page MUST act as the ERP customer code, unique per supplier.
- **FR-027**: When a supplier uses an ERP integration and the code is missing, the ordering flow MUST tell the user and offer quick entry, without losing the cart, and without changing the flow for other suppliers.

**Purchase orders**

- **FR-028**: The ERP MUST be able to create a purchase order for the key's own organization, using its chosen buying user, and receive the Dawa24 order number.
- **FR-029**: Orders MUST use Dawa24 prices. A different price MUST be refused with the correct amount, and items without an active offer MUST be refused.
- **FR-030**: Each request MUST carry a unique reference so a retry cannot create a second order.
- **FR-031**: The system MUST never create accounts or customers implicitly.

**Sensitive data**

- **FR-032**: Customer contact details, tax and license numbers, and the gateway administrator username MUST be masked by default and revealed one value at a time to permitted users, with each reveal logged. Gateway passwords, gateway virtual keys, AI provider keys and ERP keys MUST never be rendered or revealable.

**Developer experience**

- **FR-033**: Every response MUST include a clear status, stable error code, bilingual message, and request reference that the ERP can log.
- **FR-034**: The system MUST publish a machine-readable API description, a Postman collection, and step-by-step documentation, kept in step with the API.
- **FR-035**: Provider-specific behaviour MUST be separable, so a future ERP can be added without changing the shared API.

### Key Entities

- **Integration**: An organization's connection to an ERP provider. Holds warehouse, branch, buying user, and status.
- **API Key**: A credential of an integration, with prefix, permissions, limits, expiry, last use, and revocation.
- **External Product Link**: Ties an integration's product code to a supplier product.
- **Sync Batch**: One submitted set of items, with reference, status, and counts.
- **Sync Item Result**: The outcome of one item in a batch.
- **Order Import Record**: Whether an integration has imported an order, with the ERP's reference and time.
- **ERP Customer Code**: A supplier's code for a linked customer, unique per supplier.
- **Idempotency Record**: Ties a purchase order request reference to its order.
- **Audit Entry**: Who or what acted, including machine callers, reveals, and reads of sensitive data.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The owner can go from a new key to a synced product and an acknowledged order using only the Postman collection and documentation, in under 30 minutes.
- **SC-002**: A batch of 50,000 products is accepted in under 5 seconds, and processed within the 7-minute sync window.
- **SC-003**: With 50 tenants syncing every 7 minutes, website response times stay within their normal range.
- **SC-004**: Repeating any batch or purchase order 100 times creates zero duplicates.
- **SC-005**: Across automated tests, no key ever reads or changes another organization's data.
- **SC-006**: 100% of failed calls and items return a documented error code and reason.
- **SC-007**: No order is lost or double-imported across retries, restarts, or paging.
- **SC-008**: With the AI service off, every sync completes and each item gets a result.
- **SC-009**: Orders for suppliers with no ERP integration, and all existing n8n flows, behave exactly as before.
- **SC-010**: Every reveal appears in the audit log, and a sensitive value is never in the page until revealed. Gateway passwords, virtual keys and AI provider keys are never in any page.

## Assumptions

**To confirm with Orgasoft** (owner chose these defaults):

- One external code is one sellable item per tenant.
- `discount` and `tax` are percentages, and `tax` is stored or displayed, not applied to order totals, until confirmed.
- Quota means a cap per buying pharmacy branch, matching Dawa24's existing branch quota.
- Volume is about 50 tenants, 50,000 products per sync, and a few thousand orders a day.
- Their third sample, with `order_number`, `orga_orderID`, and items, is the import acknowledgement: it links a Dawa24 order to Orgasoft's order ID.

**Scope and dependencies**:

- Orgasoft sends data to Dawa24. Dawa24 does not call Orgasoft.
- Orgasoft builds its own plugin. This feature provides the API, Postman collection, and documentation.
- Work builds on `codex/audit-batches`, which must be reviewed and deployed first or together.
- The older `/api/v1` module routes are not reused, and their `MODULE_API_ENABLED` setting is unrelated to this API.
- Delivery stays in Dawa24, because it needs the buyer's PIN.
- Wallet checkout stays as it is. A purchase order uses the organization's normal payment terms.
- Multi-language messages follow the platform's Arabic-first rule.
