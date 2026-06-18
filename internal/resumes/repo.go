package resumes

import "context"

type Repo interface {
	Create(ctx context.Context, resume Resume, version ResumeVersion) (Resume, error)
	Update(ctx context.Context, ownerID, resumeID, title string, version ResumeVersion) (Resume, error)
	GetByID(ctx context.Context, ownerID, resumeID string) (Resume, error)
	ListByOwner(ctx context.Context, ownerID string, limit, offset int) ([]Resume, error)
	ListVersions(ctx context.Context, ownerID, resumeID string) ([]ResumeVersion, error)
	GetVersionByID(ctx context.Context, ownerID, resumeID, versionID string) (ResumeVersion, error)
}
