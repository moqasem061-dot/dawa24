package notifications_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
)

func TestNotificationRegistryCompleteness(t *testing.T) {
	events := notifications.AllEvents()
	if len(events) < 40 {
		t.Fatalf("expected at least 40 registered notification events, got %d", len(events))
	}

	seen := make(map[notifications.EventKey]bool, len(events))
	for _, evt := range events {
		if evt.Key == "" {
			t.Errorf("event definition has empty key: %+v", evt)
		}
		if seen[evt.Key] {
			t.Errorf("duplicate event key registered: %q", evt.Key)
		}
		seen[evt.Key] = true

		if len(evt.DefaultChannels) == 0 {
			t.Errorf("event %q has no default channels", evt.Key)
		}
		if evt.TitleAr == "" {
			t.Errorf("event %q has empty Arabic title", evt.Key)
		}
		if evt.BodyAr == "" {
			t.Errorf("event %q has empty Arabic body", evt.Key)
		}
		if evt.TitleEn == "" {
			t.Errorf("event %q has empty English title", evt.Key)
		}
		if evt.BodyEn == "" {
			t.Errorf("event %q has empty English body", evt.Key)
		}

		// Verify interpolation
		vars := map[string]string{
			"order_number":    "ORD-1234",
			"total_amount":    "500",
			"vendor_name":     "مورد الاختبار",
			"customer_name":   "صيدلية الأمل",
			"pharmacy_name":   "صيدلية الأمل",
			"request_id":      "99",
			"item_count":      "5",
			"status":          "معتمد",
			"reason":          "اختبار",
			"product_name":    "بنادول",
			"quantity":        "10",
			"quote_price":     "150",
			"decision":        "قبول",
			"proposed_amount": "450",
			"offer_title":     "عرض خاص",
			"package_title":   "باقة ماسية",
			"ad_title":        "إعلان مميز",
			"run_id":          "1",
			"rows_count":      "100",
			"branch_name":     "الفرع الرئيسي",
			"branch_code":     "BR-01",
			"org_name":        "شركة الدواء",
			"section":         "identity",
			"user_name":       "أحمد",
			"user_email":      "ahmed@test.local",
			"role_name":       "مدير فرع",
			"new_role":        "مشرف",
			"account_type":    "مورد",
			"plan_name":       "الباقة المتقدمة",
			"cycle":           "شهري",
			"cost":            "300",
			"days_left":       "3",
			"amount":          "250",
			"rating":          "5",
			"review_text":     "خدمة ممتازة",
			"sender_name":     "سارة",
			"message_snippet": "مرحباً",
			"job_title":       "صيدلي",
			"applicant_name":  "محمد",
			"applicant_phone": "01000000000",
			"document_name":   "السجل التجاري",
			"description":     "صورة حديثة",
			"deadline_days":   "15",
		}

		titleAr, bodyAr := evt.Render("ar", vars)
		if titleAr == "" || bodyAr == "" {
			t.Errorf("event %q rendered empty Arabic title or body", evt.Key)
		}

		titleEn, bodyEn := evt.Render("en", vars)
		if titleEn == "" || bodyEn == "" {
			t.Errorf("event %q rendered empty English title or body", evt.Key)
		}

		tmpl := evt.ToTemplate()
		if tmpl.Slug != string(evt.Key) {
			t.Errorf("template slug mismatch: %q != %q", tmpl.Slug, evt.Key)
		}
	}
}
