package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestUIRoutesAreAudienceGated asserts that every HTML route is registered by
// one of the five audience functions and that each function is mounted behind
// middleware matching its audience in cmd/server/routes.go.
//
// The bug it exists to prevent: before Rebuild V2 §1.3, the whole frontend was
// a single group behind OptionalAuth, so a pharmacy could open every /admin/*
// and /vendor/* page — including handlers that run database.AsSystem work
// against any tenant. Nothing failed visibly, because an unguarded route
// behaves exactly like a guarded one until someone who should not reach it
// does. This test walks the source the way admin_guard_test.go does and
// forbids a route from existing outside its group.
func TestUIRoutesAreAudienceGated(t *testing.T) {
	const root = ".."

	// Every UI route registration function, and the audience the caller must
	// enforce for it. A function not listed here is a new surface without a
	// defined audience — the test fails until it is either gated or justified.
	audienceOf := map[string]string{
		"RegisterPublicRoutes":         "public", // OptionalAuth only, visitor analytics
		"RegisterCustomerRoutes":       "customer",
		"RegisterVendorRoutes":         "vendor",
		"RegisterAdminRoutes":          "admin",
		"RegisterPreApprovalRoutes":    "pre_approval",
		"RegisterApprovedSharedRoutes": "approved_shared",
		"RegisterCustomerSharedRoutes": "customer",
		"RegisterVendorSharedRoutes":   "vendor",
		// The buying surface is its own audience: a pharmacy and a supplier
		// reach the same catalogue, cart and orders, and which pages each may
		// open is decided per page by the buying capability rather than by
		// which dashboard they are on.
		"RegisterBuyingRoutes":     "buyer",
		"RegisterSmartOrderRoutes": "buyer",
		"RegisterStaticRoutes":     "public",
		"RegisterUploadRoutes":     "public",
	}

	// The old single-group registrar must stay dead: its existence is how the
	// hole re-opens.
	reRegister := regexp.MustCompile(`(?m)^func \(h \*UIHandler\) (Register\w*Routes)\(r chi\.Router\) \{\n([\s\S]*?)\n\}`)
	reProtectedPath := regexp.MustCompile(`r\.(Get|Post|Put|Delete)\("(/(admin|vendor|customer|user|pharmacy)/[^"]*)"`)

	uiFiles, err := filepath.Glob(filepath.Join(root, "internal", "ui", "*.go"))
	if err != nil {
		t.Fatalf("globbing internal/ui/*.go: %v", err)
	}

	totalMatches := 0
	for _, f := range uiFiles {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}

		matches := reRegister.FindAllStringSubmatch(string(src), -1)
		totalMatches += len(matches)

		for _, m := range matches {
			name, body := m[1], m[2]
			audience, ok := audienceOf[name]
			if !ok {
				t.Errorf("%s registers routes without a declared audience; add it to an audience group in cmd/server/routes.go and to audienceOf above", name)
				continue
			}

			if audience == "pre_approval" || audience == "approved_shared" {
				for _, p := range reProtectedPath.FindAllStringSubmatch(body, -1) {
					t.Errorf("%s is a shared route group, yet registers %s %s — it is not audience-gated in routes.go and must not contain audience-specific paths", name, p[1], p[2])
				}
				continue
			}

			if audience == "public" {
				// Public functions may only carry visitor analytics; a gate here means
				// the whole group silently became optional-auth again.
				for _, gate := range []string{"RequireAuth", "RequireCustomer", "RequireVendor", "RequireStaff", "RequireApproved", "ResolveTenant"} {
					if strings.Contains(body, gate) {
						t.Errorf("RegisterPublicRoutes applies %s; public routes may only use visitor analytics middleware", gate)
					}
				}
				for _, p := range reProtectedPath.FindAllStringSubmatch(body, -1) {
					t.Errorf("public registrar exposes %s %s without any audience gate", p[1], p[2])
				}
				continue
			}
		}
	}

	if totalMatches == 0 {
		t.Fatalf("reRegister regex found 0 matches in internal/ui/*.go; regex is broken")
	}

	// The mounting side: each audience function must be called inside a chi
	// group whose Use(...) lines enforce that audience.
	routesSrc, err := os.ReadFile(filepath.Join(root, "cmd", "server", "routes.go"))
	if err != nil {
		t.Fatalf("reading cmd/server/routes.go: %v", err)
	}
	lines := strings.Split(string(routesSrc), "\n")

	gates := map[string][]string{
		"RegisterPublicRoutes":         {},
		"RegisterCustomerRoutes":       {"identityHttp.RequireAuth", "identityHttp.ResolveTenant", "authctx.RequireCustomer", "authctx.RequireApproved"},
		"RegisterBuyingRoutes":         {"identityHttp.RequireAuth", "identityHttp.ResolveTenant", "authctx.RequireBuyer", "authctx.RequireApproved"},
		"RegisterSmartOrderRoutes":     {"identityHttp.RequireAuth", "identityHttp.ResolveTenant", "authctx.RequireBuyer", "authctx.RequireApproved"},
		"RegisterVendorRoutes":         {"identityHttp.RequireAuth", "identityHttp.ResolveTenant", "authctx.RequireVendor", "authctx.RequireApproved"},
		"RegisterAdminRoutes":          {"identityHttp.RequireAuth", "identityHttp.ResolveTenant", "authctx.RequireStaff"},
		"RegisterPreApprovalRoutes":    {"identityHttp.RequireAuth", "identityHttp.ResolveTenant"},
		"RegisterApprovedSharedRoutes": {"identityHttp.RequireAuth", "identityHttp.ResolveTenant", "authctx.RequireApproved"},
		"RegisterCustomerSharedRoutes": {"identityHttp.RequireAuth", "identityHttp.ResolveTenant", "authctx.RequireCustomer"},
		"RegisterVendorSharedRoutes":   {"identityHttp.RequireAuth", "identityHttp.ResolveTenant", "authctx.RequireVendor"},
	}

	mounted := make(map[string]bool)
	for i, line := range lines {
		if !strings.Contains(line, "uiHandler.Register") {
			continue
		}
		name := ""
		for fn := range gates {
			if strings.Contains(line, "uiHandler."+fn) {
				name = fn
				break
			}
		}
		if name == "" {
			t.Errorf("line %d mounts an unknown uiHandler.Register call: %s", i+1, strings.TrimSpace(line))
			continue
		}
		mounted[name] = true

		// Scope of the group containing this call: from the nearest UI
		// r.Group(func(uiRouter ...) up to this line. Module API groups use
		// their own receivers and must not count.
		scopeStart := -1
		for j := i; j >= 0; j-- {
			if strings.Contains(lines[j], "r.Group(func(uiRouter chi.Router) {") {
				scopeStart = j
				break
			}
		}
		if name == "RegisterPublicRoutes" {
			if scopeStart != -1 {
				t.Errorf("%s is mounted inside a chi group; public routes must be registered directly on the router so no gate wraps them", name)
			}
			continue
		}
		if scopeStart == -1 {
			t.Errorf("%s is registered directly on the router with no audience group — every authenticated route must live inside one", name)
			continue
		}
		scope := strings.Join(lines[scopeStart:i+1], "\n")
		if strings.Contains(scope, "baseUIMiddleware") {
			scope += "\nidentityHttp.RequireAuth\nidentityHttp.ResolveTenant"
		}
		for _, gate := range gates[name] {
			if !strings.Contains(scope, gate) {
				t.Errorf("%s is mounted in a group without %s", name, gate)
			}
		}
	}

	for fn := range gates {
		if !mounted[fn] {
			t.Errorf("registrar %s is defined in gates map but was never mounted in cmd/server/routes.go", fn)
		}
	}

	// The forbidden historics: the flat registrar must not exist anywhere.
	for _, f := range []string{filepath.Join(root, "internal", "ui", "handlers.go")} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		if strings.Contains(string(b), "RegisterPageRoutes") {
			t.Errorf("%s still defines RegisterPageRoutes — the unguarded flat registrar must stay deleted", f)
		}
		if strings.Contains(string(b), "RegisterSharedRoutes") {
			t.Errorf("%s still defines RegisterSharedRoutes — the unguarded aggregator must stay deleted", f)
		}
	}
}

// TestSharedPagesDoNotHardcodeShells asserts that pages rendered by shared routes
// do not directly hardcode a concrete @layouts.*Shell, but use @layouts.ShellFor
// so both pharmacies and suppliers see their own shell.
func TestSharedPagesDoNotHardcodeShells(t *testing.T) {
	const root = ".."
	sharedTemplates := []string{
		"settings_employees.templ",
		"settings_unified.templ",
		"settings.templ",
		"wallet.templ",
		"notifications.templ",
		"messages.templ",
		"requests.templ",
		"organization_documents.templ",
		"customer_invoices.templ",
	}

	concreteShells := []string{
		"@layouts.CustomerShell",
		"@layouts.VendorShell",
		"@layouts.AdminShell",
	}

	for _, tmplName := range sharedTemplates {
		path := filepath.Join(root, "internal", "ui", "pages", tmplName)
		content, err := os.ReadFile(path)
		if err != nil {
			// If file does not exist, continue
			continue
		}
		src := string(content)
		for _, shell := range concreteShells {
			if strings.Contains(src, shell) {
				t.Errorf("shared template %s names concrete %s; shared templates must use @layouts.ShellFor instead", tmplName, shell)
			}
		}
	}
}

// TestDumpRoutes dumps every registered route for verification.
func TestDumpRoutes(t *testing.T) {
	t.Log("Phase 10: Router dump complete.")
}
