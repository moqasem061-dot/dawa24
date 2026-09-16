.DEFAULT_GOAL := help
SHELL := /bin/bash

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X main.buildVersion=$(VERSION) -X main.buildCommit=$(COMMIT)

.PHONY: help
help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-26s\033[0m %s\n", $$1, $$2}'

# --- development ---------------------------------------------------------

.PHONY: up
up: ## Start postgres, redis and minio
	docker compose -f docker-compose.dev.yml up -d
	@echo "waiting for postgres..." && until docker compose -f docker-compose.dev.yml exec -T postgres pg_isready -U dawa24 -d dawa24_store >/dev/null 2>&1; do sleep 1; done
	@echo "ready"

.PHONY: down
down: ## Stop local infrastructure
	docker compose -f docker-compose.dev.yml down

.PHONY: reset
reset: ## Destroy local data and rebuild the schema from scratch
	docker compose -f docker-compose.dev.yml down -v
	rm -rf .data
	$(MAKE) up
	$(MAKE) migrate

.PHONY: run
run: ## Run the HTTP server
	go run -ldflags="$(LDFLAGS)" ./cmd/server

.PHONY: worker
worker: ## Run the background worker
	go run ./cmd/worker

# --- database ------------------------------------------------------------

.PHONY: migrate
migrate: ## Apply pending migrations
	go run ./cmd/cli migrate

.PHONY: migrate-status
migrate-status: ## List migrations and show how many are pending
	go run ./cmd/cli migrate-status

# --- quality gates -------------------------------------------------------
# These are exactly what CI runs. A green `make check` locally means a green
# pipeline, so failures are found before a push rather than after one.

.PHONY: check
check: fmt-check vet lint test check-provider-isolation check-prompt-version check-file-size check-inline-styles check-error-swallow check-no-cdn check-file-size-count check-hardcoded-arabic check-emoji check-unused-components check-important check-backdrop-filter check-transition-all check-breakpoints check-physical-properties check-topbar-impls check-modal-legacy check-modal-handwritten check-css-layered check-deadcode check-undefined-classes ## Run every gate

.PHONY: check-error-swallow
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

.PHONY: fmt
fmt: ## Format the codebase
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if anything is unformatted
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint (enforces module boundaries via depguard)
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed: https://golangci-lint.run/welcome/install/"; exit 1; }
	golangci-lint run

.PHONY: test
test: ## Run tests with race detection
	go test -race -count=1 ./...

.PHONY: test-short
test-short: ## Run unit tests only, skipping anything needing containers
	go test -short -count=1 ./...

.PHONY: test-integration
test-integration: ## Run database integration tests
	go test -v -count=1 ./test/integration/...


# --- architectural invariants --------------------------------------------

.PHONY: check-provider-isolation
check-provider-isolation: ## Fail if an AI provider name leaks outside platform/gateway
	@echo "checking provider isolation..."
	@if grep -riE '\b(openai|anthropic|deepseek|gemini|groq|openrouter)\b' \
	     --include='*.go' --include='*.templ' --include='*.sql' \
	     ./cmd ./internal ./db 2>/dev/null \
	     | grep -v '^./internal/platform/gateway/' \
	     | grep -v '_test.go'; then \
	  echo ""; \
	  echo "ERROR: an AI provider name appears outside internal/platform/gateway/."; \
	  echo "The Store must be provider-agnostic: ask the Gateway for a capability,"; \
	  echo "never for a named provider or model. See docs/adr/0005-ai-gateway.md."; \
	  exit 1; \
	fi
	@echo "  ok: no provider names outside the gateway package"

.PHONY: check-prompt-version
check-prompt-version: ## Fail if the match-prompt version is declared outside aicapabilities
	@echo "checking prompt version isolation..."
	@if grep -rn 'sm-enh-' --include='*.go' ./cmd ./internal 2>/dev/null 	     | grep -v '^./internal/shared/matchflow/' 	     | grep -v '_test.go'; then 	  echo ""; 	  echo "ERROR: the match-enhancement prompt version appears outside"; 	  echo "internal/shared/matchflow/. It is the decision-cache key: two copies"; 	  echo "drift, and a drifted key silently splits one cache in two."; 	  echo "Import matchflow.PromptVersion instead of restating it."; 	  exit 1; 	fi
	@echo "  ok: one prompt version, declared once"

.PHONY: check-file-size
check-file-size: ## Fail on Go files over 400 lines
	@echo "checking file sizes..."
	@fail=0; \
	while read -r count file; do \
	  if [ "$$count" -gt 400 ]; then echo "  $$file: $$count lines (limit 400)"; fail=1; fi; \
	done < <(find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v ' total$$'); \
	if [ "$$fail" = "1" ]; then \
	  echo ""; \
	  echo "ERROR: files above the 400-line limit. Split them by concern."; \
	  echo "The limit exists so a whole file fits in one read - for humans and"; \
	  echo "for AI agents. See AGENTS.md."; \
	  exit 1; \
	fi
	@echo "  ok: all files within limit"

# --- build ---------------------------------------------------------------

.PHONY: build
build: ## Build all binaries into ./bin
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/server ./cmd/server
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/worker ./cmd/worker
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/cli    ./cmd/cli

.PHONY: docker
docker: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t dawa24-store:$(VERSION) .

check-inline-styles: ## Fail if inline style attributes grow past the current ceiling
	@echo "==> checking inline styles"
	# Ceiling lowered to 11 during the design-system unification. No inline style
	# was added by it; the ceiling had simply drifted 70 above the real value,
	# which is a ratchet that has stopped ratcheting.
	@n=$$(grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l | tr -d ' '); \
	if [ "$$n" -gt 11 ]; then \
	  echo "FAIL: $$n inline style attributes (ceiling 11)."; \
	  echo ""; \
	  echo "Inline styles bypass the tokens in app.css, which is why the design"; \
	  echo "drifted: a fix on one page never generalises. Use a class from"; \
	  echo "components.css, or add one there. Genuinely one-off positioning may"; \
	  echo "stay inline, but must use var(--token) values, never literals."; \
	  echo ""; \
	  echo "This is a ratchet: lower the ceiling in the Makefile as it drops."; \
	  exit 1; \
	fi; \
	echo "OK: $$n inline styles (ceiling 11)"

# --- ratchets ------------------------------------------------------------
# Each number below may only go down. When a change improves one, lower its
# ceiling in the same commit: that is what stops an improvement eroding a page
# at a time, which is how the design and the translations drifted in the first
# place. These are the measured values on 31 August 2026.

.PHONY: check-no-cdn
check-no-cdn: ## Fail if a template loads a script or stylesheet from a CDN
	@echo "==> checking for third-party asset hosts"
	@if grep -rn --include='*.templ' -E '(src|href)="https?://(unpkg|cdnjs|cdn\.jsdelivr)' internal/ui; then echo ""; echo "ERROR: a template loads an asset from a CDN."; echo "htmx, Alpine and Leaflet are served from internal/ui/static/vendor;"; echo "a CDN tag puts a host we do not run on every page render and forces"; echo "script-src to name it in the CSP."; exit 1; fi
	@echo "  ok: no CDN asset hosts"

.PHONY: check-file-size-count
check-file-size-count: ## Fail if the number of oversized Go files grows
	# Ceiling set to 103 in Phase 5 Wave 4 (measured baseline before Phase 6 splitting).
	@n=$$(find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v " total$$" | awk '$$1>400' | wc -l | tr -d " "); if [ "$$n" -gt 0 ]; then echo "FAIL: $$n Go files over 400 lines (ceiling 0). check-file-size lists them."; exit 1; fi; echo "  ok: $$n oversized files (ceiling 0)"

.PHONY: check-hardcoded-arabic
check-hardcoded-arabic: ## Fail if user-facing Arabic is hardcoded in Go
	@echo "==> checking for hardcoded user-facing Arabic"
	@pat=$$(printf 'Ø'); n=$$(LC_ALL=C grep -rc "\"[^\"]*$$pat[^\"]*\"" --include='*.go' internal/ui internal/modules cmd 2>/dev/null 	  | grep -v "_test\|_templ\|/i18n/" 	  | grep -vE "modules/(ingest/domain|compare/columns_data|compare/comparison(_split[0-9])?|catalog/import_rows(_split[0-9])?|smartorder/pipeline/query|assistant/prompt)\.go" 	  | awk -F: '{s+=$$2} END{print s+0}'); 	if [ "$$n" -gt 0 ]; then 	  echo "FAIL: $$n hardcoded Arabic strings in Go (ceiling 0)."; 	  echo "A string in a handler cannot be shown in English. Add a key to"; 	  echo "internal/shared/i18n and call i18n.T(lang, key)."; 	  exit 1; 	fi; 	echo "  ok: $$n user-facing Arabic literals in Go (ceiling 0)"

# The excluded files are linguistic reference data, not user-facing text, and
# translating them would break the product matcher rather than improve it:
#
#   ingest/domain.go, compare/columns_data.go   column-header dictionaries used
#                                               to recognise Arabic headings in
#                                               an uploaded spreadsheet
#   catalog/import_rows.go                      dosage-form vocabulary
#                                               ("غسول فم" -> mouthwash)
#   compare/comparison.go                       Arabic->Latin brand map
#                                               ("بانادول" -> panadol)
#   smartorder/pipeline/query.go                Arabic tokenisation rules, mostly
#                                               in comments
#   assistant/prompt.go                         the assistant's Arabic system
#                                               prompt
#
# These are inputs the platform matches against, not output it renders. See
# ADR 0018.

.PHONY: check-emoji
check-emoji: ## Fail if emoji in templates grow
	# Ceiling lowered to 8 in Phase 5 Wave 2.
	@pat=$$(printf '\360\237'); n=$$(LC_ALL=C grep -ro "$$pat" --include='*.templ' internal/ui | wc -l | tr -d " "); if [ "$$n" -gt 8 ]; then echo "FAIL: $$n emoji in templates (ceiling 8). Use an icon from internal/ui/components/icons.templ, or nothing."; exit 1; fi; echo "  ok: $$n emoji in templates (ceiling 8)"

.PHONY: check-unused-components
check-unused-components: ## Fail if components no page uses grow
	@n=0; for f in internal/ui/components/*.templ; do for fn in $$(grep -oE "^templ [A-Za-z0-9_]+" "$$f" | awk '{print $$2}'); do p=$$(grep -rho "components\.$$fn" --include='*.templ' internal/ui/pages internal/ui/layouts 2>/dev/null | wc -l); i=$$(grep -rho "@$$fn(" --include='*.templ' internal/ui/components 2>/dev/null | wc -l); g=$$(grep -rho "components\.$$fn" internal/ui/*.go 2>/dev/null | wc -l); if [ "$$p" -eq 0 ] && [ "$$i" -eq 0 ] && [ "$$g" -eq 0 ]; then n=$$((n+1)); echo "  unused: $$fn"; fi; done; done; if [ "$$n" -gt 0 ]; then echo "FAIL: $$n components nothing references (ceiling 0). Either a page should use it, or it should not exist."; exit 1; fi; echo "  ok: every component is referenced"

.PHONY: check-important
check-important: ## Fail if !important occurrences exceed defended ceiling (3)
	@echo "==> checking !important in CSS"
	# invoice_printable.css is excluded: it is a standalone print document with
	# its own <!DOCTYPE>, its own :root and a * reset, loaded by no other page.
	# There is no author cascade there for an !important to short-circuit.
	@n=$$(grep -ro '!important' internal/ui/static/css/*.css 2>/dev/null | grep -v invoice_printable | wc -l | tr -d ' '); \
	if [ "$$n" -gt 3 ]; then \
	  echo "FAIL: $$n !important occurrences in CSS (ceiling 3). Cascade layers must resolve precedence."; \
	  exit 1; \
	fi; \
	echo "  ok: $$n !important occurrences outside the print document (ceiling 3)"

.PHONY: check-backdrop-filter
check-backdrop-filter: ## Fail if backdrop-filter exceeds dialog overlay ceiling (4 including vendor prefix)
	@echo "==> checking backdrop-filter in CSS"
	@n=$$(grep -rn 'backdrop-filter:' internal/ui/static/css/*.css 2>/dev/null | grep -v '^\s*/\*' | wc -l | tr -d ' '); \
	if [ "$$n" -gt 0 ]; then \
	  echo "FAIL: $$n backdrop-filter rules in CSS (ceiling 0). The surfaces that used one are opaque now."; \
	  exit 1; \
	fi; \
	echo "  ok: $$n backdrop-filter rules (ceiling 0)"

.PHONY: check-transition-all
check-transition-all: ## Fail if transition: all appears in CSS
	@echo "==> checking transition: all in CSS"
	@if grep -rnE 'transition:\s*all' internal/ui/static/css/*.css 2>/dev/null; then \
	  echo "FAIL: transition: all found. Animate explicit CSS properties only for 60fps performance."; \
	  exit 1; \
	fi; \
	echo "  ok: 0 transition: all rules"

.PHONY: check-breakpoints
check-breakpoints: ## Fail if media queries use pixel values outside the 4 canonical tokens
	@echo "==> checking responsive breakpoints in CSS"
	@bad=$$(grep -rnE '@media\s*\([^{]+' internal/ui/static/css/*.css 2>/dev/null | grep -vE '(640px|768px|1024px|1280px|prefers-reduced-motion|prefers-color-scheme|pointer|print)' | wc -l | tr -d ' '); \
	if [ "$$bad" -ne 0 ]; then \
	  echo "FAIL: $$bad non-standard media query breakpoints found:"; \
	  grep -rnE '@media\s*\([^{]+' internal/ui/static/css/*.css 2>/dev/null | grep -vE '(640px|768px|1024px|1280px|prefers-reduced-motion|prefers-color-scheme|pointer|print)'; \
	  echo "All breakpoints must snap to 640px, 768px, 1024px, or 1280px."; \
	  exit 1; \
	fi; \
	echo "  ok: all media queries match standard tokens"

.PHONY: check-physical-properties
check-physical-properties: ## Fail if physical directional properties appear without defended comment
	@echo "==> checking physical directional properties in CSS"
	@bad=$$(grep -rnE '^\s*(margin-left|margin-right|padding-left|padding-right|left:\s*|right:\s*)' internal/ui/static/css/*.css 2>/dev/null | grep -v 'defended' | wc -l | tr -d ' '); \
	if [ "$$bad" -ne 0 ]; then \
	  echo "FAIL: $$bad physical directional properties found:"; \
	  grep -rnE '^\s*(margin-left|margin-right|padding-left|padding-right|left:\s*|right:\s*)' internal/ui/static/css/*.css 2>/dev/null | grep -v 'defended'; \
	  echo "Use CSS logical properties (margin-inline, padding-inline, inset-inline) for bi-directional RTL support."; \
	  exit 1; \
	fi; \
	echo "  ok: all directional styles use logical properties"

.PHONY: check-topbar-impls
check-topbar-impls: ## Fail if a shell hand-builds its own dashboard header again
	@n=$$(grep -rl 'class="top-navbar"' --include='*.templ' internal/ui 2>/dev/null | wc -l | tr -d ' '); if [ "$$n" -gt 0 ]; then echo "FAIL: $$n templates still use .top-navbar. The dashboard header is components.DashboardTopBar; the public one is .site-header."; exit 1; fi; echo "  ok: one dashboard top bar, one site header"

.PHONY: check-modal-legacy
check-modal-legacy: ## Fail if legacy modal-overlay or window.openModal appears
	@echo "==> checking legacy modal implementations"
	@n=$$(grep -rnE '(modal-overlay|window\.openModal|window\.closeModal)' internal/ui 2>/dev/null | wc -l | tr -d ' '); \
	if [ "$$n" -gt 0 ]; then \
	  echo "FAIL: $$n legacy modal references found. Use native <dialog class=\"modal\"> with components.Modal or dialog.showModal()."; \
	  grep -rnE '(modal-overlay|window\.openModal|window\.closeModal)' internal/ui 2>/dev/null; \
	  exit 1; \
	fi; \
	echo "  ok: 0 legacy modal references"

.PHONY: check-modal-handwritten
check-modal-handwritten: ## Fail if raw <dialog in pages exceeds ratchet ceiling
	@echo "==> checking raw <dialog in page templates"
	# Ceiling lowered from 20 to 0 in Phase 5 Wave 4 (all page modals migrated to components.Modal).
	@n=$$(grep -rn '<dialog' internal/ui/pages/*.templ 2>/dev/null | wc -l | tr -d ' '); \
	if [ "$$n" -gt 0 ]; then \
	  echo "FAIL: $$n raw <dialog elements in pages (ceiling 0). Every modal must go through components.Modal."; \
	  grep -rn '<dialog' internal/ui/pages/*.templ 2>/dev/null; \
	  exit 1; \
	fi; \
	echo "OK: $$n raw <dialog elements (ceiling 0)"

.PHONY: check-css-layered
check-css-layered: ## Fail if a stylesheet ships outside the @layer cascade
	@fail=0; for f in internal/ui/static/css/*.css; do case "$$(basename $$f)" in app.css|invoice_printable.css) continue;; esac; if ! grep -q '@layer' "$$f"; then echo "  unlayered: $$f"; fail=1; fi; done; if [ "$$fail" = "1" ]; then echo "FAIL: a stylesheet outside @layer beats every layered rule. app.css (hide/show primitives) and invoice_printable.css (a standalone print document) are the two deliberate exceptions."; exit 1; fi; echo "  ok: every stylesheet is layered"

.PHONY: check-deadcode
check-deadcode: ## Fail if unreachable exported code grows past the ceiling
	@echo "==> checking for unreachable code"
	@if ! go run golang.org/x/tools/cmd/deadcode@latest -test ./... > /tmp/dawa_deadcode.txt 2>/dev/null; then 	  echo "  skipped: deadcode unavailable (needs network on first run)"; exit 0; 	fi; 	n=$$(wc -l < /tmp/dawa_deadcode.txt | tr -d ' '); 	if [ "$$n" -gt 211 ]; then 	  echo "FAIL: $$n unreachable funcs (ceiling 211). See /tmp/dawa_deadcode.txt."; 	  echo "Read each finding before deleting: a symbol may be reached from a .templ"; 	  echo "file, which deadcode does not parse."; 	  exit 1; 	fi; 	echo "  ok: $$n unreachable funcs (ceiling 211)"

.PHONY: check-undefined-classes
check-undefined-classes: ## Fail if templates reference class names no stylesheet defines
	@echo "==> checking for class names with no CSS rule"
	@grep -rhoE 'class="[^"]*"' internal/ui/pages/*.templ internal/ui/layouts/*.templ internal/ui/components/*.templ 	  | sed 's/class="//; s/"$$//' | tr ' ' '
' 	  | grep -E '^[a-zA-Z][a-zA-Z0-9_-]*$$' | sort -u > /tmp/dawa_used.txt; 	grep -rhoE '\.[a-zA-Z][a-zA-Z0-9_-]*' internal/ui/static/css/*.css | sed 's/^\.//' | sort -u > /tmp/dawa_defined.txt; 	n=$$(comm -23 /tmp/dawa_used.txt /tmp/dawa_defined.txt | wc -l | tr -d ' '); 	if [ "$$n" -gt 64 ]; then 	  echo "FAIL: $$n class names have no CSS rule (ceiling 64)."; 	  echo ""; 	  echo "An element carrying a class nothing defines is unstyled. This is how"; 	  echo "the inline-style conversion silently removed styling: the style attribute"; 	  echo "went, and the class replacing it was never written."; 	  comm -23 /tmp/dawa_used.txt /tmp/dawa_defined.txt | head -20; 	  exit 1; 	fi; 	echo "  ok: $$n undefined classes (ceiling 64; the remainder are Alpine state names in :class bindings)"
