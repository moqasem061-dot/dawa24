package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GatewayPlan represents an AI plan in the AI Gateway with its token/request limits.
type GatewayPlan struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	RPMLimit    int    `json:"rpm_limit"`
	TPMLimit    int    `json:"tpm_limit"`
	Description string `json:"description,omitempty"`
}

// GatewayUser represents a user or organization account in the AI Gateway.
type GatewayUser struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	PlanID    string    `json:"plan_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// GatewayBudgetUsage tracks windowed consumption in the Gateway.
type GatewayBudgetUsage struct {
	WindowID        string    `json:"window_id"`
	Name            string    `json:"name"`
	DurationSeconds int       `json:"duration_seconds"`
	BudgetUSD       float64   `json:"budget_usd"`
	CurrentSpent    float64   `json:"current_spent"`
	ResetTime       time.Time `json:"reset_time"`
}

// GatewayUserDetail provides full user information including budget windows and extra credits.
type GatewayUserDetail struct {
	ID                    string               `json:"id"`
	Name                  string               `json:"name"`
	Email                 string               `json:"email"`
	PlanID                string               `json:"plan_id"`
	Status                string               `json:"status"`
	BudgetUsage           []GatewayBudgetUsage `json:"budget_usage"`
	RemainingExtraCredits float64              `json:"remaining_extra_credits"`
}

// GatewayUserSummary aggregates total lifetime requests, tokens, and cost.
type GatewayUserSummary struct {
	Requests        int     `json:"requests"`
	Successful      int     `json:"successful"`
	Failed          int     `json:"failed"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
	CostUSD         float64 `json:"cost_usd"`
	CostNanoUSD     int64   `json:"cost_nano_usd"`
	CreditsConsumed float64 `json:"credits_consumed"`
}

// GatewayVirtualKey represents an API Key generated for a Gateway user.
type GatewayVirtualKey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token,omitempty"`
	Key       string    `json:"key,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Secret returns the plaintext token from either Key or Token field.
func (k GatewayVirtualKey) Secret() string {
	if k.Key != "" {
		return k.Key
	}
	return k.Token
}

// AdminClient interacts with the AI Gateway's REST Admin management API.
type AdminClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewAdminClient creates a client targeting the AI Gateway Admin API.
// username and password can be explicit or passed as single API key / basic credential in password.
func NewAdminClient(baseURL, username, password string) *AdminClient {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.muhiya.com"
	}
	if username == "" && password != "" {
		if strings.Contains(password, ":") {
			parts := strings.SplitN(password, ":", 2)
			username = parts[0]
			password = parts[1]
		} else {
			username = "admin"
		}
	}
	if username == "" {
		username = "admin"
	}
	return &AdminClient{
		baseURL:    baseURL,
		username:   username,
		password:   password,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ListPlans queries all active plans configured in the AI Gateway.
func (c *AdminClient) ListPlans(ctx context.Context) ([]GatewayPlan, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/plans", nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error (%d): %s", resp.StatusCode, string(body))
	}
	var plans []GatewayPlan
	if err := json.NewDecoder(resp.Body).Decode(&plans); err != nil {
		return nil, err
	}
	return plans, nil
}

// GetUser queries a specific user's details and active budget allocations.
func (c *AdminClient) GetUser(ctx context.Context, userID string) (*GatewayUserDetail, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("empty user ID")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/users?id=%s", c.baseURL, userID), nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error (%d): %s", resp.StatusCode, string(body))
	}
	var u GatewayUserDetail
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserUsageSummary fetches total consumption counters for a user.
func (c *AdminClient) GetUserUsageSummary(ctx context.Context, userID string) (*GatewayUserSummary, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("empty user ID")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/logs/summary?user_id=%s", c.baseURL, userID), nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error (%d): %s", resp.StatusCode, string(body))
	}
	var s GatewayUserSummary
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GatewayLogEntry represents one logged AI request from the Gateway.
type GatewayLogEntry struct {
	ID               string    `json:"id"`
	VirtualKeyID     string    `json:"virtual_key_id"`
	UserID           string    `json:"user_id"`
	OwnerName        string    `json:"owner_name,omitempty"`
	ModelID          string    `json:"model_id"`
	RequestedModel   string    `json:"requested_model"`
	Model            string    `json:"model,omitempty"`
	ProviderID       string    `json:"provider_id,omitempty"`
	RequestPath      string    `json:"request_path"`
	StatusCode       int       `json:"status_code"`
	RequestStatus    string    `json:"request_status"`
	Status           string    `json:"status,omitempty"`
	Streamed         bool      `json:"streamed"`
	InputTokens      int       `json:"input_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	CacheReadTokens  int       `json:"cache_read_tokens"`
	CacheWriteTokens int       `json:"cache_write_tokens"`
	ReasoningTokens  *int      `json:"reasoning_tokens,omitempty"`
	Cost             float64   `json:"cost"`
	CostUSD          float64   `json:"cost_usd,omitempty"`
	CostNanoUSD      int64     `json:"cost_nano_usd"`
	CreditsConsumed  float64   `json:"credits_consumed"`
	LatencyMS        int       `json:"latency_ms"`
	DurationMs       int64     `json:"duration_ms,omitempty"`
	ClientApp        string    `json:"client_app"`
	Capability       string    `json:"capability,omitempty"`
	Feature          string    `json:"feature,omitempty"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// ResolvedModel is the model the Gateway says served the request.
//
// It returns "" when the Gateway named none. It used to return a hardcoded
// model id in that case, which put a specific model's name against requests
// nobody could show had used it — on a screen whose entire purpose is telling a
// tenant what they were billed for.
func (e GatewayLogEntry) ResolvedModel() string {
	if e.ModelID != "" {
		return e.ModelID
	}
	if e.RequestedModel != "" {
		return e.RequestedModel
	}
	return e.Model
}

func (e GatewayLogEntry) ResolvedStatus() string {
	if e.RequestStatus != "" {
		return e.RequestStatus
	}
	if e.Status != "" {
		return e.Status
	}
	if e.StatusCode >= 200 && e.StatusCode < 300 {
		return "success"
	}
	return "failed"
}

func (e GatewayLogEntry) TotalTokens() int {
	return e.InputTokens + e.OutputTokens
}

func (e GatewayLogEntry) ResolvedCost() float64 {
	if e.Cost > 0 {
		return e.Cost
	}
	if e.CostUSD > 0 {
		return e.CostUSD
	}
	if e.CostNanoUSD > 0 {
		return float64(e.CostNanoUSD) / 1e9
	}
	return 0
}

func (e GatewayLogEntry) ResolvedLatency() int64 {
	if e.LatencyMS > 0 {
		return int64(e.LatencyMS)
	}
	return e.DurationMs
}

// GetUserLogs fetches request log records for a user from the AI Gateway.
func (c *AdminClient) GetUserLogs(ctx context.Context, userID string, limit, offset int) ([]GatewayLogEntry, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("empty user ID")
	}
	if limit <= 0 {
		limit = 50
	}
	reqURL := fmt.Sprintf("%s/api/logs?user_id=%s&limit=%d&offset=%d", c.baseURL, userID, limit, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error (%d): %s", resp.StatusCode, string(body))
	}
	var logs []GatewayLogEntry
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return nil, err
	}
	return logs, nil
}

func (c *AdminClient) setAuth(req *http.Request) {
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
}

// AdminPanelUserID is the Gateway identity the admin panel itself uses.
//
// The platform's own AI work — catalogue enrichment, the admin assistant — is
// not done on behalf of any tenant, so it needs an identity of its own rather
// than borrowing a pharmacy's key and spending that pharmacy's budget.
const AdminPanelUserID = "dawa24-admin-panel"
