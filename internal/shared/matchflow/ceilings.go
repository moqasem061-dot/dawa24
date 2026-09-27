package matchflow

import "time"

// What one run may spend, and why.
//
// The Gateway enforces its own budget, but hitting that mid-run surfaces to the
// user as an opaque failure. Stopping first, and saying so, is the difference
// between "the system degraded and told me" and "the system broke".
//
// One table, three profiles. The numbers differ because the files differ — a
// distributor's hundred-thousand-row price list and a pharmacy's thousand-line
// order are not the same problem — but they differ by a stated argument rather
// than because two people tuned two copies.

// Ceilings bound one run of the enhancement stage.
type Ceilings struct {
	// RecallLimit is how many catalogue products are retrieved per row.
	//
	// Sixteen rather than a dozen: the model behind this has a large context and
	// the four extra rows buy the cases where the correct product ranks eighth
	// because the pharmacy misspelled the brand. Far above this the list stops
	// being a shortlist and starts being a haystack.
	RecallLimit int

	// MaxInputBytes caps one request's rendered input, in BYTES rather than
	// characters — Arabic is two bytes per character in UTF-8, and conflating
	// the two halves the budget without anyone noticing.
	//
	// Measured against the live Gateway, this mixture of Arabic, Latin and
	// digits runs about two bytes to the token: a 290 KB request reported 145k
	// input tokens. It is a backstop rather than the binding constraint — the
	// item ceiling fills roughly 300 KB at its limit — so it only ever splits a
	// batch whose catalogue window came out unusually wide. That is the right
	// shape: a request should be sized by how many answers it can safely
	// produce, not by how much text it can hold.
	MaxInputBytes int

	// MaxItemsPerRequest bounds the ANSWER, and that is what limits a batch.
	// Latency sets it rather than tokens: output is generated serially, so a
	// three-hundred-item request is still writing when a hundred-item one has
	// finished, and a batch that fails takes every row in it back to the
	// deterministic outcome.
	MaxItemsPerRequest int

	// MaxRequestsPerRun is the spend ceiling, and the one number here that is a
	// business decision rather than a measurement. Rows past it keep their
	// deterministic outcome and are reported as such rather than silently
	// dropped.
	MaxRequestsPerRun int

	// MaxConcurrent decides how long the stage takes, since the total output
	// tokens are the same however the items are divided.
	MaxConcurrent int

	// MaxWallClock bounds the stage.
	MaxWallClock time.Duration

	// MinApplyConfidence is the floor below which an answer is recorded but not
	// applied. The prompt instructs the model to answer null below the same
	// figure; this enforces it, because an instruction is not a guarantee.
	//
	// Eight tenths, because the model's confidence turns out to be well
	// calibrated and sharply bimodal: on a live 1,004-line residue it answered
	// 0.95 for everything it was sure of, and the handful it scored in the
	// seventies included "ابى ديرم كريم" matched to "هاي ديرم كريم" — two
	// different products sharing only the category suffix ديرم, which no
	// deterministic guard can tell from a brand. The model knew. Taking it at
	// its word costs almost nothing and removes exactly that class of mistake.
	MinApplyConfidence float64

	// MinPlausible is the retrieval score below which a row is not sent at all.
	//
	// It is zero in every profile (2026-09-27, at the client's direction): every
	// row — مطابق to be checked, يحتاج المراجعة and غير مطابق to be resolved — is
	// put to the model whenever retrieval found ANY candidate. A floor of 0.15–
	// 0.40 used to drop exactly the unmatched and needs-review rows whose names
	// were written most differently from the catalogue, which are the rows a
	// model is best at and a scorer worst at.
	//
	// Asking a weak row costs budget, never correctness: an answer is applied
	// only above MinApplyConfidence, after the identity screen and the pair
	// check, so "none of these" or a refusal leaves the row as it was. When a
	// huge file outruns the budget, Plan spends it in risk order — ambiguous,
	// then unresolved, then settled — so the cut falls on the safest tail.
	//
	// A row with NO candidate is still not sent: the model may only answer with
	// a product it was shown, and an empty window has nothing to choose from.
	MinPlausible float64
}

// MinMemoryConfidence is the confidence below which an answer is used but not
// remembered.
//
// The decision cache is per organisation and long-lived: an entry written today
// answers the same question for months, for every tool that asks it, without
// anybody being shown that it was a guess. That is exactly right for a
// confident answer and wrong for a hesitant one — a model that said 0.2 was
// telling us it did not know, and freezing "did not know, chose this anyway"
// into shared memory turns one weak guess into a standing fact.
//
// Forty-five per cent, at the client's direction. It sits between the
// deterministic review floor and the apply floor on purpose: everything at or
// above it was worth an opinion, and everything below it is re-asked next time,
// when the catalogue may have grown the product that was missing.
//
// Answers at or above it are remembered whatever they SAID, including "none of
// these" — a confident refusal is as reusable as a confident match, and it is
// what stops the next upload of the same price list paying to be told the same
// thing.
const MinMemoryConfidence = 0.45

// Profile names the kind of file a run is processing.
type Profile string

const (
	// ProfileOrder is a pharmacy's purchase list: a person is watching a
	// progress bar and expects an order at the end of it.
	ProfileOrder Profile = "order"
	// ProfileVendor is a supplier's price list: long, repetitive, and uploaded
	// weekly with a dozen rows changed.
	ProfileVendor Profile = "vendor"
	// ProfileCatalog is an administrator's master-catalogue file. A wrong match
	// here overwrites the entry every pharmacy reads, so it spends the least and
	// applies the least.
	ProfileCatalog Profile = "catalog"
	// ProfileSaving is a vendor or pharmacy saving-products list. Files range
	// from dozens to multiple thousands of rows, demanding high-throughput
	// concurrent execution and adaptive batching.
	ProfileSaving Profile = "saving"
)

// For returns the ceilings a profile runs under.
//
// Every profile finishes inside fifteen minutes — the engine enforces fourteen
// whatever is written here — and every profile spends that time on smaller
// batches rather than larger ones. A model answering sixty lines against one
// window attends to each of them; one answering two hundred skims the tail,
// and the tail is where the forced matches were. The pair check then takes a
// fifth of the wall clock, which is why proposals stop starting at eighty per
// cent of it.
func For(p Profile) Ceilings {
	base := Ceilings{
		RecallLimit:        16,
		MaxInputBytes:      220_000,
		MaxItemsPerRequest: 60,
		MaxRequestsPerRun:  120,
		MaxConcurrent:      8,
		MaxWallClock:       12 * time.Minute,
		MinApplyConfidence: 0.80,
		MinPlausible:       0,
	}

	switch p {
	case ProfileOrder:
		// A person is watching the bar. Small batches answer fastest.
		base.MaxItemsPerRequest = 30
		base.RecallLimit = 12
		base.MaxInputBytes = 120_000
		base.MaxRequestsPerRun = 120
		base.MaxWallClock = 8 * time.Minute

	case ProfileSaving:
		base.RecallLimit = 12
		base.MaxRequestsPerRun = 150

	case ProfileVendor:
		// Long, repetitive files. Two hundred requests of sixty is twelve
		// thousand distinct questions — the largest file this system has seen,
		// after de-duplication — and the risk ordering spends the wall clock on
		// the ambiguous rows first when it runs out.
		base.MaxRequestsPerRun = 200
		base.MaxConcurrent = 10
		base.MaxWallClock = 14 * time.Minute

	case ProfileCatalog:
		// The master catalogue is the one place a wrong match is not
		// recoverable by looking at the result, so it applies the least.
		base.MaxItemsPerRequest = 50
		base.MaxConcurrent = 6
		base.MinApplyConfidence = 0.90
	}
	return base
}

// Adaptive tunes the ceilings to the file. A small file gets small batches,
// because its whole residue fits in a few requests either way and small ones
// come back sooner. Nothing here ever raises the batch size: larger batches
// buy throughput with attention, and attention is what the answers are made of.
func Adaptive(p Profile, totalItems int) Ceilings {
	c := For(p)
	if totalItems > 0 && totalItems <= 100 && c.MaxItemsPerRequest > 20 {
		c.MaxItemsPerRequest = 20
	}
	return c
}
