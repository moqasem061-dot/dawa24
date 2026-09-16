package pages

import (
	"encoding/json"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// VendorAdsData carries all necessary information for vendor's ads page and creation wizard.
type VendorAdsData struct {
	Ads             []*promo.Ad
	ItemOptions     []VendorOfferItemOption
	ActivePurchases []*promo.SponsorshipPurchase
	TotalCredits    int
	NoticeType      string
	NoticeMsg       string
	Page            int
	PerPage         int
	TotalCount      int
}

// VendorPlacementOption defines an available ad placement slot on the platform.
type VendorPlacementOption struct {
	Key         string `json:"key"`
	TitleAr     string `json:"title_ar"`
	TitleEn     string `json:"title_en"`
	Description string `json:"description"`
	Badge       string `json:"badge"`
	Icon        string `json:"icon"`
}

// Title returns the localized title of the placement.
func (p VendorPlacementOption) Title(lang string) string {
	if lang == "en" && p.TitleEn != "" {
		return p.TitleEn
	}
	return p.TitleAr
}

// GetStandardPlacements returns all connected platform ad placement slots.
func GetStandardPlacements() []VendorPlacementOption {
	return []VendorPlacementOption{
		{
			Key:         promo.PositionHomeHero,
			TitleAr:     "معرض وبنر الواجهة الرئيسية (Hero Banner)",
			TitleEn:     "Home Landing Hero Gallery",
			Description: "يظهر في أعلى الصفحة الرئيسية بأعلى معدل مشاهدة ونقر من الصيدليات الزائرة.",
			Badge:       "الأعلى مشاهدة",
			Icon:        "sparkles",
		},
		{
			Key:         promo.PositionHomeDeals,
			TitleAr:     "شريط العروض والصفقات الترويجية (Deals Banner)",
			TitleEn:     "Home Deals & Offers Banner",
			Description: "يظهر بجوار عروض الخصم المباشر والصفقات الحية في منتصف الصفحة الرئيسية.",
			Badge:       "مميز",
			Icon:        "percent",
		},
		{
			Key:         promo.PositionHomeBottom,
			TitleAr:     "بنر أسفل الصفحة الرئيسية (Pre-Footer Banner)",
			TitleEn:     "Home Bottom Banner",
			Description: "يظهر أعلى قسم التسجيل ودعوة الانضمام أسفل الصفحة الرئيسية.",
			Badge:       "شامل",
			Icon:        "layers",
		},
		{
			Key:         promo.PositionCatalogTop,
			TitleAr:     "بنر صدارة كتالوج وسوق الأدوية (Catalog Top Header)",
			TitleEn:     "Catalog Top Header Banner",
			Description: "يظهر في أعلى نتائج البحث والتصفح داخل كتالوج التوريد الدوائي.",
			Badge:       "استراتيجي",
			Icon:        "cart",
		},
	}
}

// PlacementLabel returns the Arabic title for a placement key.
func PlacementLabel(key string) string {
	return PlacementLabelLang(key, "ar")
}

// PlacementLabelLang returns the localized title for a placement key.
func PlacementLabelLang(key, lang string) string {
	switch key {
	case promo.PositionHomeHero:
		if lang == "en" {
			return "Home (Hero Banner)"
		}
		return "الرئيسية (البانر الرئيسي)"
	case promo.PositionCatalogTop:
		if lang == "en" {
			return "Catalogue (Top Header)"
		}
		return "الكتالوج (صدارة الأصناف)"
	case promo.PositionHomeDeals:
		if lang == "en" {
			return "Home (Deals & Offers)"
		}
		return "الرئيسية (العروض والصفقات)"
	case promo.PositionHomeBanner:
		if lang == "en" {
			return "Home (Mid Banner)"
		}
		return "الرئيسية (بانر وسطي)"
	case promo.PositionHomeBottom:
		if lang == "en" {
			return "Home (Bottom Banner)"
		}
		return "الرئيسية (بانر أسفل الصفحة)"
	case "top_banner":
		if lang == "en" {
			return "Top Header Banner"
		}
		return "أعلى الموقع (الشريط العلوي)"
	case "sidebar":
		if lang == "en" {
			return "Sidebar"
		}
		return "الشريط الجانبي"
	default:
		for _, p := range GetStandardPlacements() {
			if p.Key == key {
				return p.Title(lang)
			}
		}
		if key == "" {
			return i18n.T(lang, "vendor_ads.default_placement")
		}
		return key
	}
}

// InStockItemsToJSON serializes only items that have positive available stock.
func InStockItemsToJSON(items []VendorOfferItemOption) string {
	var inStock []VendorOfferItemOption
	for _, it := range items {
		if it.AvailableStock > 0 {
			inStock = append(inStock, it)
		}
	}
	b, err := json.Marshal(inStock)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// ActiveOffersToJSON serializes active offers for sponsorship selection.
func ActiveOffersToJSON(offers []*promo.Offer) string {
	type offerOpt struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	}
	var list []offerOpt
	for _, o := range offers {
		if o != nil && o.IsActive {
			t := o.Title["ar"]
			if t == "" {
				t = o.Title["en"]
			}
			list = append(list, offerOpt{ID: o.ID, Title: t})
		}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// adsWizardState is the wizard's Alpine payload.
func adsWizardState(data VendorAdsData, lang string) string {
	labels := map[string]string{}
	for _, p := range GetStandardPlacements() {
		labels[p.Key] = p.Title(lang)
	}
	stepNames := []string{
		i18n.T(lang, "vendor_ads.wizard.step_1_name"),
		i18n.T(lang, "vendor_ads.wizard.step_2_name"),
		i18n.T(lang, "vendor_ads.wizard.step_3_name"),
		i18n.T(lang, "vendor_ads.wizard.step_4_name"),
	}
	cfg := struct {
		TotalSteps      int               `json:"totalSteps"`
		StepNames       []string          `json:"stepNames"`
		Placement       string            `json:"placement"`
		PlacementLabels map[string]string `json:"placementLabels"`
		TotalCredits    int               `json:"totalCredits"`
		CreditCost      int               `json:"creditCost"`
	}{
		TotalSteps:      4,
		StepNames:       stepNames,
		Placement:       promo.PositionHomeHero,
		PlacementLabels: labels,
		TotalCredits:    data.TotalCredits,
		CreditCost:      promo.AdCreditCost,
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return "dawaAdsWizard({})"
	}
	return fmt.Sprintf("dawaAdsWizard(%s)", string(encoded))
}
