package openai

import "testing"

func TestPromptHashDeterministic(t *testing.T) {
	messages := BuildPrompt("v2_1", "resume text", "job description", "gpt-4o-mini")
	hash1 := hashPromptString(promptStringFromMessages(messages))
	hash2 := hashPromptString(promptStringFromMessages(messages))
	if hash1 != hash2 {
		t.Fatalf("expected deterministic prompt hash, got %q and %q", hash1, hash2)
	}

	messagesAlt := BuildPrompt("v2_1", "resume text", "different job", "gpt-4o-mini")
	hashAlt := hashPromptString(promptStringFromMessages(messagesAlt))
	if hash1 == hashAlt {
		t.Fatalf("expected prompt hash to change when input changes")
	}
}
