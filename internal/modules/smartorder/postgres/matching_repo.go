package postgres

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
)

func calcTrigrams(s string) []uint32 {
	r := []rune("  " + strings.ToLower(strings.TrimSpace(s)) + " ")
	if len(r) < 3 {
		return nil
	}
	t := make([]uint32, 0, len(r)-2)
	for i := 0; i <= len(r)-3; i++ {
		h := uint32(r[i])<<16 ^ uint32(r[i+1])<<8 ^ uint32(r[i+2])
		t = append(t, h)
	}
	sort.Slice(t, func(i, j int) bool { return t[i] < t[j] })
	kept := t[:1]
	for _, x := range t[1:] {
		if x != kept[len(kept)-1] {
			kept = append(kept, x)
		}
	}
	return kept
}

func calcTrigramSimilarity(a, b []uint32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	i, j, inter := 0, 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			inter++
			i++
			j++
		} else if a[i] < b[j] {
			i++
		} else {
			j++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// ResolveByFuzzyDB uses trigram similarity over the in-memory catalogue index to find products
// whose names are similar to the unresolved lines (> 0.45), without triggering PostgreSQL statement timeouts.
func (r *Repository) ResolveByFuzzyDB(ctx context.Context, names []string, matchLang string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}

	prods, err := r.LoadMatchIndex(ctx)
	if err != nil {
		return nil, err
	}

	type indexedProd struct {
		id       int64
		trigrams []uint32
	}

	indexed := make([]indexedProd, 0, len(prods))
	for _, p := range prods {
		nm := p.NameAR
		if matchLang == "en" || nm == "" {
			nm = p.NameEN
		}
		if matchLang != "en" {
			nm = arabic.Normalize(nm)
		}
		nm = strings.ToLower(strings.TrimSpace(nm))
		if tg := calcTrigrams(nm); len(tg) > 0 {
			indexed = append(indexed, indexedProd{id: p.ID, trigrams: tg})
		}
	}

	out := make(map[string]int64, len(names))
	ambiguous := make(map[string]bool)
	bestScore := make(map[string]float64)

	for _, raw := range names {
		q := raw
		if matchLang != "en" {
			q = arabic.Normalize(q)
		}
		q = strings.ToLower(strings.TrimSpace(q))
		qTrigrams := calcTrigrams(q)
		if len(qTrigrams) == 0 {
			continue
		}

		for _, p := range indexed {
			sim := calcTrigramSimilarity(qTrigrams, p.trigrams)
			if sim > 0.45 {
				prev := bestScore[raw]
				if sim > prev {
					bestScore[raw] = sim
					out[raw] = p.id
					delete(ambiguous, raw)
				} else if sim == prev && out[raw] != p.id {
					ambiguous[raw] = true
				}
			}
		}
	}

	for key := range ambiguous {
		delete(out, key)
	}
	return out, nil
}

// ResolveByContains matches lines where the line's name is contained within
// the catalogue name (or vice versa), provided the name is sufficiently
// specific (at least 6 characters) and matches uniquely to one product.
// Evaluated against in-memory catalogue index to eliminate slow cross-joins and statement timeouts.
func (r *Repository) ResolveByContains(ctx context.Context, names []string, matchLang string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}
	var filtered []string
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if len([]rune(n)) >= 6 {
			filtered = append(filtered, n)
		}
	}
	if len(filtered) == 0 {
		return map[string]int64{}, nil
	}

	prods, err := r.LoadMatchIndex(ctx)
	if err != nil {
		return nil, err
	}

	type catalogItem struct {
		id   int64
		name string
	}
	items := make([]catalogItem, 0, len(prods))
	for _, p := range prods {
		nm := p.NameAR
		if matchLang == "en" || nm == "" {
			nm = p.NameEN
		}
		if matchLang != "en" {
			nm = arabic.Normalize(nm)
		}
		nm = strings.ToLower(strings.TrimSpace(nm))
		if nm != "" {
			items = append(items, catalogItem{id: p.ID, name: nm})
		}
	}

	out := make(map[string]int64, len(filtered))
	ambiguous := make(map[string]bool)

	for _, raw := range filtered {
		q := raw
		if matchLang != "en" {
			q = arabic.Normalize(q)
		}
		q = strings.ToLower(strings.TrimSpace(q))
		qLen := len([]rune(q))
		if qLen < 6 {
			continue
		}

		for _, p := range items {
			pLen := len([]rune(p.name))
			if strings.Contains(p.name, q) || (strings.Contains(q, p.name) && pLen >= 6) {
				if existing, seen := out[raw]; seen && existing != p.id {
					ambiguous[raw] = true
				} else {
					out[raw] = p.id
				}
			}
		}
	}

	for key := range ambiguous {
		delete(out, key)
	}
	return out, nil
}

// SaveLearnedMapping records a buyer correction (FR-016, FR-040) into customer_product_mappings AND match_decisions.
func (r *Repository) SaveLearnedMapping(ctx context.Context, orgID int64, rawName string, productID int64) error {
	normName := strings.ToLower(strings.TrimSpace(rawName))
	decKey := "manual:" + normName
	var userID *int64
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
		userID = &actor.UserID
	}

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if !isDecisionMemoryEnabled(txCtx, tx) {
			return nil
		}
		// 1. Insert into catalog.customer_product_mappings
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.customer_product_mappings (
				organization_id, customer_org_id, raw_name, product_id, source, status, is_active, created_at, updated_at
			) VALUES ($1, $1, $2, $3, 'manual', 'processed', true, now(), now())
			ON CONFLICT DO NOTHING;`, orgID, rawName, productID)
		if err != nil {
			return err
		}

		// 2. Also record in catalog.match_decisions
		if normName != "" && productID > 0 {
			_, err = tx.Exec(txCtx, `
				INSERT INTO catalog.match_decisions (
					organization_id, user_id, decision_key, norm_name, chosen_product_id,
					confidence, reason, prompt_version, hit_count, created_at, last_used_at
				) VALUES (
					$1, $2, $3, $4, $5,
					1.000, 'قرار تصحيح يدوي من الطلب الذكي', 'manual:v1', 1, now(), now()
				)
				ON CONFLICT (COALESCE(organization_id, 0), decision_key)
				DO UPDATE SET
					chosen_product_id = EXCLUDED.chosen_product_id,
					confidence = 1.000,
					user_id = COALESCE(EXCLUDED.user_id, catalog.match_decisions.user_id),
					hit_count = catalog.match_decisions.hit_count + 1,
					last_used_at = now();
			`, orgID, userID, decKey, normName, productID)
		}
		return err
	})
}

func lowerAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }

func atoi(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// SaveAlias records a confirmed name for a catalogue product.
func (r *Repository) SaveAlias(ctx context.Context, productID int64, alias, source string, confidence float64) error {
	alias = strings.ToLower(strings.TrimSpace(alias))
	if alias == "" || productID <= 0 {
		return nil
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.product_aliases (product_id, alias, source, confidence)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (alias, product_id) DO NOTHING;`,
			productID, alias, source, confidence)
		return err
	})
}
