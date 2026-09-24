package productmatch

import "strings"

// ShortBrandKey keys a name on its identity (brand, line extension, identity
// letter, doses) with brand skeletons as short as three consonants, so "Mobic
// 15mg" and "موبيك 15 مجم" agree.
//
// It is for pairing two supplier price lists row by row, never for the
// catalogue matcher: across a whole catalogue a three-consonant skeleton is
// shared by unrelated brands (see minVariantKeyRunes). Two guards make it safe
// here. A name that states no dose has no key, because the dose is what tells
// two short brands apart; and callers accept a key only when it identifies
// exactly one row on each side.
func ShortBrandKey(name string) string {
	key := identityKey(name, minShortBrandRunes)
	if !strings.Contains(key, "|d:") {
		return ""
	}
	return key
}
