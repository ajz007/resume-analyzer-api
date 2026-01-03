package util

import "testing"

func TestHashUserKey(t *testing.T) {
	id := "google:12345"
	got := HashUserKey(id)
	if got != HashUserKey(id) {
		t.Fatalf("expected stable hash, got %s", got)
	}
	for _, ch := range got {
		if !((ch >= 'a' && ch <= 'f') || (ch >= '0' && ch <= '9')) {
			t.Fatalf("hash contains non-hex character: %c", ch)
		}
	}
	if len(got) != 64 {
		t.Fatalf("expected 64 hex characters, got %d", len(got))
	}
}
