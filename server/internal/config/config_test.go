package config

import "testing"

func setRequiredConfig(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://nover:pass@localhost/nover")
	t.Setenv("ADMIN_USERNAME", "test-operator")
	t.Setenv("ADMIN_PASSWORD", "test-password")
	t.Setenv("OPERATOR_TOKEN_KEY", "test-operator-token-key-with-32-bytes-min")
	t.Setenv("REDEEM_HMAC_KEY", "test-redeem-hmac-key-with-32-bytes-min")
	t.Setenv("ACCESS_TOKEN_KEY", "test-access-token-key-with-32-bytes-min")
	t.Setenv("MAX_UPLOAD_BYTES", "1024")
}

func TestLoadDevelopmentConfig(t *testing.T) {
	setRequiredConfig(t)
	t.Setenv("APP_ENV", "development")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != "development" || cfg.MaxUploadBytes != 1024 || len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("unexpected config values: %+v", cfg)
	}
}

func TestLoadParsesMultipleAllowedOrigins(t *testing.T) {
	setRequiredConfig(t)
	t.Setenv("ALLOWED_ORIGINS", "https://store.example.com/, https://console.example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[0] != "https://store.example.com" || cfg.AllowedOrigins[1] != "https://console.example.com" {
		t.Fatalf("unexpected origins: %#v", cfg.AllowedOrigins)
	}
}

func TestLoadProductionRejectsSampleSecrets(t *testing.T) {
	setRequiredConfig(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "admin")
	if _, err := Load(); err == nil {
		t.Fatal("production config accepted the sample operator credentials")
	}
}

func TestLoadProductionRequiresAllowedEnvironmentName(t *testing.T) {
	setRequiredConfig(t)
	t.Setenv("APP_ENV", "staging")
	if _, err := Load(); err == nil {
		t.Fatal("unsupported environment should be rejected")
	}
}

func TestLoadRejectsInvalidOperatorAPIPath(t *testing.T) {
	setRequiredConfig(t)
	t.Setenv("OPERATOR_API_PATH", "/admin")
	if _, err := Load(); err == nil {
		t.Fatal("operator API path with slashes should be rejected")
	}
}
