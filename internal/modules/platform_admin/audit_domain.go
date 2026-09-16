package platformadmin

import "time"

// AuditEntry is one row of the platform audit trail, for /admin/audit.
type AuditEntry struct {
	ID               int64          `json:"id"`
	OrganizationID   *int64         `json:"organization_id,omitempty"`
	OrganizationName string         `json:"organization_name,omitempty"`
	ActorUserID      *int64         `json:"actor_user_id,omitempty"`
	ActorName        string         `json:"actor_name,omitempty"`
	ActorEmail       string         `json:"actor_email,omitempty"`
	Module           string         `json:"module,omitempty"` // Section / Module (القسم)
	Action           string         `json:"action"`           // Action (الإجراء)
	ActionLabelAr    string         `json:"action_label_ar,omitempty"`
	Title            string         `json:"title,omitempty"`       // Title (العنوان)
	Description      string         `json:"description,omitempty"` // Description (الوصف)
	Severity         string         `json:"severity,omitempty"`    // info, warning, critical (الأهمية)
	IPAddress        string         `json:"ip_address,omitempty"`  // IP address (عنوان IP)
	Route            string         `json:"route,omitempty"`       // Route / URL path (المسار)
	UserAgent        string         `json:"user_agent,omitempty"`
	EntityType       string         `json:"entity_type"`
	EntityTypeAr     string         `json:"entity_type_ar,omitempty"`
	EntityID         string         `json:"entity_id"`
	Before           map[string]any `json:"before,omitempty"`
	After            map[string]any `json:"after,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

// AuditLogFilter defines search and filter options for platform audit logs.
type AuditLogFilter struct {
	OrganizationID *int64
	ActorUserID    *int64
	Action         string
	EntityType     string
	DateFrom       *time.Time
	DateTo         *time.Time
	Search         string
	Limit          int
	Offset         int
}
