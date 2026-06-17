package resumes

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryRepo struct {
	mu             sync.RWMutex
	byID           map[string]Resume
	byOwner        map[string][]string
	versionsByID   map[string]ResumeVersion
	versionsByItem map[string][]string
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		byID:           make(map[string]Resume),
		byOwner:        make(map[string][]string),
		versionsByID:   make(map[string]ResumeVersion),
		versionsByItem: make(map[string][]string),
	}
}

func (r *MemoryRepo) Create(ctx context.Context, resume Resume, version ResumeVersion) (Resume, error) {
	if err := ctx.Err(); err != nil {
		return Resume{}, err
	}
	if !validSourceType(version.SourceType) {
		return Resume{}, ErrInvalidInput
	}
	if !validOriginType(resume.OriginType) {
		return Resume{}, ErrInvalidInput
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.byID[resume.ID] = resume
	r.byOwner[resume.OwnerID] = append(r.byOwner[resume.OwnerID], resume.ID)
	r.versionsByID[version.ID] = version
	r.versionsByItem[resume.ID] = append(r.versionsByItem[resume.ID], version.ID)
	return resume, nil
}

func (r *MemoryRepo) Update(ctx context.Context, ownerID, resumeID, title string, version ResumeVersion) (Resume, error) {
	if err := ctx.Err(); err != nil {
		return Resume{}, err
	}
	if !validSourceType(version.SourceType) {
		return Resume{}, ErrInvalidInput
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	resume, ok := r.byID[resumeID]
	if !ok {
		return Resume{}, ErrNotFound
	}
	if resume.OwnerID != ownerID {
		return Resume{}, ErrForbidden
	}

	versionIDs := r.versionsByItem[resumeID]
	version.VersionNumber = len(versionIDs) + 1
	if resume.CurrentVersionID != "" {
		parent := resume.CurrentVersionID
		version.ParentVersionID = &parent
	}
	if version.CreatedAt.IsZero() {
		version.CreatedAt = time.Now().UTC()
	}

	resume.Title = title
	resume.CurrentResume = version.Resume
	resume.CurrentVersionID = version.ID
	resume.UpdatedAt = version.CreatedAt

	r.byID[resumeID] = resume
	r.versionsByID[version.ID] = version
	r.versionsByItem[resumeID] = append(versionIDs, version.ID)
	return resume, nil
}

func (r *MemoryRepo) GetByID(ctx context.Context, ownerID, resumeID string) (Resume, error) {
	if err := ctx.Err(); err != nil {
		return Resume{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	resume, ok := r.byID[resumeID]
	if !ok {
		return Resume{}, ErrNotFound
	}
	if resume.OwnerID != ownerID {
		return Resume{}, ErrForbidden
	}
	return resume, nil
}

func (r *MemoryRepo) ListByOwner(ctx context.Context, ownerID string, limit, offset int) ([]Resume, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit, offset = normalizeLimitOffset(limit, offset)

	r.mu.RLock()
	ids := append([]string(nil), r.byOwner[ownerID]...)
	resumes := make([]Resume, 0, len(ids))
	for _, id := range ids {
		resumes = append(resumes, r.byID[id])
	}
	r.mu.RUnlock()

	sort.Slice(resumes, func(i, j int) bool {
		return resumes[i].UpdatedAt.After(resumes[j].UpdatedAt)
	})
	if offset >= len(resumes) {
		return []Resume{}, nil
	}
	end := len(resumes)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return resumes[offset:end], nil
}

func (r *MemoryRepo) ListVersions(ctx context.Context, ownerID, resumeID string) ([]ResumeVersion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	resume, ok := r.byID[resumeID]
	if !ok {
		r.mu.RUnlock()
		return nil, ErrNotFound
	}
	if resume.OwnerID != ownerID {
		r.mu.RUnlock()
		return nil, ErrForbidden
	}
	ids := append([]string(nil), r.versionsByItem[resumeID]...)
	versions := make([]ResumeVersion, 0, len(ids))
	for _, id := range ids {
		versions = append(versions, r.versionsByID[id])
	}
	r.mu.RUnlock()

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].VersionNumber > versions[j].VersionNumber
	})
	return versions, nil
}

func (r *MemoryRepo) GetVersionByID(ctx context.Context, ownerID, resumeID, versionID string) (ResumeVersion, error) {
	if err := ctx.Err(); err != nil {
		return ResumeVersion{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	resume, ok := r.byID[resumeID]
	if !ok {
		return ResumeVersion{}, ErrNotFound
	}
	if resume.OwnerID != ownerID {
		return ResumeVersion{}, ErrForbidden
	}
	version, ok := r.versionsByID[versionID]
	if !ok || version.ResumeID != resumeID {
		return ResumeVersion{}, ErrNotFound
	}
	return version, nil
}

func normalizeLimitOffset(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

var _ Repo = (*MemoryRepo)(nil)
