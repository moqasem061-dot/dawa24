// Labelled matching benchmarks.
//
// Everything else in this package measures how MUCH the engine matched.
// Nothing measured whether it matched the RIGHT thing, and those are different
// questions that pull in opposite directions: loosening a floor raises the
// settled rate and lowers the share of settled rows that are correct. A change
// argued from the settled rate alone is a change nobody can evaluate.
//
// So this file supplies labels — rows whose correct catalogue product is known
// independently of the matcher — and the harness that scores against them.
//
// Two label sources, because each answers a different question:
//
//   - CrossScriptLabels takes the 19,996-row administrative file whose supplier
//     names are Latin and whose catalogue entries are Arabic, blanks the
//     catalogue's English column so no answer can be read off it, and asks the
//     engine to bridge the two alphabets. The item-code column supplies the
//     truth and is then removed from the row.
//   - SiblingLabels renders a catalogue product's own Arabic name the way a
//     supplier writes it and asks the engine to find it again — among the
//     brand's other strengths, sizes and line extensions. That is the failure
//     this whole exercise is about: the right product is in the catalogue and a
//     sibling is chosen instead.
package corpus

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Labelled is one query whose correct answer is known.
type Labelled struct {
	Row *productmatch.Row
	// WantID is the catalogue product the row is, established outside the
	// matcher.
	WantID int64
	// Family is how many catalogue products share this one's brand stem. A
	// label with a family of one is easy by construction; the interesting
	// population is everything above it.
	Family int
}

// LoadSnapshot reads the catalogue snapshot as plain records.
//
// Separate from LoadIndex because the labelled benchmarks build several
// different indexes out of the same products — one with the English column
// blanked, one intact — and re-reading four megabytes of gzip for each is
// wasted work.
func LoadSnapshot() ([]productmatch.MasterProduct, error) {
	f, err := os.Open(filepath.Join(Dir, "catalogue.jsonl.gz"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()

	out := make([]productmatch.MasterProduct, 0, 20000)
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var p snapshotProduct
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			return nil, err
		}
		out = append(out, productmatch.MasterProduct{
			ID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN, SKU: p.SKU,
			Barcode: p.Barode, Scientific: p.Sci, DosageForm: p.Form,
			Concentration: p.Conc, Unit: p.Unit, Manufacturer: p.Maker,
			PublicPrice: p.Price,
		})
	}
	return out, sc.Err()
}

// bySKU keys the snapshot by its item code, which is what the administrative
// file carries and what makes its rows labelled.
func bySKU(products []productmatch.MasterProduct) map[string]int64 {
	out := make(map[string]int64, len(products))
	for i := range products {
		if k := sheet.NormalizeCode(products[i].SKU); k != "" {
			out[k] = products[i].ID
		}
	}
	return out
}

// familySizes counts how many catalogue products share each brand stem.
func familySizes(products []productmatch.MasterProduct) map[int64]int {
	stems := make(map[string][]int64, len(products))
	for i := range products {
		if stem := BrandStem(products[i].NameAR); stem != "" {
			stems[stem] = append(stems[stem], products[i].ID)
		}
	}
	out := make(map[int64]int, len(products))
	for _, ids := range stems {
		for _, id := range ids {
			out[id] = len(ids)
		}
	}
	return out
}

// BrandStem is the leading identifying word of a product name.
//
// One word rather than two: an Egyptian brand family writes the strength, the
// form and the pack count in every order imaginable, so the second token is
// not reliably part of the brand. It is a rough instrument and does not have to
// be sharp — it is used only to say which labels are hard.
func BrandStem(name string) string {
	toks := productmatch.DebugCoreTokens(name)
	if len(toks) == 0 {
		return ""
	}
	return toks[0]
}

// CrossScriptLabels reads the administrative file and pairs every row with the
// catalogue product its item code names.
func CrossScriptLabels(products []productmatch.MasterProduct) ([]Labelled, error) {
	codes := bySKU(products)
	family := familySizes(products)

	const file = "admin-198.xlsx"
	content, err := os.ReadFile(filepath.Join(Dir, "files", file))
	if err != nil {
		return nil, err
	}
	book, err := sheet.Open(content, file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = book.Close() }()

	analysis, err := productmatch.Analyze(book, nil)
	if err != nil {
		return nil, err
	}
	analysis.Complete()

	var out []Labelled
	_, err = productmatch.Process(book, analysis.Layout, analysis.Mapping,
		productmatch.DefaultProcessOptions(),
		func(batch []*productmatch.Row) error {
			for _, row := range batch {
				id, ok := codes[sheet.NormalizeCode(row.SKU)]
				if !ok {
					continue
				}
				// The identifier columns are cleared: they are the label, and
				// leaving them in the row would let the engine read the answer
				// off the question.
				row.SKU, row.Barcode = "", ""
				out = append(out, Labelled{Row: row, WantID: id, Family: family[id]})
			}
			return nil
		})
	return out, err
}

// BlankEnglish copies the catalogue with its English column removed.
//
// Without it the cross-script benchmark is not cross-script: the English name
// in the snapshot is character-for-character the supplier line, so every row
// would settle on an exact name match and the benchmark would measure string
// equality.
func BlankEnglish(products []productmatch.MasterProduct) []productmatch.MasterProduct {
	out := make([]productmatch.MasterProduct, len(products))
	copy(out, products)
	for i := range out {
		out[i].NameEN = ""
	}
	return out
}

// SiblingLabels renders each catalogue product the way a supplier writes it and
// asks the engine to find that product again.
//
// Only products in a brand family of two or more are included, because the
// question is which member of the family is chosen. A product nothing else
// resembles tests retrieval, not discrimination, and the corpus report already
// measures retrieval.
func SiblingLabels(products []productmatch.MasterProduct) []Labelled {
	family := familySizes(products)
	out := make([]Labelled, 0, len(products)/2)
	for i := range products {
		p := &products[i]
		if family[p.ID] < 2 || strings.TrimSpace(p.NameAR) == "" {
			continue
		}
		out = append(out, Labelled{
			Row:    &productmatch.Row{Name: supplierise(p.NameAR, i)},
			WantID: p.ID,
			Family: family[p.ID],
		})
	}
	return out
}

// supplierise rewrites a catalogue name the way an Egyptian price list writes
// it: noise appended, spacing lost, letters substituted.
//
// Deterministic in the product's own position so the benchmark is repeatable —
// a randomised corpus would move the number every run and prove nothing. The
// perturbations are drawn from what the live files actually do, and none of
// them removes an identifying attribute: the strength, the size and the line
// extension all survive, because the question being asked is whether the engine
// keeps them.
func supplierise(name string, seed int) string {
	s := strings.TrimSpace(name)
	switch seed % 5 {
	case 0:
		// Untouched. A share of every real file is written exactly as the
		// catalogue has it, and a benchmark without that case is not a
		// benchmark of a real file.
	case 1:
		s += " سعر جديد"
	case 2:
		s = glueUnits(s)
	case 3:
		s = strings.ReplaceAll(s, "ا", "أ")
	case 4:
		s = glueUnits(s) + " عرض"
	}
	return s
}

// glueUnits removes the space between a figure and the word after it, which is
// how most Egyptian price lists are typed: "500 مجم" becomes "500مجم".
func glueUnits(s string) string {
	words := strings.Fields(s)
	out := make([]string, 0, len(words))
	for i := 0; i < len(words); i++ {
		if i+1 < len(words) && isDigits(words[i]) {
			out = append(out, words[i]+words[i+1])
			i++
			continue
		}
		out = append(out, words[i])
	}
	return strings.Join(out, " ")
}

func isDigits(w string) bool {
	if w == "" {
		return false
	}
	for _, r := range w {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}
