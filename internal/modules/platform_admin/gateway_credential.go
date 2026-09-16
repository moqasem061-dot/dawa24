package platformadmin

import (
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Refusing to send the wrong secret to a third party.
//
// The Gateway administrator credential is sent as HTTP Basic auth to whatever
// host is configured in إعدادات النظام. That makes this one field the shortest
// path there is from "an operator pasted the wrong thing" to "a production
// secret has left the building".
//
// It is not a hypothetical. The live configuration was found holding
//
//	postgres:<REDACTED — was the production superuser password>
//
// — the production PostgreSQL superuser credential — which the client dutifully
// split on the colon and sent to the Gateway host on every provisioning call,
// every plan listing and every usage query. Nobody noticed, because the failure
// mode of a wrong credential here is silent: provisioning returned a user id
// with no key, and every caller fell through to its fallback.
//
// The check below is deliberately about shape, not about any particular secret.
// It cannot know which password is which, and it does not try. It knows what a
// database connection string looks like, and it knows that a value carrying the
// name of a database role or a scheme prefix is not a Gateway credential.

// databaseUsers are the role names that turn up at the front of a DSN-derived
// credential. An operator whose Gateway account really is called "postgres"
// has bigger problems than this check.
var databaseUsers = []string{"postgres", "root", "mysql", "mariadb", "dawa24_app"}

// secretSchemes are URL schemes that mean the value is a connection string, not
// a password.
var secretSchemes = []string{
	"postgres://", "postgresql://", "mysql://", "redis://", "rediss://",
	"mongodb://", "amqp://", "http://", "https://",
}

// knownDatabaseSecret is this process's own database password, registered at
// boot so the settings screen can point out when the same string is also being
// used as the Gateway credential.
//
// It is advisory, not a veto. Reusing one password across the database and the
// Gateway is a legitimate configuration — an operator who runs both and chose
// the same secret is not making a mistake, and refusing their save would leave
// AI unconfigurable with no way around it. What the platform owes them is to
// say plainly that the value travels to an external host, and then to do as
// they asked.
var knownDatabaseSecret string

// SetKnownDatabaseSecret registers the database password this process connects
// with, so MatchesDatabaseSecret can recognise it.
//
// Called once from the composition root. An empty value disables the check,
// which is what a deployment with no password in its DSN gets.
func SetKnownDatabaseSecret(secret string) {
	knownDatabaseSecret = strings.TrimSpace(secret)
}

// MatchesDatabaseSecret reports whether a credential is this process's own
// database password, in either the bare or the "user:password" form.
//
// Callers use it to warn. Nothing uses it to refuse.
func MatchesDatabaseSecret(raw string) bool {
	if knownDatabaseSecret == "" {
		return false
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return false
	}
	_, pass, _ := strings.Cut(value, ":")
	return value == knownDatabaseSecret || pass == knownDatabaseSecret
}

// ValidateAdminCredential rejects a value that is evidently not a Gateway
// administrator credential.
//
// It errs towards accepting: a false rejection blocks an operator from
// configuring AI at all, while the class of mistake it exists to catch has one
// very recognisable shape. Anything it is unsure about is allowed through.
func ValidateAdminCredential(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}

	lower := strings.ToLower(value)
	for _, scheme := range secretSchemes {
		if strings.HasPrefix(lower, scheme) {
			return apperr.Validation("gateway.credential_is_connection_string",
				i18n.TDefault("w4_mod.connection_string_232")+
					i18n.TDefault("w4_mod.w4str_233_233"), nil)
		}
	}

	user, _, hasColon := strings.Cut(value, ":")
	if !hasColon {
		return nil
	}
	user = strings.ToLower(strings.TrimSpace(user))
	for _, name := range databaseUsers {
		if user == name {
			return apperr.Validation("gateway.credential_is_database_user",
				i18n.TDefault("w4_mod.w4str_234_234")+
					i18n.TDefault("w4_mod.w4str_235_235")+
					i18n.TDefault("w4_mod.w4str_236_236"), nil)
		}
	}
	return nil
}

// CredentialLooksMisconfigured reports the same condition without producing an
// error, for a settings screen that wants to warn about a value already stored
// rather than block one being submitted.
//
// Existing deployments are the reason this exists separately: the wrong
// credential is already in the database on at least one of them, and an
// operator opening the screen needs to be told so even though they are not the
// one who typed it.
func (g *GatewaySettings) CredentialLooksMisconfigured() bool {
	if g == nil {
		return false
	}
	// Reuse of the database password is reported here and nowhere else: it is
	// worth an operator's attention, but it is their configuration to make, so
	// it warns and never blocks.
	return ValidateAdminCredential(g.APIKey) != nil || MatchesDatabaseSecret(g.APIKey)
}
