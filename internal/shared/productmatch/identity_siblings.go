package productmatch

// hasPlainSibling reports whether the index contains a plain sibling of product p.
// A plain sibling is another product that shares the same brand stem and has no
// line-extension modifier (e.g. Panadol 500mg is a plain sibling for Panadol Extra).
func (idx *Index) hasPlainSibling(p *MasterProduct) bool {
	if idx == nil || p == nil {
		return false
	}

	var stems []string
	if len(p.coreAR) > 0 {
		stems = append(stems, p.coreAR[0])
	} else if w := coreTokens(p.NameAR); len(w) > 0 {
		stems = append(stems, w[0])
	}
	if len(p.coreEN) > 0 {
		stems = append(stems, p.coreEN[0])
	} else if w := coreTokens(p.NameEN); len(w) > 0 {
		stems = append(stems, w[0])
	}

	for _, stem := range stems {
		candidates := idx.tokens[stem]
		for _, cand := range candidates {
			if cand.ID == p.ID {
				continue
			}

			// Must share the brand stem
			sharesStem := false
			if len(cand.coreAR) > 0 && cand.coreAR[0] == stem {
				sharesStem = true
			} else if len(cand.coreEN) > 0 && cand.coreEN[0] == stem {
				sharesStem = true
			} else {
				if w := coreTokens(cand.NameAR); len(w) > 0 && w[0] == stem {
					sharesStem = true
				} else if w := coreTokens(cand.NameEN); len(w) > 0 && w[0] == stem {
					sharesStem = true
				}
			}
			if !sharesStem {
				continue
			}

			// Plain sibling means candidate has NO modifier
			candMods := cand.mods
			if candMods == nil {
				candMods = modifiersIn(cand.NameAR + " " + cand.NameEN)
			}
			if len(candMods) == 0 {
				return true
			}
		}
	}

	return false
}

// HasPlainSibling exposes hasPlainSibling publicly.
func (idx *Index) HasPlainSibling(p *MasterProduct) bool {
	return idx.hasPlainSibling(p)
}

// annotateSiblings precomputes the plain sibling flag on all indexed products.
func (idx *Index) annotateSiblings() {
	if idx == nil {
		return
	}
	for _, p := range idx.products {
		p.hasPlainSibling = idx.hasPlainSibling(p)
	}
}
