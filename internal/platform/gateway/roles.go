package gateway

import (
	"context"
	"os"
)

// Role names a job the assistant needs done. The mapping to a Gateway model is
// configuration, and lives here because model identifiers must not appear
// outside this package (AGENTS.md R2).
type Role string

const (
	RolePrimary    Role = "assistant.primary"    // conversation, images, video
	RoleAttachment Role = "assistant.attachment" // documents and audio understanding
	RoleTranscribe Role = "assistant.transcribe" // speech to text

	// Capability roles. Before these existed the domain capabilities ran on
	// whatever model the call site happened to pass, which meant an operator
	// could not point them anywhere and a cost surprise had no dial to turn.
	RoleMatching Role = "matching.adjudicate"   // batch product-match adjudication
	RoleColumns  Role = "import.detect_columns" // spreadsheet header detection
	RoleExpand   Role = "search.expand_query"   // search synonym expansion
	RoleClassify Role = "support.classify"      // support ticket triage

	// Per-tool matching roles (WO-39)
	RoleMatchVendorImport   Role = "matching.vendor_import"   // /vendor/ingest
	RoleMatchSavingProducts Role = "matching.saving_products" // vendor + pharmacy saving lists
	RoleMatchSmartOrder     Role = "matching.smart_order"     // /customer/smart-order
	RoleMatchAdminCatalog   Role = "matching.admin_catalog"   // /admin/products/import
)

// defaultRoleModels is the fallback when the operator has not overridden a role.
var defaultRoleModels = map[Role]string{
	RolePrimary:    "gemma-4-31b-it",
	RoleAttachment: "gemma-4-31b-it",
	RoleTranscribe: "whisper-large-v3-turbo",

	RoleMatching: "qwen3.7-flash",
	RoleColumns:  "qwen3.7-flash",
	RoleExpand:   "qwen3.7-flash",
	RoleClassify: "qwen3.7-flash",

	RoleMatchVendorImport:   "qwen3.7-flash",
	RoleMatchSavingProducts: "qwen3.7-flash",
	RoleMatchSmartOrder:     "qwen3.7-flash",
	RoleMatchAdminCatalog:   "qwen3.7-flash",
}

func isMatchingSubRole(role Role) bool {
	switch role {
	case RoleMatchVendorImport, RoleMatchSavingProducts, RoleMatchSmartOrder, RoleMatchAdminCatalog:
		return true
	default:
		return false
	}
}

// ResolveRoleModel resolves a role's concrete model using:
// 1. DB settings row (settings.RoleModels)
// 2. Environment variable override
// 3. Package default (defaultRoleModels)
// If a specific matching role is unset in DB or env, it falls back to RoleMatching.
func ResolveRoleModel(settings Settings, role Role) string {
	if settings.RoleModels != nil {
		if m, ok := settings.RoleModels[string(role)]; ok && m != "" {
			return m
		}
		if isMatchingSubRole(role) {
			if m, ok := settings.RoleModels[string(RoleMatching)]; ok && m != "" {
				return m
			}
		}
	}
	return resolveRoleModel(role)
}

// resolveRoleModel returns the concrete Gateway model identifier using env vars and defaults.
func resolveRoleModel(role Role) string {
	var envKey string
	switch role {
	case RolePrimary:
		envKey = "GATEWAY_MODEL_ASSISTANT_PRIMARY"
	case RoleAttachment:
		envKey = "GATEWAY_MODEL_ASSISTANT_ATTACHMENT"
	case RoleTranscribe:
		envKey = "GATEWAY_MODEL_ASSISTANT_TRANSCRIBE"
	case RoleMatching:
		envKey = "GATEWAY_MODEL_MATCHING"
	case RoleColumns:
		envKey = "GATEWAY_MODEL_COLUMNS"
	case RoleExpand:
		envKey = "GATEWAY_MODEL_EXPAND"
	case RoleClassify:
		envKey = "GATEWAY_MODEL_CLASSIFY"
	case RoleMatchVendorImport:
		envKey = "GATEWAY_MODEL_MATCHING_VENDOR_IMPORT"
	case RoleMatchSavingProducts:
		envKey = "GATEWAY_MODEL_MATCHING_SAVING_PRODUCTS"
	case RoleMatchSmartOrder:
		envKey = "GATEWAY_MODEL_MATCHING_SMART_ORDER"
	case RoleMatchAdminCatalog:
		envKey = "GATEWAY_MODEL_MATCHING_ADMIN_CATALOG"
	}
	if envKey != "" {
		if override := os.Getenv(envKey); override != "" {
			return override
		}
	}
	// Fall back to general matching env var if sub-role env is not set
	if isMatchingSubRole(role) {
		if override := os.Getenv("GATEWAY_MODEL_MATCHING"); override != "" {
			return override
		}
	}
	if model, ok := defaultRoleModels[role]; ok {
		return model
	}
	if isMatchingSubRole(role) {
		if model, ok := defaultRoleModels[RoleMatching]; ok {
			return model
		}
	}
	return string(role)
}

// ResolveRoleModel returns the concrete model for a role using the client's current settings.
func (c *HTTPClient) ResolveRoleModel(ctx context.Context, role Role) string {
	settings := c.resolve(ctx)
	return ResolveRoleModel(settings, role)
}

// RoleForRequest determines the Role for an incoming Request.
func RoleForRequest(req Request) Role {
	if req.Role != "" {
		return req.Role
	}
	switch req.Feature {
	case "variant_match", "vendor_import":
		return RoleMatchVendorImport
	case "savings_import", "saving_products":
		return RoleMatchSavingProducts
	case "smart_order":
		return RoleMatchSmartOrder
	case "catalog_import", "admin_catalog":
		return RoleMatchAdminCatalog
	case "column_detect":
		return RoleColumns
	case "search_expand":
		return RoleExpand
	case "classify":
		return RoleClassify
	}
	switch req.Capability {
	case CapMatchAdjudicate, CapMatchEnhance, CapProductMatch:
		return RoleMatching
	case CapColumnDetect:
		return RoleColumns
	case CapSearchExpand:
		return RoleExpand
	}
	return ""
}

// IsRoleDisabled returns true if the operator has explicitly disabled this role.
func IsRoleDisabled(settings Settings, role Role) bool {
	if settings.RoleDisabled != nil && settings.RoleDisabled[string(role)] {
		return true
	}
	return false
}

// GetRoleMaxTokens returns the configured max-tokens override for a role, if set.
func GetRoleMaxTokens(settings Settings, role Role) int {
	if settings.RoleMaxTokens != nil {
		if tokens, ok := settings.RoleMaxTokens[string(role)]; ok && tokens > 0 {
			return tokens
		}
		if isMatchingSubRole(role) {
			if tokens, ok := settings.RoleMaxTokens[string(RoleMatching)]; ok && tokens > 0 {
				return tokens
			}
		}
	}
	return 0
}
