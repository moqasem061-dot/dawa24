package pages

import (
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
)

func doc(id int64, t attachments.DocumentType, status attachments.DocumentStatus, at time.Time) *attachments.Document {
	return &attachments.Document{
		ID:           id,
		DocumentType: t,
		Status:       status,
		CreatedAt:    at,
	}
}

func TestBuildOrganizationDocumentsData_Missing(t *testing.T) {
	customer := BuildOrganizationDocumentsData(nil, nil, false)
	if len(customer.Missing) != 1 {
		t.Fatalf("customer with no docs: want 1 missing requirement, got %d", len(customer.Missing))
	}

	vendor := BuildOrganizationDocumentsData(nil, nil, true)
	if len(vendor.Missing) != 1 {
		t.Fatalf("vendor with no docs: want 1 missing requirement, got %d", len(vendor.Missing))
	}
}

func TestBuildOrganizationDocumentsData_VerifiedClearsMissing(t *testing.T) {
	now := time.Now()
	docs := []*attachments.Document{
		doc(1, attachments.DocCommercialRegister, attachments.StatusVerified, now),
		doc(2, attachments.DocTaxCard, attachments.StatusPending, now),
	}
	customer := BuildOrganizationDocumentsData(docs, nil, false)
	if len(customer.Missing) != 0 {
		t.Fatalf("want 0 missing when at least one doc is verified, got %d", len(customer.Missing))
	}

	docsPendingOnly := []*attachments.Document{
		doc(2, attachments.DocTaxCard, attachments.StatusPending, now),
	}
	customerPending := BuildOrganizationDocumentsData(docsPendingOnly, nil, false)
	if len(customerPending.Missing) != 1 {
		t.Fatalf("pending doc alone still requires verified doc: got %d missing", len(customerPending.Missing))
	}
	if got := customerPending.MissingTitles(); got == "" {
		t.Fatal("MissingTitles must not be empty with a missing requirement")
	}
}

func TestOrganizationDocumentsData_LatestFor(t *testing.T) {
	now := time.Now()
	data := &OrganizationDocumentsData{Docs: []*attachments.Document{
		doc(1, attachments.DocCommercialRegister, attachments.StatusPending, now),
		doc(2, attachments.DocCommercialRegister, attachments.StatusVerified, now.Add(2*time.Minute)),
	}}
	if latest := data.LatestFor(attachments.DocCommercialRegister); latest == nil || latest.ID != 2 {
		t.Fatalf("want newest document (id 2), got %+v", latest)
	}
	if latest := data.LatestFor(attachments.DocTaxCard); latest != nil {
		t.Fatalf("want nil for missing type, got %+v", latest)
	}
}
