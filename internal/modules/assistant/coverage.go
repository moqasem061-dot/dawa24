package assistant

import (
	"context"
	"time"
)

// Who can actually deliver to this branch, and when.
//
// This is the one question a pharmacy asks that the assistant cannot answer
// from its own read model, and it must not try. Coverage is decided by
// workflow.CoverageService, the same rule the catalogue, the supplier profile
// and CheckAvailability all resolve through since the availability unification;
// a second copy of it living here is precisely the defect that unification
// existed to remove, and it would fail in the worst possible way — an assistant
// confidently naming a supplier that checkout then refuses.
//
// So the assistant declares what it needs and imports nothing. The interface is
// implemented in the composition root (cmd/server), over the real coverage
// service, which is how every cross-module need is met in this codebase: the
// consumer states the contract, the wiring satisfies it, and no module reaches
// into another.

// CoverageProbe answers coverage questions for one buying branch.
type CoverageProbe interface {
	// VendorsServingBranch reports which suppliers cover the branch on the
	// given weekday.
	VendorsServingBranch(ctx context.Context, branchID int64, day time.Weekday) (CoverageAnswer, error)
}

// CoverageAnswer is what the probe found.
type CoverageAnswer struct {
	// Evaluated distinguishes "coverage was assessed and nobody covers this
	// branch" from "coverage could not be assessed".
	//
	// A nil slice cannot make that distinction, and getting it wrong is exactly
	// how uncovered suppliers ended up in the catalogue count that the
	// availability review recorded as its flagship defect. An assistant that
	// answers "nobody delivers to you" when it simply could not look is worse
	// than one that says it could not look.
	Evaluated bool
	// BranchName is the branch the answer is about, so the reply can name it
	// when the caller has several.
	BranchName string
	// Vendors are the covering suppliers, with their delivery windows.
	Vendors []CoverageVendorRow
}

// CoverageVendorRow is one supplier that reaches the branch.
type CoverageVendorRow struct {
	ID   int64  `json:"-"`
	Name string `json:"supplier"`
	// Windows are the supplier's delivery windows on the day asked about,
	// formatted "HH:MM-HH:MM". Empty means the supplier covers the day without
	// publishing a time.
	Windows []string `json:"windows,omitempty"`
	// Governorate and City name where the coverage row places the delivery, so
	// an answer about "my branch" can be checked by the person reading it.
	Area string `json:"area,omitempty"`
}

// EntityRef makes a covering supplier clickable, the same way every other row
// that names a record the user can open is.
func (r CoverageVendorRow) EntityRef() Entity {
	if r.Name == "" {
		return Entity{}
	}
	return Entity{
		Kind:  EntityOrganization,
		ID:    r.ID,
		Label: r.Name,
		Title: r.Name,
	}
}

// WeekdayFromArabic maps the day names a user types onto a weekday.
//
// The model is given an enum of English day names in the schema, but users ask
// in Arabic and models pass through what they were told. Both are accepted, and
// anything else is a refusal rather than a guess — silently answering about the
// wrong day is the kind of confidently wrong answer this assistant must not
// produce.
func WeekdayFromArabic(s string) (time.Weekday, bool) {
	switch s {
	case "sunday", "الأحد", "الاحد":
		return time.Sunday, true
	case "monday", "الإثنين", "الاثنين":
		return time.Monday, true
	case "tuesday", "الثلاثاء":
		return time.Tuesday, true
	case "wednesday", "الأربعاء", "الاربعاء":
		return time.Wednesday, true
	case "thursday", "الخميس":
		return time.Thursday, true
	case "friday", "الجمعة":
		return time.Friday, true
	case "saturday", "السبت":
		return time.Saturday, true
	}
	return 0, false
}
