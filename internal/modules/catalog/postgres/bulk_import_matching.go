package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
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

// matchOrg applies the three tiers in strength order against one catalogue
// read.
//
// SKU and barcode are exact identifiers and settle a row outright. The name is
// a fallback that must never override one, so a row matched by an identifier is
// never reconsidered — and a name alone is not enough to make the decision, so
// what the name finds is a shortlist that the attributes choose from.
func (m *productMatcher) matchOrg(orgID int64, idxs []int) error {
	live, err := loadCatalogue(m.ctx, m.tx, orgID)
	if err != nil {
		return err
	}

	for _, i := range idxs {
		if _, done := m.matches[i]; done {
			continue
		}
		if id, reason, ok := resolveOne(live, m.prods[i]); ok {
			m.matches[i] = productMatch{id: id, reason: reason}
		}
	}
	return nil
}

// resolveOne decides which catalogue product one incoming row should update.
//
// The name tier is where the catalogue was being corrupted. It used to be a
// query grouping catalog.products by normalised name and taking min(id), which
// says every catalogue row sharing a name is one product. On this catalogue
// that is false in the most damaging way available: اوجمنتين 375 مجم and
// اوجمنتين 1 جم are two rows, two strengths, two medicines and one normalised
// name, so an import of either updated whichever had the lower id and the other
// strength was silently overwritten.
//
// Now the name retrieves candidates and productmatch.Discriminate — the same
// comparison the scorer and the AI adjudicator refuse pairs on — rules out the
// ones the row contradicts on dose, line extension or pharmaceutical form. A
// row whose strength nothing in the catalogue shares stays unmatched, which
// stages it as the new product it is.
func resolveOne(live *catalogue, p *catalog.Product) (int64, catalog.MatchReason, bool) {
	if sku := p.SKU; strings.TrimSpace(sku) != "" {
		if id, ok := live.lookupSKU(sku); ok {
			return id, catalog.MatchSKU, true
		}
	}
	if barcode := p.Barcode; strings.TrimSpace(barcode) != "" {
		if id, ok := live.lookupBarcode(barcode); ok {
			return id, catalog.MatchBarcode, true
		}
	}

	shortlist := live.candidatesFor(p.Name.Get(i18n.AR))
	if len(shortlist) == 0 {
		return 0, catalog.MatchNone, false
	}
	if id, ok := chooseByIdentity(shortlist, catalog.IdentityFields(p)); ok {
		return id, catalog.MatchName, true
	}
	return 0, catalog.MatchNone, false
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

func queueInsert(batch *pgx.Batch, p *catalog.Product) {
	batch.Queue(insertProductQuery,
		p.OrganizationID, p.CategoryID, p.BrandID, p.BranchID, p.Name, p.Description,
		p.SKU, p.Barcode, p.Price, p.Discount, p.OldPrice, p.Image, p.ImageLink,
		string(insertStatus(p)), p.IsFeatured, p.DosageForm, p.ScientificName,
		p.Pharmacology, p.Active, p.Concentration, p.Unit, p.ManufacturingCompanies,
		p.InstitutionalWorkIDs,
	)
}
