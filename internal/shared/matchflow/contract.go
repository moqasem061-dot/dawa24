// Package matchflow holds the contract every importer's AI matching stage
// shares: the question being asked, the shape of the request and the answer,
// and the ceilings one run may spend.
//
// It exists because three copies of that contract had already drifted. The
// prompt version is the decision cache's key, and the vendor import declared it
// "sm-enh-v3" while the smart order declared it "sm-enh-v4" — both rendering the
// same prompt through the same capability, and each filing its answers where the
// other would never look. On live data that was 2,913 cached decisions on one
// side and 3,584 on the other, for one question. The comment above the vendor's
// constant asserted the two matched.
//
// The ceilings had drifted too: 120 items per request against 200, thirty
// requests per run against twelve, six concurrent against four. Same gateway,
// same model, same question, two tables of numbers, each documented as measured.
//
// This package is a dependency-free leaf, so the modules that run the stage and
// the module that talks to the Gateway can all name the same types without
// importing one another — which the architecture forbids and which is what
// pushed each of them into declaring its own in the first place.
package matchflow

import "context"

// PromptVersion is the version of the question being asked.
//
// It is part of the decision-cache key, so changing it orphans every cached
// answer rather than silently reusing answers to a different question. Change
// it when — and only when — the rendered input or the system prompt changes.
//
// It is declared here and nowhere else. `make check-prompt-version` fails the
// build if the literal appears outside this package.
const PromptVersion = "sm-enh-v6"

// CatalogEntry is one catalogue product as the model sees it.
//
// Arabic name first, because that is what the pharmacy wrote and what the
// catalogue is authored in. The English name earns its tokens: transliteration
// is where Arabic pharmacy matching actually fails, and "ابليفاى" against
// "ابيليفاي" is obvious the moment both rows show `abilify`.
type CatalogEntry struct {
	ProductID     int64
	NameAR        string
	NameEN        string
	Scientific    string
	DosageForm    string
	Concentration string
	Manufacturer  string
}

// Item is one row the deterministic engine could not settle.
type Item struct {
	// Ref identifies the item inside this request. It is the request-local
	// index, not a database id: row ids are the caller's business and there is
	// no reason to send them to a third party.
	Ref int
	// Text is exactly what the file said, unedited. The model needs the noise —
	// "س.ج 141ج" is a price, not a strength, and only the raw text says so.
	Text string

	// The rest is what the deterministic decomposition extracted. They are
	// hints, not constraints: the decomposition is sometimes wrong, which is
	// part of why the row reached here at all.
	Brand        string
	Strength     string
	DosageForm   string
	PackSize     int
	Manufacturer string
	Scientific   string
	SKU          string
	Barcode      string
	CurrentGuess *int64
	CurrentScore float64
	Options      []int64

	// Settled says the deterministic engine already applied CurrentGuess, so
	// the question is verification rather than resolution: is this the right
	// product, not which product is it.
	//
	// The two questions are asked in one request because they are answered
	// against the same catalogue window and because splitting them doubled the
	// number of requests a file cost. They are DECIDED differently — see
	// Verdict — because the consequences differ: overruling a resolution
	// replaces a guess, and overruling a verification replaces a decision the
	// engine was confident enough to apply.
	Settled bool
}

// Batch is one request: a de-duplicated catalogue window and the items to
// resolve against it.
//
// The window is shared rather than per item, and every item may be answered with
// any product in it. That costs nothing extra and repairs the commonest
// retrieval failure there is: the right product was retrieved — for the row
// above.
type Batch struct {
	Catalog []CatalogEntry
	Items   []Item

	// Feature names the tool that asked, for the AI usage ledger. It is not
	// sent to the model and takes no part in the prompt or the cache key — a
	// pharmacy reading its usage log needs to know that the money went on the
	// smart order rather than on a catalogue import, and a capability name
	// alone cannot tell them, because both tools ask the same capability.
	Feature string

	// OrganizationID attributes consumption and quotas to the tenant who asked.
	OrganizationID int64
	UserID         int64
}

// The features that spend AI budget, in the vocabulary the usage screens use.
//
// Declared here because this package is the dependency-free leaf every importer
// already shares, and because a feature key invented separately in each module
// is how the capability names drifted in the first place.
const (
	FeatureSmartOrder    = "smart_order"
	FeatureVendorImport  = "variant_match"
	FeatureCatalogImport = "catalog_import"
	FeatureSavingsImport = "savings_import"
	FeatureAssistant     = "assistant"
	FeatureColumnDetect  = "column_detect"
	// FeatureCompareTool is the private price-comparison tool, which had no
	// catalogue matching stage at all until it was given the shared one.
	FeatureCompareTool = "compare_match"
)

// Decision is one answer, keyed by the request-local ref.
//
// A nil ProductID means "none of these", which is a correct and frequent answer
// and is recorded as such rather than treated as a failure.
type Decision struct {
	Ref        int     `json:"ref"`
	ProductID  *int64  `json:"product_id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason,omitempty"`
}

// Outcome is what a tool should do with one answer.
type Outcome int

const (
	// OutcomeKeep leaves the deterministic result exactly as it was.
	OutcomeKeep Outcome = iota
	// OutcomeApply writes the model's product id.
	OutcomeApply
	// OutcomeReview removes an applied match and asks a person.
	//
	// It exists because the interesting answer from a verification pass is not
	// "yes" and not "here is a better one" — it is "no". A row the engine
	// settled and the model rejects is the one row in the file where two
	// independent methods disagree, and it is worth more of a person's
	// attention than anything either of them was merely unsure about.
	OutcomeReview
)

// Enhancer resolves a batch against its catalogue window.
//
// Declared here so no module needs to import the gateway to run the stage, and
// so every test in every importer runs without one (AGENTS.md R2, R5).
type Enhancer interface {
	Enhance(ctx context.Context, batch Batch) ([]Decision, error)
}
