package matching

// EntryPoint records an identity-assigning or grouping entry point.
type EntryPoint struct {
	ID         string
	Workflow   string
	File       string
	Profile    string
	AIEligible bool
}

// EntryPoints lists every identity-assigning or grouping entry point across workflows
// as documented in research.md R1.
var EntryPoints = []EntryPoint{
	// Vendor ingest
	{
		ID:         "vendor_stage_run",
		Workflow:   "vendor",
		File:       "internal/modules/ingest/catalog_stage_run.go",
		Profile:    "vendor_import",
		AIEligible: false,
	},
	{
		ID:         "vendor_stage",
		Workflow:   "vendor",
		File:       "internal/modules/ingest/catalog_stage.go",
		Profile:    "vendor_import",
		AIEligible: false,
	},
	{
		ID:         "vendor_enhance_run",
		Workflow:   "vendor",
		File:       "internal/modules/ingest/catalog_enhance_run.go",
		Profile:    "vendor_enhance",
		AIEligible: true,
	},

	// Savings
	{
		ID:         "savings_matcher",
		Workflow:   "savings",
		File:       "internal/ui/saving_products_matcher.go",
		Profile:    "savings_import",
		AIEligible: false,
	},
	{
		ID:         "savings_match_choice",
		Workflow:   "savings",
		File:       "internal/ui/saving_products_match_choice.go",
		Profile:    "savings_choice",
		AIEligible: false,
	},
	{
		ID:         "savings_ai",
		Workflow:   "savings",
		File:       "internal/ui/saving_products_ai.go",
		Profile:    "savings_enhance",
		AIEligible: true,
	},

	// Smart Order
	{
		ID:         "smartorder_candidates",
		Workflow:   "smartorder",
		File:       "internal/modules/smartorder/pipeline/candidates.go",
		Profile:    "smartorder_candidates",
		AIEligible: false,
	},
	{
		ID:         "smartorder_enhance",
		Workflow:   "smartorder",
		File:       "internal/modules/smartorder/pipeline/enhance.go",
		Profile:    "smartorder_enhance",
		AIEligible: true,
	},

	// Master catalogue
	{
		ID:         "catalog_import_match",
		Workflow:   "catalog",
		File:       "internal/modules/catalog/import_match.go",
		Profile:    "master_catalog",
		AIEligible: false,
	},
	{
		ID:         "catalog_import_chunks",
		Workflow:   "catalog",
		File:       "internal/modules/catalog/import_match_chunks.go",
		Profile:    "master_catalog",
		AIEligible: false,
	},
	{
		ID:         "catalog_import_adjudicate",
		Workflow:   "catalog",
		File:       "internal/modules/catalog/import_match_adjudicate.go",
		Profile:    "master_adjudicate",
		AIEligible: true,
	},
	{
		ID:         "catalog_commit_worker",
		Workflow:   "catalog",
		File:       "internal/modules/catalog/importjobs/commit_worker.go",
		Profile:    "master_commit",
		AIEligible: false,
	},

	// Compare
	{
		ID:         "compare_matching",
		Workflow:   "compare",
		File:       "internal/modules/compare/matching.go",
		Profile:    "compare_batch",
		AIEligible: false,
	},
	{
		ID:         "compare_matching_ai",
		Workflow:   "compare",
		File:       "internal/modules/compare/matching_ai.go",
		Profile:    "compare_enhance",
		AIEligible: true,
	},
	{
		ID:         "compare_head_to_head",
		Workflow:   "compare",
		File:       "internal/modules/compare/head_to_head_comparison.go",
		Profile:    "compare_h2h",
		AIEligible: false,
	},
}
