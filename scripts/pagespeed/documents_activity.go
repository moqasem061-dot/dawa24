package main

import (
	"log"
	"net/http"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/attachments"
	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func documentsFixture(w http.ResponseWriter, r *http.Request) {
	lang, dir := productFixtureLocale(r)
	id := int64(1)
	name := "images (1).png"
	if r.URL.Query().Get("revision") == "2" {
		id = 2
		name = "replacement.png"
	}
	data := pages.BuildOrganizationDocumentsData([]*attachments.Document{{ID: id, DocumentType: attachments.DocCommercialRegister, OriginalName: name, MimeType: "image/png", Status: attachments.StatusPending, CreatedAt: time.Now()}}, nil, true)
	data.UploadPath = "/documents/upload"
	if err := pages.OrganizationDocuments(lang, dir, data, nil).Render(layouts.WithPath(r.Context(), r.URL.Path), w); err != nil {
		log.Print(err)
	}
}

func activitiesFixture(w http.ResponseWriter, r *http.Request) {
	lang, dir := productFixtureLocale(r)
	entries := []*platformadmin.AuditEntry{{ID: 1, ActorName: "أحمد", Action: "http.operation", EntityType: "/vendor/branches/{id}/edit", Route: "/vendor/branches/{id}/edit", EntityID: "15", CreatedAt: time.Now(), After: map[string]any{"method": "POST", "status": 303, "outcome": "response_303", "parameters": map[string]any{"quantity": []any{"12"}, "price": []any{"34.50"}}}}}
	if err := pages.VendorActivitiesPage(entries, lang, dir, 1, 20, 1).Render(layouts.WithPath(r.Context(), r.URL.Path), w); err != nil {
		log.Print(err)
	}
}
