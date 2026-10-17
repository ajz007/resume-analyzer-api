package analyses

import (
	"context"
	"testing"
	"time"
)

func TestMemoryRepoGetOrCreateScopesReuseToModeAndJD(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepo()
	newAnalysis := func(id string, mode AnalysisMode, jd string) Analysis {
		return Analysis{
			ID:             id,
			DocumentID:     "doc-1",
			UserID:         "user-1",
			Mode:           mode,
			JobDescription: jd,
			Status:         StatusQueued,
			CreatedAt:      time.Now().UTC(),
		}
	}

	first, created, err := repo.GetOrCreateForDocument(ctx, newAnalysis("a1", ModeJobMatch, "jd one"), false, nil)
	if err != nil || !created {
		t.Fatalf("expected first analysis to be created, created=%v err=%v", created, err)
	}

	same, created, err := repo.GetOrCreateForDocument(ctx, newAnalysis("a2", ModeJobMatch, "jd one"), false, nil)
	if err != nil || created || same.ID != first.ID {
		t.Fatalf("expected same mode+JD to reuse %q, got %q created=%v err=%v", first.ID, same.ID, created, err)
	}

	otherJD, created, err := repo.GetOrCreateForDocument(ctx, newAnalysis("a3", ModeJobMatch, "jd two"), false, nil)
	if err != nil || !created || otherJD.ID != "a3" {
		t.Fatalf("expected different JD to create a new analysis, got %q created=%v err=%v", otherJD.ID, created, err)
	}

	atsMode, created, err := repo.GetOrCreateForDocument(ctx, newAnalysis("a4", ModeATS, ""), false, nil)
	if err != nil || !created || atsMode.ID != "a4" {
		t.Fatalf("expected ATS mode to create a new analysis, got %q created=%v err=%v", atsMode.ID, created, err)
	}
}
