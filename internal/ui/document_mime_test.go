package ui

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveDocumentMime(t *testing.T) {
	tests := []struct {
		name      string
		docMime   string
		filenames []string
		expected  string
	}{
		{
			name:      "PNG from filename despite empty doc mime",
			docMime:   "",
			filenames: []string{"68468941792.png"},
			expected:  "image/png",
		},
		{
			name:      "PNG overrides application/pdf when original filename is PNG",
			docMime:   "application/pdf",
			filenames: []string{"68468941792.png"},
			expected:  "image/png",
		},
		{
			name:      "JPEG from candidate path",
			docMime:   "application/octet-stream",
			filenames: []string{"data/uploads/documents/breadcrumb.jpg"},
			expected:  "image/jpeg",
		},
		{
			name:      "WebP from clean key",
			docMime:   "",
			filenames: []string{"uploads/licenses/license_abc.webp"},
			expected:  "image/webp",
		},
		{
			name:      "PDF from filename",
			docMime:   "application/octet-stream",
			filenames: []string{"commercial_register.pdf"},
			expected:  "application/pdf",
		},
		{
			name:      "Fallback to existing specific MIME if no known extension",
			docMime:   "image/svg+xml",
			filenames: []string{"vector_badge"},
			expected:  "image/svg+xml",
		},
		{
			name:      "Default fallback to application/pdf when unknown",
			docMime:   "",
			filenames: []string{"unknown_file.xyz"},
			expected:  "application/pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDocumentMime(tt.docMime, tt.filenames...)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestSandboxDocumentContent_PDF(t *testing.T) {
	rec := httptest.NewRecorder()
	sandboxDocumentContent(rec, "application/pdf")
	csp := rec.Header().Get("Content-Security-Policy")
	assert.Contains(t, csp, "object-src 'self' blob:")
	assert.Contains(t, csp, "frame-ancestors 'self'")
}
