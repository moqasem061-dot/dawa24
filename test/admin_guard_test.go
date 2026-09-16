package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAdminRoutesRequirePermission asserts that every /api/v1/admin/ route is
// registered inside a group that calls RequirePermission.
//
// Three modules - ingest, org and promo - registered their admin routes with no
// permission middleware at all, while the other nine wrapped theirs. The
// handlers behind them use database.AsSystem, which deliberately bypasses tenant
// scoping, so the whole group was reachable by any authenticated user:
// /api/v1/admin/org/{id}/approve would let a caller approve their own
// organization, and .../suspend would let them suspend a competitor.
//
// Nothing failed visibly, because an unguarded route behaves exactly like a
// guarded one until someone who should not reach it does. A count of admin
// routes cannot see this either - the routes existed and were counted.
func TestAdminRoutesRequirePermission(t *testing.T) {
	// Tests run with the package directory as cwd.
	const root = ".."

	// Registration bodies, so a bare admin path in a comment or a test does not
	// register as a finding.
	reRegister := regexp.MustCompile(`(?s)func \(h \*Handler\) Register\w*Routes\(r chi\.Router\) \{(.*?)\n\}`)
	reAdminPath := regexp.MustCompile(`"(/api/v1/admin/[^"]*)"`)

	var unguarded []string

	err := filepath.Walk(filepath.Join(root, "internal", "modules"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for _, fn := range reRegister.FindAllStringSubmatch(string(src), -1) {
			body := fn[1]
			paths := reAdminPath.FindAllStringSubmatch(body, -1)
			if len(paths) == 0 {
				continue
			}
			if strings.Contains(body, "RequirePermission") || strings.Contains(body, "RequireAPIPermission") {
				continue
			}
			for _, p := range paths {
				unguarded = append(unguarded, filepath.ToSlash(rel)+": "+p[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking modules: %v", err)
	}

	if len(unguarded) > 0 {
		t.Errorf("%d admin route(s) registered without RequirePermission:", len(unguarded))
		for _, u := range unguarded {
			t.Errorf("  %s", u)
		}
		t.Errorf("wrap the group in r.Group(func(admin chi.Router){ admin.Use(authctx.RequirePermission(\"<module>.admin\", h.log)) ... })")
	}
}

// TestTenantScopedMutationsAreGuarded asserts that an org module handler taking
// an organization id from the URL checks it against the caller.
//
// Authentication establishes who is calling and says nothing about which tenant
// they may act on. PUT /api/v1/org/organizations/{id} read the id from the route
// and passed it to a repository running under database.AsSystem, so any
// logged-in user could rewrite another organization's credit limit and payment
// terms by changing one number in the URL.
//
// The read handlers are deliberately not covered: reviewing and following
// another organization is what a marketplace is for.
func TestTenantScopedMutationsAreGuarded(t *testing.T) {
	// Tests run with the package directory as cwd.
	const root = ".."

	mutations := map[string]bool{
		"UpdateOrg": true, "DeleteOrg": true,
		"UpdateBranch": true, "DeleteBranch": true, "CreateBranch": true,
		"UpdateMemberRole": true, "RemoveMember": true, "AddMember": true,
		"UpdateStatus": true,
	}

	reFunc := regexp.MustCompile(`func \(h \*Handler\) (\w+)\(w http\.ResponseWriter`)
	dir := filepath.Join(root, "internal", "modules", "org", "http")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading org/http: %v", err)
	}

	seen := map[string]bool{}
	var missing []string

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		// Admin handlers are guarded at the group by RequirePermission, which the
		// test above covers.
		if e.Name() == "admin.go" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		text := string(src)
		locs := reFunc.FindAllStringSubmatchIndex(text, -1)
		for i, loc := range locs {
			name := text[loc[2]:loc[3]]
			if !mutations[name] {
				continue
			}
			seen[name] = true
			end := len(text)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			if !strings.Contains(text[loc[0]:end], "SameOrgOrForbidden") {
				missing = append(missing, e.Name()+": "+name)
			}
		}
	}

	for name := range mutations {
		if !seen[name] {
			t.Errorf("handler %q not found; if it was renamed, update this test rather than dropping the check", name)
		}
	}
	for _, m := range missing {
		t.Errorf("%s takes an organization id from the URL without authctx.SameOrgOrForbidden", m)
	}
}

// TestAdminUIRoutesRequirePagePermission asserts that every admin HTML route
// (except the dashboard allowlist) is registered inside a group that calls RequirePagePermission.
func TestAdminUIRoutesRequirePagePermission(t *testing.T) {
	const root = ".."

	reAdminPath := regexp.MustCompile(`"(/(admin)/[^"]*)"`)
	dir := filepath.Join(root, "internal", "ui")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading internal/ui: %v", err)
	}

	var unguarded []string

	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "admin_routes_") || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		content := string(src)

		// Split by r.Group to inspect each group
		groups := strings.Split(content, "r.Group(")
		for _, g := range groups[1:] {
			if !strings.Contains(g, "RequirePagePermission") {
				for _, p := range reAdminPath.FindAllStringSubmatch(g, -1) {
					if p[1] != "/admin/dashboard" {
						unguarded = append(unguarded, e.Name()+": "+p[1])
					}
				}
			}
		}
	}

	if len(unguarded) > 0 {
		t.Errorf("%d admin HTML route(s) registered without RequirePagePermission:", len(unguarded))
		for _, u := range unguarded {
			t.Errorf("  %s", u)
		}
	}
}
