package productmatch

import (
	"strings"
)

// Outcome represents a 4-valued attribute comparison verdict.
type Outcome string

const (
	OutcomeAgree     Outcome = "agree"
	OutcomeConflict  Outcome = "conflict"
	OutcomeUnstated  Outcome = "unstated"
	OutcomeUncertain Outcome = "uncertain"
)

// ConflictKind represents a specific category of identity conflict.
type ConflictKind string

const (
	ConflictStrength                ConflictKind = "strength"
	ConflictStrengthComponent       ConflictKind = "strength_component"
	ConflictCombination             ConflictKind = "combination"
	ConflictModifier                ConflictKind = "modifier"
	ConflictModifierUnstatedInRow   ConflictKind = "modifier_unstated_in_row"
	ConflictForm                    ConflictKind = "form"
	ConflictRoute                   ConflictKind = "route"
	ConflictRelease                 ConflictKind = "release"
	ConflictLetter                  ConflictKind = "letter"
	ConflictConcentration           ConflictKind = "concentration"
	ConflictDimension               ConflictKind = "dimension"
	ConflictBrand                   ConflictKind = "brand"
	ConflictIdentifierVsDescription ConflictKind = "identifier_vs_description"
	ConflictPack                    ConflictKind = "pack"
)

// Comparison holds the multi-attribute evaluation and list of hard conflicts.
type Comparison struct {
	Attributes map[string]Outcome `json:"attributes"`
	Hard       []ConflictKind     `json:"hard,omitempty"`
}

// CompareFacts compares row identity facts with product identity facts.
// Note: Pack conflict is evaluated in Attributes["pack"], but is NOT added
// to Hard here; the Gate decides that based on Required Granularity.
func CompareFacts(row, prod IdentityFacts) Comparison {
	comp := Comparison{
		Attributes: make(map[string]Outcome),
	}

	compareBrand(row, prod, &comp)
	compareStrength(row, prod, &comp)
	compareConcentration(row, prod, &comp)
	compareForm(row, prod, &comp)
	compareRelease(row, prod, &comp)
	compareModifiers(row, prod, &comp)
	compareLetter(row, prod, &comp)
	comparePack(row, prod, &comp)
	compareIdentifiers(row, prod, &comp)

	return comp
}

func compareBrand(row, prod IdentityFacts, comp *Comparison) {
	if len(row.Brand) == 0 || len(prod.Brand) == 0 {
		comp.Attributes["brand"] = OutcomeUnstated
		return
	}

	rowTokens := filterBrandTokens(row.Brand)
	prodTokens := filterBrandTokens(prod.Brand)

	if len(rowTokens) == 0 || len(prodTokens) == 0 {
		comp.Attributes["brand"] = OutcomeUnstated
		return
	}

	if len(rowTokens) == 1 && len(prodTokens) == 1 {
		if brandsMatch(rowTokens[0], prodTokens[0]) {
			comp.Attributes["brand"] = OutcomeAgree
		} else {
			comp.Attributes["brand"] = OutcomeConflict
			comp.Hard = append(comp.Hard, ConflictBrand)
		}
		return
	}

	joinedRow := strings.Join(rowTokens, " ")
	joinedProd := strings.Join(prodTokens, " ")
	if brandsMatch(joinedRow, joinedProd) {
		comp.Attributes["brand"] = OutcomeAgree
		return
	}

	matches := 0
	for _, rt := range rowTokens {
		matched := false
		for _, pt := range prodTokens {
			if brandsMatch(rt, pt) {
				matched = true
				break
			}
		}
		if matched {
			matches++
		}
	}

	if matches == len(rowTokens) || matches == len(prodTokens) {
		comp.Attributes["brand"] = OutcomeAgree
	} else {
		comp.Attributes["brand"] = OutcomeConflict
		comp.Hard = append(comp.Hard, ConflictBrand)
	}
}

func filterBrandTokens(tokens []Token) []string {
	var out []string
	for _, t := range tokens {
		txt := strings.ToLower(strings.TrimSpace(t.Text))
		if txt != "" && !isNoiseOrMakerWord(txt) {
			out = append(out, txt)
		}
	}
	return out
}

func isNoiseOrMakerWord(w string) bool {
	noise := map[string]bool{
		"شركة": true, "مستحضر": true, "دواء": true, "علاج": true,
		"co": true, "pharma": true, "lab": true, "inc": true,
	}
	return noise[w]
}

func brandsMatch(a, b string) bool {
	if a == b {
		return true
	}
	normA := foldAlef(a)
	normB := foldAlef(b)
	if normA == normB {
		return true
	}
	skA := Skeleton(a)
	skB := Skeleton(b)
	if skA != "" && skB != "" && skA == skB {
		return true
	}
	return false
}

func compareForm(row, prod IdentityFacts, comp *Comparison) {
	if row.FormFamily == "" || prod.FormFamily == "" {
		comp.Attributes["form"] = OutcomeUnstated
		return
	}

	normR := normalizeForm(string(row.FormFamily))
	normP := normalizeForm(string(prod.FormFamily))

	if normR == normP {
		comp.Attributes["form"] = OutcomeAgree
	} else {
		comp.Attributes["form"] = OutcomeConflict
		comp.Hard = append(comp.Hard, ConflictForm)
	}
}

func normalizeForm(f string) string {
	f = strings.ToLower(strings.TrimSpace(f))
	switch {
	case strings.Contains(f, "tablet") || strings.Contains(f, "tab") || strings.Contains(f, "قرص") || strings.Contains(f, "أقراص") || strings.Contains(f, "اقراص"):
		return "tablet"
	case strings.Contains(f, "capsule") || strings.Contains(f, "cap") || strings.Contains(f, "كبسول"):
		return "capsule"
	case strings.Contains(f, "cream") || strings.Contains(f, "كريم"):
		return "cream"
	case strings.Contains(f, "gel") || strings.Contains(f, "جيل") || strings.Contains(f, "جل"):
		return "gel"
	case strings.Contains(f, "ointment") || strings.Contains(f, "مرهم") || strings.Contains(f, "oint"):
		return "ointment"
	case strings.Contains(f, "syrup") || strings.Contains(f, "شراب") || strings.Contains(f, "susp") || strings.Contains(f, "معلق"):
		return "syrup"
	case strings.Contains(f, "drop") || strings.Contains(f, "نقط") || strings.Contains(f, "قطر"):
		return "drop"
	case strings.Contains(f, "inj") || strings.Contains(f, "حقن") || strings.Contains(f, "امبول") || strings.Contains(f, "ampoule") || strings.Contains(f, "vial") || strings.Contains(f, "فيال"):
		return "injection"
	default:
		return f
	}
}

func compareRelease(row, prod IdentityFacts, comp *Comparison) {
	hasRowRel := row.Release != "" || hasReleaseModifier(row.Modifiers)
	hasProdRel := prod.Release != "" || hasReleaseModifier(prod.Modifiers)

	if !hasRowRel && !hasProdRel {
		comp.Attributes["release"] = OutcomeUnstated
		return
	}

	if hasRowRel != hasProdRel {
		comp.Attributes["release"] = OutcomeConflict
		comp.Hard = append(comp.Hard, ConflictRelease)
		return
	}

	comp.Attributes["release"] = OutcomeAgree
}

func hasReleaseModifier(mods []string) bool {
	for _, m := range mods {
		if m == "sr" || m == "xr" || m == "mr" || m == "cr" || m == "retard" {
			return true
		}
	}
	return false
}

func compareModifiers(row, prod IdentityFacts, comp *Comparison) {
	if len(row.Modifiers) == 0 && len(prod.Modifiers) == 0 {
		comp.Attributes["modifier"] = OutcomeUnstated
		return
	}

	rowModMap := make(map[string]bool)
	for _, m := range row.Modifiers {
		rowModMap[m] = true
	}
	prodModMap := make(map[string]bool)
	for _, m := range prod.Modifiers {
		prodModMap[m] = true
	}

	conflict := false
	for m := range rowModMap {
		if !prodModMap[m] {
			conflict = true
			break
		}
	}

	if conflict {
		comp.Attributes["modifier"] = OutcomeConflict
		comp.Hard = append(comp.Hard, ConflictModifier)
		return
	}

	if len(row.Modifiers) == 0 && len(prod.Modifiers) > 0 {
		comp.Attributes["modifier"] = OutcomeUnstated
		return
	}

	comp.Attributes["modifier"] = OutcomeAgree
}

func compareLetter(row, prod IdentityFacts, comp *Comparison) {
	if row.Letter == "" && prod.Letter == "" {
		comp.Attributes["letter"] = OutcomeUnstated
		return
	}
	if row.Letter == "" || prod.Letter == "" {
		comp.Attributes["letter"] = OutcomeUnstated
		return
	}

	if strings.EqualFold(row.Letter, prod.Letter) {
		comp.Attributes["letter"] = OutcomeAgree
	} else {
		comp.Attributes["letter"] = OutcomeConflict
		comp.Hard = append(comp.Hard, ConflictLetter)
	}
}

func comparePack(row, prod IdentityFacts, comp *Comparison) {
	if row.Pack == nil && prod.Pack == nil {
		comp.Attributes["pack"] = OutcomeUnstated
		return
	}
	if row.Pack == nil || prod.Pack == nil {
		comp.Attributes["pack"] = OutcomeUnstated
		return
	}

	if row.Pack.Count == prod.Pack.Count {
		comp.Attributes["pack"] = OutcomeAgree
	} else {
		comp.Attributes["pack"] = OutcomeConflict
		// Note: ConflictPack is NOT added to comp.Hard here.
	}
}

func compareIdentifiers(row, prod IdentityFacts, comp *Comparison) {
	if len(row.Identifiers) == 0 || len(prod.Identifiers) == 0 {
		comp.Attributes["identifier"] = OutcomeUnstated
		return
	}

	hasMatch := false
	hasConflict := false
	for _, rID := range row.Identifiers {
		for _, pID := range prod.Identifiers {
			if rID.Namespace == pID.Namespace {
				if rID.Value == pID.Value {
					hasMatch = true
				} else {
					hasConflict = true
				}
			}
		}
	}

	if hasMatch {
		comp.Attributes["identifier"] = OutcomeAgree
	} else if hasConflict {
		comp.Attributes["identifier"] = OutcomeConflict
	} else {
		comp.Attributes["identifier"] = OutcomeUnstated
	}
}
