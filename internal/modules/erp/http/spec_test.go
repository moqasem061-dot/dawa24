package http

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

// specRoutesNotYetMounted are documented operations the server does not serve
// yet (S7 mounts them). They must stay documented, and they must stay unmounted:
// when S7 mounts one, this test fails until the entry is removed here.
var specRoutesNotYetMounted = []string{"GET /offers", "POST /orders/purchase"}

// TestOpenAPIMatchesRoutes compares the routes the handler mounts with the
// operations in the embedded contract, in both directions.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	mounted := map[string]bool{}
	walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}
	if err := chi.Walk(NewHandler(nil, erp.Config{}, nil, nil).Routes(), walk); err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(openAPISpec, &doc); err != nil {
		t.Fatalf("embedded spec does not parse: %v", err)
	}
	documented := map[string]bool{}
	for path, ops := range doc.Paths {
		for method := range ops {
			switch method {
			case "get", "post", "put", "patch", "delete":
				documented[strings.ToUpper(method)+" "+path] = true
			}
		}
	}

	skip := map[string]bool{}
	for _, op := range specRoutesNotYetMounted {
		skip[op] = true
		if !documented[op] {
			t.Errorf("%s is listed as not yet mounted but the contract does not document it", op)
		}
		if mounted[op] {
			t.Errorf("%s is mounted now: remove it from specRoutesNotYetMounted", op)
		}
	}

	var drift []string
	for op := range mounted {
		if !documented[op] {
			drift = append(drift, "mounted but not documented: "+op)
		}
	}
	for op := range documented {
		if !mounted[op] && !skip[op] {
			drift = append(drift, "documented but not mounted: "+op)
		}
	}
	sort.Strings(drift)
	for _, d := range drift {
		t.Error(d)
	}
}
