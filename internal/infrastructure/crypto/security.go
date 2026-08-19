package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AccessClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type Manager struct {
	jwtKey         []byte
	authPepper     []byte
	apiKeyPepper   []byte
	credentialAEAD cipher.AEAD
	accessTTL      time.Duration
	now            func() time.Time
}

func NewManager(jwtKey, authPepper, apiKeyPepper, encryptionKey []byte, accessTTL time.Duration) (*Manager, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential AEAD: %w", err)
	}
	return &Manager{
		jwtKey:         append([]byte(nil), jwtKey...),
		authPepper:     append([]byte(nil), authPepper...),
		apiKeyPepper:   append([]byte(nil), apiKeyPepper...),
		credentialAEAD: aead,
		accessTTL:      accessTTL,
		now:            time.Now,
	}, nil
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 128 {
		return "", fmt.Errorf("password must contain 12 to 128 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (m *Manager) IssueAccessToken(userID uuid.UUID, role string) (string, time.Time, error) {
	now := m.now().UTC()
	expiresAt := now.Add(m.accessTTL)
	claims := AccessClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "nebula",
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{"nebula-control"},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        uuid.Must(uuid.NewV7()).String(),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.jwtKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return token, expiresAt, nil
}

func (m *Manager) ParseAccessToken(raw string) (AccessClaims, error) {
	var claims AccessClaims
	token, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected JWT signing method")
		}
		return m.jwtKey, nil
	},
		jwt.WithAudience("nebula-control"),
		jwt.WithIssuer("nebula"),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil || !token.Valid {
		return AccessClaims{}, fmt.Errorf("invalid access token")
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return AccessClaims{}, fmt.Errorf("invalid access token subject")
	}
	return claims, nil
}

func (m *Manager) NewOpaqueToken(prefix string, bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func (m *Manager) HashAuthState(value string) []byte {
	return hmacSHA256(m.authPepper, value)
}

func (m *Manager) HashAPIKey(value string) []byte {
	return hmacSHA256(m.apiKeyPepper, value)
}

func (m *Manager) NewAPIKey() (secret, prefix string, hash []byte, err error) {
	secret, err = m.NewOpaqueToken("neb_sk_", 32)
	if err != nil {
		return "", "", nil, err
	}
	prefixLength := 16
	if len(secret) < prefixLength {
		prefixLength = len(secret)
	}
	return secret, secret[:prefixLength], m.HashAPIKey(secret), nil
}

func (m *Manager) EncryptCredential(plaintext string) ([]byte, error) {
	if strings.TrimSpace(plaintext) == "" {
		return nil, fmt.Errorf("credential is required")
	}
	nonce := make([]byte, m.credentialAEAD.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate credential nonce: %w", err)
	}
	return m.credentialAEAD.Seal(nonce, nonce, []byte(plaintext), []byte("nebula-provider-v1")), nil
}

func (m *Manager) DecryptCredential(ciphertext []byte) (string, error) {
	nonceSize := m.credentialAEAD.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("invalid credential ciphertext")
	}
	plaintext, err := m.credentialAEAD.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], []byte("nebula-provider-v1"))
	if err != nil {
		return "", fmt.Errorf("decrypt credential: %w", err)
	}
	return string(plaintext), nil
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
