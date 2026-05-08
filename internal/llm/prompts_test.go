package llm

import "testing"

func TestPromptTemplateV2_4Recognized(t *testing.T) {
	template, recognized := PromptTemplate("v2_4")
	if !recognized {
		t.Fatalf("expected v2_4 to be recognized")
	}
	if template == "" {
		t.Fatalf("expected v2_4 prompt template to be non-empty")
	}
}

func TestPromptTemplateUnknownFallsBackToV1(t *testing.T) {
	template, recognized := PromptTemplate("unknown")
	if recognized {
		t.Fatalf("expected unknown prompt version to be unrecognized")
	}
	v1, v1Recognized := PromptTemplate("v1")
	if !v1Recognized {
		t.Fatalf("expected v1 to be recognized")
	}
	if template != v1 {
		t.Fatalf("expected unknown prompt version to fall back to v1")
	}
}
