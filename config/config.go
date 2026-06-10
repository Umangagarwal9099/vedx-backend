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
}

type StorageConfig struct {
	ProjectURL     string
	ServiceRoleKey string
	Bucket         string
}

type AppConfig struct {
	Env  string
	Port string
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
			Env:  get("APP_ENV", "development"),
			Port: get("PORT", get("APP_PORT", "8080")), // PORT is set automatically by Render
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
			ProjectURL:     require("SUPABASE_URL"),
			ServiceRoleKey: require("SUPABASE_SERVICE_KEY"),
			Bucket:         get("SUPABASE_STORAGE_BUCKET", "events"),
		},
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}
