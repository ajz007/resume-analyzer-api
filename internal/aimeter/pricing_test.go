package aimeter

import (
	"fmt"
	"testing"
)

func TestPriceLongContextBoundaryAndUnsupportedMetadata(t *testing.T) {
	for _, tc := range []struct {
		input int64
		want  int64
	}{{272000, 272000*2000 + 10000}, {272001, 272001*4000 + 15000}} {
		body := fmt.Sprintf(`{"model":"gpt-6.1-sol","service_tier":"default","usage":{"prompt_tokens":%d,"completion_tokens":1,"total_tokens":%d}}`, tc.input, tc.input+1)
		a, _ := observe(t, body, 200)
		if a.EstimateNanoUSD == nil || *a.EstimateNanoUSD != tc.want {
			t.Fatalf("%+v", a)
		}
	}
	for _, body := range []string{
		`{"model":"other","service_tier":"default","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		`{"model":"gpt-6.1-sol","service_tier":"priority","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		`{"model":"gpt-6.1-sol","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		`{"model":"gpt-6.1-sol","service_tier":"default","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"prompt_tokens_details":{"image_tokens":1}}}`,
	} {
		a, _ := observe(t, body, 200)
		if a.UsageState != "known" || a.EstimateNanoUSD != nil || a.UnpricedReason == "" {
			t.Fatalf("%+v", a)
		}
	}
}
