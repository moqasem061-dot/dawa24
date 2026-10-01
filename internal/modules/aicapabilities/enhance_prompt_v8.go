package aicapabilities

import (
	"strconv"
	"strings"
	"unicode"
)

// ItemV8 represents an item to be rendered in the v8 prompt format (contracts/ai-match-v8.md).
type ItemV8 struct {
	Ref     int
	Need    string // "product" or "pack"
	Raw     string
	Facts   string
	Options []int64
}

// CatalogEntryV8 represents a catalog entry in the v8 prompt format (contracts/ai-match-v8.md).
type CatalogEntryV8 struct {
	ID         int64
	AR         string
	EN         string
	Scientific string
	Form       string
	Route      string
	Release    string
	Strengths  string
	Content    string
	Pack       string
	Maker      string
}

// RenderEnhanceInputV8 builds the untrusted user message according to contracts/ai-match-v8.md.
// Rendering is a pure function: no clock, no map iteration, no randomness.
func RenderEnhanceInputV8(catalog []CatalogEntryV8, items []ItemV8) string {
	var b strings.Builder
	b.Grow(len(catalog)*120 + len(items)*150 + 256)

	b.WriteString("<<CATALOG pm8>>\nid|ar|en|scientific|form|route|release|strengths|content|pack|maker\n")
	for _, c := range catalog {
		b.WriteString(strconv.FormatInt(c.ID, 10))
		b.WriteByte('|')
		b.WriteString(escapeV8(c.AR))
		b.WriteByte('|')
		b.WriteString(escapeV8(c.EN))
		b.WriteByte('|')
		b.WriteString(escapeV8(c.Scientific))
		b.WriteByte('|')
		b.WriteString(escapeV8(c.Form))
		b.WriteByte('|')
		b.WriteString(escapeV8(c.Route))
		b.WriteByte('|')
		b.WriteString(escapeV8(c.Release))
		b.WriteByte('|')
		b.WriteString(escapeV8(c.Strengths))
		b.WriteByte('|')
		b.WriteString(escapeV8(c.Content))
		b.WriteByte('|')
		b.WriteString(escapeV8(c.Pack))
		b.WriteByte('|')
		b.WriteString(escapeV8(c.Maker))
		b.WriteByte('\n')
	}
	b.WriteString("<<END CATALOG pm8>>\n<<ITEMS pm8>>\nref|need|raw|facts|options\n")
	for _, it := range items {
		b.WriteString(strconv.Itoa(it.Ref))
		b.WriteByte('|')
		need := it.Need
		if need == "" {
			need = "product"
		}
		b.WriteString(escapeV8(need))
		b.WriteByte('|')
		b.WriteString(escapeV8(it.Raw))
		b.WriteByte('|')
		b.WriteString(escapeV8(it.Facts))
		b.WriteByte('|')
		opts := make([]string, len(it.Options))
		for i, id := range it.Options {
			opts[i] = strconv.FormatInt(id, 10)
		}
		b.WriteString(strings.Join(opts, ","))
		b.WriteByte('\n')
	}
	b.WriteString("<<END ITEMS pm8>>\n")

	return b.String()
}

// escapeV8 escapes values for v8 prompt rendering (contracts/ai-match-v8.md §Request).
// Escaping rules:
// - Strip control characters and bidi-override characters
// - Cap text length at 200 runes
// - Replace '<<' with '‹‹' and '>>' with '››'
// - Replace '|' with '¦'
// - Replace newlines (\r, \n, \t) with space
func escapeV8(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}

	var b strings.Builder
	b.Grow(len(s))
	runeCount := 0

	for _, r := range s {
		if runeCount >= 200 {
			break
		}
		// Strip control characters
		if unicode.IsControl(r) {
			b.WriteByte(' ')
			runeCount++
			continue
		}
		// Strip bidi override characters
		switch r {
		case '\u200E', '\u200F', '\u202A', '\u202B', '\u202C', '\u202D', '\u202E',
			'\u2066', '\u2067', '\u2068', '\u2069':
			continue
		}
		b.WriteRune(r)
		runeCount++
	}

	res := b.String()
	res = strings.ReplaceAll(res, "<<", "‹‹")
	res = strings.ReplaceAll(res, ">>", "››")
	res = strings.ReplaceAll(res, "|", "¦")
	res = strings.ReplaceAll(res, "\n", " ")
	res = strings.ReplaceAll(res, "\r", " ")
	res = strings.ReplaceAll(res, "\t", " ")

	res = strings.TrimSpace(res)
	if res == "" {
		return "-"
	}
	return res
}

// RenderEnhanceInput builds the user message in v8 prompt format from an EnhanceRequest.
// It is pure and deterministic: no clock, no map iteration, no randomness.
func RenderEnhanceInput(req EnhanceRequest) string {
	catV8 := make([]CatalogEntryV8, len(req.Catalog))
	for i, c := range req.Catalog {
		catV8[i] = CatalogEntryV8{
			ID:         c.ProductID,
			AR:         c.NameAR,
			EN:         c.NameEN,
			Scientific: c.Scientific,
			Form:       c.DosageForm,
			Strengths:  c.Concentration,
			Maker:      c.Manufacturer,
		}
	}

	itemsV8 := make([]ItemV8, len(req.Items))
	for i, it := range req.Items {
		var facts []string
		if it.Brand != "" {
			facts = append(facts, "brand="+it.Brand+"(o)")
		}
		if it.Strength != "" {
			facts = append(facts, "strength="+it.Strength+"(o)")
		}
		if it.DosageForm != "" {
			facts = append(facts, "form="+it.DosageForm+"(o)")
		}
		if it.PackSize > 0 {
			facts = append(facts, "pack="+strconv.Itoa(it.PackSize)+"(o)")
		}
		if it.Manufacturer != "" {
			facts = append(facts, "maker="+it.Manufacturer+"(o)")
		}
		if it.Scientific != "" {
			facts = append(facts, "scientific="+it.Scientific+"(o)")
		}
		factsStr := strings.Join(facts, ";")
		if factsStr == "" {
			factsStr = "-"
		}

		itemsV8[i] = ItemV8{
			Ref:     it.Ref,
			Need:    "product",
			Raw:     it.Text,
			Facts:   factsStr,
			Options: it.Options,
		}
	}

	return RenderEnhanceInputV8(catV8, itemsV8)
}
