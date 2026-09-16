package pages

// institutionalWorkCategories is the fixed 12-category vocabulary a branch's
// "corporate business" (الأعمال المؤسسية) is drawn from. It is shared by the
// vendor and pharmacy branch screens — both the checkbox editor and the
// per-branch tag rendering — so it lives here rather than being declared twice
// inside the page templates (which produced a duplicate-symbol build once the
// generated code for both pages was refreshed).
var institutionalWorkCategories = []struct {
	Key     string
	LabelAr string
	LabelEn string
	Icon    string
}{
	{Key: "group", LabelAr: "مجموعة", LabelEn: "Group", Icon: ""},
	{Key: "sector", LabelAr: "قطاع", LabelEn: "Sector", Icon: ""},
	{Key: "startup", LabelAr: "شركة ناشئة", LabelEn: "Startup", Icon: ""},
	{Key: "sole_proprietorship", LabelAr: "مؤسسة فردية", LabelEn: "Sole Proprietorship", Icon: ""},
	{Key: "audience_category", LabelAr: "فئة جماهيرية", LabelEn: "Audience Category", Icon: ""},
	{Key: "retail", LabelAr: "تجزئة", LabelEn: "Retail", Icon: ""},
	{Key: "nonprofit", LabelAr: "منظمة غير ربحية", LabelEn: "Nonprofit Organization", Icon: ""},
	{Key: "factory", LabelAr: "مصنع", LabelEn: "Factory", Icon: ""},
	{Key: "cooperative", LabelAr: "جمعية تعاونية", LabelEn: "Cooperative", Icon: ""},
	{Key: "services", LabelAr: "خدمات", LabelEn: "Services", Icon: ""},
	{Key: "pharmacy", LabelAr: "صيدلية", LabelEn: "Pharmacy", Icon: ""},
	{Key: "joint_stock", LabelAr: "شركة مساهمة", LabelEn: "Joint Stock Company", Icon: ""},
}

// formatInstitutionalWorkLabel resolves a category key to its Arabic label,
// falling back to the raw key when it is not one of the twelve.
func formatInstitutionalWorkLabel(key string) string {
	for _, c := range institutionalWorkCategories {
		if c.Key == key {
			return c.LabelAr
		}
	}
	return key
}
