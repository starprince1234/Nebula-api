package crypto

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPasswordHashAndVerify(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "correct-horse-battery-staple") {
		t.Fatal("valid password did not verify")
	}
	if VerifyPassword(hash, "wrong-password") {
		t.Fatal("invalid password verified")
	}
}

func TestManagerSecretsAndJWT(t *testing.T) {
	t.Parallel()
	manager, err := NewManager(
		bytes.Repeat([]byte("j"), 32),
		bytes.Repeat([]byte("a"), 32),
		bytes.Repeat([]byte("k"), 32),
		bytes.Repeat([]byte("e"), 32),
		15*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	userID := uuid.Must(uuid.NewV7())
	token, _, err := manager.IssueAccessToken(userID, "student")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.ParseAccessToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != userID.String() || claims.Role != "student" {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	secret, prefix, hash, err := manager.NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(prefix) >= len(secret) {
		t.Fatal("API key prefix must not reveal the full secret")
	}
	if !bytes.Equal(hash, manager.HashAPIKey(secret)) {
		t.Fatal("API key hash is inconsistent")
	}
}

func TestCredentialEncryption(t *testing.T) {
	t.Parallel()
	manager, err := NewManager(
		bytes.Repeat([]byte("j"), 32),
		bytes.Repeat([]byte("a"), 32),
		bytes.Repeat([]byte("k"), 32),
		bytes.Repeat([]byte("e"), 32),
		time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := manager.EncryptCredential("provider-secret")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("provider-secret")) {
		t.Fatal("ciphertext contains plaintext")
	}
	plaintext, err := manager.DecryptCredential(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "provider-secret" {
		t.Fatalf("unexpected plaintext %q", plaintext)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err := manager.DecryptCredential(ciphertext); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}
