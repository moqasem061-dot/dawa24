package platformadmin

import "time"

// ErrorLog represents a comprehensive diagnostic error / exception record.
type ErrorLog struct {
	ID               int64          `json:"id"`
	UserID           *int64         `json:"user_id,omitempty"`
	UserName         string         `json:"user_name,omitempty"`
	UserEmail        string         `json:"user_email,omitempty"`
	OrganizationName string         `json:"organization_name,omitempty"`
	ErrorLevel       string         `json:"error_level"` // CRITICAL, ERROR, WARNING, EXCEPTION
	ErrorMessage     string         `json:"error_message"`
	ExceptionClass   string         `json:"exception_class,omitempty"`
	StackTrace       string         `json:"stack_trace,omitempty"`
	FilePath         string         `json:"file_path,omitempty"`
	LineNumber       int            `json:"line_number,omitempty"`
	HTTPMethod       string         `json:"http_method,omitempty"`
	URLPath          string         `json:"url_path,omitempty"`
	IPAddress        string         `json:"ip_address,omitempty"`
	UserAgent        string         `json:"user_agent,omitempty"`
	RequestPayload   map[string]any `json:"request_payload,omitempty"`
	Status           string         `json:"status"` // NEW, INVESTIGATING, RESOLVED, IGNORED
	CreatedAt        time.Time      `json:"created_at"`
}

// ErrorLogFilter defines search and filter options for system diagnostic error logs.
type ErrorLogFilter struct {
	Level  string
	Status string
	Search string
	UserID *int64
	Limit  int
	Offset int
}
