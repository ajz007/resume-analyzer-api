package s3

import "testing"

func TestApplyPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prefix string
		key    string
		want   string
	}{
		{name: "no prefix", prefix: "", key: "user/file.pdf", want: "user/file.pdf"},
		{name: "simple prefix", prefix: "root", key: "user/file.pdf", want: "root/user/file.pdf"},
		{name: "prefix trailing slash", prefix: "root/", key: "user/file.pdf", want: "root/user/file.pdf"},
		{name: "prefix and key slashes", prefix: "/root/", key: "/user/file.pdf", want: "root/user/file.pdf"},
		{name: "nested prefix", prefix: "root/sub", key: "user/file.pdf", want: "root/sub/user/file.pdf"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := applyPrefix(tt.prefix, tt.key); got != tt.want {
				t.Fatalf("applyPrefix(%q, %q) = %q, want %q", tt.prefix, tt.key, got, tt.want)
			}
		})
	}
}
