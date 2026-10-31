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

func TestPromptTemplateV2_5Recognized(t *testing.T) {
	template, recognized := PromptTemplate("v2_5")
	if !recognized {
		t.Fatalf("expected v2_5 to be recognized")
	}
	v24, _ := PromptTemplate("v2_4")
	if template == "" || template == v24 {
		t.Fatalf("expected v2_5 prompt template to be non-empty and distinct from v2_4")
	}
}

func TestPromptTemplateV3Recognized(t *testing.T) {
	template, ok := PromptTemplate("v3")
	if !ok || template == "" {
		t.Fatal("V3 prompt missing")
	}
}
