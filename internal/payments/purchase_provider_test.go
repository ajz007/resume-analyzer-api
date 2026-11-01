package payments

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFindOrderUsesExactReceiptAndReadOnlyAuthenticatedLookup(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantErr    bool
	}{
		{"match", `{"items":[{"id":"order_1","receipt":"receipt-1","amount":49900,"currency":"INR","status":"created","payments":{"items":[]}}]}`, 200, false},
		{"substring only", `{"items":[{"id":"order_1","receipt":"prefix-receipt-1"}]}`, 200, true},
		{"multiple matches", `{"items":[{"id":"order_1","receipt":"receipt-1"},{"id":"order_2","receipt":"receipt-1"}]}`, 200, true},
		{"empty", `{"items":[]}`, 200, true},
		{"bad json", `{`, 200, true},
		{"provider error", `{}`, 503, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, password, ok := r.BasicAuth()
				if !ok || user != "key" || password != "secret" || r.Method != "GET" || r.URL.Path != "/v1/orders" || r.URL.Query().Get("receipt") != "receipt-1" || r.URL.Query().Get("count") != "100" || r.URL.Query().Get("expand[]") != "payments" {
					t.Error("incorrect recovery request")
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c := NewRazorpayClient("key", "secret")
			c.BaseURL = server.URL
			order, err := c.FindOrder(context.Background(), "receipt-1")
			if (err != nil) != tc.wantErr {
				t.Fatalf("%+v %v", order, err)
			}
			if !tc.wantErr && (order.ID != "order_1" || order.Amount != 49900) {
				t.Fatalf("%+v", order)
			}
		})
	}
}
