package productmatch

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

var (
	packMultiplyPattern = regexp.MustCompile(`(?i)(\d{1,4})\s*[×x*X]\s*(\d{1,4})`)
	hugeNumberPattern   = regexp.MustCompile(`\d{13,}`)
	moneyAnnotationRe   = regexp.MustCompile(`(?i)(?:س\.?ج\.?|سعر|السعر|بسعر|جنيه|ج|س)\s*(\d+(?:\.\d+)?)|(\d+(?:\.\d+)?)\s*(?:س\.?ج\.?|جنيه|ج\b)`)
)

// ExtractRowFacts extracts structured identity evidence from a Row.
func ExtractRowFacts(row *Row, corrections []Correction) (facts IdentityFacts) {
	defer func() {
		if r := recover(); r != nil {
			facts.Cert = CertUncertain
		}
	}()

	if row == nil {
		facts.Cert = CertUncertain
		return facts
	}

	facts.Prov = ProvOriginal
	facts.Cert = CertExplicit

	rawName := strings.TrimSpace(row.Name)
	cleanName := StripTradeAnnotations(rawName)

	// Check if annotation contains price/money evidence
	if len(rawName) > len(cleanName) {
		tail := strings.TrimSpace(rawName[len(cleanName):])
		if matches := moneyAnnotationRe.FindAllString(tail, -1); len(matches) > 0 {
			for _, m := range matches {
				facts.MoneyEvidence = append(facts.MoneyEvidence, strings.TrimSpace(m))
			}
		}
	}

	extractNameFacts(&facts, cleanName)

	// Additional structured row fields
	if row.DosageForm != "" && facts.FormFamily == "" {
		facts.FormFamily = FormFamily(strings.ToLower(strings.TrimSpace(row.DosageForm)))
	}
	if facts.FormFamily == "" {
		if k := formKeyOf(cleanName); k != "" {
			facts.FormFamily = FormFamily(k)
		}
	}
	if row.Concentration != "" {
		parseStrengthText(&facts, row.Concentration)
	}
	if row.Scientific != "" {
		facts.Ingredients = append(facts.Ingredients, strings.TrimSpace(row.Scientific))
	}
	if row.Manufacturer != "" {
		facts.Manufacturer = strings.TrimSpace(row.Manufacturer)
	}

	if row.Barcode != "" {
		facts.Identifiers = append(facts.Identifiers, Identifier{
			Namespace: "gtin",
			Value:     strings.TrimSpace(row.Barcode),
			Valid:     true,
			Prov:      ProvOriginal,
			Cert:      CertExplicit,
		})
	}
	if row.SKU != "" {
		facts.Identifiers = append(facts.Identifiers, Identifier{
			Namespace: "supplier_code",
			Value:     strings.TrimSpace(row.SKU),
			Valid:     true,
			Prov:      ProvOriginal,
			Cert:      CertExplicit,
		})
	}

	applyCorrections(&facts, corrections)
	return facts
}

// Facts extracts structured identity evidence from a MasterProduct.
func (p *MasterProduct) Facts() (facts IdentityFacts) {
	defer func() {
		if r := recover(); r != nil {
			facts.Cert = CertUncertain
		}
	}()

	if p == nil {
		facts.Cert = CertUncertain
		return facts
	}
	if p.curatedFacts != nil {
		facts = *p.curatedFacts
		facts.HasPlainSibling = p.hasPlainSibling
		return facts
	}

	facts.Prov = ProvOriginal
	facts.Cert = CertExplicit

	name := p.NameAR
	if strings.TrimSpace(name) == "" {
		name = p.NameEN
	}
	extractNameFacts(&facts, name)

	if p.DosageForm != "" && facts.FormFamily == "" {
		facts.FormFamily = FormFamily(strings.ToLower(strings.TrimSpace(p.DosageForm)))
	}
	if facts.FormFamily == "" {
		if k := formKeyOf(name + " " + p.NameEN); k != "" {
			facts.FormFamily = FormFamily(k)
		}
	}
	if p.Concentration != "" {
		if facts.Concentration == nil {
			facts.Concentration = extractConcentrationRatio(p.Concentration)
		}
		parseStrengthText(&facts, p.Concentration)
	}
	if p.Scientific != "" {
		facts.Ingredients = append(facts.Ingredients, strings.TrimSpace(p.Scientific))
	}
	if p.Manufacturer != "" {
		facts.Manufacturer = strings.TrimSpace(p.Manufacturer)
	}

	if p.Barcode != "" {
		facts.Identifiers = append(facts.Identifiers, Identifier{
			Namespace: "gtin",
			Value:     strings.TrimSpace(p.Barcode),
			Valid:     true,
			Prov:      ProvOriginal,
			Cert:      CertExplicit,
		})
	}
	if p.SKU != "" {
		facts.Identifiers = append(facts.Identifiers, Identifier{
			Namespace: "catalog_code",
			Value:     strings.TrimSpace(p.SKU),
			Valid:     true,
			Prov:      ProvOriginal,
			Cert:      CertExplicit,
		})
	}

	facts.HasPlainSibling = p.hasPlainSibling
	return facts
}

func extractNameFacts(facts *IdentityFacts, text string) {
	if text == "" {
		return
	}

	// Guard against huge numbers or overflow
	if hugeNumberPattern.MatchString(text) {
		facts.Cert = CertUncertain
	}

	norm := sheet.NormalizeDigits(text)
	norm = FoldDoseText(norm)

	// Brand tokens
	tokens := strings.Fields(sheet.NormalizeName(text))
	for _, tok := range tokens {
		script := "en"
		for _, r := range tok {
			if unicode.Is(unicode.Arabic, r) {
				script = "ar"
				break
			}
		}
		facts.Brand = append(facts.Brand, Token{
			Text:   tok,
			Script: script,
		})
	}

	// Modifiers
	mods := modifiersIn(norm)
	if len(mods) > 0 {
		modList := make([]string, 0, len(mods))
		for m := range mods {
			modList = append(modList, m)
		}
		sort.Strings(modList)
		facts.Modifiers = modList
	}

	// Concentration check: e.g. 250 mg / 5 mL or 50 mg/mL
	if conc := extractConcentrationRatio(norm); conc != nil {
		facts.Concentration = conc
	}

	// Strengths: parse in written order
	parseStrengthText(facts, norm)

	// Pack extraction (check multi-pack: e.g. 2 × 10)
	if m := packMultiplyPattern.FindStringSubmatch(norm); len(m) == 3 {
		multCount, _ := strconv.Atoi(m[1])
		unitCount, _ := strconv.Atoi(m[2])

		// Bound check: multiplication factors <= 3
		factors := strings.Count(norm, "x") + strings.Count(norm, "X") + strings.Count(norm, "*") + strings.Count(norm, "×")
		cert := CertParsed
		if factors > 3 {
			cert = CertUncertain
		}

		facts.Pack = &PackFacts{
			Count:      multCount * unitCount,
			Multiplier: multCount,
			SubCount:   unitCount,
			SubUnit:    fmt.Sprintf("%d", unitCount),
			Prov:       ProvOriginal,
			Cert:       cert,
		}
	} else {
		// Single pack quantities
		q := readQuantities(norm)
		if len(q.counts) > 0 {
			facts.Pack = &PackFacts{
				Count:   int(q.counts[0].value),
				SubUnit: strings.TrimPrefix(q.counts[0].class, "\x00"),
				Prov:    ProvOriginal,
				Cert:    CertParsed,
			}
		} else if len(q.residual) == 1 && q.residual[0] > 0 && q.residual[0] <= 5000 {
			facts.Pack = &PackFacts{
				Count:   int(q.residual[0]),
				SubUnit: "unit",
				Prov:    ProvOriginal,
				Cert:    CertParsed,
			}
		}
	}

	// Letter check: e.g. بتنوفيت ان or betnovate n
	letter := extractDistinguishingLetter(norm)
	if letter != "" {
		facts.Letter = letter
	}
}

func parseStrengthText(facts *IdentityFacts, text string) {
	norm := sheet.NormalizeDigits(text)
	norm = FoldDoseText(norm)

	// Find combination like 5/10 mg or 10-160 mg
	locs := strengthPattern.FindAllStringIndex(norm, -1)
	if len(locs) == 0 {
		return
	}

	seen := make(map[string]bool)
	for _, s := range facts.Strengths {
		seen[s.Value+s.Unit] = true
	}
	compIdx := len(facts.Strengths)

	for _, loc := range locs {
		sub := norm[loc[0]:loc[1]]
		numParts := numericComponents(sub)
		unit := extractUnit(sub)

		for _, n := range numParts {
			valStr := strconv.FormatFloat(n, 'f', -1, 64)
			key := valStr + unit
			if seen[key] {
				continue
			}
			seen[key] = true

			cert := CertParsed
			if len(valStr) > 12 {
				cert = CertUncertain
			}

			facts.Strengths = append(facts.Strengths, Strength{
				Value:     valStr,
				Unit:      unit,
				Component: compIdx,
				Prov:      ProvOriginal,
				Cert:      cert,
			})
			compIdx++
		}
	}
}

func extractDistinguishingLetter(text string) string {
	words := strings.Fields(text)
	for _, w := range words {
		if len(w) == 1 {
			r := rune(w[0])
			if unicode.IsLetter(r) {
				return string(r)
			}
		}
		// Arabic letter words
		switch w {
		case "ان", "ن":
			return "n"
		case "سي", "س":
			return "c"
		case "دي", "د":
			return "d"
		}
	}
	return ""
}

func extractUnit(text string) string {
	t := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(t, "ميكروجرام") || strings.Contains(t, "مكجم") || strings.Contains(t, "mcg"):
		return "mcg"
	case strings.Contains(t, "مليجرام") || strings.Contains(t, "ملجرام") || strings.Contains(t, "ملجم") ||
		strings.Contains(t, "مجم") || strings.Contains(t, "محم") || strings.Contains(t, "مج") ||
		strings.Contains(t, "مغ") || strings.Contains(t, "mg"):
		return "mg"
	case strings.Contains(t, "جرام") || strings.Contains(t, "جم") || strings.Contains(t, "gm") || strings.Contains(t, "g"):
		return "g"
	case strings.Contains(t, "وحدة") || strings.Contains(t, "وحده") || strings.Contains(t, "iu"):
		return "iu"
	case strings.Contains(t, "%"):
		return "%"
	case strings.Contains(t, "مللي") || strings.Contains(t, "ملي") || strings.Contains(t, "مل") || strings.Contains(t, "ml"):
		return "ml"
	case strings.Contains(t, "لتر") || strings.Contains(t, "l"):
		return "l"
	}
	return ""
}

func normalizeUnit(u string) string {
	return extractUnit(u)
}

func applyCorrections(facts *IdentityFacts, corrections []Correction) {
	for _, c := range corrections {
		switch c.Attribute {
		case "strength":
			facts.Strengths = []Strength{
				{Value: c.Value, Prov: ProvCorrected, Cert: CertExplicit},
			}
		case "form":
			facts.FormFamily = FormFamily(c.Value)
			facts.Prov = ProvCorrected
		case "pack":
			count, _ := strconv.Atoi(c.Value)
			facts.Pack = &PackFacts{Count: count, Prov: ProvCorrected, Cert: CertExplicit}
		}
	}
}
