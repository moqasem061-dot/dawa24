package productmatch

import (
	"fmt"
	"math/rand"
	"testing"
)

// What the engine has to survive.
//
// The stated target is a catalogue of 150,000 products and a file of 30,000
// rows. Nothing measured that before: the only stress test in the repository
// exercises a different engine entirely (compare/compare_30k_stress_test.go),
// so the cost of an import at this size was a guess.
//
// These are benchmarks rather than assertions on wall-clock time, because a
// test that fails on a slow CI box teaches nobody anything. Run them with
// -benchmem when changing retrieval or scoring; the allocation count per row is
// the number that matters, since it is what turns linear work into a garbage
// collection problem at thirty thousand rows.

// syntheticCatalogue builds a catalogue with the shape of the real one: a few
// thousand brands, each in several strengths and forms, names carrying the
// pharmaceutical furniture that real files carry.
func syntheticCatalogue(n int) []MasterProduct {
	rng := rand.New(rand.NewSource(1))

	brands := make([]string, 0, n/6+1)
	for len(brands) < cap(brands) {
		brands = append(brands, syntheticBrand(len(brands)))
	}
	strengths := []string{"5 مجم", "10 مجم", "20 مجم", "250 مجم", "500 مجم", "1 جم"}
	forms := []string{"أقراص", "كبسول", "شراب", "حقن", "كريم", "لبوس"}
	makers := []string{"فاركو", "إيبيكو", "الأمريكية", "ممفيس", "سيديكو", "جلاكسو"}

	out := make([]MasterProduct, 0, n)
	for i := 0; i < n; i++ {
		brand := brands[i%len(brands)]
		st := strengths[rng.Intn(len(strengths))]
		fm := forms[rng.Intn(len(forms))]
		out = append(out, MasterProduct{
			ID:            int64(i + 1),
			NameAR:        fmt.Sprintf("%s %s %s", brand, st, fm),
			NameEN:        fmt.Sprintf("%s %s", syntheticLatinBrand(i%len(brands)), fm),
			SKU:           fmt.Sprintf("SKU%06d", i),
			Barcode:       fmt.Sprintf("62%011d", i),
			DosageForm:    fm,
			Concentration: st,
			Manufacturer:  makers[rng.Intn(len(makers))],
		})
	}
	return out
}

// syntheticRows are what a supplier file looks like: products the catalogue
// actually carries, spelled differently, with noise appended and some brands
// misspelled by a letter.
//
// Drawn FROM the catalogue rather than assembled from the same word lists, and
// that is not a cosmetic change. The generator used to pick a brand, a strength
// and a form independently, so most of its rows named a strength no product of
// that brand was sold in — and the test then asserted that the engine matched
// four fifths of them. It was measuring how readily the scorer would settle for
// a product whose dose contradicted the row, which is the one thing this engine
// must never do. A supplier's file lists things that exist.
func syntheticRows(n int, catalogue []MasterProduct) []*Row {
	rng := rand.New(rand.NewSource(2))
	forms := map[string]string{
		"أقراص": "اقراص", "كبسول": "كبسولات", "شراب": "شراب",
		"حقن": "امبول", "كريم": "كريم", "لبوس": "لبوس",
	}

	out := make([]*Row, 0, n)
	for i := 0; i < n; i++ {
		p := catalogue[rng.Intn(len(catalogue))]
		brand := firstWord(p.NameAR)
		if i%4 == 0 {
			// One row in four carries the spelling variance this market
			// produces, so the benchmark exercises the variant channel rather
			// than only the exact one.
			brand = misspell(brand)
		}
		form := forms[p.DosageForm]
		if form == "" {
			form = p.DosageForm
		}
		out = append(out, &Row{
			Name: fmt.Sprintf("%s %s %s سعر جديد", brand, p.Concentration, form),
		})
	}
	return out
}

// firstWord is the brand at the head of a synthetic catalogue name.
func firstWord(name string) string {
	for i, r := range name {
		if r == ' ' {
			return name[:i]
		}
	}
	return name
}

// syntheticBrand builds a distinct Arabic brand name from an index.
//
// Letters, never digits, and that distinction is what makes this data honest.
// The first version of this generator numbered its brands — "بانادوليكس0",
// "بانادوليكس1" — and coreTokens strips digits from a token before indexing it,
// so all 150,000 products collapsed onto ONE identifying word. Every query then
// retrieved the whole catalogue. That is not a catalogue this engine would ever
// see, and benchmarking against it measures the generator rather than the code.
//
// It did earn its keep once: the collapse revealed that the crowded-postings
// guard exempted the first token consulted, so a single over-common word could
// pull the entire catalogue into one row. See takeInto.
func syntheticBrand(i int) string {
	const first = "بتجدرزسشصضطعغفقكلمنهو"
	f := []rune(first)
	return string([]rune{
		f[i%len(f)], 'ا', f[(i/len(f))%len(f)], 'و', f[(i/(len(f)*len(f)))%len(f)], 'ي', 'ن',
	})
}

// syntheticLatinBrand is the same brand transliterated, so the catalogue holds
// both names the way the real one does.
func syntheticLatinBrand(i int) string {
	const first = "btgdrzssdtaghfklmnhw"
	f := []rune(first)
	return string([]rune{
		f[i%len(f)], 'a', f[(i/len(f))%len(f)], 'o', f[(i/(len(f)*len(f)))%len(f)], 'i', 'n',
	})
}

// misspell drops a long vowel, which is the commonest way an Egyptian file
// disagrees with the catalogue about a brand name.
func misspell(brand string) string {
	runes := []rune(brand)
	out := make([]rune, 0, len(runes))
	dropped := false
	for _, r := range runes {
		if !dropped && r == 'و' {
			dropped = true
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func BenchmarkIndexBuild150k(b *testing.B) {
	products := syntheticCatalogue(150_000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		clone := make([]MasterProduct, len(products))
		copy(clone, products)
		_ = NewIndex(clone)
	}
}

func BenchmarkMatchRowAgainst150k(b *testing.B) {
	products := syntheticCatalogue(150_000)
	idx := NewIndex(products)
	rows := syntheticRows(1_000, products)
	opts := DefaultMatchOptions()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = idx.Match(rows[i%len(rows)], opts)
	}
}

// TestThirtyThousandRowsAgainstAHundredAndFiftyThousandProducts is the whole
// stated workload, run once, so a change that makes it quadratic is caught by
// `go test` rather than by a vendor watching a progress bar stop.
func TestThirtyThousandRowsAgainstAHundredAndFiftyThousandProducts(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test skipped in -short")
	}

	idx := NewIndex(syntheticCatalogue(150_000))
	rows := syntheticRows(30_000, syntheticCatalogue(150_000))
	opts := DefaultMatchOptions()

	var matched, unmatched int
	for _, row := range rows {
		if res := idx.Match(row, opts); res.Matched() {
			matched++
		} else {
			unmatched++
		}
	}

	// The assertion is about behaviour, not speed: at this size the engine must
	// still be resolving the great majority of rows. A retrieval change that
	// silently stops finding candidates shows up here as a collapse in the
	// match rate, which is the failure mode that would otherwise reach a
	// vendor as "the import matched nothing".
	if matched < len(rows)*8/10 {
		t.Errorf("matched %d of %d rows (%d unmatched); retrieval has regressed",
			matched, len(rows), unmatched)
	}
	t.Logf("matched %d of %d rows against 150,000 products", matched, len(rows))
}
