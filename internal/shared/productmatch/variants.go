package productmatch

// Matching the same word spelled two ways.
//
// This is the failure the Egyptian market produces more than any other, and
// until now the scorer's strongest channel could not see it at all.
//
// weightedContainment — the measure that decides almost every match — compares
// tokens by exact equality against a map. "ابيكوبريد" and "ابيكوبرايد" are the
// same medicine written by two people, and to that map they are two unrelated
// strings: the shared word contributes nothing, and the comparison falls
// through to trigrams (discounted to 0.92) or to the whole-name skeleton
// (discounted to 0.86). A pair that should have been an obvious match arrives
// at the review screen at sixty-something percent, and the pharmacist has to
// confirm by hand what arithmetic could have settled.
//
// The fix is not a fuzzy comparison in the hot loop. A file of thirty thousand
// rows against a catalogue of a hundred and fifty thousand scores hundreds of
// millions of token pairs, and an edit distance inside that is not a slow
// version of this — it is a version that never finishes.
//
// So the folding happens once, at index time. Every token gets a variant key,
// and two spellings of one word get the SAME key, which means the hot loop
// stays a map lookup. What changes is what the map is keyed by.
//
// The key is the consonant skeleton (translit.go), which already folds exactly
// the things Egyptian spelling varies in:
//
//	long vowels written or omitted   بنادول    / بانادول    -> bndl
//	weak letters and hamza seats     اوجمنتين  / أوجمنتين   -> gmntn
//	the ث/س/ص/ش confusion            زيثروماكس / زيسروماكس  -> zsrmks
//	the ج/چ/غ and ق/ك/خ classes      جاسترو    / غاسترو     -> gstr
//	doubled letters                  ammox     / amox       -> mks
//
// and, because it folds both alphabets onto one, it also does something the
// per-token channel could not do before at any price: it lets an Arabic query
// token match a Latin catalogue token directly, in the primary channel, rather
// than through the discounted whole-name skeleton at the end.

// minVariantKeyRunes is the shortest consonant skeleton allowed to stand in for
// a word.
//
// Four, not the three the whole-name skeleton uses, and the difference is the
// whole safety argument. A skeleton discards every vowel, so short words
// collide freely: "دار" and "دور" both reduce to "dr", and so do "دير" and
// "درة". Three consonants is a coincidence; four is usually a word. Brand names
// short enough to be excluded by this are also short enough to be matched
// exactly, because there is less of them to misspell.
const minVariantKeyRunes = 4

// variantWeight is what a variant-key agreement is worth, as a fraction of what
// the same word matching exactly would be worth.
//
// Below one because a fold is weaker evidence than a spelling: two genuinely
// different brands can share a skeleton, while two identical strings cannot be
// a coincidence. Close to one because in this market the fold is usually right,
// and discounting it to the level of a trigram overlap would leave it deciding
// nothing that the trigram channel was not already deciding.
const variantWeight = 0.88

// variantKeyOf folds one token onto the key its other spellings share.
//
// Returns "" for a token too short to fold safely, which the callers treat as
// "this word has no variant key" rather than as a key of its own — an empty
// string matching every other empty string would make every short word match
// every other short word.
func variantKeyOf(token string) string {
	sk := Skeleton(token)
	if len([]rune(sk)) < minVariantKeyRunes {
		return ""
	}
	return sk
}

// variantKeys folds a token list, dropping the ones with no safe key.
//
// The result is deliberately NOT parallel to the input: a caller wanting to
// know which token produced which key wants the exact channel instead, and
// keeping a slice of blanks in step would only invite somebody to index into it.
func variantKeys(tokens []string) []string {
	if len(tokens) == 0 {
		return nil
	}
	out := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		key := variantKeyOf(t)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// tokenVariantKeys folds a token list KEEPING the alignment, so a caller
// holding token i can reach its key at index i.
//
// The opposite of variantKeys, and both are wanted: retrieval walks a
// de-duplicated key set, while comparison needs the key belonging to the
// particular word it is looking at. Returns nil when no token folds, so a
// product whose every word is short costs nothing to store.
func tokenVariantKeys(tokens []string) []string {
	if len(tokens) == 0 {
		return nil
	}
	out := make([]string, len(tokens))
	any := false
	for i, t := range tokens {
		out[i] = variantKeyOf(t)
		if out[i] != "" {
			any = true
		}
	}
	if !any {
		return nil
	}
	return out
}

// DebugVariantKey exposes one token's fold, for diagnostics and for the tests
// that pin the Egyptian spelling pairs this exists to resolve.
func DebugVariantKey(token string) string { return variantKeyOf(token) }
