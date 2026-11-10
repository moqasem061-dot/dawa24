package http

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// idKeys are the fields whose values Dawa24 returns or accepts as its own ids.
var idKeys = map[string]bool{"syncId": true, "variantId": true, "requestId": true, "nextCursor": true}

// TestOpenAPIExamplesUseShortIDs walks every example in the contract and fails
// when an id the API returns is longer than 20 characters or carries a "/" or a
// "+". Schema definitions (maps) are skipped; only scalar values are checked.
func TestOpenAPIExamplesUseShortIDs(t *testing.T) {
	var doc any
	if err := yaml.Unmarshal(openAPISpec, &doc); err != nil {
		t.Fatal(err)
	}
	checked := 0
	walkShortIDs(t, "", doc, &checked)
	if checked < 10 {
		t.Fatalf("only %d id values were checked; the walk is not reaching the examples", checked)
	}
}

func walkShortIDs(t *testing.T, path string, v any, checked *int) {
	t.Helper()
	switch node := v.(type) {
	case map[string]any:
		for key, child := range node {
			here := path + "." + key
			if idKeys[key] {
				if s, ok := child.(string); ok {
					*checked++
					if len(s) > 20 || strings.ContainsAny(s, "/+") {
						t.Errorf("%s = %q: want at most 20 characters and no '/' or '+'", here, s)
					}
				}
			}
			walkShortIDs(t, here, child, checked)
		}
	case []any:
		for i, child := range node {
			walkShortIDs(t, fmt.Sprintf("%s[%d]", path, i), child, checked)
		}
	}
}
