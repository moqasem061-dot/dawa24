package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	smartorderPG "github.com/muhiya/dawa24-store/internal/modules/smartorder/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/progress"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// Wiring smart ordering.
//
// This is the composition root's job precisely because smartorder must not
// import workflow, org or commerce (AGENTS.md rule 5). Each dependency is
// adapted here into the narrow function type the module declares, so the module
// stays testable with closures and the modules stay independent of each other.

// wireSmartOrder assembles the feature and mounts it on the UI handler.
//
// Nil-safe: if anything it needs is missing the wizard reports itself
// unavailable rather than taking the rest of the customer surface down with it.
func wireSmartOrder(
	db *database.DB,
	uiHandler *ui.UIHandler,
	orgSvc *org.Service,
	wfCoverage *workflow.CoverageService,
	commSvc *commerce.Service,
	availability commerce.AvailabilityProbe,
	ai gateway.Client,
	publisher *progress.Publisher,
	log *slog.Logger,
) *smartorder.Service {
	if db == nil || uiHandler == nil {
		return nil
	}

	// Decorated so every event and status change announces itself. The buyer's
	// screen used to ask for the number twice a second for the whole run.
	repo := smartorder.WithProgressNotifications(smartorderPG.New(db), publisher)
	svc := smartorder.NewService(repo, log)

	var gate smartorder.AvailabilityGate
	if commSvc != nil {
		gate = newCommerceAvailabilityGate(commSvc)
		svc.SetAvailabilityGate(gate)
	}

	// Process in this process. The River worker stays registered for
	// deployments that run it, and the two cannot collide: the runner only
	// claims a run that is still `queued`. See smartorder_runner.go.
	uiHandler.SetSmartOrder(svc, inlineSmartOrderRunner(db, orgSvc, commSvc, ai, publisher, log))

	if commSvc != nil {
		uiHandler.SetFinalizer(smartorder.NewFinalizer(
			repo,
			placeSmartOrder(commSvc, orgSvc, wfCoverage, log),
			&reverifier{gate: gate},
		))
	}
	return svc
}

// placeSmartOrder adapts commerce checkout.
//
// Smart orders go through the **same** checkout as the ordinary cart, so
// multi-vendor shipment partitioning, order numbering, status history and the
// documents gate are identical (FR-048). A separate order-creation path would
// drift from the one the rest of the platform uses, and the drift would only
// show up in an invoice.
func placeSmartOrder(commSvc *commerce.Service, orgSvc *org.Service, wfCoverage *workflow.CoverageService, log *slog.Logger) smartorder.PlaceOrderFunc {
	return func(ctx context.Context, req smartorder.PlaceOrderRequest) (int64, error) {
		items := make([]commerce.CheckoutLineItem, 0, len(req.Lines))
		for _, l := range req.Lines {
			variantID := l.VariantID
			// The discount is carried as an amount per line rather than a
			// percentage, because that is what the order snapshot stores and it
			// keeps the arithmetic exact.
			gross, err := l.UnitPrice.MulInt(int64(l.Quantity))
			if err != nil {
				return 0, err
			}
			discount := money.Zero
			if gross.Minor() > l.LineNet.Minor() {
				if d, subErr := gross.Sub(l.LineNet); subErr == nil {
					discount = d
				}
			}
			raw := l.RawName
			if raw == "" {
				raw = i18n.TDefault("w4_mod.24_150")
			}
			pName := i18n.New(raw, raw)
			netUnitPrice := l.UnitPrice
			if int64(l.Quantity) > 0 {
				netUnitPrice = money.FromMinor(l.LineNet.Minor() / int64(l.Quantity))
			}
			items = append(items, commerce.CheckoutLineItem{
				VendorOrgID:      l.VendorOrgID,
				ProductVariantID: &variantID,
				ProductName:      pName,
				UnitPrice:        netUnitPrice,
				Quantity:         int(l.Quantity),
				DiscountAmount:   discount,
				ListPrice:        l.UnitPrice,
			})
		}

		branchID := req.BranchID
		if branchID <= 0 {
			return 0, fmt.Errorf("smart order requires a receiving branch")
		}

		// Calculate dynamic distance-based delivery fees per vendor
		vendorShippingFees := make(map[int64]money.Amount)
		if orgSvc != nil {
			var custLat, custLon *float64
			var custCityID *int64
			if branchID > 0 {
				if cb, err := orgSvc.GetBranch(database.AsSystem(ctx), branchID); err == nil && cb != nil {
					custLat = cb.Latitude
					custLon = cb.Longitude
					custCityID = cb.CityID
				}
			}

			for _, it := range items {
				if it.VendorOrgID <= 0 {
					continue
				}
				if _, exists := vendorShippingFees[it.VendorOrgID]; exists {
					continue
				}

				distMeters := 5000
				hasCoords := custLat != nil && custLon != nil
				hasCity := custCityID != nil && *custCityID > 0

				if wfCoverage != nil && (hasCoords || hasCity) {
					coord := workflow.Coord{CityID: custCityID}
					if hasCoords {
						coord.Lat = *custLat
						coord.Lon = *custLon
					}
					_, actualMeters, err := wfCoverage.ServesPoint(ctx, it.VendorOrgID, time.Now().Weekday(), coord)
					if err == nil && actualMeters > 0 {
						distMeters = actualMeters
					}
				}

				fee, matched, err := orgSvc.CalculateDeliveryFee(database.AsSystem(ctx), it.VendorOrgID, distMeters)
				if err == nil && matched {
					vendorShippingFees[it.VendorOrgID] = fee
				}
			}
		}

		log.InfoContext(ctx, "placing smart order",
			"run_id", req.SourceRunID, "lines", len(items), "branch_id", branchID, "total", req.Total.String())

		order, err := commSvc.Checkout(ctx, commerce.CheckoutInput{
			CustomerID:         req.UserID,
			CustomerOrgID:      req.OrganizationID,
			BranchID:           &branchID,
			Items:              items,
			VendorShippingFees: vendorShippingFees,
			Notes:              i18n.TDefault("w4_mod.s_476_476"),
		})
		if err != nil {
			return 0, err
		}
		return order.ID, nil
	}
}

// commerceAvailabilityGate adapts commerce.Service to smartorder.AvailabilityGate.
type commerceAvailabilityGate struct {
	commSvc *commerce.Service
}

func newCommerceAvailabilityGate(commSvc *commerce.Service) smartorder.AvailabilityGate {
	return &commerceAvailabilityGate{commSvc: commSvc}
}

func (g *commerceAvailabilityGate) Check(ctx context.Context, buyerOrgID, buyerBranchID int64, when time.Time,
	lines []smartorder.GateLine) (map[int64]smartorder.GateVerdict, error) {
	if g.commSvc == nil {
		verdicts := make(map[int64]smartorder.GateVerdict, len(lines))
		for _, l := range lines {
			verdicts[l.VariantID] = smartorder.GateVerdict{
				Allowed: false,
				Reason:  "variant_invalid",
			}
		}
		return verdicts, nil
	}
	cLines := make([]commerce.AvailabilityLine, len(lines))
	for i, l := range lines {
		cLines[i] = commerce.AvailabilityLine{
			VariantID:   l.VariantID,
			VendorOrgID: l.VendorOrgID,
			Quantity:    l.Quantity,
		}
	}
	results, err := g.commSvc.CheckAvailabilityBatch(ctx, buyerOrgID, buyerBranchID, when, cLines)
	if err != nil {
		return nil, err
	}
	verdicts := make(map[int64]smartorder.GateVerdict, len(results))
	for vid, r := range results {
		verdicts[vid] = smartorder.GateVerdict{
			Allowed:     r.Allowed,
			MaxQuantity: r.MaxQuantity,
			Reason:      string(r.Reason),
		}
	}
	return verdicts, nil
}

// reverifier re-checks one candidate against the world as it is now.
type reverifier struct {
	gate smartorder.AvailabilityGate
}

// Recheck asks the AvailabilityGate whether this candidate is still orderable.
func (rv *reverifier) Recheck(ctx context.Context, buyerOrgID, branchID int64,
	c smartorder.Candidate, qty float64) (bool, smartorder.IneligibleReason, error) {

	if rv.gate == nil {
		return false, smartorder.ReasonInactive, nil
	}

	intQty := int(qty)
	if intQty <= 0 {
		intQty = 1
	}

	verdicts, err := rv.gate.Check(ctx, buyerOrgID, branchID, time.Now(), []smartorder.GateLine{
		{
			VariantID:   c.VariantID,
			VendorOrgID: c.VendorOrgID,
			Quantity:    intQty,
		},
	})
	if err != nil {
		return false, "", err
	}

	verdict, ok := verdicts[c.VariantID]
	if !ok {
		return false, smartorder.ReasonInactive, nil
	}

	if verdict.Allowed {
		return true, "", nil
	}

	return false, smartorder.EvaluateReason(verdict.Reason), nil
}
