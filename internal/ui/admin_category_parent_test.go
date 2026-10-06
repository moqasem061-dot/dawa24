package ui

import (
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
)

func TestValidCategoryParentRejectsMissingSelfAndDescendant(t *testing.T) {
	root, child, grandchild := int64(1), int64(2), int64(3)
	categories := []*catalog.Category{
		{ID: root},
		{ID: child, ParentID: &root},
		{ID: grandchild, ParentID: &child},
	}
	if !validCategoryParent(categories, nil, child) {
		t.Fatal("root category parent should be valid")
	}
	for _, test := range []struct {
		name     string
		parentID int64
		selfID   int64
	}{{"missing", 9, child}, {"self", child, child}, {"descendant", grandchild, root}} {
		t.Run(test.name, func(t *testing.T) {
			if validCategoryParent(categories, &test.parentID, test.selfID) {
				t.Fatal("invalid category parent was accepted")
			}
		})
	}
}

func TestHasCategoryChildrenIgnoresDeletedChild(t *testing.T) {
	parentID := int64(1)
	categories := []*catalog.Category{{ID: 1}, {ID: 2, ParentID: &parentID}}
	if !hasCategoryChildren(categories, 1) {
		t.Fatal("active child was not found")
	}
	categories[1].DeletedAt = new(time.Time)
	if hasCategoryChildren(categories, 1) {
		t.Fatal("deleted child blocked category deletion")
	}
}
