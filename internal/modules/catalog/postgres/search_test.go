package postgres

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
)

var placeholderPattern = regexp.MustCompile(`\$(\d+)`)

// placeholderNumbers returns the distinct $n placeholders a statement names, in
// ascending order.
func placeholderNumbers(sql string) []int {
	seen := map[int]bool{}
	for _, m := range placeholderPattern.FindAllStringSubmatch(sql, -1) {
		n, _ := strconv.Atoi(m[1])
		seen[n] = true
	}
	var out []int
	for n := 1; n <= 64; n++ {
		if seen[n] {
			out = append(out, n)
		}
	}
	return out
}

func numbersFrom(first, last int) []int {
	var out []int
	for n := first; n <= last; n++ {
		out = append(out, n)
	}
	return out
}

// The page and its total bind the same values in the same order. A placeholder
// renumbered in one place and not the other makes the pager count by a different
// value than the page lists, and pgx refuses a statement whose arguments do not
// match its placeholders.
func TestProductFilterBindsTwelveValuesInOrder(t *testing.T) {
	if got := len(productFilterArgs(catalog.SearchParams{})); got != 12 {
		t.Fatalf("productFilterArgs supplies %d values, want 12", got)
	}
	if got, want := placeholderNumbers(productFilterSQL), numbersFrom(1, 12); !reflect.DeepEqual(got, want) {
		t.Fatalf("filter names placeholders %v, want %v", got, want)
	}
}

// The page adds exactly LIMIT and OFFSET after the shared filter.
func TestProductListAddsLimitAndOffsetAfterTheFilter(t *testing.T) {
	sql := productListSQL(catalog.SearchParams{Query: "panadol", Sort: "name"})
	if got, want := placeholderNumbers(sql), numbersFrom(1, 14); !reflect.DeepEqual(got, want) {
		t.Fatalf("list names placeholders %v, want %v", got, want)
	}
	if !strings.Contains(sql, "LIMIT $13 OFFSET $14") {
		t.Fatal("the page no longer pages by $13 and $14")
	}
	if !strings.Contains(sql, productFilterSQL) {
		t.Fatal("the page does not use the shared filter, so its rows can differ from its total")
	}
}

// Raw input must never be the pattern of an ILIKE: in a pattern the admin's own
// '%' and '_' are wildcards. Normalised input may be, because normalisation
// removes them.
func TestProductMatchNeverUsesRawInputAsAnILIKEPattern(t *testing.T) {
	for _, raw := range []string{"$1", "$11", "$7", "$12"} {
		for _, bad := range []string{"ILIKE '%' || " + raw, "ILIKE " + raw} {
			if strings.Contains(productFilterSQL, bad) {
				t.Errorf("%q matches raw input as a pattern; use strpos(lower(...), lower(%s))", bad, raw)
			}
		}
	}
}
