package pages

import (
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/attachments"
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
	if len(customer.Missing) != 2 {
		t.Fatalf("customer with no docs: want 2 missing requirements, got %d", len(customer.Missing))
	}

	vendor := BuildOrganizationDocumentsData(nil, nil, true)
	if len(vendor.Missing) != 2 {
		t.Fatalf("vendor with no docs: want 2 missing requirements, got %d", len(vendor.Missing))
	}
}

func TestBuildOrganizationDocumentsData_VerifiedClearsMissing(t *testing.T) {
	now := time.Now()
	docs := []*attachments.Document{
		doc(1, attachments.DocCommercialRegister, attachments.StatusVerified, now),
		doc(2, attachments.DocPharmacyLicense, attachments.StatusVerified, now),
		doc(3, attachments.DocTaxCard, attachments.StatusPending, now),
	}
	customer := BuildOrganizationDocumentsData(docs, nil, false)
	if len(customer.Missing) != 0 {
		t.Fatalf("want 0 missing when both mandatory docs are verified, got %d", len(customer.Missing))
	}

	docsPendingOnly := []*attachments.Document{
		doc(1, attachments.DocCommercialRegister, attachments.StatusVerified, now),
		doc(2, attachments.DocPharmacyLicense, attachments.StatusPending, now),
		doc(3, attachments.DocTaxCard, attachments.StatusVerified, now),
	}
	customerPending := BuildOrganizationDocumentsData(docsPendingOnly, nil, false)
	if len(customerPending.Missing) != 1 {
		t.Fatalf("pending mandatory doc still requires verified doc: got %d missing", len(customerPending.Missing))
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
