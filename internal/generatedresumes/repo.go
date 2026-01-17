package generatedresumes

import "context"

// Repo defines persistence operations for generated resumes.
type Repo interface {
	Create(ctx context.Context, resume GeneratedResume) error
	GetByID(ctx context.Context, userID, generatedResumeID string) (GeneratedResume, error)
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]GeneratedResume, error)
}
