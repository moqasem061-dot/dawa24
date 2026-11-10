package erp

import (
	"context"
)

// fakeActivity keeps the activity reads' rows in memory. Its filters follow
// the SQL: calls by id with the result filter, batches by id with the state
// filter the live panel uses, every read limited to its organization.
type fakeActivity struct {
	actCounts     ActivityCounts
	actCalls      []fakeCall
	actBatches    []BatchRow
	actImports    []OrderImportRow
	actLastFilter CallFilter
	actCountsOrg  int64
}

// fakeCall is one stored call with the organization it belongs to.
type fakeCall struct {
	org int64
	row CallRow
}

func (f *fakeActivity) ActivityCounts(_ context.Context, orgID int64) (ActivityCounts, error) {
	f.actCountsOrg = orgID
	return f.actCounts, nil
}

func (f *fakeActivity) ListCalls(_ context.Context, orgID int64, filter CallFilter) ([]CallRow, error) {
	f.actLastFilter = filter
	var out []CallRow
	for _, c := range f.actCalls {
		if c.org != orgID || (filter.BeforeID > 0 && c.row.ID >= filter.BeforeID) {
			continue
		}
		if filter.Result == "ok" && c.row.Status >= 400 || filter.Result == "error" && c.row.Status < 400 {
			continue
		}
		out = append(out, c.row)
		if len(out) == filter.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeActivity) ListBatches(_ context.Context, orgID, beforeID int64, limit int) ([]BatchRow, error) {
	var out []BatchRow
	for _, b := range f.actBatches {
		if b.OrganizationID != orgID || (beforeID > 0 && b.ID >= beforeID) {
			continue
		}
		out = append(out, b)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeActivity) ListActiveBatches(_ context.Context, orgID int64, limit int) ([]BatchRow, error) {
	var out []BatchRow
	for _, b := range f.actBatches {
		if b.OrganizationID != orgID || (b.State != "queued" && b.State != "processing") {
			continue
		}
		out = append(out, b)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeActivity) ListOrderImports(_ context.Context, _ int64, limit int) ([]OrderImportRow, error) {
	if limit > 0 && len(f.actImports) > limit {
		return f.actImports[:limit], nil
	}
	return f.actImports, nil
}
