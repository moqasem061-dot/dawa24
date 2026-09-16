package pipeline

import "context"

// The seam between the pipeline and the AI gateway.
//
// The pipeline defines Enhancer in its own terms; the composition root wires an
// implementation backed by aicapabilities. Nothing in this package knows the
// gateway exists, which is what keeps provider and model names out of it
// (AGENTS.md R2) and what lets every test here run without one.

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

// gatewayEnhancer adapts a GatewayEnhancer to the pipeline's Enhancer.
type gatewayEnhancer struct{ gw GatewayEnhancer }

// NewGatewayEnhancer wires the gateway into the pipeline.
//
// A nil implementation yields a nil Enhancer, which the pipeline treats as "AI
// unavailable" — so an unconfigured Gateway is a quiet skip rather than a
// runtime panic on the first import.
func NewGatewayEnhancer(gw GatewayEnhancer) Enhancer {
	if gw == nil {
		return nil
	}
	return &gatewayEnhancer{gw: gw}
}

// Enhance translates the pipeline's request, calls the gateway, and translates
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
