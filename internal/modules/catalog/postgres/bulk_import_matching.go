package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// productMatch is an incoming row tied to a catalogue row already on file.
type productMatch struct {
	id     int64
	reason catalog.MatchReason
}

// resolveExistingProducts finds, for each incoming product, the catalogue row it
// should update — matching on SKU first, then barcode, then name and attributes.
//
// A product that already carries an id is left exactly as it is. That is the
// commit honouring the screen: the administrator reviewed a row as "update
// product 4412", and re-deriving the answer here is how a commit came to write
// somewhere else than the review said it would.
func resolveExistingProducts(ctx context.Context, tx pgx.Tx, prods []*catalog.Product) (map[int]productMatch, error) {
	byOrg := map[int64][]int{}
	decided := map[int]productMatch{}
	for i, p := range prods {
		if p.ID > 0 {
			decided[i] = productMatch{id: p.ID, reason: catalog.MatchNone}
			continue
		}
		byOrg[p.OrganizationID] = append(byOrg[p.OrganizationID], i)
	}

	m := &productMatcher{ctx: ctx, tx: tx, prods: prods, matches: decided}
	for orgID, idxs := range byOrg {
		if err := m.matchOrg(orgID, idxs); err != nil {
			return nil, err
		}
	}
	return m.matches, nil
}

// productMatcher resolves incoming rows to catalogue ids one organisation at a
// time, accumulating decisions as it goes.
type productMatcher struct {
	ctx     context.Context
	tx      pgx.Tx
	prods   []*catalog.Product
	matches map[int]productMatch
}

// matchOrg applies the three strategies in strength order. SKU and barcode are
// exact identifiers; a name match is a fallback and must never override one, so
// an index already matched is left alone by every later strategy.
func (m *productMatcher) matchOrg(orgID int64, idxs []int) error {
	for _, column := range []struct {
		reason catalog.MatchReason
		name   string
		key    func(*catalog.Product) string
	}{
		{catalog.MatchSKU, "sku", func(p *catalog.Product) string { return p.SKU }},
		{catalog.MatchBarcode, "barcode", func(p *catalog.Product) string { return p.Barcode }},
	} {
		wanted := m.pending(idxs, func(p *catalog.Product) string {
			return strings.ToLower(strings.TrimSpace(column.key(p)))
		})
		if len(wanted) == 0 {
			continue
		}
		found, err := lookupByColumn(m.ctx, m.tx, orgID, column.name, keysOf(wanted))
		if err != nil {
			return err
		}
		m.record(wanted, found, column.reason)
	}

	return m.matchByName(orgID, idxs)
}

// matchByName is the fallback tier: no identifier agreed, so the name has to
// carry the decision — and a name alone is not enough to make one.
//
// It used to be. The query grouped catalog.products by normalised name and took
// min(id), which says that every catalogue row sharing a name is one product.
// On this catalogue that is false in the most damaging way available: اوجمنتين
// 375 مجم and اوجمنتين 1 جم are two rows, two strengths, two medicines and one
// normalised name, so an import of either updated whichever had the lower id
// and the other strength was silently overwritten.
//
// So the name now retrieves CANDIDATES and the attributes decide between them.
// A candidate the row contradicts on dose, line extension or pharmaceutical
// form is not a match at any score — see productmatch.Discriminate, which is
// the same comparison the scorer and the AI adjudicator refuse pairs on. What
// survives is chosen deterministically, and a row whose strength nothing in the
// catalogue shares stays unmatched, which stages it as the new product it is.
func (m *productMatcher) matchByName(orgID int64, idxs []int) error {
	wanted := m.pending(idxs, func(p *catalog.Product) string { return p.Name.Get(i18n.AR) })
	if len(wanted) == 0 {
		return nil
	}

	candidates, err := lookupNameCandidates(m.ctx, m.tx, orgID, keysOf(wanted))
	if err != nil {
		return err
	}

	for name, rows := range wanted {
		shortlist := candidates[arabic.Normalize(name)]
		if len(shortlist) == 0 {
			continue
		}
		for _, i := range rows {
			if id, ok := chooseByIdentity(shortlist, catalog.IdentityFields(m.prods[i])); ok {
				m.matches[i] = productMatch{id: id, reason: catalog.MatchName}
			}
		}
	}
	return nil
}

// nameCandidate is one catalogue row that shares a normalised name with an
// incoming row, carrying the attributes that tell the two apart.
type nameCandidate struct {
	id     int64
	fields productmatch.ProductFields
}

// chooseByIdentity picks the catalogue row an incoming row should update, from
// the rows that share its name.
//
// Contradicted candidates are dropped outright. Among what remains, a candidate
// that states the same attribute the row states is preferred over one that is
// silent about it: where the catalogue holds both "اوجمنتين" and "اوجمنتين 1 جم"
// and the row says 1 g, the row that says so too is the one meant, and the bare
// one is the older, vaguer entry.
//
// The final tie-break is the lowest id, which is what the previous min(id)
// query returned. Two catalogue rows that genuinely cannot be told apart are a
// duplicate somebody already created; picking the same one every time keeps a
// re-import updating it rather than adding a third.
func chooseByIdentity(shortlist []nameCandidate, row productmatch.ProductFields) (int64, bool) {
	var (
		bestID           int64
		bestCorroborated bool
		found            bool
	)
	for _, c := range shortlist {
		if productmatch.Separable(row, c.fields) {
			continue
		}
		corroborated := statesSameAttributes(row, c.fields)
		if !found || beats(c.id, corroborated, bestID, bestCorroborated) {
			bestID, bestCorroborated, found = c.id, corroborated, true
		}
	}
	return bestID, found
}

// beats orders two surviving candidates: corroboration first, then the lower
// id. Both halves are deliberate — the first picks the entry that says what the
// row says, the second makes the choice repeatable across runs.
func beats(id int64, corroborated bool, bestID int64, bestCorroborated bool) bool {
	if corroborated != bestCorroborated {
		return corroborated
	}
	return id < bestID
}

// statesSameAttributes reports whether both sides name the attributes that
// separate one strength of a brand from another. Discriminate has already
// ruled out the pairs that disagree, so where both speak, both agree.
func statesSameAttributes(row, candidate productmatch.ProductFields) bool {
	both := func(a, b string) bool {
		return strings.TrimSpace(a) != "" && strings.TrimSpace(b) != ""
	}
	return both(row.Concentration, candidate.Concentration) ||
		both(row.DosageForm, candidate.DosageForm)
}

// pending groups the still-unmatched rows by the key one strategy looks them up
// with, so the lookup is a single set-based query rather than one per row.
func (m *productMatcher) pending(idxs []int, key func(*catalog.Product) string) map[string][]int {
	wanted := map[string][]int{}
	for _, i := range idxs {
		if _, done := m.matches[i]; done {
			continue
		}
		if k := key(m.prods[i]); strings.TrimSpace(k) != "" {
			wanted[k] = append(wanted[k], i)
		}
	}
	return wanted
}

func (m *productMatcher) record(wanted map[string][]int, found map[string]int64, reason catalog.MatchReason) {
	for k, id := range found {
		for _, i := range wanted[k] {
			if _, done := m.matches[i]; !done {
				m.matches[i] = productMatch{id: id, reason: reason}
			}
		}
	}
}

func keysOf(m map[string][]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// lookupByColumn resolves an exact identifier column to product ids. The column
// name is chosen from a fixed set in the caller, never from input.
func lookupByColumn(ctx context.Context, tx pgx.Tx, orgID int64, column string, keys []string) (map[string]int64, error) {
	out := make(map[string]int64, len(keys))
	if len(keys) == 0 {
		return out, nil
	}

	const batchSize = 2000
	query := fmt.Sprintf(`
		SELECT lower(btrim(p.%s)) AS k, min(p.id) AS id
		FROM catalog.products p
		WHERE p.organization_id = $1
		  AND p.deleted_at IS NULL
		  AND btrim(p.%s) <> ''
		  AND lower(btrim(p.%s)) = ANY($2::text[])
		GROUP BY 1
	`, column, column, column)

	for start := 0; start < len(keys); start += batchSize {
		end := start + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		rows, err := tx.Query(ctx, query, orgID, keys[start:end])
		if err != nil {
			return nil, fmt.Errorf("catalog postgres: match products by %s: %w", column, err)
		}
		for rows.Next() {
			var key string
			var id int64
			if err := rows.Scan(&key, &id); err != nil {
				rows.Close()
				return nil, fmt.Errorf("catalog postgres: scan %s match: %w", column, err)
			}
			out[key] = id
		}
		rows.Close()
		if rows.Err() != nil {
			return nil, rows.Err()
		}
	}
	return out, nil
}

// lookupNameCandidates loads every catalogue row whose normalised Arabic name
// is one the caller asked about, with the attributes that tell same-named rows
// apart.
//
// One scan of the organisation's products, normalised and bucketed in Go. The
// alternative — a WHERE platform.normalize_arabic(name) = ANY($2) per batch —
// was measured at thirty seconds and over against fifty here, because it cannot
// use an index on a function of a JSONB field and re-normalises every row for
// every batch. It is also what made the tier fragile: the same expression run
// by Postgres and by Go had to agree, and a statement timeout on a large
// catalogue failed the whole import rather than the tier.
//
// It returns ALL the rows behind a name, not one. Which of them an incoming row
// means is a question about strengths and forms, and it is answered in Go by
// chooseByIdentity; a GROUP BY that returns min(id) answers it by accident.
func lookupNameCandidates(
	ctx context.Context, tx pgx.Tx, orgID int64, names []string,
) (map[string][]nameCandidate, error) {
	out := map[string][]nameCandidate{}
	if len(names) == 0 {
		return out, nil
	}

	wanted := make(map[string]bool, len(names))
	for _, raw := range names {
		if norm := arabic.Normalize(raw); norm != "" {
			wanted[norm] = true
		}
	}
	if len(wanted) == 0 {
		return out, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT p.id,
		       COALESCE(p.name->>'ar', ''),
		       COALESCE(p.name->>'en', ''),
		       COALESCE(p.concentration, ''),
		       COALESCE(p.dosage_form, ''),
		       COALESCE(p.unit, '')
		FROM catalog.products p
		WHERE p.organization_id = $1
		  AND p.deleted_at IS NULL
		  AND btrim(COALESCE(p.name->>'ar', '')) <> ''
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load name candidates: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c nameCandidate
		if err := rows.Scan(&c.id, &c.fields.NameAR, &c.fields.NameEN,
			&c.fields.Concentration, &c.fields.DosageForm, &c.fields.Unit); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan name candidate: %w", err)
		}
		norm := arabic.Normalize(c.fields.NameAR)
		if wanted[norm] {
			out[norm] = append(out[norm], c)
		}
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("catalog postgres: read name candidates: %w", rows.Err())
	}
	return out, nil
}

func queueInsert(batch *pgx.Batch, p *catalog.Product) {
	batch.Queue(insertProductQuery,
		p.OrganizationID, p.CategoryID, p.BrandID, p.BranchID, p.Name, p.Description,
		p.SKU, p.Barcode, p.Price, p.Discount, p.OldPrice, p.Image, p.ImageLink,
		string(insertStatus(p)), p.IsFeatured, p.DosageForm, p.ScientificName,
		p.Pharmacology, p.Active, p.Concentration, p.Unit, p.ManufacturingCompanies,
		p.InstitutionalWorkIDs,
	)
}
