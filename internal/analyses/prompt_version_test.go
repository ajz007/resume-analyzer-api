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

func TestNormalizePromptVersionEnvV2_5Accepted(t *testing.T) {
	t.Setenv(analysisPromptVersionEnv, "v2_5")

	if got := NormalizePromptVersion(""); got != "v2_5" {
		t.Fatalf("expected env default v2_5, got %q", got)
	}
}

func TestUsesV2_4Schema(t *testing.T) {
	for _, v := range []string{"v2_4", "v2_5", " V2_5 "} {
		if !usesV2_4Schema(v) {
			t.Fatalf("expected %q to use the v2_4 schema", v)
		}
	}
	for _, v := range []string{"", "v1", "v2_3"} {
		if usesV2_4Schema(v) {
			t.Fatalf("expected %q not to use the v2_4 schema", v)
		}
	}
}
