package erp

import "strconv"

// ParseShortID reads an id Dawa24 hands out as decimal text (a sync id, a
// variant id, a page cursor). It accepts 1 to 19 ASCII digits that fit an int64
// and are greater than zero; anything else, a sign included, reports false so
// the caller can fall back to the older UUID form or to not-found.
func ParseShortID(s string) (int64, bool) {
	if len(s) < 1 || len(s) > 19 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
