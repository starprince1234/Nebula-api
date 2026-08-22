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

func TestLoadParsesBootstrapTestAccounts(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("TEST_STUDENT_1_NAME", " Student One ")
	t.Setenv("TEST_STUDENT_1_EMAIL", " STUDENT.ONE@EXAMPLE.COM ")
	t.Setenv("TEST_STUDENT_1_PASSWORD", "student-password-one")
	t.Setenv("TEST_MENTOR_1_NAME", " Mentor One ")
	t.Setenv("TEST_MENTOR_1_EMAIL", " MENTOR.ONE@EXAMPLE.COM ")
	t.Setenv("TEST_MENTOR_1_PASSWORD", "mentor-password-one")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BootstrapTestAccounts) != 2 {
		t.Fatalf("expected two bootstrap test accounts, got %d", len(cfg.BootstrapTestAccounts))
	}
	student := cfg.BootstrapTestAccounts[0]
	if student.Role != "student" || student.Name != "Student One" || student.Email != "student.one@example.com" {
		t.Fatalf("unexpected student bootstrap account: %#v", student)
	}
	mentor := cfg.BootstrapTestAccounts[1]
	if mentor.Role != "mentor" || mentor.Name != "Mentor One" || mentor.Email != "mentor.one@example.com" {
		t.Fatalf("unexpected mentor bootstrap account: %#v", mentor)
	}
}

func TestLoadRejectsPartialBootstrapTestAccount(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("TEST_STUDENT_1_EMAIL", "student.one@example.com")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TEST_STUDENT_1") {
		t.Fatalf("expected partial bootstrap account validation error, got %v", err)
	}
}

func TestLoadRejectsDuplicateBootstrapTestAccountEmail(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("TEST_STUDENT_1_NAME", "Student One")
	t.Setenv("TEST_STUDENT_1_EMAIL", "shared@example.com")
	t.Setenv("TEST_STUDENT_1_PASSWORD", "student-password-one")
	t.Setenv("TEST_MENTOR_1_NAME", "Mentor One")
	t.Setenv("TEST_MENTOR_1_EMAIL", "SHARED@EXAMPLE.COM")
	t.Setenv("TEST_MENTOR_1_PASSWORD", "mentor-password-one")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("expected duplicate bootstrap account email error, got %v", err)
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
	for _, role := range []string{"STUDENT", "MENTOR"} {
		for index := 1; index <= 3; index++ {
			prefix := "TEST_" + role + "_" + string(rune('0'+index))
			t.Setenv(prefix+"_NAME", "")
			t.Setenv(prefix+"_EMAIL", "")
			t.Setenv(prefix+"_PASSWORD", "")
		}
	}
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "noreply@example.com")
}
