package ingest

import "github.com/muhiya/dawa24-store/internal/shared/productmatch"

// MappingSnapshot is the confirmed column reading, flattened for storage.
type MappingSnapshot struct {
	HeaderRow    int                     `json:"header_row"`
	FirstDataRow int                     `json:"first_data_row"`
	Headers      []string                `json:"headers"`
	Columns      []MappedColumn          `json:"columns"`
	Conflicts    []productmatch.Conflict `json:"conflicts,omitempty"`
	Notes        []productmatch.Note     `json:"notes,omitempty"`
}

// MappedColumn is one column as the vendor confirmed it.
type MappedColumn struct {
	Index      int                     `json:"index"`
	Header     string                  `json:"header"`
	Field      productmatch.Field      `json:"field,omitempty"`
	Label      string                  `json:"label,omitempty"`
	Confidence productmatch.Confidence `json:"confidence,omitempty"`
	Source     productmatch.Source     `json:"source,omitempty"`
	Score      float64                 `json:"score"`
	Why        []string                `json:"why,omitempty"`
	Ignored    bool                    `json:"ignored"`
}

// SnapshotMapping flattens a resolved mapping for storage.
func SnapshotMapping(layout productmatch.Layout, m *productmatch.Mapping) *MappingSnapshot {
	if m == nil {
		return nil
	}
	snap := &MappingSnapshot{
		HeaderRow:    layout.HeaderRow,
		FirstDataRow: layout.FirstDataRow,
		Headers:      layout.Headers,
		Conflicts:    m.Conflicts,
		Notes:        m.Notes,
	}
	for _, c := range m.Columns {
		col := MappedColumn{
			Index:      c.Index,
			Header:     c.Header,
			Field:      c.Field,
			Confidence: c.Confidence,
			Source:     c.Source,
			Score:      c.Score,
			Why:        c.Why,
			Ignored:    c.Ignored,
		}
		if c.Field != "" {
			col.Label = c.Field.Label()
		}
		snap.Columns = append(snap.Columns, col)
	}
	return snap
}

// MappedIdentifiers reports which identifier columns the vendor bound, read
// back from the stored snapshot.
//
// The settings screen and the matching run are separated by a page load and a
// database round trip, so the run cannot ask the live Mapping what was bound —
// it has only this. Without it the matching options would have to take the
// vendor's toggle at face value, and a toggle switched on before the column was
// unmapped would enable a tier for a column that no longer exists.
func (s *MappingSnapshot) MappedIdentifiers() productmatch.MappedColumns {
	out := productmatch.MappedColumns{}
	if s == nil {
		return out
	}
	for _, c := range s.Columns {
		switch c.Field {
		case productmatch.FieldSKU:
			out.Code = true
		case productmatch.FieldBarcode:
			out.Barcode = true
		}
	}
	return out
}
