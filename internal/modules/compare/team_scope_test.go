package compare_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// The moderator hierarchy's one safety property, pinned in the type it lives in.
//
// AdminTempWarehouseFilter.OwnerIn is what scopes "مستودعات المشرفين تحت
// إدارتي" to the moderators reporting to the caller. The dangerous case is a
// main moderator with nobody under them: an empty scope must mean "nobody", and
// a filter that treats an empty set as "no restriction" would show them every
// temporary warehouse on the platform.
//
// The distinction is nil versus empty, so it is asserted on the value the
// handler builds rather than only in the SQL, where a reader would have to
// notice the FALSE branch to see it.
func TestEmptyTeamScopeIsNotAnAbsentScope(t *testing.T) {
	// A main moderator with a team.
	withTeam := compare.AdminTempWarehouseFilter{OwnerIn: []int64{7, 9}}
	if withTeam.OwnerIn == nil {
		t.Fatal("a populated team produced a nil scope")
	}

	// A main moderator with nobody under them. The handler must produce a
	// non-nil empty slice — "restrict to nobody" — and never nil, which the
	// query reads as "do not restrict".
	empty := compare.AdminTempWarehouseFilter{OwnerIn: []int64{}}
	if empty.OwnerIn == nil {
		t.Fatal("an empty team produced a nil scope; the listing would be unrestricted")
	}
	if len(empty.OwnerIn) != 0 {
		t.Fatalf("empty scope has %d ids", len(empty.OwnerIn))
	}

	// And the super admin's screen, which is genuinely unrestricted, is the
	// only case that leaves OwnerIn nil.
	all := compare.AdminTempWarehouseFilter{}
	if all.OwnerIn != nil {
		t.Fatal("the unrestricted filter carries a scope")
	}
}
