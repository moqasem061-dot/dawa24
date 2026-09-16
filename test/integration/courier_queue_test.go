package integration_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	commercePostgres "github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
	orgPostgres "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The dispatch board's queries, run against a real PostgreSQL.
//
// They exist because these are the one part of إدارة الشحنات that no unit test
// can hold: the predicate, its parameters and its ordering are assembled as
// text, and PostgreSQL is the only thing that reads them. The first version of
// courierQueueClause bound a courier id to every queue, including the two
// company-wide ones that never reference it — which compiles, passes every Go
// test, and answers "could not determine data type of parameter $4" to the
// first dispatcher who opens the board.
//
// Nothing here asserts about rows. The database is whatever the environment
// holds; what is being proved is that each query shape is one PostgreSQL will
// accept and scan.
func TestCourierQueueShapesRun(t *testing.T) {
	db := connectTestDB(t)
	if db == nil {
		return
	}
	ctx := database.AsSystem(context.Background())
	repo := commercePostgres.NewRepository(db)

	const (
		orgID   int64 = 1
		userID  int64 = 1
		bigPage       = 5
	)

	for _, queue := range []commerce.CourierQueue{
		commerce.CourierQueueMine,
		commerce.CourierQueueCompleted,
		commerce.CourierQueueUnassigned,
		commerce.CourierQueueAll,
	} {
		for _, search := range []string{"", "TRK"} {
			name := string(queue)
			if search != "" {
				name += "/searched"
			}
			t.Run(name, func(t *testing.T) {
				// Page two as well as page one: the LIMIT and OFFSET
				// placeholders are numbered after the predicate's own
				// parameters, so a queue that binds one fewer argument numbers
				// them differently from a queue that binds one more.
				for _, offset := range []int{0, bigPage} {
					if _, _, err := repo.ListCourierQueue(ctx, commerce.CourierQueueFilter{
						VendorOrgID:   orgID,
						CourierUserID: userID,
						Queue:         queue,
						Search:        search,
						Limit:         bigPage,
						Offset:        offset,
					}); err != nil {
						t.Fatalf("offset %d: %v", offset, err)
					}
				}
			})
		}
	}

	t.Run("counts", func(t *testing.T) {
		if _, err := repo.CourierQueueCounts(ctx, orgID, userID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("workload", func(t *testing.T) {
		if _, err := repo.ListCourierWorkload(ctx, orgID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("one parcel scoped to its supplier", func(t *testing.T) {
		// A shipment id that does not exist must come back as a clean
		// not-found rather than a scan or SQL error.
		if _, err := repo.GetVendorShipment(ctx, -1, orgID); err == nil {
			t.Error("a shipment id that cannot exist was found")
		}
	})

	t.Run("couriers of a company", func(t *testing.T) {
		orgRepo := orgPostgres.NewRepository(db)
		if _, err := orgRepo.ListMembersHolding(ctx, orgID, "vendor.delivery.view"); err != nil {
			t.Fatal(err)
		}
	})
}

// TestCourierAssignmentColumnsExist holds the schema the board reads to the
// migration that adds it. A query above would fail without them, but not in a
// way that names the missing column.
func TestCourierAssignmentColumnsExist(t *testing.T) {
	db := connectTestDB(t)
	if db == nil {
		return
	}
	ctx := database.AsSystem(context.Background())

	for _, column := range []string{"courier_user_id", "courier_assigned_at", "courier_assigned_by"} {
		var exists bool
		err := db.QueryRowUnscoped(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				 WHERE table_schema = 'commerce'
				   AND table_name = 'order_shipments'
				   AND column_name = $1);`, column).Scan(&exists)
		if err != nil {
			t.Fatalf("checking %s: %v", column, err)
		}
		if !exists {
			t.Errorf("commerce.order_shipments has no %s column; migration 188 has not been applied", column)
		}
	}
}
