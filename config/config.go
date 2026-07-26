package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App      AppConfig
	Database DatabaseConfig
	JWT      JWTConfig
	Storage  StorageConfig
	Zoom     ZoomConfig
	Resend   ResendConfig
}

// ResendConfig holds credentials for sending transactional email (session
// confirmations/reminders, batch start reminders) via Resend's HTTP API.
// Raw SMTP is deliberately not used here — outbound SMTP ports are blocked or
// silently dropped on Render (and many other PaaS hosts), while HTTPS never
// is. Optional: when unset, Configured() returns false and email sending is
// skipped rather than failing the parent operation.
type ResendConfig struct {
	APIKey    string
	FromEmail string
	FromName  string
}

func (r ResendConfig) Configured() bool {
	return r.APIKey != "" && r.FromEmail != ""
}

// StorageConfig holds credentials for Cloudflare R2 (S3-compatible object
// storage). All uploads live in one bucket under folder prefixes (events/,
// materials/, blogs/, banners/, assessments/, assignments/, resources/,
// projects/) rather than separate buckets — R2 doesn't price per-bucket, so
// splitting them buys nothing but extra credential/domain config.
type StorageConfig struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	PublicURL       string
}

// ZoomConfig holds Server-to-Server OAuth credentials for the Zoom API.
// Optional: when unset, Configured() returns false and session creation
// simply skips Zoom meeting creation instead of failing.
type ZoomConfig struct {
	AccountID          string
	ClientID           string
	ClientSecret       string
	WebhookSecretToken string
}

func (z ZoomConfig) Configured() bool {
	return z.AccountID != "" && z.ClientID != "" && z.ClientSecret != ""
}

// WebhookConfigured reports whether the Event Subscriptions "Secret Token" has
// been set, required to validate the webhook endpoint URL and verify incoming
// event signatures.
func (z ZoomConfig) WebhookConfigured() bool {
	return z.WebhookSecretToken != ""
}

type AppConfig struct {
	Env       string
	Port      string
	PublicURL string
	Timezone  string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

type JWTConfig struct {
	Secret      string
	ExpiryHours int
}

// Load reads all configuration from environment variables.
// Required vars: DB_HOST, DB_USER, DB_PASSWORD, JWT_SECRET
func Load() (*Config, error) {
	var missing []string

	require := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	get := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}

	getInt := func(key string, fallback int) int {
		v := os.Getenv(key)
		if v == "" {
			return fallback
		}
		i, err := strconv.Atoi(v)
		if err != nil {
			return fallback
		}
		return i
	}

	cfg := &Config{
		App: AppConfig{
			Env:       get("APP_ENV", "development"),
			Port:      get("PORT", get("APP_PORT", "8080")), // PORT is set automatically by Render
			PublicURL: get("APP_PUBLIC_URL", ""),            // base URL used to build session share links, e.g. https://app.example.com
			Timezone:  get("APP_TIMEZONE", "Asia/Kolkata"),  // used when scheduling Zoom meetings
		},
		Database: DatabaseConfig{
			Host:     require("DB_HOST"),
			Port:     get("DB_PORT", "5432"),
			User:     require("DB_USER"),
			Password: require("DB_PASSWORD"),
			Name:     get("DB_NAME", "postgres"),
			SSLMode:  get("DB_SSL_MODE", "require"),
		},
		JWT: JWTConfig{
			Secret:      require("JWT_SECRET"),
			ExpiryHours: getInt("JWT_EXPIRY_HOURS", 24),
		},
		Storage: StorageConfig{
			AccountID:       require("R2_ACCOUNT_ID"),
			AccessKeyID:     require("R2_ACCESS_KEY_ID"),
			SecretAccessKey: require("R2_SECRET_ACCESS_KEY"),
			Bucket:          require("R2_BUCKET"),
			PublicURL:       require("R2_PUBLIC_URL"),
		},
		Zoom: ZoomConfig{
			AccountID:          get("ZOOM_ACCOUNT_ID", ""),
			ClientID:           get("ZOOM_CLIENT_ID", ""),
			ClientSecret:       get("ZOOM_CLIENT_SECRET", ""),
			WebhookSecretToken: get("ZOOM_WEBHOOK_SECRET_TOKEN", ""),
		},
		Resend: ResendConfig{
			APIKey:    get("RESEND_API_KEY", ""),
			FromEmail: get("RESEND_FROM_EMAIL", ""),
			FromName:  get("RESEND_FROM_NAME", "Vedex"),
		},
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}
