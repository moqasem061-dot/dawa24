package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ProvisionAdminPanel creates (or refreshes) the admin panel's Gateway user and
// issues it a virtual key.
//
// It exists because the admin credentials configured in إعدادات النظام are
// basic-auth for the /api management surface, not a Bearer token for /v1. Using
// them to call a completion is a 401 every time, which is exactly why the AI
// features appeared to do nothing. This turns the credentials an operator does
// have into the key the runtime actually needs.
func (c *AdminClient) ProvisionAdminPanel(ctx context.Context, planID string) (userID, virtualKey string, err error) {
	if planID == "" {
		planID = "plan-pos-free"
	}

	user := GatewayUser{
		ID:     AdminPanelUserID,
		Name:   "Dawa24 Admin Panel",
		Email:  "admin-panel@dawa24.app",
		PlanID: planID,
		Status: "active",
	}
	if err := c.upsertUser(ctx, user); err != nil {
		return AdminPanelUserID, "", err
	}

	key, err := c.issueKey(ctx, AdminPanelUserID, "Dawa24-Admin-Panel-Key")
	if err != nil {
		return AdminPanelUserID, "", err
	}
	return AdminPanelUserID, key, nil
}

// upsertUser creates the user, falling back to an update when it already exists.
func (c *AdminClient) upsertUser(ctx context.Context, user GatewayUser) error {
	body, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("gateway admin: encode user: %w", err)
	}

	status, raw, err := c.send(ctx, http.MethodPost, "/api/users", body)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusCreated {
		return nil
	}

	// Already registered: bring its plan and status back into line.
	status, raw, err = c.send(ctx, http.MethodPut, "/api/users", body)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("gateway admin: register %q failed (%d): %s", user.ID, status, truncateBody(raw))
	}
	return nil
}

// issueKey mints a virtual key for a Gateway user.
func (c *AdminClient) issueKey(ctx context.Context, userID, name string) (string, error) {
	body, err := json.Marshal(map[string]string{"name": name, "user_id": userID})
	if err != nil {
		return "", fmt.Errorf("gateway admin: encode key request: %w", err)
	}

	status, raw, err := c.send(ctx, http.MethodPost, "/api/keys", body)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", fmt.Errorf("gateway admin: issue key for %q failed (%d): %s", userID, status, truncateBody(raw))
	}

	var created GatewayVirtualKey
	if err := json.Unmarshal(raw, &created); err != nil {
		return "", fmt.Errorf("gateway admin: decode issued key: %w", err)
	}
	if created.Secret() == "" {
		return "", fmt.Errorf("gateway admin: gateway issued an empty key for %q", userID)
	}
	return created.Secret(), nil
}

// send performs one authenticated management call and returns its raw result.
func (c *AdminClient) send(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("gateway admin: build %s %s: %w", method, path, err)
	}
	c.setAuth(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("gateway admin: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("gateway admin: read %s %s: %w", method, path, err)
	}
	return resp.StatusCode, raw, nil
}

// Ping checks that the configured admin credentials actually work, so the
// settings screen can tell an operator their credentials are wrong instead of
// leaving every AI feature quietly disabled.
func (c *AdminClient) Ping(ctx context.Context) error {
	status, raw, err := c.send(ctx, http.MethodGet, "/api/plans", nil)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("gateway admin: credentials rejected (%d)", status)
	}
	if status != http.StatusOK {
		return fmt.Errorf("gateway admin: unexpected status %d: %s", status, truncateBody(raw))
	}
	return nil
}

// truncateBody keeps an upstream error readable in a log line.
func truncateBody(raw []byte) string {
	const limit = 300
	text := strings.TrimSpace(string(raw))
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}

// ValidateVirtualKey reports whether a Bearer key the Gateway issued still
// works.
//
// A key can stop working without anything local changing: issuing a new one for
// the same user revokes the previous, so a second app instance booting — or a
// re-run of provisioning — silently invalidates the key the first one stored.
// Checking costs one credential-free request against the model list, which is
// far cheaper than discovering the problem as a failed import.
func (c *AdminClient) ValidateVirtualKey(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("gateway admin: empty virtual key")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("gateway admin: build key check: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gateway admin: check key: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("gateway admin: virtual key rejected (%d)", resp.StatusCode)
	}
	return nil
}
