package analyses

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"resume-backend/internal/documents"
	"resume-backend/internal/llm"
	"resume-backend/internal/queue"
	sharedcrypto "resume-backend/internal/shared/crypto"
	"resume-backend/internal/shared/server/middleware"
	"resume-backend/internal/shared/storage/object"
	local "resume-backend/internal/shared/storage/object/local"
)

func TestStartAnalysisDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store, queueStub := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 300),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", resp.Code)
	}

	var created struct {
		AnalysisID string `json:"analysisId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.AnalysisID == "" {
		t.Fatalf("expected analysisId, got empty")
	}

	analysis, err := analysisRepo.GetByID(context.Background(), created.AnalysisID)
	if err != nil {
		t.Fatalf("get analysis: %v", err)
	}
	if analysis.JobDescription == "" {
		t.Fatalf("expected jobDescription to be stored, got empty")
	}
	if analysis.PromptVersion != "v2_3" {
		t.Fatalf("expected promptVersion v2_3, got %q", analysis.PromptVersion)
	}
	if len(queueStub.messages) != 1 {
		t.Fatalf("expected 1 queued message, got %d", len(queueStub.messages))
	}
}

func TestStartAnalysisWithBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store, _ := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	jobDescription := strings.Repeat("a", 300)
	payload := map[string]string{
		"jobDescription": jobDescription,
		"promptVersion":  "v2",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", resp.Code)
	}

	var created struct {
		AnalysisID string `json:"analysisId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.AnalysisID == "" {
		t.Fatalf("expected analysisId, got empty")
	}

	analysis, err := analysisRepo.GetByID(context.Background(), created.AnalysisID)
	if err != nil {
		t.Fatalf("get analysis: %v", err)
	}
	if analysis.JobDescription != jobDescription {
		t.Fatalf("expected jobDescription to match payload, got %q", analysis.JobDescription)
	}
	if analysis.PromptVersion != "v2" {
		t.Fatalf("expected promptVersion v2, got %q", analysis.PromptVersion)
	}
}

func TestStartAnalysisAllowsEmptyJobDescriptionForATS(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store, _ := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	payload := map[string]string{
		"mode": "ATS",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", resp.Code)
	}

	var created struct {
		AnalysisID string `json:"analysisId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.AnalysisID == "" {
		t.Fatalf("expected analysisId, got empty")
	}

	analysis, err := analysisRepo.GetByID(context.Background(), created.AnalysisID)
	if err != nil {
		t.Fatalf("get analysis: %v", err)
	}
	if analysis.Mode != ModeATS {
		t.Fatalf("expected mode ATS, got %q", analysis.Mode)
	}
	if analysis.JobDescription != "" {
		t.Fatalf("expected empty jobDescription, got %q", analysis.JobDescription)
	}
}

func TestStartAnalysisRejectsShortJobDescriptionForJobMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, _, store, _ := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	payload := map[string]string{
		"mode":           "JOB_MATCH",
		"jobDescription": strings.Repeat("a", 299),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.Code)
	}
}

func TestStartAnalysisRejectsLongJobDescription(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, _, store, _ := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 50001),
		"promptVersion":  "v1",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.Code)
	}
}

func TestStartAnalysisIdempotentDoublePostSingleJob(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store, _ := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 300),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", resp.Code)
	}
	var first map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	firstID, _ := first["analysisId"].(string)
	if firstID == "" {
		t.Fatalf("expected analysisId in first response")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	addGuestHeader(req2)
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, req2)

	if resp2.Code != http.StatusAccepted && resp2.Code != http.StatusOK {
		t.Fatalf("expected status 202 or 200, got %d", resp2.Code)
	}
	var second map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	secondID, _ := second["analysisId"].(string)
	if secondID != firstID {
		t.Fatalf("expected same analysisId, got %q and %q", firstID, secondID)
	}

	analyses, err := analysisRepo.ListByUser(context.Background(), userID, 100, 0)
	if err != nil {
		t.Fatalf("list analyses: %v", err)
	}
	count := 0
	for _, a := range analyses {
		if a.DocumentID == documentID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 analysis for document, got %d", count)
	}
}

func TestStartAnalysisCompletedReturnsResult(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store, _ := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	result := map[string]any{
		"summary":    "done",
		"ats":        map[string]any{"score": 74.0},
		"finalScore": 74.0,
	}
	analysis := Analysis{
		ID:         "analysis-completed",
		DocumentID: documentID,
		UserID:     userID,
		Status:     StatusCompleted,
		Result:     result,
		CreatedAt:  time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 300),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	var decoded struct {
		AnalysisID string         `json:"analysisId"`
		Status     string         `json:"status"`
		Result     map[string]any `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if decoded.AnalysisID != analysis.ID {
		t.Fatalf("expected analysisId %q, got %q", analysis.ID, decoded.AnalysisID)
	}
	if decoded.Status != StatusCompleted {
		t.Fatalf("expected completed status, got %q", decoded.Status)
	}
	if decoded.Result == nil {
		t.Fatalf("expected result in response")
	}
	if got, ok := decoded.Result["finalScore"].(float64); !ok || got != 74 {
		t.Fatalf("expected finalScore 74, got %v", decoded.Result["finalScore"])
	}
	atsRaw, ok := decoded.Result["ats"].(map[string]any)
	if !ok {
		t.Fatalf("expected ats in response")
	}
	if got, ok := atsRaw["score"].(float64); !ok || got != 74 {
		t.Fatalf("expected ats.score 74, got %v", atsRaw["score"])
	}
}

func TestStartAnalysisFailedRequiresRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, docRepo, analysisRepo, store, _ := setupAnalysisRouter(t)
	userID := "guest:test-guest"
	documentID := seedDocument(t, docRepo, store, userID)

	msg := "boom"
	analysis := Analysis{
		ID:           "analysis-failed",
		DocumentID:   documentID,
		UserID:       userID,
		Status:       StatusFailed,
		ErrorMessage: &msg,
		CreatedAt:    time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	payload := map[string]string{
		"jobDescription": strings.Repeat("a", 300),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+documentID+"/analyze", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", resp.Code)
	}
}

func TestListAnalysesIncludesFinalScore(t *testing.T) {
	gin.SetMode(gin.TestMode)

	analysisRepo := NewMemoryRepo()
	svc := &Service{Repo: analysisRepo}
	handler := NewHandler(svc, nil)

	analysis := Analysis{
		ID:         "analysis-list",
		DocumentID: "doc-1",
		UserID:     "user-1",
		Status:     StatusCompleted,
		Result: map[string]any{
			"finalScore": 74.0,
			"matchScore": 81.0,
			"ats":        map[string]any{"score": 74.0},
			"summary":    "done",
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/analyses", nil)
	c.Set("userId", "user-1")
	c.Set("isGuest", false)

	handler.listAnalyses(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var payload []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 item, got %d", len(payload))
	}
	item := payload[0]
	if item["finalScore"] != 74.0 {
		t.Fatalf("expected finalScore 74, got %v", item["finalScore"])
	}
	if item["matchScore"] != 81.0 {
		t.Fatalf("expected matchScore 81, got %v", item["matchScore"])
	}
}

func TestCreateShareForOwnedAnalysisReturnsTokenAndURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, analysisRepo, _, _ := setupAnalysisRouter(t)
	analysis := Analysis{
		ID:         "analysis-share-create",
		DocumentID: "doc-share-create",
		UserID:     "guest:test-guest",
		Status:     StatusCompleted,
		Result: map[string]any{
			"summary": "ok",
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyses/"+analysis.ID+"/shares", nil)
	addGuestHeader(req)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.Code)
	}

	var payload struct {
		ShareID  string `json:"shareId"`
		Token    string `json:"token"`
		ShareURL string `json:"shareUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ShareID == "" {
		t.Fatalf("expected shareId")
	}
	if payload.Token == "" {
		t.Fatalf("expected token")
	}
	if payload.ShareURL == "" || !strings.Contains(payload.ShareURL, "/app/share/"+payload.Token) {
		t.Fatalf("expected shareUrl to include token, got %q", payload.ShareURL)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/analyses/"+analysis.ID+"/shares", nil)
	addGuestHeader(req2)
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusCreated {
		t.Fatalf("expected second status 201, got %d", resp2.Code)
	}
	var payload2 struct {
		ShareID  string `json:"shareId"`
		Token    string `json:"token"`
		ShareURL string `json:"shareUrl"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&payload2); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if payload2.ShareID != payload.ShareID {
		t.Fatalf("expected same shareId %q, got %q", payload.ShareID, payload2.ShareID)
	}
	if payload2.Token != payload.Token {
		t.Fatalf("expected same token %q, got %q", payload.Token, payload2.Token)
	}
	if payload2.ShareURL != payload.ShareURL {
		t.Fatalf("expected same shareUrl %q, got %q", payload.ShareURL, payload2.ShareURL)
	}
}

func TestPublicGetShareByTokenReturnsSanitizedAnalysisResult(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, analysisRepo, _, _ := setupAnalysisRouter(t)
	analysis := Analysis{
		ID:             "analysis-share-public",
		DocumentID:     "doc-share-public",
		UserID:         "guest:test-guest",
		JobDescription: "private job description",
		Status:         StatusCompleted,
		Result: map[string]any{
			"summary": "ok",
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/analyses/"+analysis.ID+"/shares", nil)
	addGuestHeader(createReq)
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create share status 201, got %d", createResp.Code)
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create share response: %v", err)
	}
	if created.Token == "" {
		t.Fatalf("expected token")
	}

	publicReq := httptest.NewRequest(http.MethodGet, "/api/v1/shares/"+created.Token, nil)
	publicResp := httptest.NewRecorder()
	router.ServeHTTP(publicResp, publicReq)
	if publicResp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", publicResp.Code)
	}

	var payload map[string]any
	if err := json.NewDecoder(publicResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["id"] != analysis.ID {
		t.Fatalf("expected id %q, got %v", analysis.ID, payload["id"])
	}
	if _, ok := payload["result"].(map[string]any); !ok {
		t.Fatalf("expected result object")
	}
	if _, exists := payload["jobDescription"]; exists {
		t.Fatalf("expected no jobDescription in shared response")
	}
	if _, exists := payload["documentId"]; exists {
		t.Fatalf("expected no documentId in shared response")
	}
}

func TestPublicGetRevokedShareReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, analysisRepo, _, _ := setupAnalysisRouter(t)
	analysis := Analysis{
		ID:         "analysis-share-revoked",
		DocumentID: "doc-share-revoked",
		UserID:     "guest:test-guest",
		Status:     StatusCompleted,
		Result: map[string]any{
			"summary": "ok",
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/analyses/"+analysis.ID+"/shares", nil)
	addGuestHeader(createReq)
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create share status 201, got %d", createResp.Code)
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create share response: %v", err)
	}

	revokeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/shares/"+created.Token, nil)
	addGuestHeader(revokeReq)
	revokeResp := httptest.NewRecorder()
	router.ServeHTTP(revokeResp, revokeReq)
	if revokeResp.Code != http.StatusNoContent {
		t.Fatalf("expected revoke status 204, got %d", revokeResp.Code)
	}

	publicReq := httptest.NewRequest(http.MethodGet, "/api/v1/shares/"+created.Token, nil)
	publicResp := httptest.NewRecorder()
	router.ServeHTTP(publicResp, publicReq)
	if publicResp.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", publicResp.Code)
	}
}

func TestNonOwnerCannotCreateOrRevokeShare(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router, _, analysisRepo, _, _ := setupAnalysisRouter(t)
	analysis := Analysis{
		ID:         "analysis-share-owner-check",
		DocumentID: "doc-share-owner-check",
		UserID:     "guest:test-guest",
		Status:     StatusCompleted,
		Result: map[string]any{
			"summary": "ok",
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := analysisRepo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("create analysis: %v", err)
	}

	ownerCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/analyses/"+analysis.ID+"/shares", nil)
	addGuestHeader(ownerCreateReq)
	ownerCreateResp := httptest.NewRecorder()
	router.ServeHTTP(ownerCreateResp, ownerCreateReq)
	if ownerCreateResp.Code != http.StatusCreated {
		t.Fatalf("expected owner create status 201, got %d", ownerCreateResp.Code)
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(ownerCreateResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode owner create response: %v", err)
	}

	nonOwnerCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/analyses/"+analysis.ID+"/shares", nil)
	nonOwnerCreateReq.Header.Set("X-Guest-Id", "other-guest")
	nonOwnerCreateResp := httptest.NewRecorder()
	router.ServeHTTP(nonOwnerCreateResp, nonOwnerCreateReq)
	if nonOwnerCreateResp.Code != http.StatusForbidden {
		t.Fatalf("expected non-owner create status 403, got %d", nonOwnerCreateResp.Code)
	}

	nonOwnerRevokeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/shares/"+created.Token, nil)
	nonOwnerRevokeReq.Header.Set("X-Guest-Id", "other-guest")
	nonOwnerRevokeResp := httptest.NewRecorder()
	router.ServeHTTP(nonOwnerRevokeResp, nonOwnerRevokeReq)
	if nonOwnerRevokeResp.Code != http.StatusForbidden {
		t.Fatalf("expected non-owner revoke status 403, got %d", nonOwnerRevokeResp.Code)
	}
}

type stubLLM struct{}

func (stubLLM) AnalyzeResume(ctx context.Context, input llm.AnalyzeInput) (json.RawMessage, error) {
	_ = ctx
	_ = input
	return json.RawMessage(`{
  "summary": {"overallAssessment": "ok", "strengths": [], "weaknesses": []},
  "ats": {"score": 80, "missingKeywords": [], "formattingIssues": []},
  "issues": [],
  "bulletRewrites": [],
  "missingInformation": [],
  "actionPlan": {"quickWins": [], "mediumEffort": [], "deepFixes": []}
}`), nil
}

type stubQueue struct {
	messages []queue.Message
	err      error
}

func (s *stubQueue) Send(ctx context.Context, msg queue.Message) error {
	_ = ctx
	if s.err != nil {
		return s.err
	}
	s.messages = append(s.messages, msg)
	return nil
}

func setupAnalysisRouter(t *testing.T) (*gin.Engine, *documents.MemoryRepo, *MemoryRepo, object.ObjectStore, *stubQueue) {
	t.Helper()
	docRepo := documents.NewMemoryRepo()
	analysisRepo := NewMemoryRepo()
	storeDir := t.TempDir()
	store := local.New(storeDir)
	tokenCipher, err := sharedcrypto.NewTokenCipher(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatalf("create token cipher: %v", err)
	}
	queueStub := &stubQueue{}
	svc := &Service{
		Repo:             analysisRepo,
		DocRepo:          docRepo,
		Store:            store,
		LLM:              stubLLM{},
		JobQueue:         queueStub,
		ShareTokenCipher: tokenCipher,
		UIBaseURL:        "https://rethinkresume.com",
	}
	handler := NewHandler(svc, docRepo)

	router := gin.New()
	router.Use(middleware.Auth("dev"))
	api := router.Group("/api/v1")
	handler.RegisterRoutes(api)

	return router, docRepo, analysisRepo, store, queueStub
}

func seedDocument(t *testing.T, repo *documents.MemoryRepo, store object.ObjectStore, userID string) string {
	t.Helper()

	extractedKey, _, _, err := store.Save(context.Background(), userID, "resume.txt", bytes.NewReader([]byte("resume text")))
	if err != nil {
		t.Fatalf("save extracted text: %v", err)
	}
	doc := documents.Document{
		ID:               "doc-" + userID,
		UserID:           userID,
		FileName:         "resume.pdf",
		MimeType:         "application/pdf",
		SizeBytes:        123,
		StorageKey:       "test-key",
		ExtractedTextKey: extractedKey,
		CreatedAt:        time.Now().UTC(),
	}
	if err := repo.Create(context.Background(), doc); err != nil {
		t.Fatalf("create document: %v", err)
	}
	return doc.ID
}

func addGuestHeader(req *http.Request) {
	req.Header.Set("X-Guest-Id", "test-guest")
}
