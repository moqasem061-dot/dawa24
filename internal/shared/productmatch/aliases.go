package productmatch

import (
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// WithAliases attaches curated and human-confirmed alias texts as a retrieval channel (US8 T111).
// It returns a shallow copy of the Index with aliases configured, ensuring the
// underlying cached Index is not mutated.
//
// Aliases act strictly as a retrieval hint: they pull candidate products into the
// evaluation pool, but the candidate must still pass through Gate.Evaluate.
func (idx *Index) WithAliases(aliases map[string][]int64) *Index {
	if idx == nil {
		return nil
	}
	clone := &Index{
		catalogRevision: idx.catalogRevision,
		products:        idx.products,
		byID:            idx.byID,
		byBarcode:       idx.byBarcode,
		bySKU:           idx.bySKU,
		byName:          idx.byName,
		tokens:          idx.tokens,
		keyTokens:       idx.keyTokens,
		df:              idx.df,
		makers:          idx.makers,
		makerKeys:       idx.makerKeys,
		total:           idx.total,
	}
	if len(aliases) == 0 {
		return clone
	}

	clone.aliases = make(map[string][]int64, len(aliases))
	clone.aliasVariants = make(map[string][]int64, len(aliases))

	for raw, ids := range aliases {
		if len(ids) == 0 {
			continue
		}
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		norm := sheet.NormalizeKey(trimmed)
		if norm != "" {
			clone.aliases[norm] = appendUniqueIDs(clone.aliases[norm], ids)
		}
		lower := strings.ToLower(trimmed)
		if lower != "" && lower != norm {
			clone.aliases[lower] = appendUniqueIDs(clone.aliases[lower], ids)
		}
		if vKey := variantKeyOf(norm); vKey != "" {
			clone.aliasVariants[vKey] = appendUniqueIDs(clone.aliasVariants[vKey], ids)
		}
	}
	return clone
}

// WithAliases returns an Index modifier that attaches aliases to an Index.
func WithAliases(aliases map[string][]int64) func(*Index) *Index {
	return func(idx *Index) *Index {
		return idx.WithAliases(aliases)
	}
}

// aliasCandidates gathers catalogue products referenced by in-scope aliases for the query.
func (idx *Index) aliasCandidates(q *query) []*MasterProduct {
	if idx == nil || len(idx.aliases) == 0 || q == nil {
		return nil
	}

	seenIDs := make(map[int64]bool)
	var out []*MasterProduct

	addByID := func(id int64) {
		if seenIDs[id] {
			return
		}
		seenIDs[id] = true
		if p, ok := idx.Lookup(id); ok && p != nil {
			out = append(out, p)
		}
	}

	lookupKeys := make([]string, 0, 8)

	// 1. Normalized raw query name
	if q.rawName != "" {
		if norm := sheet.NormalizeKey(q.rawName); norm != "" {
			lookupKeys = append(lookupKeys, norm)
		}
		if lower := strings.ToLower(strings.TrimSpace(q.rawName)); lower != "" {
			lookupKeys = append(lookupKeys, lower)
		}
	}

	// 2. Query nameKey
	if q.nameKey != "" {
		lookupKeys = append(lookupKeys, q.nameKey)
	}

	// 3. Row names if available
	if q.row != nil {
		if q.row.Name != "" {
			if norm := sheet.NormalizeKey(q.row.Name); norm != "" {
				lookupKeys = append(lookupKeys, norm)
			}
		}
		if q.row.NameEN != "" {
			if norm := sheet.NormalizeKey(q.row.NameEN); norm != "" {
				lookupKeys = append(lookupKeys, norm)
			}
		}
	}

	// 4. Token phrases / n-grams from query tokens (e.g. multi-word brand prefixes)
	toks := q.tokens
	if len(toks) > 8 {
		toks = toks[:8]
	}
	for i := 0; i < len(toks); i++ {
		for j := i + 1; j <= len(toks); j++ {
			phrase := strings.Join(toks[i:j], " ")
			lookupKeys = append(lookupKeys, phrase)
		}
	}

	// Check exact alias keys
	for _, k := range lookupKeys {
		if ids, ok := idx.aliases[k]; ok {
			for _, id := range ids {
				addByID(id)
			}
		}
	}

	// Check variant skeleton keys if variant map populated
	if len(idx.aliasVariants) > 0 {
		for _, k := range lookupKeys {
			if vKey := variantKeyOf(k); vKey != "" {
				if ids, ok := idx.aliasVariants[vKey]; ok {
					for _, id := range ids {
						addByID(id)
					}
				}
			}
		}
	}

	return out
}

func appendUniqueIDs(slice []int64, add []int64) []int64 {
	for _, a := range add {
		found := false
		for _, s := range slice {
			if s == a {
				found = true
				break
			}
		}
		if !found {
			slice = append(slice, a)
		}
	}
	return slice
}

// isAliasMatch reports whether p was retrieved via an in-scope alias for query q.
func (idx *Index) isAliasMatch(q *query, p *MasterProduct) bool {
	if idx == nil || len(idx.aliases) == 0 || q == nil || p == nil {
		return false
	}
	candidates := idx.aliasCandidates(q)
	for _, c := range candidates {
		if c != nil && c.ID == p.ID {
			return true
		}
	}
	return false
}
