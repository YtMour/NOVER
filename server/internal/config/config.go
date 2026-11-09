package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Environment        string
	Port               string
	DatabaseURL        string
	AdminUsername      string
	AdminPassword      string
	OperatorTokenKey   []byte
	RedeemHMACKey      []byte
	AccessTokenKey     []byte
	StorageDir         string
	MaxUploadBytes     int64
	AllowedOrigin      string
	DownloadTTLSeconds int
}

func Load() (Config, error) {
	cfg := Config{
		Environment:        env("APP_ENV", "development"),
		Port:               env("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		AdminUsername:      env("ADMIN_USERNAME", "admin"),
		AdminPassword:      env("ADMIN_PASSWORD", "admin"),
		OperatorTokenKey:   []byte(os.Getenv("OPERATOR_TOKEN_KEY")),
		RedeemHMACKey:      []byte(os.Getenv("REDEEM_HMAC_KEY")),
		AccessTokenKey:     []byte(os.Getenv("ACCESS_TOKEN_KEY")),
		StorageDir:         env("STORAGE_DIR", "./runtime-data/files"),
		AllowedOrigin:      env("ALLOWED_ORIGIN", "http://localhost:5173"),
		DownloadTTLSeconds: 300,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
		return Config{}, errors.New("ADMIN_USERNAME and ADMIN_PASSWORD are required")
	}
	if len(cfg.OperatorTokenKey) < 32 {
		return Config{}, errors.New("OPERATOR_TOKEN_KEY must contain at least 32 characters")
	}
	if len(cfg.RedeemHMACKey) < 32 {
		return Config{}, errors.New("REDEEM_HMAC_KEY must contain at least 32 characters")
	}
	if len(cfg.AccessTokenKey) < 32 {
		return Config{}, errors.New("ACCESS_TOKEN_KEY must contain at least 32 characters")
	}
	if cfg.Environment != "development" && cfg.Environment != "production" {
		return Config{}, errors.New("APP_ENV must be development or production")
	}
	if cfg.Environment == "production" && (cfg.AdminUsername == "admin" || cfg.AdminPassword == "admin" || string(cfg.OperatorTokenKey) == "local-operator-token-key-change-me-0123456789" || string(cfg.RedeemHMACKey) == "local-redeem-hmac-key-change-me-0123456789" || string(cfg.AccessTokenKey) == "local-access-token-key-change-me-0123456789") {
		return Config{}, errors.New("production environment requires replacing the sample credentials and secrets")
	}
	maxUpload, err := strconv.ParseInt(env("MAX_UPLOAD_BYTES", "524288000"), 10, 64)
	if err != nil || maxUpload < 1 {
		return Config{}, fmt.Errorf("MAX_UPLOAD_BYTES must be a positive integer")
	}
	cfg.MaxUploadBytes = maxUpload
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
