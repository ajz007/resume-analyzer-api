package analyses

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const shareTokenBytes = 32

// CreateShare creates a new share token for an analysis owned by requesterUserID.
func (s *Service) CreateShare(ctx context.Context, analysisID, requesterUserID string) (AnalysisShare, string, error) {
	if strings.TrimSpace(analysisID) == "" || strings.TrimSpace(requesterUserID) == "" {
		return AnalysisShare{}, "", errors.New("analysisID and requesterUserID are required")
	}
	if s.ShareTokenCipher == nil {
		return AnalysisShare{}, "", errors.New("share token cipher is not configured")
	}

	analysis, err := s.Repo.GetByID(ctx, analysisID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AnalysisShare{}, "", ErrNotFound
		}
		return AnalysisShare{}, "", err
	}
	if analysis.UserID != requesterUserID {
		return AnalysisShare{}, "", ErrForbidden
	}
	ownerUserID, ownerGuestID := parseShareOwner(requesterUserID)
	if ownerUserID == nil && ownerGuestID == nil {
		return AnalysisShare{}, "", errors.New("invalid share owner")
	}

	existing, err := s.Repo.GetActiveShareByAnalysisOwner(ctx, analysisID, ownerUserID, ownerGuestID)
	if err == nil {
		token, decryptErr := s.ShareTokenCipher.Decrypt(existing.TokenCipher)
		if decryptErr != nil {
			return AnalysisShare{}, "", decryptErr
		}
		return existing, token, nil
	}
	if !errors.Is(err, ErrShareNotFound) {
		return AnalysisShare{}, "", err
	}

	token, err := newShareToken()
	if err != nil {
		return AnalysisShare{}, "", err
	}
	tokenHash := hashShareToken(token)
	tokenCipher, err := s.ShareTokenCipher.Encrypt(token)
	if err != nil {
		return AnalysisShare{}, "", err
	}

	share := AnalysisShare{
		ID:           uuid.NewString(),
		AnalysisID:   analysisID,
		OwnerUserID:  ownerUserID,
		OwnerGuestID: ownerGuestID,
		TokenHash:    tokenHash,
		TokenCipher:  tokenCipher,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.Repo.CreateShare(ctx, share); err != nil {
		return AnalysisShare{}, "", err
	}
	return share, token, nil
}

// GetSharedAnalysisByToken returns a shared analysis for a public share token.
func (s *Service) GetSharedAnalysisByToken(ctx context.Context, token string) (Analysis, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Analysis{}, ErrShareNotFound
	}

	share, err := s.Repo.GetShareByTokenHash(ctx, hashShareToken(token))
	if err != nil {
		if errors.Is(err, ErrShareNotFound) {
			return Analysis{}, ErrShareNotFound
		}
		return Analysis{}, err
	}
	if share.RevokedAt != nil {
		return Analysis{}, ErrShareNotFound
	}

	analysis, err := s.Repo.GetByID(ctx, share.AnalysisID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Analysis{}, ErrShareNotFound
		}
		return Analysis{}, err
	}
	return analysis, nil
}

// RevokeShareByToken revokes a share token if requesterUserID owns it.
func (s *Service) RevokeShareByToken(ctx context.Context, token, requesterUserID string) error {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(requesterUserID) == "" {
		return errors.New("token and requesterUserID are required")
	}

	share, err := s.Repo.GetShareByTokenHash(ctx, hashShareToken(token))
	if err != nil {
		if errors.Is(err, ErrShareNotFound) {
			return ErrShareNotFound
		}
		return err
	}
	if share.RevokedAt != nil {
		return ErrShareNotFound
	}
	if !shareOwnedByUserID(share, requesterUserID) {
		return ErrForbidden
	}
	return s.Repo.RevokeShare(ctx, share.ID, time.Now().UTC())
}

func newShareToken() (string, error) {
	buf := make([]byte, shareTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashShareToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func parseShareOwner(userID string) (*string, *string) {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "guest:") {
		guestID := strings.TrimPrefix(trimmed, "guest:")
		if guestID == "" {
			return nil, nil
		}
		return nil, &guestID
	}
	return &trimmed, nil
}

func shareOwnedByUserID(share AnalysisShare, userID string) bool {
	ownerUserID, ownerGuestID := parseShareOwner(userID)
	if ownerUserID != nil {
		return share.OwnerUserID != nil && *share.OwnerUserID == *ownerUserID && share.OwnerGuestID == nil
	}
	if ownerGuestID != nil {
		return share.OwnerGuestID != nil && *share.OwnerGuestID == *ownerGuestID && share.OwnerUserID == nil
	}
	return false
}
