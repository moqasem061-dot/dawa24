package productmatch

// Value evidence for the commercial and stock fields.
//
// These are the columns that make an import dangerous. A misread name is
// visible to anyone who opens the catalogue afterwards; a discount column read
// as a price sells nine thousand products at thirty pounds each and nobody
// notices until the orders arrive.
//
// So the numeric detectors are deliberately blunt about what they can tell
// apart on their own — a price and a cost look identical in isolation — and the
// separation is left to the header and to the cross-column pass, which can see
// that one column is consistently smaller than another.

// detectMoney is the shared detector for every price-shaped field.
//
// It cannot distinguish a public price from a cost price, and does not pretend
// to: both are non-negative amounts with a wide spread. What it does is refuse
// the columns that are not money at all, and refuse the column that is plainly
// a percentage — which is the mistake that actually happens, because a discount
// column and a cheap-medicine price column both hold two-digit numbers.
func detectMoney(s *shape, _ *Vocabulary) verdict {
	if s.numeric < 0.7 {
		return verdict{veto: true, why: "العمود ليس أرقاماً ولا يمكن أن يكون سعراً"}
	}
	if s.negative > 0.05 {
		return verdict{veto: true, why: "العمود يحتوي قيماً سالبة ولا يصلح كسعر"}
	}
	if s.percent >= 0.5 {
		return verdict{veto: true, why: "القيم مكتوبة كنسب مئوية (%) وليست مبالغ"}
	}
	if s.dated >= 0.5 {
		return verdict{veto: true, why: "القيم تواريخ وليست مبالغ"}
	}
	if s.gtin >= 0.6 {
		return verdict{veto: true, why: "القيم بواركود دولية وليست أسعاراً"}
	}

	if s.percentBand() {
		// Every value between zero and a hundred, tightly clustered, and on a
		// whole or half step. That is a discount list, not a price list.
		return verdict{
			score: 0.15,
			why:   "جميع القيم بين 0 و 100 ومتقاربة — الشكل المعتاد لنسبة الخصم وليس للسعر",
		}
	}

	score := 0.35*ramp(s.spread, 1.5, 12) +
		0.25*ramp(s.hi98, 60, 600) +
		0.20*clamp(s.decimal*2) +
		0.20*s.numeric
	why := ""
	if score >= 0.6 {
		why = "قيم موجبة متفاوتة المدى بكسور عشرية — الشكل المعتاد للأسعار"
	}
	return verdict{score: clamp(score), why: why, decisive: score >= 0.6 && s.moneyBand()}
}

// detectDiscountPercent recognises a percentage column.
//
// This is where the headers stop helping. Four different real distributors
// label this column "القائمة", "المرجح", "جملة" and "مندوب" — the list, the
// weighted, wholesale, and representative. None of those words means discount.
// All four columns hold numbers between nine and fifty-nine, on half steps,
// beside a column that runs from nine to three thousand. That is the evidence,
// and it is conclusive where the header is silent.
func detectDiscountPercent(s *shape, _ *Vocabulary) verdict {
	if s.numeric < 0.8 {
		return verdict{veto: true, why: "العمود ليس أرقاماً ولا يمكن أن يكون نسبة خصم"}
	}
	if s.negative > 0.02 {
		return verdict{veto: true, why: "العمود يحتوي قيماً سالبة ولا يصلح كنسبة خصم"}
	}
	if s.pctBand < 0.9 {
		return verdict{veto: true, why: "قيم العمود تتجاوز 100 ولا يمكن أن تكون نسبة مئوية"}
	}
	if s.percent >= 0.5 {
		return verdict{score: 1, why: "القيم مكتوبة صراحةً كنسب مئوية (%)", decisive: true}
	}
	if s.percentBand() {
		return verdict{
			score:    0.92,
			why:      "جميع القيم بين 0 و 100، متقاربة، وعلى خطوات نصف صحيحة — نمط نسبة الخصم",
			decisive: true,
		}
	}
	// Inside the band but loosely spread: possible, not convincing. A cheap
	// product range prices like this too.
	return verdict{score: clamp(0.25 + 0.35*ramp(4-s.spread, 0, 3))}
}

// detectBonus recognises a quantity-offer column such as "1+1" or "10+2".
func detectBonus(s *shape, _ *Vocabulary) verdict {
	if s.bonus >= 0.5 {
		return verdict{score: clamp(0.7 + 0.3*s.bonus), why: "القيم عروض كمية بصيغة «1+1»", decisive: true}
	}
	if s.distinct > 40 {
		return verdict{score: 0.05}
	}
	return verdict{score: clamp(0.4 * s.bonus)}
}

// detectQuantity recognises a stock column.
//
// Stock is whole, non-negative, and — unlike every other whole-number column in
// a supplier file — contains zeros, because a price list that carries stock
// carries the items that ran out. That single signal separates it from an item
// code, which is also whole and also unique-ish but never zero.
func detectQuantity(s *shape, _ *Vocabulary) verdict {
	if s.numeric < 0.85 {
		return verdict{veto: true, why: "العمود ليس أرقاماً ولا يمكن أن يكون كمية"}
	}
	if s.decimal > 0.25 {
		return verdict{veto: true, why: "القيم تحتوي كسوراً عشرية ولا تصلح ككمية مخزون"}
	}
	if s.negative > 0.05 {
		return verdict{veto: true, why: "لا يمكن أن يكون الرصيد سالباً في أغلب الصفوف"}
	}
	if s.gtin >= 0.6 || (s.leadZero >= 0.3) {
		return verdict{veto: true, why: "القيم أكواد وليست كميات"}
	}
	if s.unique >= 0.95 && s.zero == 0 {
		// Distinct in every row and never zero: that is an identifier.
		return verdict{score: 0.12, why: "القيم فريدة في كل صف ولا تتكرر — الأرجح أنها كود وليست كمية"}
	}
	score := 0.30*s.integer +
		0.25*clamp(s.zero*6) +
		0.25*(1-s.unique) +
		0.20*ramp(s.hi98, 20, 500)
	why := ""
	if score >= 0.6 {
		why = "أعداد صحيحة موجبة متكررة تتضمن أصفاراً — الشكل المعتاد لرصيد المخزون"
	}
	return verdict{score: clamp(score), why: why, decisive: score >= 0.7 && s.zero > 0}
}

// detectSmallCount recognises the order and re-order thresholds: small whole
// numbers that barely vary, very often one single value repeated down the file.
func detectSmallCount(s *shape, _ *Vocabulary) verdict {
	if !s.countBand() {
		return verdict{veto: true, why: "القيم ليست أعداداً صحيحة موجبة"}
	}
	if s.hi98 > 5000 {
		return verdict{veto: true, why: "القيم أكبر من أن تكون حداً أدنى"}
	}
	// Almost no variety is the signature; a real quantity column varies.
	constancy := 1 - s.unique
	return verdict{score: clamp(0.55*constancy + 0.25*ramp(50-s.median, 0, 45) + 0.20*s.integer)}
}

// detectBatch recognises a lot-number column: short mixed alphanumerics, high
// but not total uniqueness, and — crucially — not a pure sequence, which is
// what an item code is.
func detectBatch(s *shape, _ *Vocabulary) verdict {
	if s.wordy >= 0.4 {
		return verdict{veto: true, why: "القيم جمل وليست أرقام تشغيلات"}
	}
	if s.avgRunes > 24 {
		return verdict{veto: true, why: "القيم أطول من أن تكون رقم تشغيلة"}
	}
	if s.dated >= 0.6 {
		return verdict{veto: true, why: "القيم تواريخ وليست أرقام تشغيلات"}
	}
	// A batch number usually mixes letters and digits, which is what separates
	// it from the item code sitting two columns to its left.
	mixed := clamp(s.codeish - s.digits)
	return verdict{score: clamp(0.45*mixed + 0.30*s.unique + 0.25*ramp(16-s.avgRunes, 2, 12))}
}

// detectExpiry recognises a date column.
//
// Bare Excel serials are accepted only weakly, because a quantity column of
// values in the forty-thousands parses as a date in 2009 and the arithmetic
// gives no hint that it should not. A written date does give that hint, and is
// scored accordingly.
func detectExpiry(s *shape, _ *Vocabulary) verdict {
	switch {
	case s.dated >= 0.8:
		return verdict{score: 1, why: "جميع القيم تواريخ مكتوبة بصيغة صالحة", decisive: true}
	case s.dated >= 0.5:
		return verdict{score: 0.8, why: "أغلب القيم تواريخ صالحة", decisive: true}
	case s.serial >= 0.8:
		return verdict{score: 0.55, why: "القيم أرقام تسلسلية لتواريخ Excel — تحتاج تأكيداً"}
	case s.dated >= 0.2:
		return verdict{score: 0.35}
	}
	return verdict{veto: true, why: "لا توجد قيم بصيغة تاريخ في هذا العمود"}
}

// detectWarehouse and detectBranch recognise a location column, preferring the
// names the vendor's own organisation already uses.
func detectWarehouse(s *shape, v *Vocabulary) verdict {
	return detectLocation(s, v.hit(s.p.Sample, v.Warehouses), "مخازن")
}

func detectBranch(s *shape, v *Vocabulary) verdict {
	return detectLocation(s, v.hit(s.p.Sample, v.Branches), "فروع")
}

func detectLocation(s *shape, known float64, noun string) verdict {
	if s.numeric >= 0.8 {
		return verdict{veto: true, why: "العمود أرقام ولا يصلح كاسم موقع"}
	}
	if known >= 0.5 {
		return verdict{score: clamp(0.7 + 0.3*known), why: "القيم أسماء " + noun + " مسجلة لدى المنشأة", decisive: true}
	}
	repetition := 1 - s.unique
	if repetition < 0.6 || s.distinct > 40 {
		return verdict{score: 0.05}
	}
	return verdict{score: clamp(0.5*repetition + 0.5*ramp(15-float64(s.distinct), 0, 13))}
}
