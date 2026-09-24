package config

import "crypto/rand"

// randomSecret makes a session secret (52 random base32 characters, about 260
// bits) for a process started without SESSION_SECRET, so a deploy that forgot
// it still boots. Sessions signed with it end when the process does.
func randomSecret() string {
	return rand.Text() + rand.Text()
}
