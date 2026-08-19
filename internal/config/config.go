package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv       string
	HTTPAddress  string
	PublicAppURL string
	DatabaseURL  string
	RedisURL     string

	JWTSigningKey []byte
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	AuthPepper    []byte

	APIKeyPepper          []byte
	ProviderEncryptionKey []byte

	BootstrapTeacherName     string
	BootstrapTeacherEmail    string
	BootstrapTeacherPassword string

	SMTPHost        string
	SMTPPort        int
	SMTPUser        string
	SMTPPass        string
	SMTPFrom        string
	SMTPFromName    string
	SMTPTLSMode     string
	SMTPTimeout     time.Duration
	VerificationTTL time.Duration
	SendCooldown    time.Duration
	MaxAttempts     int
	Lockout         time.Duration
	InvitationTTL   time.Duration

	SSEStreamMaxLength int64
	SSEHeartbeat       time.Duration

	UpstreamConnectTimeout        time.Duration
	UpstreamResponseHeaderTimeout time.Duration
	GatewayMaxRequestBytes        int64
	VideoTaskRouteTTL             time.Duration
}

func LoadDatabaseURL() (string, error) {
	return required("DATABASE_URL")
}

func Load() (Config, error) {
	var cfg Config
	cfg.AppEnv = value("APP_ENV", "development")
	cfg.HTTPAddress = value("HTTP_ADDRESS", ":8080")
	cfg.PublicAppURL = value("PUBLIC_APP_URL", "http://localhost:5173")

	var err error
	if cfg.DatabaseURL, err = required("DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if cfg.RedisURL, err = required("REDIS_URL"); err != nil {
		return Config{}, err
	}
	if cfg.JWTSigningKey, err = secret("JWT_SIGNING_KEY", 32); err != nil {
		return Config{}, err
	}
	if cfg.AuthPepper, err = secret("AUTH_STATE_HASH_PEPPER", 32); err != nil {
		return Config{}, err
	}
	if cfg.APIKeyPepper, err = secret("API_KEY_HASH_PEPPER", 32); err != nil {
		return Config{}, err
	}
	rawEncryptionKey, err := required("PROVIDER_CREDENTIAL_ENCRYPTION_KEY")
	if err != nil {
		return Config{}, err
	}
	cfg.ProviderEncryptionKey, err = base64.StdEncoding.DecodeString(rawEncryptionKey)
	if err != nil || len(cfg.ProviderEncryptionKey) != 32 {
		return Config{}, fmt.Errorf("PROVIDER_CREDENTIAL_ENCRYPTION_KEY must be base64 for exactly 32 bytes")
	}

	if cfg.AccessTTL, err = durationFromInt("ACCESS_TOKEN_TTL_MINUTES", 15, time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.RefreshTTL, err = durationFromInt("REFRESH_TOKEN_TTL_HOURS", 168, time.Hour); err != nil {
		return Config{}, err
	}

	cfg.BootstrapTeacherName = strings.TrimSpace(os.Getenv("BOOTSTRAP_TEACHER_NAME"))
	cfg.BootstrapTeacherEmail = normalizeEmail(os.Getenv("BOOTSTRAP_TEACHER_EMAIL"))
	cfg.BootstrapTeacherPassword = os.Getenv("BOOTSTRAP_TEACHER_PASSWORD")
	if anyNonEmpty(cfg.BootstrapTeacherName, cfg.BootstrapTeacherEmail, cfg.BootstrapTeacherPassword) &&
		!allNonEmpty(cfg.BootstrapTeacherName, cfg.BootstrapTeacherEmail, cfg.BootstrapTeacherPassword) {
		return Config{}, fmt.Errorf("bootstrap teacher name, email and password must be configured together")
	}
	if strings.HasPrefix(cfg.BootstrapTeacherPassword, "replace_with") {
		return Config{}, fmt.Errorf("BOOTSTRAP_TEACHER_PASSWORD must be replaced")
	}

	cfg.SMTPHost = strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if cfg.SMTPPort, err = intValue("SMTP_PORT", 587); err != nil {
		return Config{}, err
	}
	cfg.SMTPUser = os.Getenv("SMTP_USER")
	cfg.SMTPPass = os.Getenv("SMTP_PASS")
	cfg.SMTPFrom = normalizeEmail(os.Getenv("SMTP_FROM"))
	cfg.SMTPFromName = value("SMTP_FROM_NAME", "Nebula API")
	cfg.SMTPTLSMode = strings.ToLower(value("SMTP_TLS_MODE", "starttls"))
	if cfg.SMTPTLSMode != "starttls" && cfg.SMTPTLSMode != "tls" && cfg.SMTPTLSMode != "none" {
		return Config{}, fmt.Errorf("SMTP_TLS_MODE must be starttls, tls or none")
	}
	if cfg.SMTPTimeout, err = durationFromInt("SMTP_TIMEOUT_SECONDS", 30, time.Second); err != nil {
		return Config{}, err
	}
	if cfg.VerificationTTL, err = durationFromInt("EMAIL_VERIFICATION_CODE_TTL_MINUTES", 10, time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.SendCooldown, err = durationFromInt("EMAIL_VERIFICATION_SEND_COOLDOWN_SECONDS", 60, time.Second); err != nil {
		return Config{}, err
	}
	if cfg.MaxAttempts, err = intValue("EMAIL_VERIFICATION_MAX_ATTEMPTS", 5); err != nil {
		return Config{}, err
	}
	if cfg.Lockout, err = durationFromInt("EMAIL_VERIFICATION_LOCKOUT_MINUTES", 15, time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.InvitationTTL, err = durationFromInt("TEACHER_INVITATION_TTL_HOURS", 24, time.Hour); err != nil {
		return Config{}, err
	}

	if cfg.SSEStreamMaxLength, err = int64Value("SSE_STREAM_MAX_LENGTH", 1000); err != nil {
		return Config{}, err
	}
	if cfg.SSEHeartbeat, err = durationFromInt("SSE_HEARTBEAT_SECONDS", 15, time.Second); err != nil {
		return Config{}, err
	}
	if cfg.UpstreamConnectTimeout, err = durationFromInt("UPSTREAM_CONNECT_TIMEOUT_SECONDS", 10, time.Second); err != nil {
		return Config{}, err
	}
	if cfg.UpstreamResponseHeaderTimeout, err = durationFromInt("UPSTREAM_RESPONSE_HEADER_TIMEOUT_SECONDS", 60, time.Second); err != nil {
		return Config{}, err
	}
	if cfg.GatewayMaxRequestBytes, err = int64Value("GATEWAY_MAX_REQUEST_BYTES", 100<<20); err != nil {
		return Config{}, err
	}
	if cfg.VideoTaskRouteTTL, err = durationFromInt("VIDEO_TASK_ROUTE_TTL_HOURS", 24, time.Hour); err != nil {
		return Config{}, err
	}

	if _, err := url.ParseRequestURI(cfg.PublicAppURL); err != nil {
		return Config{}, fmt.Errorf("PUBLIC_APP_URL: %w", err)
	}
	if cfg.MaxAttempts < 1 || cfg.SSEStreamMaxLength < 1 || cfg.GatewayMaxRequestBytes < 1 {
		return Config{}, fmt.Errorf("numeric limits must be positive")
	}
	return cfg, nil
}

func (c Config) CookieSecure() bool {
	return c.AppEnv != "development" && c.AppEnv != "test"
}

func required(name string) (string, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return v, nil
}

func secret(name string, minLength int) ([]byte, error) {
	v, err := required(name)
	if err != nil {
		return nil, err
	}
	if len(v) < minLength {
		return nil, fmt.Errorf("%s must contain at least %d bytes", name, minLength)
	}
	if strings.HasPrefix(v, "replace_with") {
		return nil, fmt.Errorf("%s must be replaced", name)
	}
	return []byte(v), nil
}

func value(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func intValue(name string, fallback int) (int, error) {
	v := value(name, strconv.Itoa(fallback))
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return n, nil
}

func int64Value(name string, fallback int64) (int64, error) {
	v := value(name, strconv.FormatInt(fallback, 10))
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return n, nil
}

func durationFromInt(name string, fallback int, unit time.Duration) (time.Duration, error) {
	n, err := intValue(name, fallback)
	if err != nil {
		return 0, err
	}
	return time.Duration(n) * unit, nil
}

func normalizeEmail(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func anyNonEmpty(values ...string) bool {
	for _, value := range values {
		if value != "" {
			return true
		}
	}
	return false
}

func allNonEmpty(values ...string) bool {
	for _, value := range values {
		if value == "" {
			return false
		}
	}
	return true
}
