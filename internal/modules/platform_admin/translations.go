package platformadmin

import "time"

// Translation represents a platform localization entry.
type Translation struct {
	ID          int64     `json:"id"`
	Key         string    `json:"key"`
	Namespace   string    `json:"namespace"`
	TextAR      string    `json:"text_ar"`
	TextEN      string    `json:"text_en"`
	Description string    `json:"description,omitempty"`
	IsCustom    bool      `json:"is_custom"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TranslationFilter specifies search and filtering criteria for translations.
type TranslationFilter struct {
	Query     string `json:"query"`
	Namespace string `json:"namespace"`
	Custom    *bool  `json:"custom"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

// TranslationStats summarizes translation metrics for the admin overview.
type TranslationStats struct {
	TotalKeys       int `json:"total_keys"`
	CustomOverrides int `json:"custom_overrides"`
	TotalNamespaces int `json:"total_namespaces"`
}
