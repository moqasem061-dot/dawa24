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
	// This is the one ceiling that saves most of the money, and it was missing.
	// On a live smart order — run 43, 8,790 rows — the deterministic engine left
	// 7,626 unmatched because the file was cosmetics the catalogue does not
	// stock. All 7,939 residual lines were sent anyway: thirty requests, the
	// ceiling hit, 345 seconds of wall clock against 1.6 seconds of
	// deterministic work, and 156 rows improved. No model could have answered
	// the rest, because the answer was not in the catalogue.
	//
	// A row whose best retrieved candidate scores below this has no plausible
	// answer to choose between, and "unmatched" is already the honest result.
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
func For(p Profile) Ceilings {
	base := Ceilings{
		RecallLimit:        16,
		MaxInputBytes:      400_000,
		MaxItemsPerRequest: 120,
		MaxRequestsPerRun:  30,
		MaxConcurrent:      6,
		MaxWallClock:       8 * time.Minute,
		MinApplyConfidence: 0.80,
		MinPlausible:       0.30,
	}

	switch p {
	case ProfileOrder:
		base.MaxItemsPerRequest = 25
		base.RecallLimit = 6
		base.MaxConcurrent = 4
		base.MaxInputBytes = 80_000
		base.MaxRequestsPerRun = 40
		base.MaxWallClock = 5 * time.Minute

	case ProfileSaving:
		base.MaxItemsPerRequest = 80
		base.RecallLimit = 8
		base.MaxConcurrent = 6
		base.MaxInputBytes = 280_000
		base.MaxRequestsPerRun = 60
		base.MaxWallClock = 10 * time.Minute
		base.MinPlausible = 0.15

	case ProfileVendor:
		// A supplier file is long and its rows repeat, so more per request.
		//
		// The request ceiling used to be twenty, which with two hundred items
		// apiece capped one run at four thousand distinct questions. A real
		// Egyptian distributor's list is nine thousand rows of which seven or
		// eight thousand are distinct after de-duplication, so the ceiling was
		// reached on the file this whole flow exists for — and the screen told
		// the vendor their column mapping was probably wrong, which it was not.
		//
		// Sixty requests of two hundred is twelve thousand questions, which
		// covers the largest file anyone has uploaded to this system with room
		// over. The cost is bounded by the plausibility gate long before it is
		// bounded by this: rows the catalogue cannot plausibly answer are never
		// sent, and on a live file that is most of the residue.
		base.MaxItemsPerRequest = 200
		base.MaxRequestsPerRun = 60
		base.MaxConcurrent = 6
		// Six concurrent requests of two hundred items finish sixty requests in
		// well under this; the deadline exists so a Gateway that has stopped
		// answering ends the stage rather than the import.
		base.MaxWallClock = 20 * time.Minute

	case ProfileCatalog:
		// The master catalogue is the one place a wrong match is not
		// recoverable by looking at the result, so this profile is deliberately
		// the most conservative of the three.
		base.MaxItemsPerRequest = 100
		base.MaxRequestsPerRun = 12
		base.MaxConcurrent = 4
		base.MinApplyConfidence = 0.90
		base.MinPlausible = 0.40
	}
	return base
}

// Adaptive dynamically adjusts ceilings based on the total number of items,
// ensuring small uploads get fast low-latency batches while large uploads
// (> 1,000 items) pack into high-throughput concurrent requests without delay.
func Adaptive(p Profile, totalItems int) Ceilings {
	c := For(p)
	if totalItems <= 0 {
		return c
	}
	switch {
	case totalItems <= 100:
		if c.MaxItemsPerRequest > 40 {
			c.MaxItemsPerRequest = 40
		}
	case totalItems <= 1000:
		if p != ProfileCatalog && p != ProfileVendor {
			if c.MaxItemsPerRequest < 75 {
				c.MaxItemsPerRequest = 75
			}
			if c.MaxInputBytes < 250_000 {
				c.MaxInputBytes = 250_000
			}
			if c.MaxConcurrent < 6 {
				c.MaxConcurrent = 6
			}
		}
	default:
		if p != ProfileCatalog && p != ProfileVendor {
			if c.MaxItemsPerRequest < 100 {
				c.MaxItemsPerRequest = 100
			}
			if c.MaxInputBytes < 350_000 {
				c.MaxInputBytes = 350_000
			}
			if c.MaxRequestsPerRun < 60 {
				c.MaxRequestsPerRun = 60
			}
			if c.MaxConcurrent < 6 {
				c.MaxConcurrent = 6
			}
		}
	}
	return c
}
