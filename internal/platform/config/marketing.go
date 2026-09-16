package config

import "strings"

// Marketing configures the read-only content digest the social-media workflow
// in n8n reads. An empty token leaves the route unmounted.
type Marketing struct {
	// BridgeToken is the Bearer secret n8n presents. It is separate from the
	// Telegram bridge token so either can be rotated without the other.
	BridgeToken string
}

func loadMarketing(fail func(string, ...any)) Marketing {
	m := Marketing{BridgeToken: strings.TrimSpace(getStr("MARKETING_BRIDGE_TOKEN", ""))}
	if m.BridgeToken != "" && len(m.BridgeToken) < minBridgeTokenLen {
		fail("MARKETING_BRIDGE_TOKEN must be at least %d characters (use: openssl rand -hex 32)", minBridgeTokenLen)
	}
	return m
}
