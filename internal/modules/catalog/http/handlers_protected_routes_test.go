package http_test

import (
	"net/http"
)

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/catalog/search"},
	{http.MethodGet, "/api/v1/catalog/products/1"},
	{http.MethodPut, "/api/v1/catalog/products/1"},
	{http.MethodDelete, "/api/v1/catalog/products/1"},
	{http.MethodPost, "/api/v1/catalog/products"},
	{http.MethodGet, "/api/v1/catalog/products"},
	{http.MethodPost, "/api/v1/catalog/products/bulk-status"},
	{http.MethodPost, "/api/v1/catalog/products/1/variants"},
	{http.MethodGet, "/api/v1/catalog/products/1/variants/1"},
	{http.MethodPut, "/api/v1/catalog/products/1/variants/1"},
	{http.MethodDelete, "/api/v1/catalog/products/1/variants/1"},
	{http.MethodGet, "/api/v1/catalog/categories/1"},
	{http.MethodPut, "/api/v1/catalog/categories/1"},
	{http.MethodDelete, "/api/v1/catalog/categories/1"},
	{http.MethodGet, "/api/v1/catalog/brands/1"},
	{http.MethodPut, "/api/v1/catalog/brands/1"},
	{http.MethodDelete, "/api/v1/catalog/brands/1"},
	{http.MethodGet, "/api/v1/catalog/categories"},
	{http.MethodPost, "/api/v1/catalog/categories"},
	{http.MethodGet, "/api/v1/catalog/brands"},
	{http.MethodPost, "/api/v1/catalog/brands"},
	{http.MethodPost, "/api/v1/catalog/pricing/customer"},
	{http.MethodGet, "/api/v1/catalog/pricing/customer"},
	{http.MethodPost, "/api/v1/catalog/alerts"},
	{http.MethodGet, "/api/v1/catalog/alerts"},
}
