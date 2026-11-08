package erp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// ListReview pages the supplier's open review items (the queue the buyer's
// needs_review rows fed). The organization comes from the tenant context the
// page gate set.
func (s *Service) ListReview(ctx context.Context, orgID int64, page Page) ([]ReviewItem, error) {
	return s.repo.ListReview(ctx, orgID, page)
}

// ResolveReview links the item to the product the supplier picked and marks the
// review resolved: the variant is created through the same write path, the
// human_confirm knowledge is stored, and one audit row records who decided.
func (s *Service) ResolveReview(ctx context.Context, orgID, id, productID, actorUserID int64) error {
	item, err := s.repo.GetReview(ctx, orgID, id)
	if err != nil {
		return NewError("ERP_SYNC_NOT_FOUND", 404, nil)
	}
	if item.State != "open" {
		return NewError("ERP_SYNC_NOT_FOUND", 404, nil)
	}
	if productID <= 0 {
		return NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"productId"}})
	}
	if s.ports.VariantWriter == nil {
		return NewError("ERP_UNAVAILABLE", 503, nil)
	}

	// The write path reuses the item normalizer over the stored payload (the
	// exact bytes the batch carried), so the variant the supplier confirms is
	// the variant the batch described — same price rules, same exclusions.
	variantWrite := VariantWrite{
		ProductID: productID,
		Name: map[string]string{
			"ar": reviewDisplayName(item),
			"en": reviewDisplayName(item),
		},
	}
	var raw provider.RawItem
	if jsonErr := json.Unmarshal(item.Payload, &raw); jsonErr == nil {
		if normalized, itemErr := NormalizeItem(raw); itemErr == nil {
			variantWrite.SKU = normalized.Code
			variantWrite.Barcode = normalized.Barcode
			variantWrite.Unit = normalized.Unit
			variantWrite.BatchNumber = normalized.Batch
			variantWrite.Price = normalized.PublicPrice
			if normalized.Discount != nil {
				variantWrite.Discount = *normalized.Discount
			}
			variantWrite.ExpiryDate = normalized.Expiry
		}
	}

	result, err := s.ports.VariantWriter.Write(ctx, orgID, []VariantWrite{variantWrite})
	if err != nil {
		return s.processError(err)
	}
	if _, bad := result.Failures[0]; bad {
		return NewError("ERP_ITEM_WRITE_FAILED", 422, nil)
	}
	variantID := result.IDs[0]

	// The human's choice is a manual_review link: future syncs of the same
	// code resolve through it before the matcher ever runs.
	externalCode := item.ExternalCode
	if err := s.repo.WriteResults(ctx, orgID, nil, []LinkWrite{{
		IntegrationID: item.IntegrationID, VariantID: variantID,
		ProductID: &productID, ExternalCode: externalCode,
		LinkSource: "manual_review",
	}}, nil); err != nil {
		return s.processError(err)
	}

	audit := AuditRecord{
		Action: "erp.review.resolve", EntityType: "erp.review_items",
		EntityID: fmt.Sprint(id), ActorUserID: actorUserID,
	}
	if err := s.repo.ResolveReviewRow(ctx, orgID, id, variantID, productID, actorUserID, audit); err != nil {
		return NewError("ERP_SYNC_NOT_FOUND", 404, nil)
	}

	// human_confirm knowledge: a human picked this product, which is stronger
	// evidence than any engine decision (trap 17).
	if s.ports.MatchMemory != nil {
		_ = s.ports.MatchMemory.Remember(ctx, productmatch.Scope{
			SourceNamespace: fmt.Sprintf("supplier:%d", orgID),
			Workflow:        productmatch.WorkflowVendor,
			EntryPoint:      "erp_review_resolve",
		}, KnowledgeWrite{
			SourceNamespace:   fmt.Sprintf("supplier:%d", orgID),
			Kind:              "human_confirm",
			TargetProductID:   &productID,
			TargetGranularity: "product",
			PolicyVersion:     erpPolicyVersion,
			NormalizerVersion: productmatch.NormalizerVersion,
			DisplayText:       reviewDisplayName(item),
		})
	}
	return nil
}

// DismissReview closes the item without linking anything: the supplier decided
// this ERP code must never map to this marketplace product.
func (s *Service) DismissReview(ctx context.Context, orgID, id, actorUserID int64) error {
	item, err := s.repo.GetReview(ctx, orgID, id)
	if err != nil {
		return NewError("ERP_SYNC_NOT_FOUND", 404, nil)
	}
	if item.State != "open" {
		return NewError("ERP_SYNC_NOT_FOUND", 404, nil)
	}
	audit := AuditRecord{
		Action: "erp.review.dismiss", EntityType: "erp.review_items",
		EntityID: fmt.Sprint(id), ActorUserID: actorUserID,
	}
	return s.repo.DismissReviewRow(ctx, orgID, id, actorUserID, audit)
}

func reviewDisplayName(item ReviewItem) string {
	if facts := ReviewFactsOf(item); facts.Name != "" {
		return facts.Name
	}
	return item.ExternalCode
}

// ReviewFacts is what the ERP sent for a review item, for the supplier to
// recognise it: the product name, barcode, manufacturer and public price.
type ReviewFacts struct {
	Name         string
	Barcode      string
	Manufacturer string
	Price        string
}

// ReviewFactsOf reads the stored payload. A payload that does not decode
// leaves the facts empty; the page then shows the ERP code alone.
func ReviewFactsOf(item ReviewItem) ReviewFacts {
	var raw provider.RawItem
	if err := json.Unmarshal(item.Payload, &raw); err != nil {
		return ReviewFacts{}
	}
	facts := ReviewFacts{Name: raw.ArName, Barcode: raw.Barcode, Manufacturer: raw.Manufacturer}
	if raw.PublicPrice != nil {
		facts.Price = raw.PublicPrice.String()
	}
	return facts
}
