package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	keyLenBytes  = 32
	nonceLenGCM  = 12
	minCipherLen = nonceLenGCM + 1
)

type TokenCipher struct {
	aead cipher.AEAD
}

func NewTokenCipher(key string) (*TokenCipher, error) {
	keyBytes, err := decodeKey(key)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &TokenCipher{aead: aead}, nil
}

func (c *TokenCipher) Encrypt(plain string) (string, error) {
	if c == nil || c.aead == nil {
		return "", errors.New("token cipher not configured")
	}
	nonce := make([]byte, nonceLenGCM)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plain), nil)
	out := append(nonce, sealed...)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

func (c *TokenCipher) Decrypt(encoded string) (string, error) {
	if c == nil || c.aead == nil {
		return "", errors.New("token cipher not configured")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", err
	}
	if len(raw) < minCipherLen {
		return "", errors.New("ciphertext too short")
	}
	nonce := raw[:nonceLenGCM]
	body := raw[nonceLenGCM:]
	plain, err := c.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func decodeKey(key string) ([]byte, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return nil, errors.New("share token encryption key is required")
	}
	if decoded, err := hex.DecodeString(trimmed); err == nil && len(decoded) == keyLenBytes {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) == keyLenBytes {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil && len(decoded) == keyLenBytes {
		return decoded, nil
	}
	return nil, fmt.Errorf("share token encryption key must decode to %d bytes", keyLenBytes)
}
