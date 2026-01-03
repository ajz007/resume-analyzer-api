package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Claims represents the identity contained in a JWT.
type Claims struct {
	Sub     string `json:"sub"`
	Email   string `json:"email,omitempty"`
	Name    string `json:"name,omitempty"`
	Picture string `json:"picture,omitempty"`
	Exp     int64  `json:"exp,omitempty"`
	Iat     int64  `json:"iat,omitempty"`
}

var (
	errMissingSecret = errors.New("jwt secret not configured")
	ErrInvalidToken  = errors.New("invalid token")
)

// SignJWT signs the given claims with HS256 using the configured secret.
func SignJWT(claims Claims) (string, error) {
	secret, err := secretKey()
	if err != nil {
		return "", err
	}
	if claims.Sub == "" {
		return "", errors.New("sub is required")
	}

	now := time.Now().UTC().Unix()
	if claims.Iat == 0 {
		claims.Iat = now
	}
	if claims.Exp == 0 {
		claims.Exp = now + int64(24*time.Hour/time.Second)
	}

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	segments := []string{
		base64.RawURLEncoding.EncodeToString(headerJSON),
		base64.RawURLEncoding.EncodeToString(payloadJSON),
	}
	signingInput := strings.Join(segments, ".")

	sig := sign(signingInput, secret)
	segments = append(segments, sig)
	return strings.Join(segments, "."), nil
}

// VerifyJWT verifies a token and returns its claims.
func VerifyJWT(token string) (Claims, error) {
	secret, err := secretKey()
	if err != nil {
		return Claims{}, err
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}

	signingInput := strings.Join(parts[0:2], ".")
	expectedSig := sign(signingInput, secret)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return Claims{}, ErrInvalidToken
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}

	if claims.Sub == "" {
		return Claims{}, ErrInvalidToken
	}

	if claims.Exp > 0 && time.Now().UTC().Unix() > claims.Exp {
		return Claims{}, ErrInvalidToken
	}

	return claims, nil
}

func sign(input string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func secretKey() ([]byte, error) {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	if env == "production" || env == "prod" {
		if secret == "" {
			return nil, fmt.Errorf("%w: JWT_SECRET required in production", errMissingSecret)
		}
	}
	if secret == "" {
		secret = "dev-secret"
	}
	return []byte(secret), nil
}
