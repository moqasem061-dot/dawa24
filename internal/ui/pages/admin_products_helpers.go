package pages

import (
	"net/url"
	"strconv"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func derefBrandID(bid *int64) int64 {
	if bid != nil {
		return *bid
	}
	return 0
}

func derefCategoryID(cid *int64) int64 {
	if cid != nil {
		return *cid
	}
	return 0
}

func getCategoryDisplayLabel(c *catalog.Category, categories []*catalog.Category) string {
	if c == nil {
		return ""
	}
	name := c.Name.Get("ar")
	if name == "" {
		name = c.Name.Get("en")
	}
	if c.ParentID != nil && *c.ParentID > 0 {
		for _, parent := range categories {
			if parent.ID == *c.ParentID {
				parentName := parent.Name.Get("ar")
				if parentName == "" {
					parentName = parent.Name.Get("en")
				}
				if parentName != "" {
					return parentName + " " + name
				}
			}
		}
	}
	return name
}

func getCategoryLabel(cid *int64, categories []*catalog.Category, fallback string) string {
	if cid != nil && *cid > 0 {
		for _, c := range categories {
			if c.ID == *cid {
				return getCategoryDisplayLabel(c, categories)
			}
		}
	}
	return fallback
}

func getBrandLabel(bid *int64, brands []*catalog.Brand, fallback string) string {
	if bid != nil && *bid > 0 {
		for _, b := range brands {
			if b.ID == *bid {
				name := b.Name.Get("ar")
				if name == "" {
					name = b.Name.Get("en")
				}
				if name != "" {
					return name
				}
			}
		}
	}
	return fallback
}

func adminProductsQuery(q, status, dosage, manufacturer, sort string, brandID, catID int64) url.Values {
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	if status != "" && status != "all" {
		v.Set("status", status)
	}
	if dosage != "" && dosage != "all" {
		v.Set("dosage", dosage)
	}
	if manufacturer != "" {
		v.Set("manufacturer", manufacturer)
	}
	if sort != "" && sort != "newest" {
		v.Set("sort", sort)
	}
	if brandID > 0 {
		v.Set("brand_id", strconv.FormatInt(brandID, 10))
	}
	if catID > 0 {
		v.Set("category_id", strconv.FormatInt(catID, 10))
	}
	return v
}

func buildAdminProductsPageURL(page, limit int, q, status, dosage, manufacturer, sort string, brandID, catID int64) string {
	vals := adminProductsQuery(q, status, dosage, manufacturer, sort, brandID, catID)
	vals.Set("page", strconv.Itoa(page))
	vals.Set("limit", strconv.Itoa(limit))
	return "/admin/products?" + vals.Encode()
}

func buildAdminProductsSortURL(targetSort, currentSort, q, status, dosage, manufacturer string, brandID, catID int64, limit int) string {
	nextSort := targetSort + "_asc"
	if currentSort == targetSort+"_asc" {
		nextSort = targetSort + "_desc"
	}
	vals := adminProductsQuery(q, status, dosage, manufacturer, nextSort, brandID, catID)
	if limit > 0 {
		vals.Set("limit", strconv.Itoa(limit))
	}
	return "/admin/products?" + vals.Encode()
}

func renderSortIcon(targetSort, currentSort string) string {
	if currentSort == targetSort+"_asc" {
		return " ▲"
	}
	if currentSort == targetSort+"_desc" {
		return " ▼"
	}
	return " ↕"
}

func totalPagesHelper(total, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	p := (total + pageSize - 1) / pageSize
	if p < 1 {
		return 1
	}
	return p
}

func minIntHelper(a, b int) int {
	if a < b {
		return a
	}
	return b
}
