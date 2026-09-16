package productmatch

import (
	"fmt"
	"sort"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// scorePairs evaluates every field against every column.
func scorePairs(headers []string, shapes []*shape, vocab *Vocabulary, fields *FieldSet) ([]pair, map[Field]map[int]string) {
	vetoes := map[Field]map[int]string{}
	var pairs []pair
	named := map[int]bool{}

	for col := range shapes {
		header := ""
		if col < len(headers) {
			header = headers[col]
		}
		evidence := headerEvidence(header)
		for _, he := range evidence {
			if he.Score >= scoreStrong && !he.Blocked {
				named[col] = true
				break
			}
		}

		for _, spec := range Specs {
			f := spec.Field
			if !fields.Allows(f) {
				continue
			}
			he := evidence[f]
			if he.Blocked {
				continue
			}
			v := valueEvidence(f, shapes[col], vocab)
			if v.veto {
				if vetoes[f] == nil {
					vetoes[f] = map[int]string{}
				}
				vetoes[f][col] = v.why
				// A veto unmaps the column — unless the header names this field
				// outright, in which case the file has told us what the column
				// is and the values are telling us some of its cells are wrong.
				//
				// Those are different problems with different remedies. A
				// column headed "السعر" holding one "راجع الادارة" among four
				// hundred prices is the price column with a bad cell, and the
				// row parser already rejects that row by number and says why.
				// Unmapping the column instead discards all four hundred prices
				// and reports nothing an admin can act on, which on a real file
				// is silent and total.
				if he.Score < scoreExact {
					continue
				}
				v = verdict{why: v.why}
			}
			_, hasDetector := detectors[f]
			score, why := combineScores(he.Score, v, hasDetector)
			if score < minPairScore {
				continue
			}
			// A column whose header clearly names a field is not available to a
			// field the header does not mention at all, however the values
			// profile. Both halves matter: the named field may still lose to
			// another field the header also suggests, because that is a genuine
			// ambiguity for the values to settle — but a field with no header
			// support whatsoever is not in that conversation.
			//
			// Two real files needed this. A column headed "كود الصنف" was bound
			// to `price` because its codes carried a trailing ".00" and so
			// profiled as money; the item code went unmapped and 453 rows whose
			// name cell was blank were rejected as having no identity. And a
			// column headed "معرف الصنف (ID)" was bound to `sku` because its
			// ids are short unique integers — the platform's own product id has
			// no value signature that distinguishes it from an item code, so
			// its header is the only witness there will ever be.
			//
			// When the named field is itself vetoed on evidence, the right
			// outcome is an unmapped column the user is asked about, not a
			// confident wrong binding.
			if named[col] && he.Score <= 0 {
				continue
			}
			pairs = append(pairs, pair{
				field:        f,
				column:       col,
				score:        score,
				why:          why,
				headerScore:  clamp(float64(he.Score) / 130),
				valueScore:   v.score,
				hasValue:     hasDetector,
				namedExactly: he.Score >= scoreExact,
			})
		}
	}

	// Deterministic order: best first, then leftmost column, then catalogue
	// order of the field. Two identically named columns therefore always
	// resolve the same way, run after run.
	fieldRank := map[Field]int{}
	for i, spec := range Specs {
		fieldRank[spec.Field] = i
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		if pairs[i].column != pairs[j].column {
			return pairs[i].column < pairs[j].column
		}
		return fieldRank[pairs[i].field] < fieldRank[pairs[j].field]
	})
	return pairs, vetoes
}

// assign walks the scored readings best first, claiming each field and each
// column once.
func assign(m *Mapping, pairs []pair) {
	takenCol := make(map[int]bool, len(m.Columns))
	for _, p := range pairs {
		if p.score < minBindScore {
			break // the slice is sorted, so nothing after this qualifies either
		}
		if _, done := m.ByField[p.field]; done || takenCol[p.column] {
			continue
		}
		m.ByField[p.field] = p.column
		takenCol[p.column] = true

		c := m.Columns[p.column]
		c.Field = p.field
		c.Score = p.score
		c.Confidence = confidenceOf(p.score)
		c.Source = SourceAuto
		c.Why = p.why
	}
}

// attachCandidates records the runner-up readings for every column, so the
// review screen's dropdown is ordered by evidence rather than alphabetically.
func attachCandidates(m *Mapping, pairs []pair) {
	byColumn := map[int][]Candidate{}
	for _, p := range pairs {
		byColumn[p.column] = append(byColumn[p.column], Candidate{
			Field: p.field,
			Label: p.field.Label(),
			Score: p.score,
			Why:   p.why,
		})
	}
	for col, cands := range byColumn {
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
		if len(cands) > 6 {
			cands = cands[:6]
		}
		m.Columns[col].Candidates = cands
	}
}

// previewOf takes a few real values from a column for the review screen.
func previewOf(p *sheet.ColumnProfile) []string {
	if p == nil {
		return nil
	}
	n := min(4, len(p.Sample))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, p.Sample[i])
	}
	return out
}

// flagConflicts records everything the vendor should look at.
func flagConflicts(m *Mapping, pairs []pair, vetoes map[Field]map[int]string, headers []string) {
	m.Conflicts = append(m.Conflicts, headerValueConflicts(m, pairs, vetoes)...)
	m.Conflicts = append(m.Conflicts, ambiguityConflicts(m, pairs)...)
	m.Conflicts = append(m.Conflicts, duplicateHeaderConflicts(headers)...)
	sort.SliceStable(m.Conflicts, func(i, j int) bool { return m.Conflicts[i].Column < m.Conflicts[j].Column })
}

// headerValueConflicts reports the columns whose title and contents disagree.
//
// Both directions matter. A column headed "السعر" whose values are all between
// zero and a hundred on half steps is a discount list mislabelled, and binding
// it as the price would publish nine thousand products at thirty pounds. A
// column headed "القائمة" whose values say discount is the same disagreement
// seen from the other side, and is the reason the values are allowed to win.
func headerValueConflicts(m *Mapping, pairs []pair, vetoes map[Field]map[int]string) []Conflict {
	var out []Conflict
	byPair := map[Field]map[int]pair{}
	for _, p := range pairs {
		if byPair[p.field] == nil {
			byPair[p.field] = map[int]pair{}
		}
		byPair[p.field][p.column] = p
	}

	for _, c := range m.Columns {
		if c.Field == "" || c.Header == "" {
			continue
		}
		p, ok := byPair[c.Field][c.Index]
		if !ok || !p.hasValue {
			continue
		}
		if p.headerScore >= 0.45 && p.valueScore <= 0.25 {
			out = append(out, Conflict{
				Kind:     ConflictHeaderValue,
				Field:    c.Field,
				Column:   c.Index,
				Severity: SeverityWarning,
				Message: fmt.Sprintf(
					"عنوان العمود «%s» يشير إلى «%s»، لكن القيم بداخله لا تطابق ذلك. راجع هذا العمود قبل المتابعة.",
					c.Header, c.Field.Label()),
			})
		}
		if p.headerScore < 0.2 && p.valueScore >= 0.7 {
			out = append(out, Conflict{
				Kind:     ConflictHeaderValue,
				Field:    c.Field,
				Column:   c.Index,
				Severity: SeverityInfo,
				Message: fmt.Sprintf(
					"عنوان العمود «%s» غير واضح، وتم تحديده كـ«%s» بناءً على محتوى الصفوف.",
					c.Header, c.Field.Label()),
			})
		}
	}

	// A field whose every candidate column was vetoed is worth saying out loud
	// when its header was promising.
	for field, cols := range vetoes {
		for col, why := range cols {
			if why == "" || col >= len(m.Columns) {
				continue
			}
			hdr := m.Columns[col].Header
			if hdr == "" {
				continue
			}
			if ev := headerEvidence(hdr)[field]; ev.Score >= scoreStrong {
				out = append(out, Conflict{
					Kind:     ConflictHeaderValue,
					Field:    field,
					Column:   col,
					Severity: SeverityWarning,
					Message: fmt.Sprintf(
						"العمود «%s» عنوانه يدل على «%s» لكن %s؛ لم يتم ربطه تلقائياً.",
						hdr, field.Label(), why),
				})
			}
		}
	}
	return out
}

// ambiguityConflicts reports bindings whose runner-up was almost as strong.
func ambiguityConflicts(m *Mapping, pairs []pair) []Conflict {
	var out []Conflict
	for _, c := range m.Columns {
		if c.Field == "" || len(c.Candidates) < 2 {
			continue
		}
		best, second := c.Candidates[0], c.Candidates[1]
		if best.Field != c.Field || best.Score-second.Score > 0.08 {
			continue
		}
		// A runner-up that is already bound to a better-scoring column is not a
		// competing reading of this one. Reporting those made the review screen
		// warn that "سعر الجمهور" might be the item code on every well-mapped
		// file in the corpus, which is the fastest way to teach a vendor to
		// click past warnings without reading them.
		if other, bound := m.ByField[second.Field]; bound && other != c.Index {
			continue
		}
		out = append(out, Conflict{
			Kind:     ConflictAmbiguous,
			Field:    c.Field,
			Column:   c.Index,
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"العمود «%s» يحتمل «%s» أو «%s» بنفس الدرجة تقريباً. اختر الصحيح يدوياً.",
				c.Header, best.Label, second.Label),
		})
	}
	return out
}

// duplicateHeaderConflicts reports two columns sharing a title, which is how a
// file that lists a price per branch confuses every importer that assumes
// headers are unique.
func duplicateHeaderConflicts(headers []string) []Conflict {
	seen := map[string]int{}
	var out []Conflict
	for i, h := range headers {
		key := sheet.NormalizeKey(h)
		if key == "" {
			continue
		}
		if first, dup := seen[key]; dup {
			out = append(out, Conflict{
				Kind:     ConflictDuplicateHeader,
				Column:   i,
				Severity: SeverityInfo,
				Message: fmt.Sprintf(
					"العمودان %d و %d يحملان نفس العنوان «%s»؛ تم التعامل معهما كعمودين مختلفين.",
					first+1, i+1, sheet.CleanCell(h)),
			})
			continue
		}
		seen[key] = i
	}
	return out
}
