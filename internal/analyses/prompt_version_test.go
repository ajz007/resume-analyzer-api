package analyses

import "testing"

func TestNormalizePromptVersionEmptyDefaultsToV2_3(t *testing.T) {
	t.Setenv(analysisPromptVersionEnv, "")

	if got := NormalizePromptVersion(""); got != "v2_3" {
		t.Fatalf("expected v2_3 default, got %q", got)
	}
}

func TestNormalizePromptVersionEnvV2_4ChangesDefault(t *testing.T) {
	t.Setenv(analysisPromptVersionEnv, "v2_4")

	if got := NormalizePromptVersion(""); got != "v2_4" {
		t.Fatalf("expected env default v2_4, got %q", got)
	}
}

func TestNormalizePromptVersionInvalidEnvFallsBackToV2_3(t *testing.T) {
	t.Setenv(analysisPromptVersionEnv, "v9")

	if got := NormalizePromptVersion(""); got != "v2_3" {
		t.Fatalf("expected invalid env to fall back to v2_3, got %q", got)
	}
}

func TestNormalizePromptVersionExplicitV2_3HonoredWithEnvV2_4(t *testing.T) {
	t.Setenv(analysisPromptVersionEnv, "v2_4")

	if got := NormalizePromptVersion("v2_3"); got != "v2_3" {
		t.Fatalf("expected explicit v2_3, got %q", got)
	}
}

func TestNormalizePromptVersionExplicitV2_4Honored(t *testing.T) {
	t.Setenv(analysisPromptVersionEnv, "")

	if got := NormalizePromptVersion(" v2_4 "); got != "v2_4" {
		t.Fatalf("expected explicit v2_4, got %q", got)
	}
}
