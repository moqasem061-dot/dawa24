# PHASE A — Truth: delete the lies

**Blocks:** everything.
**Principle:** a screen that lies to the user is worse than a missing screen.
Nothing in Phases B–E matters while a "delete" button reports success and
deletes nothing.

**Rule for this phase: you are removing code.** If a task tempts you to add a
feature, stop — that belongs in Phase C.

---

## TASK A.0 — Build the real test harness (do this first)

Nothing in this plan is verifiable until this exists.

### A.0.1 The problem

Every Phase 8/9 test does this:
```go
handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
```
Fourteen `nil` services, then assert 200. Only a page that reads nothing can
pass. **These tests certify fakeness.**

### A.0.2 Build `internal/ui/testsupport_test.go`

```go
// newRealUIHandler builds a UIHandler wired to real services over a real
// database. Tests that construct it with nil services prove nothing: a page
// that returns 200 with no services is by definition a page that reads no data.
func newRealUIHandler(t *testing.T, db *database.DB) http.Handler

// doGET / doPOST drive the handler over HTTP as a given actor.
func doGET(t *testing.T, h http.Handler, path string, actor authctx.Actor) *httptest.ResponseRecorder
func doPOST(t *testing.T, h http.Handler, path string, form url.Values, actor authctx.Actor) *httptest.ResponseRecorder

// testDB returns a real pool. Reuse test/integration/rls_test.go's getTestDB
// logic — do NOT write a second one.
func testDB(t *testing.T) *database.DB
```

Seed helpers, each returning the inserted row's ID:
`seedOrg`, `seedBranch`, `seedUser`, `seedProduct`, `seedOffer`, `seedOrder`,
`seedInvoice`, `seedPayment`, `seedWarehouse`, `seedPolicy`.
Each cleans up with `t.Cleanup`.

Model them on `test/integration/coverage_chain_test.go`, which already does this
correctly — copy its `db.InTx(database.AsSystem(ctx), ...)` insert-and-defer-delete
shape.

### A.0.3 Make skips loud

`getTestDB` skips when `DATABASE_URL` is unset. Locally **22 tests skip** and the
run still prints `ok`. A developer running `go test ./...` sees green while
proving nothing.

Add to `internal/ui/testsupport_test.go` and `test/integration`:
```go
// A skipped integration test is not a passing one. In CI, DATABASE_URL is
// always set (.github/workflows/ci.yml provides a postgres service), so a skip
// there means the harness broke.
if os.Getenv("CI") == "true" && dbURL == "" {
    t.Fatal("DATABASE_URL must be set in CI")
}
```
That guard already exists in `rls_test.go` — extend it to every integration
helper, and add `make test-integration` that fails if **zero** integration tests
actually ran.

### A.0.4 Completion criteria

- [ ] `newRealUIHandler` exists and is used by at least one passing test
- [ ] All 10 seed helpers exist with cleanup
- [ ] `make test-integration` fails when no integration test executes
- [ ] `docs/PLAN_V6/PROGRESS.md` row A.0 marked done

---

## TASK A.1 — Delete the 2FA security theatre

**Severity: highest.** REVIEW §2.1.

### A.1.1 What is there now

`internal/ui/platform_hardening_handlers.go`:

| Handler | Body | Tells the user |
|---|---|---|
| `Security2FAEnableSubmit` | one redirect line | "تم تفعيل المصادقة الثنائية (2FA) بنجاح" |
| `Security2FADisableSubmit` | one redirect line | "تم تعطيل المصادقة الثنائية" |
| `Security2FARecoverySubmit` | one redirect line | "تم توليد رموز الاسترداد" |
| `Auth2FAChallengeSubmit` | accepts **any** 6+ character string | redirects to `/customer/dashboard` |

And `LoginSubmit` (`internal/ui/public_handlers.go`) calls
`h.idSvc.Login(...)`, receives `res`, and **never reads `res.RequiresMFA`** even
though `identity.Service.Login` sets it (`service.go:174`).

### A.1.2 Decision: DELETE, do not repair

Laravel has real 2FA (`Google2FAService`, 124 lines). Go does not, and a partial
implementation of an authentication control is a liability. Remove it cleanly
now; rebuild it properly as a single scoped task in Phase C (Task C.9) with a
real TOTP library.

**Delete:**
- `Security2FAEnrollmentPage`, `Security2FAEnableSubmit`, `Security2FADisableSubmit`,
  `Security2FARecoverySubmit`, `Auth2FAChallengePage`, `Auth2FAChallengeSubmit`
- their routes in `internal/ui/handlers.go`
- `Security2FAEnrollmentPage` / `Auth2FAChallengePage` templ funcs in
  `internal/ui/pages/platform_hardening.templ`
- the 2FA entries in `/settings/security`
- the tests in `internal/ui/platform_phase9_test.go` that assert these render

**Keep:** `identity.user_mfa`, `AdminResetMFA`, and `LoginResult.RequiresMFA` —
the backend contract is correct and Task C.9 will use it.

### A.1.3 Close the login hole regardless

Even with the pages gone, `LoginSubmit` must honour the field, or the day
someone populates `user_mfa` the factor is silently bypassed:

```go
res, err := h.idSvc.Login(ctx, identity.LoginInput{...})
if err != nil { ... }
if res.RequiresMFA {
    // No challenge UI exists yet (PLAN_V6 Task C.9). Refusing is the safe
    // failure: never issue a session to an account that asked for a second
    // factor we cannot verify.
    h.log.WarnContext(ctx, "login refused: MFA required but no challenge implemented", "user", res.UserID)
    http.Redirect(w, r, "/auth/login?error=mfa_unavailable", http.StatusSeeOther)
    return
}
http.SetCookie(...)
```

### A.1.4 Tests

- **D2**: a user with a `user_mfa` row **cannot** obtain a session cookie
- **D4**: that login attempt returns an error, not a session
- assert `/settings/security/2fa` and `/auth/2fa-challenge` return **404**

### A.1.5 Completion criteria

- [ ] No route serves a 2FA page
- [ ] `LoginSubmit` refuses when `RequiresMFA`
- [ ] A seeded MFA user cannot log in (D2 proves it)
- [ ] Entry in `DELETED.md` with the reason and the Phase C task that restores it

---

## TASK A.2 — Delete the fake PDF generators

REVIEW §2.4.

`InvoicePDFDownload` and `OrderPDFDownload` serve ~120 bytes of text under
`Content-Type: application/pdf`. It is not a PDF. It contains no invoice data.

**Delete both handlers, both routes, and any download button pointing at them.**
Real PDF generation is Phase C Task C.10.

Rationale: a download button that produces a corrupt file is a support ticket
for every user who clicks it. Removing the button costs nothing; leaving it
costs trust.

- [ ] Handlers and routes removed
- [ ] Download buttons removed from `customer_invoices.templ` and any order pages
- [ ] `DELETED.md` entry naming Task C.10 as the restore point

---

## TASK A.3 — Delete the fabricated datasets

REVIEW §2.2. **Five screens render invented data as if it came from the database.**

### A.3.1 `defaultModelRegistry` — invented row counts

`internal/ui/admin_trash_handlers.go:13` hardcodes
"1240 products / 14 trashed", "14200 orders / 23 trashed", etc. An administrator
reads these as real statistics.

Delete the `var defaultModelRegistry` block. The trash screens are rebuilt in
Phase C Task C.7 against `information_schema`.

### A.3.2 The four reference screens

`internal/ui/admin_reference_handlers.go` — each builds a literal
`[]pages.ReferenceItem`:

| Handler | Fabricates | Real table (currently dead) |
|---|---|---|
| `AdminCountriesPage` | Egypt, Saudi Arabia | `platform_admin.countries` |
| `AdminSocialMediaPage` | facebook/x/linkedin URLs | `org.organization_social_media` |
| `AdminHighlightSectionsPage` | two invented sections | `promo.highlight_sections` |
| `AdminApiIntegrationsPage` | **"Twilio ****4a8f", "Paymob ****9e2c"** | `platform_admin.api_integrations` |

The API-integrations screen is the worst: it presents credentials for providers
that are not integrated at all.

**Action now:** replace each literal array with a real query **or** delete the
screen. Decide per screen using `00_MASTER.md` §A.2:

| Screen | Laravel has it? | Decision |
|---|---|---|
| countries | yes (`CountriesCo`) | **connect** in Phase C |
| social-media | yes (`Admin/SocialMedia`) | **connect** in Phase C |
| highlight-sections | yes (`Admin/HighlightSections`) | **connect** in Phase C |
| api-integrations | yes (`ApiIntegrations`) | **connect** in Phase C — and never render a real key |

Until Phase C connects them, they must render an **empty state**, not fiction:

```go
var items []pages.ReferenceItem   // real query lands in Phase C Task C.6
```

An empty list is honest. Invented rows are not.

### A.3.3 Tests

- **D1** for each: seed one row, assert the page contains it *(written now,
  failing now, passing after Phase C)* — this is the standing proof that the
  literal never comes back
- assert the page body does **not** contain `"Twilio"`, `"Paymob"`,
  `"facebook.com/dawa24"`, or `"1240"`

### A.3.4 Completion criteria

- [ ] Zero hardcoded `[]pages.ReferenceItem` literals in handlers
- [ ] `defaultModelRegistry` deleted
- [ ] The five screens render empty, not fiction
- [ ] Failing D1 tests committed and listed in `PROGRESS.md` as expected-red until Phase C

---

## TASK A.4 — Fix or remove every no-op destructive action

REVIEW §2.3. **21 submit handlers never call a service.** These are the ones that
claim success:

| Handler | File | Claims |
|---|---|---|
| `AdminTrashRestoreSubmit` | `admin_trash_handlers.go` | "تم استرجاع السجل بنجاح" |
| `AdminTrashPurgeSubmit` | `admin_trash_handlers.go` | "تم الحذف النهائي للسجل" |
| `VendorBranchDeleteSubmit` | `vendor_handlers.go:342` | "تم حذف الفرع بنجاح" |
| `VendorTeamToggleSubmit` | `vendor_handlers.go:451` | "تم تحديث حالة حساب الموظف" |
| `CustomerReportIssueSubmit` | `platform_hardening_handlers.go` | "تم إرسال البلاغ بنجاح" |

Plus, verify each of these by reading the body — the scan flagged them for having
no `h.*Svc.` call, but some may use a differently-named dependency:

`AdminFeatureToggleSubmit` · `CompareFileArchiveSubmit` · `CompareFileDeleteSubmit` ·
`CompareFileMappingSubmit` · `CompareFileRenameSubmit` · `CompareFileUnarchiveSubmit` ·
`CompareRowManualMatchSubmit` · `CompareUploadSubmit` · `CompareRunSubmit` ·
`OrganizationDocumentDeleteSubmit` · `OrganizationDocumentsUploadSubmit` ·
`UploadAPISubmit`

Re-run the scan yourself and confirm the list:
```bash
python3 - <<'EOF'
import re, glob
for path in glob.glob('internal/ui/*.go'):
    if path.endswith('_test.go'): continue
    for p in re.split(r'\nfunc \(h \*UIHandler\) ', open(path,encoding='utf-8',errors='ignore').read())[1:]:
        name = p.split('(')[0]
        if not name.endswith('Submit'): continue
        body = p[:p.find('\nfunc ')] if p.find('\nfunc ')>0 else p
        if not re.search(r'h\.[a-zA-Z]+Svc\.', body) and 'h.storage' not in body:
            print(f"{path}  {name}")
EOF
```

### A.4.1 For each, choose one — no third option

**(a) Connect it** — if the service method exists. Two of these are trivial:

```go
// VendorBranchDeleteSubmit — org.Service already has DeleteBranch
func (h *UIHandler) VendorBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther); return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", "معرف الفرع غير صالح."); return
	}
	if err := h.orgSvc.DeleteBranch(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "delete branch", "error", err, "branch", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/branches", "error", h.safeMessage(err, langOf(r))); return
	}
	h.redirectWithNotice(w, r, "/vendor/branches", "success", "تم حذف الفرع بنجاح.")
}
```

**Note the tenancy argument.** `DeleteBranch` must take `actor.OrganizationID`
and scope the `WHERE` clause by it. Never delete by ID alone from a form.

**(b) Remove the button and the route** — if the service does not exist yet and
the feature belongs in Phase C. Record it in `DELETED.md` with its Phase C task.

Do **not** leave a button that reports success.

### A.4.2 Tests — D2 and D4 for every one you connect

```go
// D2 — it actually happened
require.False(t, branchExists(t, db, br.ID))

// D4 — Law 3: no success on failure
rec := doPOST(t, h, "/vendor/branches/999999/delete", nil, actorFor(org))
require.NotContains(t, rec.Header().Get("Location"), "success")

// D3 — org B cannot delete org A's branch, and the row survives
doPOST(t, h, fmt.Sprintf("/vendor/branches/%d/delete", brA.ID), nil, actorFor(orgB))
require.True(t, branchExists(t, db, brA.ID))
```

### A.4.3 Completion criteria

- [ ] The scan returns zero submit handlers without a service call *(or each remaining one is documented as intentionally side-effect-free)*
- [ ] Every connected handler has D2 + D3 + D4
- [ ] Every removed button is in `DELETED.md`
- [ ] **No handler emits "success" on a path that did not write**

---

## TASK A.5 — Fix the misleading doc comments

Several comments describe behaviour the function does not have. The worst:

```go
// CustomerReportIssueSubmit saves issue report into workflow.report_issues.
func (h *UIHandler) CustomerReportIssueSubmit(...) {
	h.log.InfoContext(ctx, "issue reported", ...)   // saves nothing
	h.redirectWithNotice(..., "success", "تم إرسال البلاغ بنجاح...")
}
```

Sweep every handler touched in Phases A–C: the doc comment must describe what the
code does. A comment that describes an intention is a lie that outlives the
review.

```bash
# candidates: comments mentioning a table the function never queries
grep -rn "^// [A-Z][a-zA-Z]* \(saves\|stores\|writes\|persists\|updates\|deletes\|creates\)" internal/ui/*.go
```

- [ ] Every doc comment in `internal/ui/` matches its function's behaviour

---

## PHASE A COMPLETION GATE

```bash
make check
go test ./internal/ui/... ./test/... -race
make test-integration        # must actually run, not skip
```

- [ ] `newRealUIHandler` exists; at least one D1 test passes against real Postgres
- [ ] `make test-integration` fails if zero integration tests execute
- [ ] No 2FA route exists; a seeded MFA user cannot obtain a session
- [ ] No PDF route exists
- [ ] Zero hardcoded data literals in `internal/ui/*.go`
- [ ] Zero submit handlers that report success without writing
- [ ] Every doc comment is accurate
- [ ] `DELETED.md` lists every removal with its restore task
- [ ] `PROGRESS.md` rows A.0–A.5 complete

**The measure of this phase: after it, nothing in the product tells the user
something happened when it did not.**
