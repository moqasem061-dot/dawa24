package config

import (
	"regexp"
	"strings"
)

// Telegram configures the bridge that makes a Telegram bot a second interface
// to the assistant and the notification feed. Both values empty leaves the
// integration off: the bridge routes are not mounted and the settings page
// hides the Telegram card.
type Telegram struct {
	// BotUsername is the bot's @name without the @, used to build t.me links.
	BotUsername string
	// BridgeToken is the shared secret n8n presents as a Bearer token. It is
	// the only thing that lets a caller speak for a Telegram user, so it must
	// be long and random; see loadTelegram.
	BridgeToken string
	// BotToken is the optional Telegram Bot API token (e.g. 123456:ABC...).
	// When provided, it allows Dawa24 to download attached documents and photos directly.
	BotToken string
	// GatewayToken is the Telegram Gateway API token for phone verification OTPs (https://gatewayapi.telegram.org).
	// When empty, Telegram Gateway operates in sandbox/mock mode.
	GatewayToken string
}

// Enabled reports whether the bridge is fully configured.
func (t Telegram) Enabled() bool { return t.BotUsername != "" && t.BridgeToken != "" }

var botUsername = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,31}$`)

// minBridgeTokenLen is 32 characters: `openssl rand -hex 32` produces 64.
const minBridgeTokenLen = 32

func loadTelegram(fail func(string, ...any)) Telegram {
	t := Telegram{
		BotUsername:  strings.TrimPrefix(strings.TrimSpace(getStr("TELEGRAM_BOT_USERNAME", "")), "@"),
		BridgeToken:  strings.TrimSpace(getStr("TELEGRAM_BRIDGE_TOKEN", "")),
		BotToken:     strings.TrimSpace(getStr("TELEGRAM_BOT_TOKEN", "")),
		GatewayToken: strings.TrimSpace(getStr("TELEGRAM_GATEWAY_TOKEN", "")),
	}
	if t.BotUsername == "" && t.BridgeToken == "" {
		return t
	}
	// Half-configured is refused outright: a bot with no token would show a
	// link button whose messages nobody can answer, and a token with no bot
	// opens an endpoint nothing legitimate calls.
	if t.BotUsername == "" || t.BridgeToken == "" {
		fail("TELEGRAM_BOT_USERNAME and TELEGRAM_BRIDGE_TOKEN must be set together")
	}
	if t.BotUsername != "" && !botUsername.MatchString(t.BotUsername) {
		fail("TELEGRAM_BOT_USERNAME is not a valid Telegram bot username, got %q", t.BotUsername)
	}
	if t.BridgeToken != "" && len(t.BridgeToken) < minBridgeTokenLen {
		fail("TELEGRAM_BRIDGE_TOKEN must be at least %d characters (use: openssl rand -hex 32)", minBridgeTokenLen)
	}
	return t
}
