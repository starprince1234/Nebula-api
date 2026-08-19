package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadValidatesSecurityConfiguration(t *testing.T) {
	setValidEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ProviderEncryptionKey) != 32 {
		t.Fatalf("expected 32-byte provider key, got %d", len(cfg.ProviderEncryptionKey))
	}
	if cfg.CookieSecure() {
		t.Fatal("development cookies should not require HTTPS")
	}
}

func TestLoadRejectsShortJWTKey(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("JWT_SIGNING_KEY", "short")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "JWT_SIGNING_KEY") {
		t.Fatalf("expected JWT key validation error, got %v", err)
	}
}

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("JWT_SIGNING_KEY", strings.Repeat("j", 32))
	t.Setenv("AUTH_STATE_HASH_PEPPER", strings.Repeat("a", 32))
	t.Setenv("API_KEY_HASH_PEPPER", strings.Repeat("k", 32))
	t.Setenv("PROVIDER_CREDENTIAL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("e", 32))))
	t.Setenv("BOOTSTRAP_TEACHER_NAME", "")
	t.Setenv("BOOTSTRAP_TEACHER_EMAIL", "")
	t.Setenv("BOOTSTRAP_TEACHER_PASSWORD", "")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "noreply@example.com")
}
