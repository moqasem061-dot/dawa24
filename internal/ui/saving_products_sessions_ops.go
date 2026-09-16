package ui

import (
	"context"
	"errors"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// UpdateProgress updates the progress and phase message.
func (s *SavingImportSessionStore) UpdateProgress(id string, progress int, phase string, processed int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok {
		sess.Progress = progress
		sess.ProgressPhase = phase
		sess.ProcessedRows = processed
	}
}

// CompleteProcessing finalizes processing and marks session as ready.
func (s *SavingImportSessionStore) CompleteProcessing(id string, items []*StagedSavingItem, matched, unlinked int, totalQty float64, totalVal money.Amount) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok {
		sess.Status = SessionStateReady
		sess.Phase = SavingPhaseReview
		sess.Progress = 100
		sess.ProgressPhase = i18n.T("ar", "ops.saving.processing_complete")
		sess.Items = items
		sess.MatchedRows = matched
		sess.UnlinkedRows = unlinked
		sess.TotalQuantity = totalQty
		sess.TotalValue = totalVal
		sess.ProcessedRows = len(items)
	}
}

// UpdateStagedItem updates an individual staged item in place.
func (s *SavingImportSessionStore) UpdateStagedItem(id string, orgID int64, itemIndex int, name string, price *money.Amount, qty *float64, isIncluded *bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok || sess.OrgID != orgID {
		return errors.New(i18n.T("ar", "ops.session_not_found"))
	}
	for _, it := range sess.Items {
		if it.Index == itemIndex {
			if name != "" {
				it.NameProduct = name
			}
			if price != nil {
				it.Price = *price
			}
			if qty != nil {
				it.Quantity = *qty
			}
			if isIncluded != nil {
				it.Included = *isIncluded
			}
			it.TotalValue = money.FromMinor(int64(it.Quantity * float64(it.Price.Minor())))
			s.recalculateTotals(sess)
			return nil
		}
	}
	return errors.New(i18n.T("ar", "ops.item_not_found"))
}

// AssignStagedItemMatch manually links or unlinks a staged item to/from a catalog product.
func (s *SavingImportSessionStore) AssignStagedItemMatch(id string, orgID int64, itemIndex int, productID int64, masterName, masterSKU string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok || sess.OrgID != orgID {
		return errors.New(i18n.T("ar", "ops.session_not_found"))
	}
	for _, it := range sess.Items {
		if it.Index == itemIndex {
			if productID > 0 {
				pid := productID
				it.ProductID = &pid
				it.MasterProductName = masterName
				it.MasterProductSKU = masterSKU
				it.MatchType = "manual"
				it.Confidence = 1.0
			} else {
				it.ProductID = nil
				it.MasterProductName = ""
				it.MasterProductSKU = ""
				it.MatchType = "unlinked"
				it.Confidence = 0.0
			}
			s.recalculateTotals(sess)
			return nil
		}
	}
	return errors.New(i18n.T("ar", "ops.item_not_found"))
}

// ToggleStagedItem flips inclusion flag for an item.
func (s *SavingImportSessionStore) ToggleStagedItem(id string, orgID int64, itemIndex int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok || sess.OrgID != orgID {
		return false, errors.New(i18n.T("ar", "ops.session_not_found"))
	}
	for _, it := range sess.Items {
		if it.Index == itemIndex {
			it.Included = !it.Included
			s.recalculateTotals(sess)
			return it.Included, nil
		}
	}
	return false, errors.New(i18n.T("ar", "ops.item_not_found"))
}

func (s *SavingImportSessionStore) recalculateTotals(sess *SavingImportSession) {
	matched := 0
	unlinked := 0
	var totalQty float64
	var totalValMinor int64
	for _, it := range sess.Items {
		if it.ProductID != nil && *it.ProductID > 0 {
			matched++
		} else {
			unlinked++
		}
		if it.Included {
			totalQty += it.Quantity
			totalValMinor += it.TotalValue.Minor()
		}
	}
	sess.MatchedRows = matched
	sess.UnlinkedRows = unlinked
	sess.TotalQuantity = totalQty
	sess.TotalValue = money.FromMinor(totalValMinor)
}

// FilterItems filters and sorts items for table display and pagination.
func (s *SavingImportSessionStore) FilterItems(session *SavingImportSession, filter SavingRowFilter) ([]*StagedSavingItem, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var filtered []*StagedSavingItem
	q := strings.ToLower(strings.TrimSpace(filter.Search))

	for _, it := range session.Items {
		if filter.MatchFilter == "matched" && (it.ProductID == nil || *it.ProductID <= 0) {
			continue
		}
		if filter.MatchFilter == "unmatched" && (it.ProductID != nil && *it.ProductID > 0) {
			continue
		}
		if q != "" {
			nameMatch := strings.Contains(strings.ToLower(it.NameProduct), q)
			skuMatch := strings.Contains(strings.ToLower(it.SKU), q)
			masterMatch := strings.Contains(strings.ToLower(it.MasterProductName), q) || strings.Contains(strings.ToLower(it.MasterProductSKU), q)
			if !nameMatch && !skuMatch && !masterMatch {
				continue
			}
		}
		filtered = append(filtered, it)
	}

	total := len(filtered)

	isDesc := strings.EqualFold(filter.SortOrder, "desc")
	switch filter.SortBy {
	case "name":
		sort.Slice(filtered, func(i, j int) bool {
			if isDesc {
				return filtered[i].NameProduct > filtered[j].NameProduct
			}
			return filtered[i].NameProduct < filtered[j].NameProduct
		})
	case "catalog":
		sort.Slice(filtered, func(i, j int) bool {
			if isDesc {
				return filtered[i].MasterProductName > filtered[j].MasterProductName
			}
			return filtered[i].MasterProductName < filtered[j].MasterProductName
		})
	case "score":
		sort.Slice(filtered, func(i, j int) bool {
			if isDesc {
				return filtered[i].Confidence > filtered[j].Confidence
			}
			return filtered[i].Confidence < filtered[j].Confidence
		})
	case "qty":
		sort.Slice(filtered, func(i, j int) bool {
			if isDesc {
				return filtered[i].Quantity > filtered[j].Quantity
			}
			return filtered[i].Quantity < filtered[j].Quantity
		})
	case "price":
		sort.Slice(filtered, func(i, j int) bool {
			if isDesc {
				return filtered[i].Price.Minor() > filtered[j].Price.Minor()
			}
			return filtered[i].Price.Minor() < filtered[j].Price.Minor()
		})
	case "total":
		sort.Slice(filtered, func(i, j int) bool {
			if isDesc {
				return filtered[i].TotalValue.Minor() > filtered[j].TotalValue.Minor()
			}
			return filtered[i].TotalValue.Minor() < filtered[j].TotalValue.Minor()
		})
	default:
		// Weakest match first.
		//
		// The default used to be the row's position in the pharmacy's file,
		// which tells the reviewer nothing: the rows needing a decision are
		// scattered evenly among the ones that do not, so finding them means
		// reading all of them. Worst first puts every doubtful row on page one.
		//
		// An unlinked row has no confidence at all and sorts before everything,
		// which is right — nothing matched is the worst outcome there is. The
		// file position breaks ties so the order stays stable across reloads.
		sort.SliceStable(filtered, func(i, j int) bool {
			a, b := filtered[i], filtered[j]
			ca, cb := reviewConfidence(a), reviewConfidence(b)
			if ca != cb {
				if isDesc {
					return ca > cb
				}
				return ca < cb
			}
			return a.Index < b.Index
		})
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 25
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit
	if offset >= total {
		return nil, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total
}

// FailSession marks a session as failed with an error message.
func (s *SavingImportSessionStore) FailSession(id string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok {
		sess.Status = SessionStateFailed
		sess.Progress = 0
		sess.ProgressPhase = i18n.TDefault("w4_ui.s_95_95")
		sess.ErrorMessage = errMsg
	}
}

// CancelSession marks a session as cancelled and discards items.
func (s *SavingImportSessionStore) CancelSession(id string, orgID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok && sess.OrgID == orgID {
		sess.Status = SessionStateCancelled
		sess.Items = nil
		delete(s.sessions, id)
		return true
	}
	return false
}

// CommitSession writes staged items to the database via catalog service.
func (s *SavingImportSessionStore) CommitSession(ctx context.Context, id string, orgID, userID int64, catSvc *catalog.Service) (int, int, error) {
	s.mu.Lock()
	sess, ok := s.sessions[id]
	if !ok || sess.OrgID != orgID || sess.Status != SessionStateReady {
		s.mu.Unlock()
		return 0, 0, fmt.Errorf("%s", i18n.TDefault("w4_ui.s_96_96"))
	}

	itemsToCommit := make([]*catalog.SavingProduct, 0, len(sess.Items))
	for _, it := range sess.Items {
		if !it.Included {
			continue
		}
		itemsToCommit = append(itemsToCommit, &catalog.SavingProduct{
			OrganizationID: orgID,
			UserID:         &userID,
			ProductID:      it.ProductID,
			NameProduct:    it.NameProduct,
			SKU:            it.SKU,
			Quantity:       it.Quantity,
			Price:          it.Price,
		})
	}
	s.mu.Unlock()

	if len(itemsToCommit) == 0 {
		return 0, 0, fmt.Errorf("%s", i18n.TDefault("w4_ui.s_97_97"))
	}

	added, updated, err := catSvc.BatchUpsertSavingProducts(ctx, orgID, &userID, itemsToCommit)
	if err != nil {
		return 0, 0, err
	}

	s.mu.Lock()
	if sess, ok := s.sessions[id]; ok {
		sess.Status = SessionStateCommitted
		sess.Phase = SavingPhaseCompleted
		sess.InsertedCount = added
		sess.UpdatedCount = updated
	}
	s.mu.Unlock()

	return added, updated, nil
}

// reviewConfidence is the confidence a review screen should sort by.
//
// An unlinked row reports zero however confident the engine was about refusing
// it: the reviewer's question is "what still needs me", and a row with no
// product is top of that list whatever number happens to sit in its confidence
// field.
func reviewConfidence(it *StagedSavingItem) float64 {
	if it == nil || it.ProductID == nil || *it.ProductID <= 0 {
		return 0
	}
	return it.Confidence
}
