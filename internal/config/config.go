// Package config reads the server configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config is the runtime configuration.
type Config struct {
	// DBPath is the SQLite database file (RM_DB_PATH). It must be on a
	// local disk, never a network file system.
	DBPath string
	// Listen is the HTTP listen address (RM_LISTEN).
	Listen string
	// TrustedProxyHeader names the header the reverse proxy sets with the
	// client address, e.g. X-Forwarded-For or X-Real-IP
	// (RM_TRUSTED_PROXY_HEADER). Empty means use the TCP peer address.
	TrustedProxyHeader string
	// CookieSecure marks cookies Secure (RM_COOKIE_SECURE, default true).
	// Disable only for local development over plain HTTP.
	CookieSecure bool
	// LogLevel is debug, info, warn or error (RM_LOG_LEVEL).
	LogLevel slog.Level
}

// FromEnv reads the configuration.
func FromEnv() (Config, error) {
	c := Config{
		DBPath:             env("RM_DB_PATH", "data/register_manager.db"),
		Listen:             env("RM_LISTEN", ":8080"),
		TrustedProxyHeader: strings.TrimSpace(os.Getenv("RM_TRUSTED_PROXY_HEADER")),
		CookieSecure:       true,
	}
	if v := os.Getenv("RM_COOKIE_SECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("RM_COOKIE_SECURE: %w", err)
		}
		c.CookieSecure = b
	}
	if err := c.LogLevel.UnmarshalText([]byte(env("RM_LOG_LEVEL", "info"))); err != nil {
		return c, fmt.Errorf("RM_LOG_LEVEL: %w", err)
	}
	return c, nil
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
