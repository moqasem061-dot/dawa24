package http

import (
	_ "embed"
	"net/http"
)

//go:embed spec/openapi.yaml
var openAPISpec []byte

func (h *Handler) spec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(openAPISpec)
}
