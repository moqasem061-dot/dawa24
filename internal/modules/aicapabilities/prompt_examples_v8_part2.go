package aicapabilities

import (
	"encoding/json"
	"fmt"
	"strings"
)

var promptExamplesV8Part2 = []PromptExampleV8{
	{
		Category:    CategoryLineExtension,
		Description: "Modifiers like Day/Night define distinct identities",
		Catalog: []CatalogEntryV8{
			{
				ID:         909,
				AR:         "سليبكير داي 25 مجم",
				EN:         "sleepcare day 25mg",
				Scientific: "diphenhydramine",
				Form:       "tablet",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "25mg",
				Content:    "-",
				Pack:       "20 tab",
				Maker:      "CareCorp",
			},
			{
				ID:         910,
				AR:         "سليبكير نايت 25 مجم",
				EN:         "sleepcare night 25mg",
				Scientific: "diphenhydramine",
				Form:       "tablet",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "25mg",
				Content:    "-",
				Pack:       "20 tab",
				Maker:      "CareCorp",
			},
		},
		Item: ItemV8{
			Ref:     6,
			Need:    "product",
			Raw:     "سليبكير نايت 25 مجم أقراص",
			Facts:   "brand=سليبكير(o);mod=نايت(o);strength=25mg(o)",
			Options: []int64{909, 910},
		},
		SelectedID: ptrInt64(910),
		Evidence:   map[string]string{"st": "a", "fm": "a", "rt": "a", "rl": "a", "md": "a", "pk": "u", "br": "a"},
		Confidence: "hi",
	},
	{
		Category:    CategoryManufacturerTrap,
		Description: "Shared manufacturer is never evidence of identity; abstain if brand is absent",
		Catalog: []CatalogEntryV8{
			{
				ID:         911,
				AR:         "ألفاميد 500 مجم",
				EN:         "alfamed 500mg",
				Scientific: "amoxicillin",
				Form:       "capsule",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "500mg",
				Content:    "-",
				Pack:       "16 cap",
				Maker:      "DeltaLab",
			},
			{
				ID:         912,
				AR:         "هيموكير 500 مجم",
				EN:         "hemocure 500mg",
				Scientific: "tranexamic acid",
				Form:       "tablet",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "500mg",
				Content:    "-",
				Pack:       "20 tab",
				Maker:      "DeltaLab",
			},
		},
		Item: ItemV8{
			Ref:     7,
			Need:    "product",
			Raw:     "بيوجران 500 مجم دلتالاب",
			Facts:   "brand=بيوجران(o);strength=500mg(o);maker=دلتالاب(p)",
			Options: []int64{911, 912},
		},
		SelectedID: nil,
		Abstain:    "absent",
		Evidence:   map[string]string{"br": "c"},
		Confidence: "hi",
	},
	{
		Category:    CategoryPrefixTrap,
		Description: "Shared 4-5 letter prefix is not identity; distinct trade names must not be conflated",
		Catalog: []CatalogEntryV8{
			{
				ID:         913,
				AR:         "سيفالكس 500 مجم",
				EN:         "cephalex 500mg",
				Scientific: "cephalexin",
				Form:       "capsule",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "500mg",
				Content:    "-",
				Pack:       "12 cap",
				Maker:      "AlexPharma",
			},
		},
		Item: ItemV8{
			Ref:     8,
			Need:    "product",
			Raw:     "Cephalor 500mg caps",
			Facts:   "brand=cephalor(o);strength=500mg(o);form=capsule(o)",
			Options: []int64{913},
		},
		SelectedID: nil,
		Abstain:    "absent",
		Evidence:   map[string]string{"br": "c"},
		Confidence: "hi",
	},
	{
		Category:    CategoryUnstatedAmbiguous,
		Description: "When variants differ in attributes omitted by row, abstain with ambiguous",
		Catalog: []CatalogEntryV8{
			{
				ID:         914,
				AR:         "فيزيوكول قطرة للعين 15 مل",
				EN:         "physiocool eye drops 15ml",
				Scientific: "sodium chloride",
				Form:       "eye drops",
				Route:      "ophthalmic",
				Release:    "ordinary",
				Strengths:  "0.9%",
				Content:    "15ml",
				Pack:       "1 bottle",
				Maker:      "PhysioMed",
			},
			{
				ID:         915,
				AR:         "فيزيوكول قطرة للأذن 15 مل",
				EN:         "physiocool ear drops 15ml",
				Scientific: "sodium chloride",
				Form:       "ear drops",
				Route:      "otic",
				Release:    "ordinary",
				Strengths:  "0.9%",
				Content:    "15ml",
				Pack:       "1 bottle",
				Maker:      "PhysioMed",
			},
		},
		Item: ItemV8{
			Ref:     9,
			Need:    "product",
			Raw:     "Physiocool drops 15ml",
			Facts:   "brand=physiocool(o);form=drops(o);content=15ml(o)",
			Options: []int64{914, 915},
		},
		SelectedID: nil,
		Abstain:    "ambiguous",
		Evidence:   map[string]string{"rt": "u", "fm": "u"},
		Confidence: "hi",
	},
	{
		Category:    CategoryAbsentNoCandidate,
		Description: "Target medicine is completely absent from options; abstain with absent",
		Catalog: []CatalogEntryV8{
			{
				ID:         916,
				AR:         "سالبوتامول 100 ميكروجرام بخاخ",
				EN:         "salbutamol 100mcg inhaler",
				Scientific: "salbutamol",
				Form:       "inhaler",
				Route:      "inhalation",
				Release:    "ordinary",
				Strengths:  "100mcg",
				Content:    "200 doses",
				Pack:       "1 inhaler",
				Maker:      "AeroMed",
			},
			{
				ID:         917,
				AR:         "بوديسونيد 200 ميكروجرام",
				EN:         "budesonide 200mcg turbuhaler",
				Scientific: "budesonide",
				Form:       "turbuhaler",
				Route:      "inhalation",
				Release:    "ordinary",
				Strengths:  "200mcg",
				Content:    "100 doses",
				Pack:       "1 turbuhaler",
				Maker:      "PulmoCorp",
			},
		},
		Item: ItemV8{
			Ref:     10,
			Need:    "product",
			Raw:     "Fluticazole 50mcg nasal spray",
			Facts:   "brand=fluticazole(o);strength=50mcg(o);form=nasal spray(o)",
			Options: []int64{916, 917},
		},
		SelectedID: nil,
		Abstain:    "absent",
		Evidence:   map[string]string{"br": "c", "st": "c"},
		Confidence: "hi",
	},
}

// RenderExamplesV8 renders the synthetic examples into a compact text block for the system prompt.
func RenderExamplesV8(examples []PromptExampleV8) string {
	var b strings.Builder
	b.WriteString("# EXAMPLES\n")
	for i, ex := range examples {
		fmt.Fprintf(&b, "\n### Example %d: %s (%s)\n", i+1, ex.Description, ex.Category)
		b.WriteString(RenderEnhanceInputV8(ex.Catalog, []ItemV8{ex.Item}))
		b.WriteString("Response:\n")

		ans := map[string]any{
			"r": ex.Item.Ref,
			"s": ex.SelectedID,
			"e": ex.Evidence,
		}
		if ex.Abstain != "" {
			ans["x"] = ex.Abstain
		}
		if ex.Confidence != "" {
			ans["c"] = ex.Confidence
		}
		wrapper := map[string]any{"a": []any{ans}}
		outBytes, _ := json.Marshal(wrapper)
		b.Write(outBytes)
		b.WriteByte('\n')
	}
	return b.String()
}

func ptrInt64(i int64) *int64 {
	return &i
}
