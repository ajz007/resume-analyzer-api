package resumes_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	resumespkg "resume-backend/internal/resumes"
	storagedb "resume-backend/internal/shared/storage/db"
	modelv1 "resume-backend/resume/modelv1"
)

func TestPGRepoResumeLineageMigrationsAndRoundTrip(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping Postgres integration test")
	}

	ctx := context.Background()
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	schema := "resume_lineage_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	if _, err := database.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer database.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	if _, err := database.ExecContext(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	if err := storagedb.RunMigrations(ctx, database); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	assertColumnExists(t, database, schema, "resumes", "source_resume_id")
	assertColumnExists(t, database, schema, "resumes", "source_version_id")
	assertColumnExists(t, database, schema, "resumes", "origin_type")
	assertColumnExists(t, database, schema, "resume_versions", "source_version_id")

	repo := &resumespkg.PGRepo{DB: database}
	ownerID := "google:pg-owner"
	now := time.Now().UTC()
	sourceResumeID := uuid.NewString()
	sourceVersionID := uuid.NewString()
	source, err := repo.Create(ctx, resumespkg.Resume{
		ID:               sourceResumeID,
		OwnerID:          ownerID,
		Title:            "Source Resume",
		Status:           resumespkg.StatusDraft,
		OriginType:       resumespkg.OriginManual,
		CurrentVersionID: sourceVersionID,
		CurrentResume:    validResume(),
		CreatedAt:        now,
		UpdatedAt:        now,
	}, resumespkg.ResumeVersion{
		ID:            sourceVersionID,
		ResumeID:      sourceResumeID,
		VersionNumber: 1,
		SourceType:    resumespkg.SourceManual,
		Resume:        validResume(),
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create source resume: %v", err)
	}

	tailoredResume := validResume()
	tailoredResume.Summary = modelv1.Summary{Text: "Tailored summary with source-supported Go API experience."}
	tailoredResumeID := uuid.NewString()
	tailoredVersionID := uuid.NewString()
	tailored, err := repo.Create(ctx, resumespkg.Resume{
		ID:               tailoredResumeID,
		OwnerID:          ownerID,
		Title:            "Tailored Resume",
		Status:           resumespkg.StatusDraft,
		SourceResumeID:   source.ID,
		SourceVersionID:  source.CurrentVersionID,
		OriginType:       resumespkg.OriginAITailored,
		CurrentVersionID: tailoredVersionID,
		CurrentResume:    tailoredResume,
		CreatedAt:        now.Add(time.Second),
		UpdatedAt:        now.Add(time.Second),
	}, resumespkg.ResumeVersion{
		ID:              tailoredVersionID,
		ResumeID:        tailoredResumeID,
		VersionNumber:   1,
		SourceType:      resumespkg.SourceAITailored,
		SourceVersionID: source.CurrentVersionID,
		Resume:          tailoredResume,
		CreatedAt:       now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("create tailored resume: %v", err)
	}
	if tailored.SourceResumeID != source.ID {
		t.Fatalf("expected source resume id %s, got %s", source.ID, tailored.SourceResumeID)
	}
	if tailored.SourceVersionID != source.CurrentVersionID {
		t.Fatalf("expected source version id %s, got %s", source.CurrentVersionID, tailored.SourceVersionID)
	}
	if tailored.OriginType != resumespkg.OriginAITailored {
		t.Fatalf("expected tailored origin, got %q", tailored.OriginType)
	}

	got, err := repo.GetByID(ctx, ownerID, tailored.ID)
	if err != nil {
		t.Fatalf("get tailored resume: %v", err)
	}
	if got.SourceResumeID != source.ID || got.SourceVersionID != source.CurrentVersionID || got.OriginType != resumespkg.OriginAITailored {
		t.Fatalf("lineage did not round-trip through get: %#v", got)
	}

	list, err := repo.ListByOwner(ctx, ownerID, 10, 0)
	if err != nil {
		t.Fatalf("list resumes: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 resumes, got %d", len(list))
	}

	versions, err := repo.ListVersions(ctx, ownerID, tailored.ID)
	if err != nil {
		t.Fatalf("list tailored versions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 tailored version, got %d", len(versions))
	}
	if versions[0].SourceVersionID != source.CurrentVersionID {
		t.Fatalf("expected version sourceVersionId %s, got %s", source.CurrentVersionID, versions[0].SourceVersionID)
	}
}

func assertColumnExists(t *testing.T, database *sql.DB, schema, table, column string) {
	t.Helper()
	var count int
	query := `
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2 AND column_name = $3`
	if err := database.QueryRowContext(context.Background(), query, schema, table, column).Scan(&count); err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	if count != 1 {
		t.Fatalf("expected column %s.%s to exist in schema %s, got %d", table, column, schema, count)
	}
}
