// Package config reads the bot's configuration from environment variables
// only, so the same binary runs with `go run` and in Docker.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // TZ_REPORTS must resolve without system zoneinfo.
)

// Update modes for MODE_DEFAULT.
const (
	ModePolling = "polling"
	ModeWebhook = "webhook"
)

// Config is the bot's configuration. Secrets are never logged: use it as a
// slog value (it implements slog.LogValuer).
type Config struct {
	BotToken          string         // BALE_BOT_TOKEN (required)
	LLMAPIKey         string         // LLM_API_KEY (Metis key)
	OwnerID           int64          // OWNER_BALE_ID (required)
	WebhookURL        string         // WEBHOOK_URL, public base URL
	WebhookSecretPath string         // WEBHOOK_SECRET_PATH
	ModeDefault       string         // MODE_DEFAULT: "", "polling" or "webhook"
	ListenAddr        string         // LISTEN_ADDR, default ":8080"
	DataDir           string         // DATA_DIR, default "./data"
	ReportsTZ         *time.Location // TZ_REPORTS, default Asia/Tehran
}

// Load reads the configuration through getenv (normally os.Getenv).
func Load(getenv func(string) string) (Config, error) {
	get := func(k, def string) string {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		BotToken:          get("BALE_BOT_TOKEN", ""),
		LLMAPIKey:         get("LLM_API_KEY", ""),
		WebhookURL:        strings.TrimRight(get("WEBHOOK_URL", ""), "/"),
		WebhookSecretPath: get("WEBHOOK_SECRET_PATH", ""),
		ModeDefault:       strings.ToLower(get("MODE_DEFAULT", "")),
		ListenAddr:        get("LISTEN_ADDR", ":8080"),
		DataDir:           get("DATA_DIR", "./data"),
	}

	var errs []error
	if c.BotToken == "" {
		errs = append(errs, errors.New("BALE_BOT_TOKEN is required"))
	}
	if owner := get("OWNER_BALE_ID", ""); owner == "" {
		errs = append(errs, errors.New("OWNER_BALE_ID is required"))
	} else if id, err := strconv.ParseInt(owner, 10, 64); err != nil || id == 0 {
		errs = append(errs, fmt.Errorf("OWNER_BALE_ID %q is not a Bale user id", owner))
	} else {
		c.OwnerID = id
	}
	switch c.ModeDefault {
	case "", ModePolling, ModeWebhook:
	default:
		errs = append(errs, fmt.Errorf("MODE_DEFAULT %q must be polling or webhook", c.ModeDefault))
	}
	tz := get("TZ_REPORTS", "Asia/Tehran")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		errs = append(errs, fmt.Errorf("TZ_REPORTS %q: %w", tz, err))
	}
	c.ReportsTZ = loc
	return c, errors.Join(errs...)
}

// LogValue implements slog.LogValuer with secrets redacted.
func (c Config) LogValue() slog.Value {
	set := func(s string) string {
		if s == "" {
			return "unset"
		}
		return "set"
	}
	tz := ""
	if c.ReportsTZ != nil {
		tz = c.ReportsTZ.String()
	}
	return slog.GroupValue(
		slog.String("bale_bot_token", set(c.BotToken)),
		slog.String("llm_api_key", set(c.LLMAPIKey)),
		slog.Int64("owner_bale_id", c.OwnerID),
		slog.String("webhook_url", c.WebhookURL),
		slog.String("webhook_secret_path", set(c.WebhookSecretPath)),
		slog.String("mode_default", c.ModeDefault),
		slog.String("listen_addr", c.ListenAddr),
		slog.String("data_dir", c.DataDir),
		slog.String("tz_reports", tz),
	)
}
