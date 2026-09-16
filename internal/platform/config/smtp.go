package config

import "time"

// SMTP configures outbound email delivery via SMTP.
type SMTP struct {
	Host       string
	Port       int
	Username   string
	Password   string
	FromEmail  string
	FromName   string
	Encryption string // "starttls", "tls", "none"
	Enabled    bool
	Timeout    time.Duration
}

func loadSMTP() SMTP {
	host := getStr("SMTP_HOST", "")
	port := getInt("SMTP_PORT", 587)
	enc := getStr("SMTP_ENCRYPTION", "")
	if enc == "" {
		if port == 465 {
			enc = "tls"
		} else {
			enc = "starttls"
		}
	}

	fromEmail := getStr("SMTP_FROM_EMAIL", "no-reply@dawa24.net")
	fromName := getStr("SMTP_FROM_NAME", "Dawa24 | دوا 24")

	return SMTP{
		Host:       host,
		Port:       port,
		Username:   getStr("SMTP_USERNAME", ""),
		Password:   getStr("SMTP_PASSWORD", ""),
		FromEmail:  fromEmail,
		FromName:   fromName,
		Encryption: enc,
		Enabled:    host != "" && getBool("SMTP_ENABLED", true),
		Timeout:    getDuration("SMTP_TIMEOUT", 10*time.Second),
	}
}
