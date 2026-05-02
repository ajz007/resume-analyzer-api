package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestTokenCipherRoundTrip(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef"
	cipher, err := NewTokenCipher(hex.EncodeToString([]byte(key)))
	if err != nil {
		t.Fatalf("NewTokenCipher: %v", err)
	}
	ciphertext, err := cipher.Encrypt("token-123")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	plain, err := cipher.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if plain != "token-123" {
		t.Fatalf("expected token-123, got %q", plain)
	}
}

func TestTokenCipherAcceptsBase64Key(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	encoded := base64.StdEncoding.EncodeToString(key)
	cipher, err := NewTokenCipher(encoded)
	if err != nil {
		t.Fatalf("NewTokenCipher: %v", err)
	}
	ciphertext, err := cipher.Encrypt("abc")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	plain, err := cipher.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if plain != "abc" {
		t.Fatalf("expected abc, got %q", plain)
	}
}

func TestTokenCipherRejectsInvalidKey(t *testing.T) {
	if _, err := NewTokenCipher("short"); err == nil {
		t.Fatalf("expected error for invalid key")
	}
}
