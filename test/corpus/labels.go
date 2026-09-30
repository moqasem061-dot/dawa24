package corpus

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
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
	return LoadSnapshotDir(Dir)
}

// LoadSnapshotDir reads the catalogue snapshot from the specified directory.
// If not found, it falls back to test/corpus.
func LoadSnapshotDir(dir string) ([]productmatch.MasterProduct, error) {
	path := filepath.Join(dir, "catalogue.jsonl.gz")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fallback := filepath.Join("test", "corpus", "catalogue.jsonl.gz")
		if _, err2 := os.Stat(fallback); err2 == nil {
			path = fallback
		}
	}

	f, err := os.Open(path)
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
	path := filepath.Join(Dir, "files", file)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fallback := filepath.Join("test", "corpus", "files", file)
		if _, err2 := os.Stat(fallback); err2 == nil {
			path = fallback
		}
	}
	content, err := os.ReadFile(path)
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
func supplierise(name string, seed int) string {
	s := strings.TrimSpace(name)
	switch seed % 5 {
	case 0:
		// Untouched
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
