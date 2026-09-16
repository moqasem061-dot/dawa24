package identity

import (
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SessionPlan is a concurrent sign-in licensing tier.
type SessionPlan struct {
	ID               int64        `json:"id"`
	Name             i18n.Text    `json:"name"`
	MaxLoginSessions int          `json:"max_login_sessions"`
	Price            money.Amount `json:"price"`
	DurationDays     int          `json:"duration_days"`
	IsFree           bool         `json:"is_free"`
	IsActive         bool         `json:"is_active"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

// DeviceDetails contains parsed client hardware and software environment.
type DeviceDetails struct {
	DeviceName string `json:"device_name"` // e.g. i18n.TDefault("w4_mod.windows_google_chrome_169")
	DeviceType string `json:"device_type"` // "desktop", "mobile", "tablet", "unknown"
	Browser    string `json:"browser"`     // "Chrome", "Safari", "Firefox", "Edge", etc.
	OS         string `json:"os"`          // "Windows", "macOS", "iOS", "Android", "Linux"
	Icon       string `json:"icon"`        // "💻", "📱", "📟"
}

// ErrSessionEvictedConcurrentLimit indicates the session was terminated because the organization exceeded its concurrent session limit.
var ErrSessionEvictedConcurrentLimit = apperr.New(apperr.KindUnauthorized, "session.evicted_concurrent_limit", i18n.TDefault("w4_mod.w4str_170_170"))

// ErrSessionEvictedDuplicateLogin indicates the user's prior session was terminated because their account was signed in from another device or browser.
var ErrSessionEvictedDuplicateLogin = apperr.New(apperr.KindUnauthorized, "session.evicted_duplicate_login", "تم تسجيل خروجك تلقائياً نظراً لتسجيل الدخول إلى حسابك من جهاز أو متصفح آخر. لا يمكن استخدام الحساب من أكثر من مكان في نفس الوقت.")

// ErrSessionIdleTimeout indicates the session was terminated because of user inactivity beyond configured idle limit.
var ErrSessionIdleTimeout = apperr.New(apperr.KindUnauthorized, "session.idle_timeout", "انتهت صلاحية الجلسة لعدم وجود نشاط. يرجى تسجيل الدخول مجدداً.")

// ParseUserAgentDevice analyzes a User-Agent string to produce human-readable Arabic device metadata.
func ParseUserAgentDevice(ua, ip string) DeviceDetails {
	uaLower := strings.ToLower(ua)
	det := DeviceDetails{
		DeviceType: "desktop",
		Browser:    i18n.TDefault("w4s_mod.s_1_1"),
		OS:         i18n.TDefault("w4s_mod.s_2_2"),
		Icon:       "💻",
	}

	if strings.TrimSpace(ua) == "" {
		det.DeviceName = i18n.TDefault("w4s_mod.s_3_3")
		return det
	}

	// 1. Detect OS
	if strings.Contains(uaLower, "windows") {
		det.OS = "Windows"
		det.DeviceType = "desktop"
		det.Icon = "💻"
	} else if strings.Contains(uaLower, "iphone") {
		det.OS = "iOS (iPhone)"
		det.DeviceType = "mobile"
		det.Icon = "📱"
	} else if strings.Contains(uaLower, "ipad") {
		det.OS = "iPadOS"
		det.DeviceType = "tablet"
		det.Icon = "📟"
	} else if strings.Contains(uaLower, "android") {
		det.OS = "Android"
		if strings.Contains(uaLower, "mobile") {
			det.DeviceType = "mobile"
			det.Icon = "📱"
		} else {
			det.DeviceType = "tablet"
			det.Icon = "📟"
		}
	} else if strings.Contains(uaLower, "macintosh") || strings.Contains(uaLower, "mac os") {
		det.OS = "macOS"
		det.DeviceType = "desktop"
		det.Icon = "💻"
	} else if strings.Contains(uaLower, "linux") {
		det.OS = "Linux"
		det.DeviceType = "desktop"
		det.Icon = "💻"
	}

	// 2. Detect Browser
	if strings.Contains(uaLower, "edg/") || strings.Contains(uaLower, "edge/") {
		det.Browser = "Edge"
	} else if strings.Contains(uaLower, "samsungbrowser") {
		det.Browser = "Samsung Internet"
	} else if strings.Contains(uaLower, "chrome") || strings.Contains(uaLower, "crios") {
		det.Browser = "Chrome"
	} else if strings.Contains(uaLower, "firefox") || strings.Contains(uaLower, "fxios") {
		det.Browser = "Firefox"
	} else if strings.Contains(uaLower, "safari") && !strings.Contains(uaLower, "chrome") {
		det.Browser = "Safari"
	} else if strings.Contains(uaLower, "opera") || strings.Contains(uaLower, "opr/") {
		det.Browser = "Opera"
	}

	// 3. Compose Friendly Name
	typeLabel := i18n.TDefault("w4s_mod.s_4_4")
	if det.DeviceType == "mobile" {
		typeLabel = i18n.TDefault("w4s_mod.s_5_5")
	} else if det.DeviceType == "tablet" {
		typeLabel = i18n.TDefault("w4s_mod.s_6_6")
	}

	det.DeviceName = typeLabel + " (" + det.OS + " - " + det.Browser + ")"
	return det
}
