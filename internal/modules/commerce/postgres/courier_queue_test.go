package postgres

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
)

// TestCourierQueueClauseParameterConsistency verifies that for every queue type,
// the WHERE clause references exactly all bound parameters $1 through $N where N = len(args),
// without any missing parameters (e.g. unreferenced $2) that would cause Postgres error 42P18.
func TestCourierQueueClauseParameterConsistency(t *testing.T) {
	queues := []commerce.CourierQueue{
		commerce.CourierQueueMine,
		commerce.CourierQueueCompleted,
		commerce.CourierQueueFailed,
		commerce.CourierQueueUnassigned,
		commerce.CourierQueueAll,
	}

	dollarRe := regexp.MustCompile(`\$(\d+)`)

	for _, q := range queues {
		t.Run(string(q), func(t *testing.T) {
			filter := commerce.CourierQueueFilter{
				VendorOrgID:   10,
				CourierUserID: 25,
				Queue:         q,
				Search:        "test",
			}

			where, args := courierQueueClause(filter)
			nArgs := len(args)
			if nArgs == 0 {
				t.Fatalf("queue %s returned 0 args", q)
			}

			// Extract all $N parameter references
			matches := dollarRe.FindAllStringSubmatch(where, -1)
			referenced := make(map[int]bool)
			for _, m := range matches {
				idx, err := strconv.Atoi(m[1])
				if err != nil {
					t.Fatalf("invalid parameter index: %s", m[1])
				}
				referenced[idx] = true
			}

			// Ensure every $1 .. $nArgs is referenced
			for i := 1; i <= nArgs; i++ {
				if !referenced[i] {
					t.Errorf("queue %q has %d args, but $%d is NOT referenced in the query (causes SQLSTATE 42P18)", q, nArgs, i)
				}
			}

			// Ensure no parameter index exceeds nArgs
			for idx := range referenced {
				if idx > nArgs {
					t.Errorf("queue %q references $%d, but args only has %d items", q, idx, nArgs)
				}
			}

			// Verify pagination integration: LIMIT $(nArgs+1) OFFSET $(nArgs+2)
			pageArgs := append(append([]any{}, args...), 25, 0)
			pagination := fmt.Sprintf("\n\tLIMIT $%d OFFSET $%d;", nArgs+1, nArgs+2)
			fullQuery := "SELECT * FROM s WHERE " + where + pagination

			fullMatches := dollarRe.FindAllStringSubmatch(fullQuery, -1)
			fullReferenced := make(map[int]bool)
			for _, m := range fullMatches {
				idx, err := strconv.Atoi(m[1])
				if err != nil {
					t.Fatalf("invalid parameter index: %s", m[1])
				}
				fullReferenced[idx] = true
			}

			totalParams := len(pageArgs)
			for i := 1; i <= totalParams; i++ {
				if !fullReferenced[i] {
					t.Errorf("queue %q full paginated query missing parameter $%d", q, i)
				}
			}
		})
	}
}
