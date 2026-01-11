package service

import (
	"context"
	"testing"
)

type mockLLMClient struct {
	responses []string
	calls     int
}

func (m *mockLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	response := m.responses[m.calls]
	m.calls++
	return response, nil
}

func TestBuildResumeModelRetriesOnSchemaViolation(t *testing.T) {
	invalid := `{"header":{"name":"","title":"","email":"","phone":"","location":"","links":[]},` +
		`"summary":[],"skills":{"languages":[],"frameworks":[],"databases":[],"cloudDevOps":[],"observability":[],"tools":[]},` +
		`"experience":[],"projects":[],"education":[],"achievements":[],"certifications":[]}`
	valid := `{"header":{"name":"Ada Lovelace","title":"","email":"","phone":"","location":"","links":[]},` +
		`"summary":[],"skills":{"languages":["Go"],"frameworks":[],"databases":[],"cloudDevOps":[],"observability":[],"tools":[]},` +
		`"experience":[{"id":"exp_1","company":"Example","role":"Engineer","location":"","start":"2020-01","end":"Present","highlights":["exp_1_b1: Built systems."]}],` +
		`"projects":[],"education":[],"achievements":[],"certifications":[]}`

	mock := &mockLLMClient{
		responses: []string{
			"narrative preface " + invalid + " trailing",
			valid,
		},
	}

	prevClient := Client
	Client = mock
	defer func() {
		Client = prevClient
	}()

	resumeModel, err := BuildResumeModel(context.Background(), "Sample resume text")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resumeModel.Header.Name != "Ada Lovelace" {
		t.Fatalf("expected header name, got %q", resumeModel.Header.Name)
	}
	if mock.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.calls)
	}
}

func TestBuildResumeModelFailsWithoutClient(t *testing.T) {
	prevClient := Client
	Client = nil
	defer func() {
		Client = prevClient
	}()

	_, err := BuildResumeModel(context.Background(), "Sample resume text")
	if err == nil {
		t.Fatal("expected error when client is not configured")
	}
}

func TestBuildResumeModelValidatesOutput(t *testing.T) {
	response := `{"header":{"name":"Grace Hopper","title":"","email":"","phone":"","location":"","links":["https://example.com"]},` +
		`"summary":[],"skills":{"languages":[],"frameworks":[],"databases":[],"cloudDevOps":[],"observability":[],"tools":[]},` +
		`"experience":[],"projects":[],"education":[],"achievements":[],"certifications":[]}`

	mock := &mockLLMClient{
		responses: []string{response},
	}

	prevClient := Client
	Client = mock
	defer func() {
		Client = prevClient
	}()

	got, err := BuildResumeModel(context.Background(), "Sample resume text")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if got.Header.Name != "Grace Hopper" {
		t.Fatalf("expected header name, got %q", got.Header.Name)
	}
}
