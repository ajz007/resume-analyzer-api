package s3

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

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

func TestSaveWithKeySendsSeekableSnapshot(t *testing.T) {
	data := []byte("saved resume bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/bucket/documents/user/tailored.docx" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, data) || r.ContentLength != int64(len(data)) {
			t.Errorf("unexpected request body or length: %v, %d", err, r.ContentLength)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := awss3.New(awss3.Options{
		Region: "us-east-1", BaseEndpoint: aws.String(server.URL), UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	})
	store := &Store{client: client, bucket: "bucket"}
	size, err := store.SaveWithKey(context.Background(), "documents/user/tailored.docx", "application/octet-stream", bytes.NewReader(data))
	if err != nil || size != int64(len(data)) {
		t.Fatalf("snapshot upload size=%d err=%v", size, err)
	}
}
