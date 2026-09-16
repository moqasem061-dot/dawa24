package config

import (
	"regexp"
	"strings"
)

// WhatsApp configures the bridge that makes a WhatsApp Business number an
// interface to the assistant and the notification feed. BusinessNumber and
// BridgeToken empty leave the integration off: the bridge routes are not
// mounted and the settings page hides the WhatsApp card.
type WhatsApp struct {
	// BusinessNumber is the WhatsApp Business number in international form
	// without "+", used to build wa.me links.
	BusinessNumber string
	// BridgeToken is the shared secret n8n presents as a Bearer token.
	BridgeToken string
	// NotificationTemplate is an approved utility template with one body
	// parameter, for notifications outside WhatsApp's 24-hour window. Empty
	// drops those notifications.
	NotificationTemplate string
	// TemplateLanguage is the template's language code.
	TemplateLanguage string
}

// Enabled reports whether the bridge is fully configured.
func (w WhatsApp) Enabled() bool { return w.BusinessNumber != "" && w.BridgeToken != "" }

var (
	businessNumber   = regexp.MustCompile(`^[1-9][0-9]{7,14}$`)
	templateName     = regexp.MustCompile(`^[a-z0-9_]{1,512}$`)
	templateLanguage = regexp.MustCompile(`^[a-z]{2,3}(_[A-Z]{2})?$`)
)

func loadWhatsApp(fail func(string, ...any)) WhatsApp {
	w := WhatsApp{
		BusinessNumber:       strings.TrimPrefix(strings.TrimSpace(getStr("WHATSAPP_BUSINESS_NUMBER", "")), "+"),
		BridgeToken:          strings.TrimSpace(getStr("WHATSAPP_BRIDGE_TOKEN", "")),
		NotificationTemplate: strings.TrimSpace(getStr("WHATSAPP_NOTIFICATION_TEMPLATE", "")),
		TemplateLanguage:     strings.TrimSpace(getStr("WHATSAPP_TEMPLATE_LANGUAGE", "ar")),
	}
	if w.BusinessNumber == "" && w.BridgeToken == "" {
		return w
	}
	// Half-configured is refused outright, for the same reasons as Telegram.
	if w.BusinessNumber == "" || w.BridgeToken == "" {
		fail("WHATSAPP_BUSINESS_NUMBER and WHATSAPP_BRIDGE_TOKEN must be set together")
	}
	if w.BusinessNumber != "" && !businessNumber.MatchString(w.BusinessNumber) {
		fail("WHATSAPP_BUSINESS_NUMBER must be the number in international form without + or spaces, got %q", w.BusinessNumber)
	}
	if w.BridgeToken != "" && len(w.BridgeToken) < minBridgeTokenLen {
		fail("WHATSAPP_BRIDGE_TOKEN must be at least %d characters (use: openssl rand -hex 32)", minBridgeTokenLen)
	}
	if w.NotificationTemplate != "" && !templateName.MatchString(w.NotificationTemplate) {
		fail("WHATSAPP_NOTIFICATION_TEMPLATE is not a valid template name, got %q", w.NotificationTemplate)
	}
	if !templateLanguage.MatchString(w.TemplateLanguage) {
		fail("WHATSAPP_TEMPLATE_LANGUAGE is not a valid language code, got %q", w.TemplateLanguage)
	}
	return w
}
