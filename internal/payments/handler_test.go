package payments

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyHandlerReturnsRecoverablePaymentStates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider fakePaymentFetcher
		status   int
		code     string
	}{
		{"pending", fakePaymentFetcher{payment: ProviderPayment{ID: "pay_1", OrderID: "order_1", Amount: 49900, Currency: "INR", Status: "authorized"}}, 409, "payment_pending"},
		{"unavailable", fakePaymentFetcher{err: fmt.Errorf("provider unavailable")}, 503, "payment_verification_unavailable"},
		{"mismatch", fakePaymentFetcher{payment: ProviderPayment{ID: "pay_1", OrderID: "wrong", Amount: 49900, Currency: "INR", Status: "captured"}}, 400, "payment_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTestService()
			order, err := svc.CreateOrder(context.Background(), "google:u1", "a1", "INR")
			if err != nil {
				t.Fatal(err)
			}
			svc.Payments = tc.provider
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("userId", "google:u1"); c.Next() })
			NewHandler(svc).RegisterRoutes(router.Group("/api/v1"))
			sig := hmacHex([]byte(order.OrderID+"|pay_1"), testKeySecret)
			req := httptest.NewRequest("POST", "/api/v1/payments/verify", strings.NewReader(fmt.Sprintf(`{"orderId":%q,"paymentId":"pay_1","signature":%q}`, order.OrderID, sig)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}
