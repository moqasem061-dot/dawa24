package postgres

import (
	"os"
	"strings"
	"testing"
)

// Deleting a special offer that was ever ordered failed: commerce.orders.offer_id
// is ON DELETE RESTRICT, so the hard DELETE errored and the vendor saw
// "تعذر حذف العرض الخاص". The offer is trashed instead.
func TestDeleteSpecialOfferSoftDeletes(t *testing.T) {
	body, err := os.ReadFile("highlight_sections_items_repo.go")
	if err != nil {
		t.Fatalf("read repo: %v", err)
	}
	src := string(body)
	i := strings.Index(src, "func (r *Repository) DeleteSpecialOffer(")
	if i < 0 {
		t.Fatal("DeleteSpecialOffer not found")
	}
	fn := src[i:]
	fn = fn[:strings.Index(fn, "\n}\n")]
	if strings.Contains(fn, "DELETE FROM promo.offers") {
		t.Error("special offers are still hard-deleted")
	}
	if !strings.Contains(fn, "SET deleted_at = now()") {
		t.Error("special offers are not soft-deleted")
	}
}
