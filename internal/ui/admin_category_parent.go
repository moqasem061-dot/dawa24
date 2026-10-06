package ui

import "github.com/moqasem061-dot/dawa24/internal/modules/catalog"

func validCategoryParent(categories []*catalog.Category, parentID *int64, categoryID int64) bool {
	if parentID == nil {
		return true
	}
	byID := make(map[int64]*catalog.Category, len(categories))
	for _, item := range categories {
		if item != nil {
			byID[item.ID] = item
		}
	}
	return categoryParentChainValid(byID, *parentID, categoryID, len(categories))
}

func categoryParentChainValid(categories map[int64]*catalog.Category, parentID, categoryID int64, maxDepth int) bool {
	visited := make(map[int64]bool, maxDepth)
	for depth := 0; depth < maxDepth && parentID > 0; depth++ {
		if parentID == categoryID || visited[parentID] {
			return false
		}
		visited[parentID] = true
		parent := categories[parentID]
		if parent == nil || parent.DeletedAt != nil {
			return false
		}
		if parent.ParentID == nil {
			return true
		}
		parentID = *parent.ParentID
	}
	return false
}

func hasCategoryChildren(categories []*catalog.Category, categoryID int64) bool {
	for _, item := range categories {
		if item != nil && item.ParentID != nil && *item.ParentID == categoryID && item.DeletedAt == nil {
			return true
		}
	}
	return false
}
