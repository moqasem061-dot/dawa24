# PHASE D — Silence: make failures visible

**Depends on:** Phase C (which rewrites many of these sites already).
**Principle:** a query that fails must never look like "no data".

---

## Why this phase is last, and why it is not optional

PLAN_V5 Phase 0 Task 0.4 ordered the elimination of the
`if x, err := …; err == nil {}` swallow and a `make check` gate to stop it
returning. What actually happened:

| Pattern | Before | After |
|---|---|---|
| `if x, err := …; err == nil {` | 60 | **44** |
| `x, _ = h.someSvc.Method(…)` | — | **103** |
| `_ = pages.X(…).Render(ctx, w)` | — | **86** |
| **Total silent-failure sites** | **60** | **233** |

The ban was defeated by respelling the pattern, and no gate was added. The
count nearly quadrupled.

This is not a style issue. It is the mechanism by which a broken query renders
an empty page with no log line — which is precisely the "I click and nothing
happens" symptom that started this whole review.

---

## TASK D.1 — Eliminate all 233 sites

### D.1.1 Find them

```bash
echo "A. banned conditional form:"
grep -rn 'err == nil {' internal/ui/*.go internal/modules/*/*.go | grep -v _test

echo "B. blank-identifier service calls:"
grep -rnE '[a-zA-Z]+, _ = h\.[a-zA-Z]+Svc\.' internal/ui/*.go | grep -v _test

echo "C. discarded render errors:"
grep -rn '_ = pages\.' internal/ui/*.go | grep -v _test

echo "D. discarded anything else from a service:"
grep -rnE '_, _ = h\.[a-zA-Z]+Svc\.|_ = h\.[a-zA-Z]+Svc\.' internal/ui/*.go | grep -v _test
```

### D.1.2 The decision rule — apply per site, no batching

| Situation | Required treatment |
|---|---|
| **The page is meaningless without this data** (a list screen's list) | log `Error` with actor + org + identifiers, then `h.renderError(w, r, err)`, then `return` |
| **A secondary widget** (a dashboard stat card) | log `Warn`, set `data.<X>Unavailable = true`, and the template renders `@components.ErrorState` **in that section** |
| **Genuinely optional** (a nullable lookup where absence is normal) | keep it optional, log at `Debug`, and add a comment saying why silence is correct |
| **A render error** (`_ = pages.X(...).Render(...)`) | always log at `Error` — the response is already partially written, so you cannot re-render, but you must know it happened |

**`data.<X>Unavailable` is not optional bookkeeping.** A struct field nobody
renders is not a fix. Every one you add must have a matching template branch.

### D.1.3 The distinction that matters

```
Empty list  = "the database has no rows"      → @components.EmptyState
Error       = "we could not read the database" → @components.ErrorState
```

A user must be able to tell these apart. Today they cannot, anywhere.

### D.1.4 Correct shapes

**Primary data:**
```go
items, err := h.xSvc.List(ctx, actor.OrganizationID, 50, 0)
if err != nil {
	h.log.ErrorContext(ctx, "list x", "error", err, "org", actor.OrganizationID)
	h.renderError(w, r, err)
	return
}
```

**Secondary widget:**
```go
stats, err := h.xSvc.Stats(ctx, actor.OrganizationID)
if err != nil {
	// The dashboard is still useful without the stat row; the section renders
	// an error state so the number is never silently shown as zero.
	h.log.WarnContext(ctx, "dashboard stats unavailable", "error", err, "org", actor.OrganizationID)
	data.StatsUnavailable = true
} else {
	data.Stats = stats
}
```

**Render:**
```go
if err := pages.XPage(items, lang, dir).Render(ctx, w); err != nil {
	h.log.ErrorContext(ctx, "render x page", "error", err)
}
```

### D.1.5 Order of work

Do it **file by file**, not pattern by pattern. A file-at-a-time sweep lets you
see which loads are primary and which are secondary; a global find-and-replace
cannot make that judgement and will produce 233 wrong answers.

Commit per file. `PROGRESS.md` tracks the remaining count after each.

---

## TASK D.2 — Add the CI gate

Without this, the pattern returns. It already returned once.

### D.2.1 `make check-error-swallow`

Add to the `Makefile`, and wire it into `check`:

```makefile
check: fmt-check vet lint test check-provider-isolation check-file-size check-error-swallow

check-error-swallow: ## Fail if a service error is silently discarded
	@echo "==> checking for swallowed errors"
	@bad=$$( \
	  { grep -rn 'err == nil {' internal/ui/*.go internal/modules/*/*.go 2>/dev/null | grep -v '_test.go'; \
	    grep -rnE '[a-zA-Z]+, _ = h\.[a-zA-Z]+Svc\.' internal/ui/*.go 2>/dev/null | grep -v '_test.go'; \
	    grep -rn '_ = pages\.' internal/ui/*.go 2>/dev/null | grep -v '_test.go'; \
	  } | grep -v 'nolint:errswallow' | wc -l ); \
	if [ "$$bad" -ne 0 ]; then \
	  echo "FAIL: $$bad swallowed-error site(s):"; \
	  { grep -rn 'err == nil {' internal/ui/*.go internal/modules/*/*.go; \
	    grep -rnE '[a-zA-Z]+, _ = h\.[a-zA-Z]+Svc\.' internal/ui/*.go; \
	    grep -rn '_ = pages\.' internal/ui/*.go; } | grep -v '_test.go' | grep -v 'nolint:errswallow'; \
	  echo ""; \
	  echo "Each site must surface the error (see docs/PLAN_V6/04_PHASE_D_SILENCE.md §D.1.4)."; \
	  echo "If silence is genuinely correct, annotate the line with // nolint:errswallow and say why."; \
	  exit 1; \
	fi; \
	echo "OK: no swallowed errors"
```

### D.2.2 The escape hatch is deliberate and narrow

`// nolint:errswallow` **must** be followed by a reason on the same line or the
line above. Review every one. If there are more than ten in the whole codebase,
the rule is being worked around rather than followed.

### D.2.3 Prove the gate works

1. Reintroduce one swallow.
2. Run `make check`. **It must fail**, naming the file and line.
3. Remove it. `make check` passes.
4. Record both outputs in the commit message.

A gate that has never been seen failing is not known to work — that is exactly
how the PLAN_V5 guard tests passed over a dead admin panel.

---

## TASK D.3 — Sweep the module layer

Phases A–C focus on `internal/ui`. Run the same scan over `internal/modules/`:

```bash
grep -rn 'err == nil {' internal/modules/ --include=*.go | grep -v _test
grep -rnE ', _ = ' internal/modules/ --include=*.go | grep -v _test | grep -v '_ = tx.Exec'
```

Service and repository layers must return errors, never swallow them. A
repository that returns `(nil, nil)` on a failed query is the same bug one layer
down, and it is invisible from the handler.

---

## TASK D.4 — Verify the error path is reachable

For **every** screen connected in Phase C, D4 asserted an error surfaces. Now
confirm the mechanism end to end:

1. Point the app at a database with the relevant table renamed.
2. Open each list screen.
3. Every one must show an **error state** — not an empty list, not a 500 page,
   not a blank panel.
4. Every one must have produced a log line carrying actor, org and route.

Record which screens were checked in `PROGRESS.md`. Sample at least 20 if
checking all is impractical, prioritising the screens connected earliest in
Phase C.

---

## PHASE D COMPLETION GATE

```bash
make check                 # includes check-error-swallow
make test-integration
go test ./... -race
```

- [ ] `make check-error-swallow` reports **0** sites
- [ ] The gate was demonstrated failing, then passing (outputs in the commit)
- [ ] `nolint:errswallow` count ≤ 10, each with a written reason
- [ ] `internal/modules/` swept
- [ ] Empty state and error state are visually distinct and correct on ≥20 screens
- [ ] Every error path logs actor, org and route
