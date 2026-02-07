package analyses

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPGRepoCreateIncludesPromptMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := &PGRepo{DB: db}
	analysis := Analysis{
		ID:              "analysis-1",
		DocumentID:      "doc-1",
		UserID:          "user-1",
		Status:          StatusQueued,
		JobDescription:  "jd",
		PromptVersion:   "v2_1",
		AnalysisVersion: "gpt-5-mini:v1",
		PromptHash:      "deadbeef",
		Provider:        "openai",
		Model:           "gpt-4o-mini",
		CreatedAt:       time.Now().UTC(),
	}

	mock.ExpectExec("INSERT INTO analyses").
		WithArgs(
			analysis.ID,
			analysis.DocumentID,
			analysis.UserID,
			analysis.Status,
			nil,
			sqlmock.AnyArg(), // analysis_raw
			sqlmock.AnyArg(), // analysis_result
			nil,              // analysis_completed_at
			analysis.JobDescription,
			analysis.PromptVersion,
			ModeJobMatch,
			analysis.AnalysisVersion,
			analysis.PromptHash,
			analysis.Provider,
			analysis.Model,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), analysis); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
