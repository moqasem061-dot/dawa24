package erp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// chunkCtx is everything a chunk is allowed to know about its batch. It is
// built from the batch row and the integration, never from the API key, so a
// key revoked or rotated after acceptance does not stop an accepted batch
// (trap 25 / plan S2 step 7).
type chunkCtx struct {
	OrganizationID int64
	IntegrationID  int64
	WarehouseID    int64
	BranchID       int64
	Provider       string
}

// plannedRow is one pending item's destiny inside this chunk: the variant it
// targets (0 = create), whether the catalog/stock must be written, the link it
// produces, the review it lands in, or the failure it ends with.
type plannedRow struct {
	row        SyncItemRow
	item       Item
	state      string // created | updated | unchanged | needs_review | failed
	code       string // the stable item-level code, when any
	message    map[string]string
	variantID  int64
	productID  int64
	sku        string
	writerRef  int          // the Ref handed to the VariantWriter for this row
	loadedLink *ProductLink // the link tier's existing link, for its refresh
	wantWrite  bool         // catalog + stock writes happen for created/updated only
	linkWrite  *LinkWrite   // set for every row that creates or refreshes a link
	remember   *KnowledgeWrite
	review     *ReviewWrite
}

type chunkPlan struct {
	rows      []plannedRow
	writes    []VariantWrite
	quotas    map[int64]*int
	stock     []StockWrite
	links     []LinkWrite
	reviews   []ReviewWrite
	remembers []KnowledgeWrite
}

// resolveItems is plan S2 step 7 (c) to (f): one links query, hash
// short-circuit, the sku and barcode tiers, then exactly one Matcher.Prepare
// call for every row the tiers could not settle. A matcher error is not an
// error: those rows stay in review with ERP_ITEM_MATCHER_UNAVAILABLE.
func (s *Service) resolveItems(ctx context.Context, cc chunkCtx, batchID int64, chunkNo int, pending []SyncItemRow) (*chunkPlan, error) {
	plan := &chunkPlan{quotas: map[int64]*int{}}

	rawItems := make(map[int64]provider.RawItem, len(pending))
	codes := make([]string, 0, len(pending))
	for _, row := range pending {
		if row.State != "pending" {
			continue // a retried chunk skips rows that are not pending (T102)
		}
		var raw provider.RawItem
		if err := json.Unmarshal(row.Payload, &raw); err != nil {
			plan.rows = append(plan.rows, plannedRow{
				row: row, state: "failed", code: "ERP_BAD_JSON",
				message: bilingual("ERP_BAD_JSON"),
			})
			continue
		}
		item, itemErr := NormalizeItem(raw)
		if itemErr != nil {
			plan.rows = append(plan.rows, plannedRow{
				row: row, state: "failed", code: itemErr.Code, message: bilingual(itemErr.Code),
			})
			continue
		}
		rawItems[row.ID] = raw
		codes = append(codes, item.Code)
		plan.rows = append(plan.rows, plannedRow{row: row, item: item})
	}
	if len(plan.rows) == 0 {
		return plan, nil
	}

	links, err := s.repo.LoadLinksByCode(ctx, cc.OrganizationID, cc.IntegrationID, codes)
	if err != nil {
		return nil, err
	}

	// Tier 0: existing links. Hash-equal rows are unchanged; deleted variants
	// go to review; the rest update through the link (tier (e)).
	var unmatched []int
	for i := range plan.rows {
		row := &plan.rows[i]
		if row.row.State != "pending" || itemOf(row) == nil {
			continue
		}
		link, linked := links[row.item.Code]
		if !linked {
			unmatched = append(unmatched, i)
			continue
		}
		if link.VariantDeleted {
			row.state = "needs_review"
			row.code = "ERP_ITEM_DELETED_LINK"
			row.message = bilingual(row.code)
			row.review = s.reviewWrite(cc, *row, "ERP_ITEM_DELETED_LINK")
			continue
		}
		hash := row.item.Hash(cc.WarehouseID, cc.BranchID)
		if link.LastHash == hash {
			row.state = "unchanged"
			continue
		}
		row.variantID = link.VariantID
		row.productID = deref(link.ProductID)
		row.loadedLink = &link
		row.state = "updated"
		row.wantWrite = true
		row.sku = row.item.Code
	}

	// Tier 1: sku, tier 2: barcode. Only live variants are considered (the
	// lookups read deleted_at IS NULL), so a soft-deleted twin never resurrects.
	var tierCodes []string
	for _, i := range unmatched {
		tierCodes = append(tierCodes, plan.rows[i].item.Code)
	}
	bySKU, err := s.repo.FindVariantsBySKU(ctx, cc.OrganizationID, tierCodes)
	if err != nil {
		return nil, err
	}
	var toBarcodes []int
	for _, i := range unmatched {
		row := &plan.rows[i]
		ids := bySKU[row.item.Code]
		switch {
		case len(ids) == 1:
			row.variantID = ids[0]
			row.wantWrite = true
			row.sku = row.item.Code
			// First appearance of this external code: the item is created even
			// though the tier matched an existing variant.
			row.state = "created"
			row.linkWrite = &LinkWrite{
				IntegrationID: cc.IntegrationID, VariantID: ids[0],
				ExternalCode: row.item.Code, LinkSource: "sku_match", LastHash: row.item.Hash(cc.WarehouseID, cc.BranchID),
			}
		case len(ids) > 1:
			row.state = "needs_review"
			row.code = "ERP_ITEM_MATCH_REVIEW"
			row.message = bilingual(row.code)
			row.review = s.reviewWrite(cc, *row, "ERP_ITEM_MATCH_REVIEW")
		default:
			toBarcodes = append(toBarcodes, i)
		}
	}
	var byBarcode map[string][]int64
	if len(toBarcodes) > 0 {
		barcodes := make([]string, 0, len(toBarcodes))
		for _, i := range toBarcodes {
			barcodes = append(barcodes, plan.rows[i].item.Barcode)
		}
		byBarcode, err = s.repo.FindVariantsByBarcode(ctx, cc.OrganizationID, barcodes)
		if err != nil {
			return nil, err
		}
	}
	var toMatcher []int
	for _, i := range toBarcodes {
		row := &plan.rows[i]
		if row.item.Barcode == "" {
			toMatcher = append(toMatcher, i)
			continue
		}
		ids := byBarcode[row.item.Barcode]
		switch {
		case len(ids) == 1:
			row.variantID = ids[0]
			row.wantWrite = true
			row.sku = row.item.Code
			// First appearance of this external code: the item is created even
			// though the tier matched an existing variant.
			row.state = "created"
			row.linkWrite = &LinkWrite{
				IntegrationID: cc.IntegrationID, VariantID: ids[0],
				ExternalCode: row.item.Code, LinkSource: "barcode_match", LastHash: row.item.Hash(cc.WarehouseID, cc.BranchID),
			}
		case len(ids) > 1:
			row.state = "needs_review"
			row.code = "ERP_ITEM_BARCODE_CONFLICT"
			row.message = bilingual(row.code)
			row.review = s.reviewWrite(cc, *row, "ERP_ITEM_BARCODE_CONFLICT")
		default:
			toMatcher = append(toMatcher, i)
		}
	}

	// Tier 3: one deterministic Prepare call for every row still unresolved. A
	// Gateway outage is not an error: those rows stay in review until the
	// supplier resolves them, and the chunk completes regardless (trap 18).
	if len(toMatcher) > 0 {
		req := productmatch.PrepareRequest{
			Scope: productmatch.Scope{
				EffectiveOrg:    cc.OrganizationID,
				SourceNamespace: fmt.Sprintf("supplier:%d", cc.OrganizationID),
				Workflow:        productmatch.WorkflowVendor,
				EntryPoint:      "erp_sync",
			},
			ConsumerRef: fmt.Sprintf("erp.sync:%d:%d", batchID, chunkNo),
			Rows:        make([]productmatch.Row, 0, len(toMatcher)),
		}
		for _, i := range toMatcher {
			req.Rows = append(req.Rows, matcherRow(plan.rows[i].item))
		}
		result, err := s.ports.Matcher.Prepare(ctx, req)
		if err != nil {
			for _, i := range toMatcher {
				row := &plan.rows[i]
				row.state = "needs_review"
				row.code = "ERP_ITEM_MATCHER_UNAVAILABLE"
				row.message = bilingual(row.code)
				row.review = s.reviewWrite(cc, *row, "ERP_ITEM_MATCHER_UNAVAILABLE")
			}
		} else {
			for at, i := range toMatcher {
				row := &plan.rows[i]
				decision := matchflow.RowDecision{}
				if at < len(result.Decisions) {
					decision = result.Decisions[at]
				}
				if (decision.State == matchflow.StateAccepted || decision.State == matchflow.StateConfirmed) && decision.ProductID != nil && *decision.ProductID > 0 {
					row.state = "created"
					row.wantWrite = true
					row.productID = *decision.ProductID
					row.sku = row.item.Code
					hash := row.item.Hash(cc.WarehouseID, cc.BranchID)
					row.linkWrite = &LinkWrite{
						IntegrationID: cc.IntegrationID, ExternalCode: row.item.Code,
						ProductID:  decision.ProductID,
						LinkSource: "engine", LastHash: hash,
					}
					// The decision carries the policy and normalizer versions
					// that produced it — the knowledge row must be replayable
					// under the same versions.
					policyVersion, normalizerVersion := decision.Versions.Policy, decision.Versions.Normalizer
					if policyVersion == "" {
						policyVersion = erpPolicyVersion
					}
					if normalizerVersion == "" {
						normalizerVersion = productmatch.NormalizerVersion
					}
					plan.remembers = append(plan.remembers, KnowledgeWrite{
						SourceNamespace: req.Scope.SourceNamespace,
						QuestionSignature: productmatch.QuestionSignature(
							productmatch.ExtractRowFacts(&req.Rows[at], nil), row.item.ArName, normalizerVersion),
						Kind:              "supplier_code",
						TargetProductID:   decision.ProductID,
						PolicyVersion:     policyVersion,
						NormalizerVersion: normalizerVersion,
						DisplayText:       row.item.ArName,
					})
				} else {
					row.state = "needs_review"
					row.code = "ERP_ITEM_MATCH_REVIEW"
					row.message = bilingual(row.code)
					row.review = s.reviewWrite(cc, *row, "ERP_ITEM_MATCH_REVIEW")
					if len(decision.Candidates) > 0 {
						if raw, err := json.Marshal(decision.Candidates); err == nil {
							row.review.Candidates = raw
						}
					}
				}
			}
		}
	}
	return plan, nil
}

// erpPolicyVersion stamps the knowledge rows the sync writes when the matcher
// decision did not carry its own policy version. The erp module cannot import
// the matching module's policy loader (module boundary), so it tags its own
// deterministic decisions with a stable prefix instead.
const erpPolicyVersion = "erp-v1"

// reviewWrite builds the upsert row for the supplier's review screen; the
// payload carries the stored item JSON so the resolver sees what the batch sent.
func (s *Service) reviewWrite(cc chunkCtx, row plannedRow, reason string) *ReviewWrite {
	return &ReviewWrite{
		IntegrationID: cc.IntegrationID,
		ExternalCode:  row.item.Code,
		ReasonCode:    reason,
		Payload:       row.row.Payload,
	}
}

func itemOf(row *plannedRow) *Item {
	if row.row.State != "pending" {
		return nil
	}
	if row.item.Code == "" && row.row.ExternalCode == "" {
		return nil
	}
	return &row.item
}

func matcherRow(item Item) productmatch.Row {
	return productmatch.Row{
		Name:         item.ArName,
		SKU:          item.Code,
		Barcode:      item.Barcode,
		Manufacturer: item.Manufacturer,
		Unit:         item.Unit,
		PublicPrice:  item.PublicPrice,
		Quantity:     item.Stock,
		HasQuantity:  true,
		BatchNumber:  item.Batch,
		ExpiryDate:   item.Expiry,
	}
}

func deref(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// bilingual renders an item-level code's stored {ar,en} message pair straight
// from the catalog, so rows carried in erp.sync_items stay bilingual without
// the reader re-deriving anything. The local alias keeps the printf vet check
// away from the dynamic key (same idiom as internal/modules/erp/http).
func bilingual(code string) map[string]string {
	translate := i18n.T
	key := "erp.err." + lowerCode(code)
	return map[string]string{
		"ar": translate(i18n.AR, key),
		"en": translate(i18n.EN, key),
	}
}

func lowerCode(code string) string {
	out := make([]byte, 0, len(code))
	for i := 0; i < len(code); i++ {
		c := code[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return strings.TrimPrefix(string(out), "erp_")
}
