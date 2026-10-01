package aicapabilities

// PromptExampleCategory identifies which FR-072 edge case the example illustrates.
type PromptExampleCategory string

const (
	CategoryArabicShorthand     PromptExampleCategory = "arabic_shorthand"
	CategoryCrossScriptNames    PromptExampleCategory = "cross_script_names"
	CategoryStrengthDistinction PromptExampleCategory = "strength_distinction"
	CategoryFormDistinction     PromptExampleCategory = "form_distinction"
	CategoryPackDistinction     PromptExampleCategory = "pack_distinction"
	CategoryLineExtension       PromptExampleCategory = "line_extension"
	CategoryManufacturerTrap    PromptExampleCategory = "manufacturer_trap"
	CategoryPrefixTrap          PromptExampleCategory = "prefix_trap"
	CategoryUnstatedAmbiguous   PromptExampleCategory = "unstated_ambiguous"
	CategoryAbsentNoCandidate   PromptExampleCategory = "absent_no_candidate"
)

// PromptExampleV8 represents one normative few-shot example for prompt v8.
type PromptExampleV8 struct {
	Category    PromptExampleCategory
	Description string
	Catalog     []CatalogEntryV8
	Item        ItemV8
	SelectedID  *int64
	Abstain     string
	Evidence    map[string]string
	Confidence  string
}

// PromptExamplesV8 defines the 10 normative synthetic examples covering all FR-072 categories.
// Invariant: None of the names or raw texts in these examples may appear in adversarial or held-out sets.
var PromptExamplesV8 = append(promptExamplesV8Part1, promptExamplesV8Part2...)

var promptExamplesV8Part1 = []PromptExampleV8{
	{
		Category:    CategoryArabicShorthand,
		Description: "Egyptian commercial shorthand with price noise, packs, and colloquial words",
		Catalog: []CatalogEntryV8{
			{
				ID:         901,
				AR:         "كيروفلو اكسترا",
				EN:         "kiroflu extra",
				Scientific: "paracetamol+caffeine",
				Form:       "tablet",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "500mg;65mg",
				Content:    "-",
				Pack:       "20 tab",
				Maker:      "KiroPharma",
			},
		},
		Item: ItemV8{
			Ref:     1,
			Need:    "product",
			Raw:     "كيروفلو اكسترا 20 ق س.ج 45ج",
			Facts:   "brand=كيروفلو(o);mod=extra(o);pack=20 tab(p);price=45(o,money)",
			Options: []int64{901},
		},
		SelectedID: ptrInt64(901),
		Evidence:   map[string]string{"st": "a", "fm": "a", "rt": "a", "rl": "a", "md": "a", "pk": "a", "br": "a"},
		Confidence: "hi",
	},
	{
		Category:    CategoryCrossScriptNames,
		Description: "Latin raw trade name matching Arabic catalog record",
		Catalog: []CatalogEntryV8{
			{
				ID:         902,
				AR:         "سولفادكس 200 مجم كبسولات",
				EN:         "solvadex 200mg capsules",
				Scientific: "sulbactam",
				Form:       "capsule",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "200mg",
				Content:    "-",
				Pack:       "10 cap",
				Maker:      "SolvaEgypt",
			},
		},
		Item: ItemV8{
			Ref:     2,
			Need:    "product",
			Raw:     "Solvadex 200mg capsules",
			Facts:   "brand=solvadex(o);strength=200mg(o);form=capsule(o)",
			Options: []int64{902},
		},
		SelectedID: ptrInt64(902),
		Evidence:   map[string]string{"st": "a", "fm": "a", "rt": "a", "rl": "a", "md": "a", "pk": "u", "br": "a"},
		Confidence: "hi",
	},
	{
		Category:    CategoryStrengthDistinction,
		Description: "Hard strength exclusion; do not round or pick the nearest strength",
		Catalog: []CatalogEntryV8{
			{
				ID:         903,
				AR:         "ماكسيبريل 5 مجم",
				EN:         "maxipril 5mg",
				Scientific: "enalapril",
				Form:       "tablet",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "5mg",
				Content:    "-",
				Pack:       "20 tab",
				Maker:      "DeltaMed",
			},
			{
				ID:         904,
				AR:         "ماكسيبريل 10 مجم",
				EN:         "maxipril 10mg",
				Scientific: "enalapril",
				Form:       "tablet",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "10mg",
				Content:    "-",
				Pack:       "20 tab",
				Maker:      "DeltaMed",
			},
		},
		Item: ItemV8{
			Ref:     3,
			Need:    "product",
			Raw:     "Maxipril 10mg tab",
			Facts:   "brand=maxipril(o);strength=10mg(o);form=tab(o)",
			Options: []int64{903, 904},
		},
		SelectedID: ptrInt64(904),
		Evidence:   map[string]string{"st": "a", "fm": "a", "rt": "a", "rl": "a", "md": "a", "pk": "u", "br": "a"},
		Confidence: "hi",
	},
	{
		Category:    CategoryFormDistinction,
		Description: "Topical form distinction: cream is not ointment",
		Catalog: []CatalogEntryV8{
			{
				ID:         905,
				AR:         "درماجلوب 1% كريم 30 جم",
				EN:         "dermaglobe 1% cream 30g",
				Scientific: "hydrocortisone",
				Form:       "cream",
				Route:      "topical",
				Release:    "ordinary",
				Strengths:  "1%",
				Content:    "30g",
				Pack:       "1 tube",
				Maker:      "GlobePharma",
			},
			{
				ID:         906,
				AR:         "درماجلوب 1% مرهم 30 جم",
				EN:         "dermaglobe 1% ointment 30g",
				Scientific: "hydrocortisone",
				Form:       "ointment",
				Route:      "topical",
				Release:    "ordinary",
				Strengths:  "1%",
				Content:    "30g",
				Pack:       "1 tube",
				Maker:      "GlobePharma",
			},
		},
		Item: ItemV8{
			Ref:     4,
			Need:    "product",
			Raw:     "Dermaglobe 1% ointment 30g",
			Facts:   "brand=dermaglobe(o);strength=1%(o);form=ointment(o);content=30g(o)",
			Options: []int64{905, 906},
		},
		SelectedID: ptrInt64(906),
		Evidence:   map[string]string{"st": "a", "fm": "a", "rt": "a", "rl": "a", "md": "a", "pk": "a", "br": "a"},
		Confidence: "hi",
	},
	{
		Category:    CategoryPackDistinction,
		Description: "Pack granularity requirement enforces exact sellable pack agreement",
		Catalog: []CatalogEntryV8{
			{
				ID:         907,
				AR:         "نيوكلير 10 مجم 10 أقراص",
				EN:         "neoclear 10mg 10 tablets",
				Scientific: "loratadine",
				Form:       "tablet",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "10mg",
				Content:    "-",
				Pack:       "10 tab",
				Maker:      "NeoPharma",
			},
			{
				ID:         908,
				AR:         "نيوكلير 10 مجم 30 قرص",
				EN:         "neoclear 10mg 30 tablets",
				Scientific: "loratadine",
				Form:       "tablet",
				Route:      "oral",
				Release:    "ordinary",
				Strengths:  "10mg",
				Content:    "-",
				Pack:       "30 tab",
				Maker:      "NeoPharma",
			},
		},
		Item: ItemV8{
			Ref:     5,
			Need:    "pack",
			Raw:     "Neoclear 10mg 30 tablets",
			Facts:   "brand=neoclear(o);strength=10mg(o);pack=30 tab(o)",
			Options: []int64{907, 908},
		},
		SelectedID: ptrInt64(908),
		Evidence:   map[string]string{"st": "a", "fm": "a", "rt": "a", "rl": "a", "md": "a", "pk": "a", "br": "a"},
		Confidence: "hi",
	},
}
