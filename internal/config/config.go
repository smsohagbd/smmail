package config

import "os"

type Config struct {
	SQLitePath       string
	HTTPListenAddr   string
	SMTPListenAddr   string
	AdminToken       string
	AdminUsername    string
	AdminPassword    string
	UpstreamSMTPHost string
	UpstreamSMTPPort string
	UpstreamSMTPUser string
	UpstreamSMTPPass string
	UpstreamMailFrom string
}

func Load() Config {
	return Config{
		SQLitePath:       getenv("SQLITE_PATH", "./smtp.db"),
		HTTPListenAddr:   getenv("HTTP_LISTEN_ADDR", ":8080"),
		SMTPListenAddr:   getenv("SMTP_LISTEN_ADDR", ":2525"),
		AdminToken:       getenv("ADMIN_TOKEN", "change-me-admin-token"),
		AdminUsername:    getenv("ADMIN_USERNAME", "admin"),
		AdminPassword:    getenv("ADMIN_PASSWORD", "admin123"),
		UpstreamSMTPHost: getenv("UPSTREAM_SMTP_HOST", ""),
		UpstreamSMTPPort: getenv("UPSTREAM_SMTP_PORT", "587"),
		UpstreamSMTPUser: getenv("UPSTREAM_SMTP_USER", ""),
		UpstreamSMTPPass: getenv("UPSTREAM_SMTP_PASS", ""),
		UpstreamMailFrom: getenv("UPSTREAM_MAIL_FROM", ""),
	}
}

func getenv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
