package ingest

import "context"

// The seam between the vendor import and the AI gateway.
//
// The import declares Enhancer in its own terms; the composition root wires an
// implementation backed by aicapabilities — the same capability, the same
// system prompt and the same response contract the smart order uses. Nothing in
// this module knows the gateway exists, which is what keeps provider and model
// names out of it and what lets every test here run without one.

// GatewayEnhancer is the shape aicapabilities exposes. Declaring it here rather
// than importing that module keeps the dependency pointing inward.
type GatewayEnhancer interface {
	EnhanceBatch(ctx context.Context, batch GatewayBatch) ([]GatewayOutcome, error)
}

// GatewayBatch mirrors aicapabilities.EnhanceRequest.
type GatewayBatch struct {
	Catalog []GatewayProduct
	Items   []GatewayItem
}

// GatewayProduct mirrors aicapabilities.CatalogEntry.
type GatewayProduct struct {
	ProductID     int64
	NameAR        string
	NameEN        string
	Scientific    string
	DosageForm    string
	Concentration string
	Manufacturer  string
}

// GatewayItem mirrors aicapabilities.EnhanceItem.
type GatewayItem struct {
	Ref          int
	Text         string
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
	Settled      bool
}

// GatewayOutcome mirrors aicapabilities.EnhanceDecision.
type GatewayOutcome struct {
	Ref        int
	ProductID  *int64
	Confidence float64
	Reason     string
}

// gatewayEnhancer adapts a GatewayEnhancer to the import's Enhancer.
type gatewayEnhancer struct{ gw GatewayEnhancer }

// NewGatewayEnhancer wires the gateway into the vendor import.
//
// A nil implementation yields a nil Enhancer, which the import treats as "AI
// unavailable" — so a deployment with no gateway simply does not offer the
// switch, rather than offering one that silently does nothing.
func NewGatewayEnhancer(gw GatewayEnhancer) Enhancer {
	if gw == nil {
		return nil
	}
	return &gatewayEnhancer{gw: gw}
}

// Enhance translates the import's request, calls the gateway, and translates
// the answer back.
func (g *gatewayEnhancer) Enhance(ctx context.Context, batch EnhanceBatch) ([]EnhanceOutcome, error) {
	out := GatewayBatch{
		Catalog: make([]GatewayProduct, 0, len(batch.Catalog)),
		Items:   make([]GatewayItem, 0, len(batch.Items)),
	}
	for _, c := range batch.Catalog {
		out.Catalog = append(out.Catalog, GatewayProduct(c))
	}
	for _, it := range batch.Items {
		out.Items = append(out.Items, GatewayItem(it))
	}

	decisions, err := g.gw.EnhanceBatch(ctx, out)
	if err != nil {
		return nil, err
	}

	results := make([]EnhanceOutcome, 0, len(decisions))
	for _, d := range decisions {
		results = append(results, EnhanceOutcome(d))
	}
	return results, nil
}

// SetEnhancer installs the AI matching port. Leaving it unset means the stage
// is skipped, which is the same path a disabled gateway takes.
func (s *Service) SetEnhancer(e Enhancer) { s.enhancer = e }

// SetMatchMemory installs the shared decision cache. It is optional: without it
// the stage still works and simply pays for every question.
func (s *Service) SetMatchMemory(m MatchMemory) { s.memory = m }

// AIAvailable reports whether the import screen may offer the AI switch.
func (s *Service) AIAvailable() bool { return s != nil && s.enhancer != nil }
