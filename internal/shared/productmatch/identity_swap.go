package productmatch

// When the headers are right and the columns are not.
//
// A real pharmacy file in production has a column headed "اسم صنف الصيدلية"
// holding 20203380, 60202971, 10105879, and a column headed "كود SKU" holding
// "ليدى سبيد استيك 65 جرام". Someone's export swapped two columns and left the
// titles where they were. Every header rule in this package reads that file
// exactly backwards, and it is right to: the header is what a header says.
//
// The values disagree, and here they win. This is the one place where value
// evidence is allowed to overturn what the headers settled, and it is allowed
// only for this pair, only when both sides contradict their own header, and only
// when the contradiction is emphatic across the whole column. A file where the
// codes merely look wordy, or the names merely look numeric, is left alone.
//
// The correction works from the headers rather than from the bindings, because
// in the case it exists for there usually are no bindings: the name detector
// vetoes a column of digits and the code detector vetoes a column of sentences,
// so both columns come out of the resolver unclaimed and the file imports with
// no identity at all.
//
// It belongs in the shared engine rather than in one importer because nothing
// about it is specific to one. A swapped export is a property of the ERP that
// wrote the file, and every importer receives files from the same ERPs.

// swapEvidenceShare is how much of a column must contradict its header before a
// swap is believed.
//
// Four fifths, because the cost of being wrong is binding the name column to
// the item code — which is the very failure this file exists to undo, in the
// opposite direction.
const swapEvidenceShare = 0.80

// looksLikeCodes reports that a column holds identifiers rather than names.
func looksLikeCodes(s *shape) bool {
	if s == nil {
		return false
	}
	return s.fill > 0 && s.digits >= swapEvidenceShare && s.wordy <= 0.1 && s.avgRunes <= 24
}

// looksLikeNames reports that a column holds product names rather than codes.
func looksLikeNames(s *shape) bool {
	if s == nil {
		return false
	}
	return s.fill > 0 && s.wordy >= swapEvidenceShare && s.digits <= 0.2
}

// fixIdentitySwap exchanges the name and code columns when the values in both
// contradict their own headers.
//
// It records a note and a conflict as well as making the change: silently
// swapping two columns and saying nothing would be the same class of mistake as
// reading them the wrong way round.
func fixIdentitySwap(m *Mapping, shapes []*shape, headers []string) {
	nameCol := headerNamedColumn(headers, FieldName)
	if nameCol < 0 || nameCol >= len(shapes) {
		return
	}

	codeField, codeCol := FieldSKU, headerNamedColumn(headers, FieldSKU)
	if codeCol < 0 {
		codeField, codeCol = FieldBarcode, headerNamedColumn(headers, FieldBarcode)
	}
	if codeCol < 0 || codeCol >= len(shapes) || codeCol == nameCol {
		return
	}

	if !looksLikeCodes(shapes[nameCol]) || !looksLikeNames(shapes[codeCol]) {
		return
	}

	// Whatever either column was bound to is wrong by construction, so both
	// bindings are replaced rather than exchanged: the resolver may have left
	// one or both unclaimed, or given one to an unrelated field on value
	// evidence alone.
	release(m, nameCol)
	release(m, codeCol)

	bind(m, FieldName, codeCol)
	bind(m, codeField, nameCol)

	note := "عمودا «" + m.Columns[nameCol].Header + "» و«" + m.Columns[codeCol].Header +
		"» يبدو أن محتواهما متبادل: القيم تحت عنوان الاسم أكواد، والقيم تحت عنوان الكود أسماء أصناف. " +
		"تم تبديلهما تلقائياً — راجع الجدول للتأكد."
	m.Notes = append(m.Notes, Note{Severity: SeverityWarning, Message: note})
	m.Conflicts = append(m.Conflicts, Conflict{
		Kind:     ConflictHeaderValue,
		Field:    FieldName,
		Severity: SeverityWarning,
		Column:   nameCol,
		Message:  note,
	})
}

// release drops whatever field a column was bound to.
func release(m *Mapping, col int) {
	c := m.Columns[col]
	if c.Field == "" {
		return
	}
	delete(m.ByField, c.Field)
	c.Field = ""
	c.Score = 0
	c.Why = nil
}

// bind claims a column for a field, displacing any column already holding it.
func bind(m *Mapping, f Field, col int) {
	if prev, ok := m.ByField[f]; ok && prev != col {
		release(m, prev)
	}
	m.ByField[f] = col
	c := m.Columns[col]
	c.Field = f
	c.Source = SourceAuto
	c.Confidence = ConfidenceMedium
	c.Score = 0.6
	c.Why = []string{"تم تصحيح تبادل عمودي الاسم والكود بناءً على محتوى القيم"}
}

// headerNamedColumn finds the column whose header names a field, and means it.
//
// "Names it" has to be the header's *best* reading, not merely a possible one.
// A column headed "معرف الصنف (ID)" scores 27 for the product name, because it
// contains the word الصنف — and 69 for the platform's product id, which is what
// it actually says. Taking the first column with any name evidence picked that
// one, and the correction below then swapped the wrong pair.
func headerNamedColumn(headers []string, f Field) int {
	best, bestScore := -1, 0
	for i, h := range headers {
		evidence := headerEvidence(h)
		he := evidence[f]
		if he.Blocked || he.Score < scoreFloor {
			continue
		}
		outranked := false
		for other, oe := range evidence {
			if other != f && !oe.Blocked && oe.Score > he.Score {
				outranked = true
				break
			}
		}
		if outranked {
			continue
		}
		if he.Score > bestScore {
			best, bestScore = i, he.Score
		}
	}
	return best
}
