package openai

import "testing"

func TestRejectsTemperature(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{name: "gpt5", model: "gpt-5", want: true},
		{name: "gpt5 variant", model: "gpt-5-mini", want: true},
		{name: "gpt5 uppercase", model: " GPT-5o ", want: true},
		{name: "gpt6", model: "gpt-6.1-sol", want: true},
		{name: "gpt4", model: "gpt-4o", want: false},
		{name: "empty", model: "", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := rejectsTemperature(tt.model); got != tt.want {
				t.Fatalf("rejectsTemperature(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}
