package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const razorpayDefaultBaseURL = "https://api.razorpay.com"

// RazorpayClient creates orders and fetches authoritative payment state.
type RazorpayClient struct {
	KeyID     string
	KeySecret string
	BaseURL   string
	HTTP      *http.Client
}

// NewRazorpayClient constructs a client with a sane timeout.
func NewRazorpayClient(keyID, keySecret string) *RazorpayClient {
	return &RazorpayClient{
		KeyID:     keyID,
		KeySecret: keySecret,
		BaseURL:   razorpayDefaultBaseURL,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
	}
}

type razorpayOrderRequest struct {
	Amount   int64             `json:"amount"`
	Currency string            `json:"currency"`
	Receipt  string            `json:"receipt"`
	Notes    map[string]string `json:"notes,omitempty"`
}

type razorpayOrderResponse struct {
	ID string `json:"id"`
}

// CreateOrder creates a Razorpay order and returns its id (order_...).
func (c *RazorpayClient) CreateOrder(ctx context.Context, amountMinor int64, currency, receipt string, notes map[string]string) (string, error) {
	body, err := json.Marshal(razorpayOrderRequest{
		Amount:   amountMinor,
		Currency: currency,
		Receipt:  truncate(receipt, 40),
		Notes:    notes,
	})
	if err != nil {
		return "", err
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = razorpayDefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/orders", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.KeyID, c.KeySecret)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("razorpay create order: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("razorpay create order: status %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	var out razorpayOrderResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("razorpay create order: decode: %w", err)
	}
	if out.ID == "" {
		return "", fmt.Errorf("razorpay create order: missing order id")
	}
	return out.ID, nil
}

// VerifyPaymentSignature checks the signature returned by Razorpay Checkout:
// hex(HMAC_SHA256(order_id + "|" + payment_id, key_secret)).
func VerifyPaymentSignature(orderID, paymentID, signature, keySecret string) bool {
	return verifyHMAC([]byte(orderID+"|"+paymentID), signature, keySecret)
}

// VerifyWebhookSignature checks X-Razorpay-Signature over the raw request body.
func VerifyWebhookSignature(body []byte, signature, webhookSecret string) bool {
	return verifyHMAC(body, signature, webhookSecret)
}

func verifyHMAC(message []byte, signature, secret string) bool {
	if secret == "" || signature == "" {
		return false
	}
	expected := hmacHex(message, secret)
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature)))
}

func hmacHex(message []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// FetchPayment reads provider state without capturing or otherwise changing a payment.
func (c *RazorpayClient) FetchPayment(ctx context.Context, paymentID string) (ProviderPayment, error) {
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = razorpayDefaultBaseURL
	}
	// Provider ids are single path segments, never paths or query strings.
	if !strings.HasPrefix(paymentID, "pay_") || strings.ContainsAny(paymentID, "/?#%") {
		return ProviderPayment{}, fmt.Errorf("invalid payment id")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/payments/"+url.PathEscape(paymentID), nil)
	if err != nil {
		return ProviderPayment{}, err
	}
	req.SetBasicAuth(c.KeyID, c.KeySecret)
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProviderPayment{}, fmt.Errorf("razorpay fetch payment: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProviderPayment{}, fmt.Errorf("razorpay fetch payment: status %d", resp.StatusCode)
	}
	var payment ProviderPayment
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payment); err != nil {
		return ProviderPayment{}, fmt.Errorf("razorpay fetch payment: decode: %w", err)
	}
	return payment, nil
}
