package productmatch

// Value evidence.
//
// One detector per field, each answering the same question about one column:
// given nothing but these values, how well could they be this field, and could
// they definitely not be?
//
// The veto is the important half. A score merely ranks; a veto forbids. It is
// what makes the engine safe to run on a badly labelled file, because the worst
// outcome of a misleading header is then an unmapped column the vendor is asked
// about — never a column of stock counts written into the price.

// verdict is one detector's answer for one column.
type verdict struct {
	// score is how well the values fit, from 0 (no evidence) through 0.5
	// (plausible) to 1 (unmistakable).
	score float64
	// veto forbids the binding outright, whatever the header claims.
	veto bool
	// decisive says the values identify this field on their own, so the binding
	// may be made with no header support at all. Only signatures nothing else in
	// a catalogue file shares earn it: a valid GS1 check digit, a URL, a written
	// date, a vocabulary of dosage forms, a column of percentages beside a
	// column of prices. A field whose values merely *could* be it — a small
	// whole number that might be a pack size or a discount — never does, because
	// binding those on shape alone is how an importer invents data.
	decisive bool
	// why explains the verdict in Arabic when it is worth showing. Only
	// vetoes and strong scores need one.
	why string
}

// detector measures one column against one field.
type detector func(s *shape, v *Vocabulary) verdict

// detectors is the registry. A field with no detector contributes no value
// evidence and is decided by its header alone, which is correct for the free
// text fields where the values carry no signature at all.
var detectors = map[Field]detector{
	FieldName:          detectName,
	FieldNameEN:        detectNameEN,
	FieldScientific:    detectScientific,
	FieldSKU:           detectSKU,
	FieldBarcode:       detectBarcode,
	FieldManufacturer:  detectManufacturer,
	FieldDosageForm:    detectDosageForm,
	FieldConcentration: detectConcentration,
	FieldUnit:          detectUnit,
	FieldPackSize:      detectPackSize,
	FieldCategory:      detectCategory,

	FieldPublicPrice: detectMoney,
	FieldPrice:       detectMoney,
	FieldNetPrice:    detectMoney,
	FieldCostPrice:   detectMoney,
	FieldDiscountPct: detectDiscountPercent,
	FieldDiscountAmt: detectMoney,
	FieldBonus:       detectBonus,

	FieldQuantity:     detectQuantity,
	FieldMinOrderQty:  detectSmallCount,
	FieldMinThreshold: detectSmallCount,
	FieldBatchNumber:  detectBatch,
	FieldExpiryDate:   detectExpiry,
	FieldWarehouse:    detectWarehouse,
	FieldBranch:       detectBranch,

	FieldStatus:     detectStatus,
	FieldNegotiable: detectBoolean,
	FieldImage:      detectImage,
}

// valueEvidence measures a column against a field, or reports no opinion.
func valueEvidence(f Field, s *shape, v *Vocabulary) verdict {
	if s.empty() {
		return verdict{}
	}
	d, ok := detectors[f]
	if !ok {
		return verdict{}
	}
	return d(s, v)
}

// --- identity -------------------------------------------------------------

// detectName recognises the product name column.
//
// A name is several words of letters, rarely repeated, and never a number. The
// last part is the veto that matters: in a file whose header order is
// discount / price / code / name, mislabelling the code column as the name
// produces a catalogue of numbered ghosts, and it is the only mistake here that
// cannot be noticed by eye afterwards.
func detectName(s *shape, _ *Vocabulary) verdict {
	if s.numeric >= 0.9 {
		return verdict{veto: true, why: "العمود أرقام فقط ولا يمكن أن يكون اسم صنف"}
	}
	if s.url >= 0.5 {
		return verdict{veto: true, why: "العمود روابط وليس أسماء"}
	}
	if s.avgRunes < 4 {
		return verdict{veto: true, why: "القيم أقصر من أن تكون أسماء أصناف"}
	}
	score := 0.40*s.wordy +
		0.25*ramp(s.avgRunes, 6, 22) +
		0.20*s.unique +
		0.15*clamp(s.arabic+s.latin)
	why := ""
	if score >= 0.6 {
		why = "القيم نصوص متعددة الكلمات وغير مكررة — الشكل المعتاد لأسماء الأصناف"
	}
	return verdict{score: clamp(score), why: why, decisive: score >= 0.6}
}

// detectNameEN is the name detector restricted to Latin text.
func detectNameEN(s *shape, _ *Vocabulary) verdict {
	if s.latin < 0.6 {
		return verdict{veto: true, why: "العمود ليس نصاً لاتينياً"}
	}
	return detectName(s, nil)
}

// detectScientific recognises an active-ingredient column: Latin words, more
// repetition than a trade name because many products share one molecule.
func detectScientific(s *shape, _ *Vocabulary) verdict {
	if s.numeric >= 0.8 {
		return verdict{veto: true, why: "العمود أرقام ولا يمكن أن يكون اسماً علمياً"}
	}
	if s.avgRunes < 4 {
		return verdict{veto: true}
	}
	// Repetition is the distinguishing signal, so a column of unique values
	// scores no better than plausible: the header has to carry that decision.
	repetition := 1 - s.unique
	return verdict{score: clamp(0.35*s.latin + 0.25*repetition + 0.2*ramp(s.avgRunes, 5, 20) + 0.2*s.wordy)}
}

// detectSKU recognises the supplier's own item code.
//
// The shape is: one token, short, almost always unique, usually digits. What it
// must not be is a barcode — those are handled by their own detector and would
// otherwise win here too, because a barcode is also a unique numeric token.
func detectSKU(s *shape, _ *Vocabulary) verdict {
	if s.wordy >= 0.4 {
		return verdict{veto: true, why: "القيم جمل وليست أكواداً"}
	}
	if s.avgRunes > 24 {
		return verdict{veto: true, why: "القيم أطول من أن تكون كود صنف"}
	}
	if s.gtin >= 0.7 {
		// Every value carries a valid GS1 check digit. That is a barcode, and
		// binding it as the item code hides the real one.
		return verdict{score: 0.15, why: "القيم بواركود دولية صالحة، والأرجح أنها عمود الباركود"}
	}
	score := 0.35*s.unique +
		0.25*clamp(s.codeish+s.digits) +
		0.20*ramp(12-s.avgRunes, 2, 10) +
		0.20*clamp(s.integer+s.leadZero)
	why := ""
	if score >= 0.65 {
		why = "قيم قصيرة وفريدة بلا مسافات — الشكل المعتاد لكود الصنف"
	}
	return verdict{score: clamp(score), why: why, decisive: score >= 0.7}
}

// detectBarcode recognises a GTIN column.
//
// The check digit is decisive and is the one place in this engine where a
// single signal is allowed to settle a binding on its own: thirteen-digit
// numbers that all satisfy the GS1 modulo-10 check are not coincidence.
func detectBarcode(s *shape, _ *Vocabulary) verdict {
	if s.digits < 0.5 {
		return verdict{veto: true, why: "العمود ليس أرقاماً متصلة ولا يمكن أن يكون باركود"}
	}
	length, share := s.p.DominantDigitLen()
	switch {
	case s.gtin >= 0.8:
		return verdict{score: 1, why: "جميع القيم بواركود دولية صالحة (رقم التحقق مطابق)", decisive: true}
	case s.gtin >= 0.5:
		return verdict{score: 0.85, why: "أغلب القيم بواركود دولية صالحة", decisive: true}
	case length >= 12 && length <= 14 && share >= 0.8:
		return verdict{score: 0.6, why: "القيم بطول الباركود الدولي لكن رقم التحقق غير مطابق"}
	case length >= 8 && length <= 14 && share >= 0.8:
		return verdict{score: 0.35}
	}
	return verdict{score: 0.1}
}

// detectManufacturer recognises a company column: short repeated names, and
// ideally names the catalogue already knows as brands.
func detectManufacturer(s *shape, v *Vocabulary) verdict {
	if s.numeric >= 0.8 {
		return verdict{veto: true, why: "العمود أرقام ولا يمكن أن يكون اسم شركة"}
	}
	if known := v.hit(s.p.Sample, v.Brands); known >= 0.5 {
		return verdict{score: clamp(0.7 + 0.3*known), why: "أغلب القيم أسماء شركات مسجلة بالفعل في الكتالوج", decisive: true}
	}
	// Many products, few manufacturers: repetition is the signature.
	repetition := 1 - s.unique
	if repetition < 0.3 {
		return verdict{score: 0.15}
	}
	return verdict{score: clamp(0.45*repetition + 0.3*ramp(s.avgRunes, 4, 25) + 0.25*clamp(s.arabic+s.latin))}
}

// detectDosageForm recognises a pharmaceutical form column by its vocabulary.
func detectDosageForm(s *shape, _ *Vocabulary) verdict {
	hit := vocabRate(s.p.Sample, formWords)
	if hit >= 0.6 && s.distinct <= 60 {
		return verdict{score: clamp(0.6 + 0.4*hit), why: "القيم أشكال صيدلية معروفة (أقراص، شراب، كريم…)", decisive: true}
	}
	if s.numeric >= 0.8 {
		return verdict{veto: true, why: "العمود أرقام ولا يمكن أن يكون شكلاً صيدلياً"}
	}
	if s.distinct > 100 {
		return verdict{score: 0.05, why: "عدد القيم المختلفة أكبر من أن يكون قائمة أشكال صيدلية"}
	}
	return verdict{score: clamp(0.3*hit + 0.2*ramp(40-float64(s.distinct), 0, 35))}
}

// detectConcentration recognises a strength column: a number welded to a dose
// unit, which is a shape nothing else in a catalogue file has.
func detectConcentration(s *shape, _ *Vocabulary) verdict {
	if s.concentration >= 0.6 {
		return verdict{score: clamp(0.65 + 0.35*s.concentration), why: "القيم أرقام مقترنة بوحدات جرعة (مجم، مل، وحدة دولية)", decisive: true}
	}
	if s.wordy >= 0.6 {
		return verdict{veto: true, why: "القيم جمل كاملة وليست تركيزات"}
	}
	return verdict{score: clamp(0.5 * s.concentration)}
}

// detectUnit recognises a packaging-unit column by its vocabulary and by how
// few distinct values it holds.
func detectUnit(s *shape, _ *Vocabulary) verdict {
	hit := vocabRate(s.p.Sample, unitWords)
	if hit >= 0.6 && s.distinct <= 40 {
		return verdict{score: clamp(0.6 + 0.4*hit), why: "القيم وحدات بيع معروفة (علبة، شريط، زجاجة…)", decisive: true}
	}
	if s.numeric >= 0.8 {
		return verdict{veto: true, why: "العمود أرقام ولا يمكن أن يكون وحدة بيع"}
	}
	if s.distinct > 80 || s.avgRunes > 20 {
		return verdict{score: 0.05}
	}
	return verdict{score: clamp(0.35*hit + 0.25*ramp(25-float64(s.distinct), 0, 22))}
}

// detectPackSize recognises the count inside one package: a small whole number
// repeated across many products.
func detectPackSize(s *shape, _ *Vocabulary) verdict {
	if !s.countBand() {
		return verdict{veto: true, why: "القيم ليست أعداداً صحيحة"}
	}
	if s.hi98 > 1000 {
		return verdict{score: 0.05, why: "القيم أكبر من أن تكون عدد وحدات داخل العبوة"}
	}
	return verdict{score: clamp(0.5*s.packish + 0.3*(1-s.unique) + 0.2*ramp(60-s.median, 0, 55))}
}

// detectCategory recognises a grouping column: very few distinct text values
// spread over many rows, ideally ones the catalogue already knows.
func detectCategory(s *shape, v *Vocabulary) verdict {
	if s.numeric >= 0.8 {
		return verdict{veto: true, why: "العمود أرقام ولا يمكن أن يكون تصنيفاً"}
	}
	if known := v.hit(s.p.Sample, v.Categories); known >= 0.5 {
		return verdict{score: clamp(0.65 + 0.35*known), why: "أغلب القيم تصنيفات موجودة بالفعل في الكتالوج", decisive: true}
	}
	repetition := 1 - s.unique
	if repetition < 0.5 || s.distinct > 120 {
		return verdict{score: 0.08}
	}
	return verdict{score: clamp(0.55*repetition + 0.45*ramp(40-float64(s.distinct), 0, 38))}
}

// --- attributes -----------------------------------------------------------

// detectStatus recognises a lifecycle column: a handful of known state words.
func detectStatus(s *shape, _ *Vocabulary) verdict {
	if s.distinct > 8 {
		return verdict{veto: true, why: "عدد القيم المختلفة أكبر من أن يكون حالة"}
	}
	hit := vocabRate(s.p.Sample, statusWords)
	if hit >= 0.7 {
		return verdict{score: clamp(0.6 + 0.4*hit), why: "القيم كلمات حالة معروفة", decisive: true}
	}
	return verdict{score: clamp(0.5 * hit)}
}

// detectBoolean recognises a yes/no column.
func detectBoolean(s *shape, _ *Vocabulary) verdict {
	if s.distinct > 4 {
		return verdict{veto: true, why: "العمود يحتوي أكثر من قيمتين ولا يصلح كحقل نعم/لا"}
	}
	if s.boolean >= 0.8 {
		return verdict{score: 0.9, why: "القيم نعم/لا فقط"}
	}
	return verdict{score: clamp(0.6 * s.boolean)}
}

// detectImage recognises a URL column.
func detectImage(s *shape, _ *Vocabulary) verdict {
	if s.url >= 0.8 {
		return verdict{score: 1, why: "جميع القيم روابط مباشرة", decisive: true}
	}
	if s.url >= 0.3 {
		return verdict{score: 0.6}
	}
	return verdict{veto: true, why: "القيم ليست روابط"}
}
