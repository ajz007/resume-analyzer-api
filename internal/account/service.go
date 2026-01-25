package account

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"resume-backend/internal/analyses"
	"resume-backend/internal/documents"
)

type Service struct {
	DocRepo      documents.DocumentsRepo
	AnalysisRepo analyses.Repo
}

type ClaimResult struct {
	MigratedDocuments int `json:"migratedDocuments"`
	MigratedAnalyses  int `json:"migratedAnalyses"`
}

func NewService(docRepo documents.DocumentsRepo, analysisRepo analyses.Repo) *Service {
	return &Service{DocRepo: docRepo, AnalysisRepo: analysisRepo}
}

func (s *Service) ClaimGuest(ctx context.Context, guestUserID, authedUserID string) (ClaimResult, error) {
	if strings.TrimSpace(guestUserID) == "" || strings.TrimSpace(authedUserID) == "" {
		return ClaimResult{}, errors.New("guestUserID and authedUserID are required")
	}

	if docPG, ok := s.DocRepo.(*documents.PGRepo); ok && docPG != nil && docPG.DB != nil {
		if analysisPG, ok := s.AnalysisRepo.(*analyses.PGRepo); ok && analysisPG != nil && analysisPG.DB != nil {
			return claimWithTx(ctx, docPG.DB, guestUserID, authedUserID)
		}
	}

	docCount, err := claimDocs(ctx, s.DocRepo, guestUserID, authedUserID)
	if err != nil {
		return ClaimResult{}, err
	}
	analysisCount, err := claimAnalyses(ctx, s.AnalysisRepo, guestUserID, authedUserID)
	if err != nil {
		return ClaimResult{}, err
	}
	return ClaimResult{MigratedDocuments: docCount, MigratedAnalyses: analysisCount}, nil
}

func claimWithTx(ctx context.Context, db *sql.DB, guestUserID, authedUserID string) (ClaimResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ClaimResult{}, err
	}
	defer tx.Rollback()

	docRes, err := tx.ExecContext(ctx, `UPDATE documents SET user_id = $1 WHERE user_id = $2 AND deleted_at IS NULL`, authedUserID, guestUserID)
	if err != nil {
		return ClaimResult{}, err
	}
	docCount, _ := docRes.RowsAffected()

	analysisRes, err := tx.ExecContext(ctx, `UPDATE analyses SET user_id = $1 WHERE user_id = $2 AND deleted_at IS NULL`, authedUserID, guestUserID)
	if err != nil {
		return ClaimResult{}, err
	}
	analysisCount, _ := analysisRes.RowsAffected()

	if err := tx.Commit(); err != nil {
		return ClaimResult{}, err
	}
	return ClaimResult{MigratedDocuments: int(docCount), MigratedAnalyses: int(analysisCount)}, nil
}

type guestDocClaimer interface {
	ClaimGuest(ctx context.Context, guestUserID, authedUserID string) (int, error)
}

type guestAnalysisClaimer interface {
	ClaimGuest(ctx context.Context, guestUserID, authedUserID string) (int, error)
}

func claimDocs(ctx context.Context, repo documents.DocumentsRepo, guestUserID, authedUserID string) (int, error) {
	if claimer, ok := repo.(guestDocClaimer); ok {
		return claimer.ClaimGuest(ctx, guestUserID, authedUserID)
	}
	return 0, errors.New("documents repo does not support claim")
}

func claimAnalyses(ctx context.Context, repo analyses.Repo, guestUserID, authedUserID string) (int, error) {
	if claimer, ok := repo.(guestAnalysisClaimer); ok {
		return claimer.ClaimGuest(ctx, guestUserID, authedUserID)
	}
	return 0, errors.New("analyses repo does not support claim")
}
