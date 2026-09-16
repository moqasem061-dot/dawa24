package assistant

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// DigestMaxChars caps the maximum length of an attachment summary.
const DigestMaxChars = 6000

// Conversation represents a multi-turn assistant session.
type Conversation struct {
	ID             int64      `json:"id"`
	PublicID       uuid.UUID  `json:"public_id"`
	OrganizationID int64      `json:"organization_id"`
	UserID         int64      `json:"user_id"`
	Title          string     `json:"title"`
	AgentRole      string     `json:"agent_role"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// ConversationSummary holds enriched metadata about an assistant conversation for administration and auditing.
type ConversationSummary struct {
	ID                int64     `json:"id"`
	PublicID          uuid.UUID `json:"public_id"`
	OrganizationID    int64     `json:"organization_id"`
	OrganizationName  string    `json:"organization_name"`
	OrganizationType  string    `json:"organization_type,omitempty"`
	UserID            int64     `json:"user_id"`
	UserName          string    `json:"user_name"`
	UserEmail         string    `json:"user_email"`
	UserPhone         string    `json:"user_phone"`
	UserRole          string    `json:"user_role,omitempty"`
	Title             string    `json:"title"`
	MessageCount      int       `json:"message_count"`
	TotalInputTokens  int       `json:"total_input_tokens"`
	TotalOutputTokens int       `json:"total_output_tokens"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// IsFlagged marks a conversation the platform already has a reason to look
	// at: the model reached for something the caller was refused, or a turn
	// ended in an error. Both are recorded facts rather than a judgement, and
	// both work with the Gateway switched off.
	IsFlagged       bool `json:"is_flagged"`
	DeniedToolCalls int  `json:"denied_tool_calls"`
}

// AdminConversationFilter narrows the assistant conversation audit.
type AdminConversationFilter struct {
	Search            string
	UserQuery         string
	OrganizationQuery string
	DateFrom          *time.Time
	DateTo            *time.Time
	FlaggedOnly       bool

	Limit  int
	Offset int
}

// Normalize clamps the page window.
func (f *AdminConversationFilter) Normalize() {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 25
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
}

// AssistantStats provides aggregated metrics for the AI Assistant platform overview.
type AssistantStats struct {
	TotalConversations int `json:"total_conversations"`
	TotalMessages      int `json:"total_messages"`
	TotalInputTokens   int `json:"total_input_tokens"`
	TotalOutputTokens  int `json:"total_output_tokens"`
	ActiveUsers        int `json:"active_users"`
}

// Message represents one turn in a conversation.
type Message struct {
	ID             int64        `json:"id"`
	ConversationID int64        `json:"conversation_id"`
	OrganizationID int64        `json:"organization_id"`
	Role           string       `json:"role"` // system | user | assistant | tool
	Content        string       `json:"content"`
	Attachments    []Attachment `json:"attachments"`
	// Entities are the records this message refers to, with the dashboard link
	// for each. Persisted with the message so reopening a conversation from
	// history keeps its links instead of degrading to plain text.
	Entities         []Entity  `json:"entities,omitempty"`
	PromptVersion    string    `json:"prompt_version"`
	ModelRole        string    `json:"model_role"`
	InputTokens      int       `json:"input_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	GatewayRequestID string    `json:"gateway_request_id"`
	CreatedAt        time.Time `json:"created_at"`
}

// Attachment is one uploaded file as a message records it.
//
// DataURL is json:"-" and that matters. It used to be a persisted field, so
// every uploaded file was base64-encoded into assistant.messages.attachments
// and handed back to the browser on every history load — a 10 MB PDF costing
// ~13 MB of JSONB per conversation. The bytes now live in object storage; this
// struct carries only what a message needs to name the file, and the DataURL is
// filled in transiently while a turn is being assembled.
type Attachment struct {
	Handle      string  `json:"handle"`
	Filename    string  `json:"filename"`
	MIMEType    string  `json:"mime_type"`
	SizeMB      float64 `json:"size_mb"`
	DataURL     string  `json:"-"`
	ContentHash string  `json:"content_hash"`
	UserID      int64   `json:"user_id"`
	OrgID       int64   `json:"org_id"`
	// RowID is the assistant.attachments primary key. Zero for a legacy row
	// saved before attachments became first-class.
	RowID int64 `json:"row_id,omitempty"`
}

// AttachmentRow is a stored attachment: metadata here, bytes in object storage.
type AttachmentRow struct {
	ID             int64
	PublicID       uuid.UUID
	OrganizationID int64
	UserID         int64
	ConversationID *int64
	Filename       string
	MIMEType       string
	SizeBytes      int64
	ContentHash    string
	StorageKey     string
	Digest         string
	Referenced     bool
	CreatedAt      time.Time
}

// TurnStatus is the lifecycle of one question.
type TurnStatus string

const (
	TurnRunning   TurnStatus = "running"
	TurnDone      TurnStatus = "done"
	TurnFailed    TurnStatus = "failed"
	TurnCancelled TurnStatus = "cancelled"
)

// Turn is one question and its answer, owned by the server rather than by the
// HTTP request that asked.
//
// This is what makes an answer survive a dropped connection. Persisting only on
// the streaming request's own context meant a navigation, a Wi-Fi drop or a
// proxy timeout discarded an answer the tenant had already been billed for.
type Turn struct {
	ID             int64
	PublicID       uuid.UUID
	ConversationID int64
	OrganizationID int64
	UserID         int64
	AgentRole      string
	Status         TurnStatus
	Question       string
	Answer         string
	ErrorCode      string
	InputTokens    int
	OutputTokens   int
	ToolCalls      int
	// Entities are the records the answer names, resolved to dashboard links.
	// Held on the turn as well as on the message because the polling fallback
	// reads the turn, and a reader who never received the stream must still get
	// a clickable answer.
	Entities   []Entity
	CreatedAt  time.Time
	FinishedAt *time.Time
}

// ToolAudit records one tool invocation and the decision made about it.
type ToolAudit struct {
	TurnID         int64
	OrganizationID int64
	UserID         int64
	AgentRole      string
	ToolName       string
	Decision       string
	Permission     string
	// Detail is a short, sanitized reason CLASS -- 'timeout', 'read_failed',
	// 'unknown_field', a permission key. It exists because a decision alone
	// cannot distinguish a broken tool from a refused one when both surface to
	// the user as an answer with no data in it. It must never carry SQL, a
	// table or column name, or anything a model wrote verbatim.
	Detail    string
	LatencyMS int
	RowCount  int
}

// ConversationRetention is how long a conversation is kept before the worker
// deletes it. Stated here rather than only in SQL because the drawer tells the
// user the rule, and the two must not be able to disagree.
const ConversationRetention = 6 * 30 * 24 * time.Hour // ~6 months

// Digest is the attachment model's structured understanding of one file.
type Digest struct {
	Filename  string   `json:"filename"`
	Kind      string   `json:"kind"`
	Summary   string   `json:"summary"`
	KeyFacts  []string `json:"key_facts"`
	Verbatim  string   `json:"verbatim,omitempty"`
	Truncated bool     `json:"truncated"`
}

// RenderBlock formats a digest into a delimited Markdown block for the primary model context.
func (d Digest) RenderBlock() string {
	var s strings.Builder
	s.WriteString(i18n.TDefault("w4_mod.w4str_51_51"))
	s.WriteString(d.Filename)
	s.WriteString(i18n.TDefault("w4_mod.n_52"))
	if d.Summary != "" {
		s.WriteString(d.Summary)
		s.WriteByte('\n')
	}
	if len(d.KeyFacts) > 0 {
		s.WriteString(i18n.TDefault("w4_mod.n_53"))
		for _, k := range d.KeyFacts {
			s.WriteString("- ")
			s.WriteString(k)
			s.WriteByte('\n')
		}
	}
	if d.Verbatim != "" {
		s.WriteString(i18n.TDefault("w4_mod.n_54"))
		s.WriteString(d.Verbatim)
		s.WriteByte('\n')
	}
	if d.Truncated {
		s.WriteString(i18n.TDefault("w4_mod.n_55"))
	}
	s.WriteString(i18n.TDefault("w4_mod.w4str_56_56"))
	return s.String()
}

// MemoryScope distinguishes org-wide shared knowledge from user-specific preferences.
type MemoryScope string

const (
	MemoryScopeOrganization MemoryScope = "organization"
	MemoryScopeUser         MemoryScope = "user"
)

// MemoryCategory classifies a remembered fact for targeted prompt injection.
type MemoryCategory string

const (
	MemoryCategoryGeneral     MemoryCategory = "general"
	MemoryCategoryProfile     MemoryCategory = "business_profile"
	MemoryCategoryProcurement MemoryCategory = "procurement"
	MemoryCategoryFinancial   MemoryCategory = "financial"
	MemoryCategoryLogistics   MemoryCategory = "logistics"
	MemoryCategoryPreferences MemoryCategory = "preferences"
	MemoryCategoryContacts    MemoryCategory = "contacts"
)

// MemorySource tracks how the memory was acquired.
type MemorySource string

const (
	MemorySourceUserExplicit MemorySource = "user_explicit"
	MemorySourceAIExtracted  MemorySource = "ai_extracted"
	MemorySourceAdminManual  MemorySource = "admin_manual"
)

// Memory represents a persistent fact, preference, or business rule remembered by Capsule.
type Memory struct {
	ID             int64          `json:"id"`
	PublicID       uuid.UUID      `json:"public_id"`
	OrganizationID int64          `json:"organization_id"`
	UserID         *int64         `json:"user_id,omitempty"`
	Scope          MemoryScope    `json:"scope"`
	Category       MemoryCategory `json:"category"`
	Key            string         `json:"key"`
	Content        string         `json:"content"`
	Confidence     float64        `json:"confidence"`
	Source         MemorySource   `json:"source"`
	SourceConvID   *int64         `json:"source_conversation_id,omitempty"`
	SourceTurnID   *int64         `json:"source_turn_id,omitempty"`
	IsActive       bool           `json:"is_active"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// CategoryLabelAr returns a clear, localized Arabic label for the category.
func (c MemoryCategory) CategoryLabelAr() string {
	switch c {
	case MemoryCategoryProfile:
		return "ملف ونشاط المنشأة"
	case MemoryCategoryProcurement:
		return "المشتريات والتوريد"
	case MemoryCategoryFinancial:
		return "التفضيلات المالية والسداد"
	case MemoryCategoryLogistics:
		return "اللوجستيات ومواعيد الاستلام"
	case MemoryCategoryPreferences:
		return "تفضيلات التعامل والتحليل"
	case MemoryCategoryContacts:
		return "جهات الاتصال والمسؤولين"
	default:
		return "معلومات عامة"
	}
}
