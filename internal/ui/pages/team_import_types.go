package pages

import (
	"time"
)

// TeamImportPhase represents the wizard step of an employee import session.
type TeamImportPhase string

const (
	TeamPhaseUpload    TeamImportPhase = "upload"
	TeamPhaseMapping   TeamImportPhase = "mapping"
	TeamPhaseReview    TeamImportPhase = "review"
	TeamPhaseCompleted TeamImportPhase = "completed"
)

// TeamDetectedCols holds column indices for employee data detected in spreadsheet.
type TeamDetectedCols struct {
	NameCol     int `json:"name_col"`
	EmailCol    int `json:"email_col"`
	PhoneCol    int `json:"phone_col"`
	RoleCol     int `json:"role_col"`
	JobTitleCol int `json:"job_title_col"`
	BranchCol   int `json:"branch_col"`
	CodeCol     int `json:"code_col"`
	NotesCol    int `json:"notes_col"`
}

// ExcelRoleInfo represents a distinct role found in the spreadsheet and its mapped platform role.
type ExcelRoleInfo struct {
	RawName        string `json:"raw_name"`         // Role name as written in the Excel file
	RowCount       int    `json:"row_count"`        // How many rows carry this role
	MatchedRoleID  int64  `json:"matched_role_id"`  // Mapped tenant role ID
	MatchedRoleKey string `json:"matched_role_key"` // Mapped tenant role key
	MatchedName    string `json:"matched_name"`     // Display name of matched role
	IsAutoMatched  bool   `json:"is_auto_matched"`
}

// TeamImportRow represents one parsed employee row from the spreadsheet.
type TeamImportRow struct {
	Index              int    `json:"index"` // 1-based data row number
	RawName            string `json:"raw_name"`
	Email              string `json:"email"`
	Phone              string `json:"phone"`
	RawRole            string `json:"raw_role"` // Role as written in file
	AssignedRoleID     int64  `json:"assigned_role_id"`
	AssignedRoleKey    string `json:"assigned_role_key"`
	AssignedRoleName   string `json:"assigned_role_name"`
	JobTitle           string `json:"job_title"`
	EmployeeCode       string `json:"employee_code"`
	RawBranch          string `json:"raw_branch"`
	AssignedBranchID   *int64 `json:"assigned_branch_id,omitempty"`
	AssignedBranchName string `json:"assigned_branch_name,omitempty"`

	// Validation flags
	IsValid         bool   `json:"is_valid"`
	ValidationError string `json:"validation_error,omitempty"`
	IsExistingUser  bool   `json:"is_existing_user"` // Whether user account already exists in DB
	ImportStatus    string `json:"import_status"`    // "ready", "imported", "skipped", "failed"
}

// TeamRoleOption is a selectable role in the organization.
type TeamRoleOption struct {
	ID      int64  `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	IsOwner bool   `json:"is_owner"`
}

// TeamBranchOption is a selectable branch in the organization.
type TeamBranchOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// TeamImportSession holds in-flight state for a user's bulk team import wizard.
type TeamImportSession struct {
	ID             string             `json:"id"`
	OrganizationID int64              `json:"organization_id"`
	OrgType        string             `json:"org_type"` // "vendor" or "customer"
	UserID         int64              `json:"user_id"`
	Filename       string             `json:"filename"`
	Phase          TeamImportPhase    `json:"phase"`
	TotalRows      int                `json:"total_rows"`
	Headers        []string           `json:"headers"`
	SampleRows     [][]string         `json:"sample_rows"`
	RawDataRows    [][]string         `json:"raw_data_rows"`
	DetectedCols   TeamDetectedCols   `json:"detected_cols"`
	ExcelRoles     []*ExcelRoleInfo   `json:"excel_roles"`
	DefaultRoleID  int64              `json:"default_role_id"`
	Rows           []*TeamImportRow   `json:"rows"`
	CompanyRoles   []TeamRoleOption   `json:"company_roles"`
	Branches       []TeamBranchOption `json:"branches"`

	// Results after commit
	ImportedCount int `json:"imported_count"`
	SkippedCount  int `json:"skipped_count"`
	FailedCount   int `json:"failed_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TeamImportView is the viewmodel passed to the Templ page.
type TeamImportView struct {
	Audience   string               `json:"audience"`   // "vendor" or "customer"
	BaseURL    string               `json:"base_url"`   // "/vendor/team" or "/customer/team"
	ImportURL  string               `json:"import_url"` // "/vendor/team/import" or "/customer/team/import"
	Session    *TeamImportSession   `json:"session,omitempty"`
	Sessions   []*TeamImportSession `json:"sessions,omitempty"`
	NoticeType string               `json:"notice_type,omitempty"`
	NoticeMsg  string               `json:"notice_msg,omitempty"`
	Fatal      string               `json:"fatal,omitempty"`
}

// WizardStep returns 1..4 for WizardRail rendering.
func (v TeamImportView) WizardStep() int {
	if v.Session == nil {
		return 1
	}
	switch v.Session.Phase {
	case TeamPhaseMapping:
		return 2
	case TeamPhaseReview:
		return 3
	case TeamPhaseCompleted:
		return 4
	default:
		return 1
	}
}
