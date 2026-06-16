package resumes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type PGRepo struct {
	DB *sql.DB
}

func (r *PGRepo) Create(ctx context.Context, resume Resume, version ResumeVersion) (Resume, error) {
	if !validSourceType(version.SourceType) {
		return Resume{}, ErrInvalidInput
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Resume{}, err
	}
	defer tx.Rollback()

	resumeJSON, err := json.Marshal(resume.CurrentResume)
	if err != nil {
		return Resume{}, err
	}
	versionJSON, err := json.Marshal(version.Resume)
	if err != nil {
		return Resume{}, err
	}
	changeSummaryJSON, err := marshalNullableJSON(version.ChangeSummary)
	if err != nil {
		return Resume{}, err
	}

	const insertResume = `
INSERT INTO resumes (
    id, owner_id, title, status, current_version_id, current_resume_json, created_at, updated_at
) VALUES ($1, $2, $3, $4, NULL, $5, $6, $7)`
	if _, err := tx.ExecContext(ctx, insertResume,
		resume.ID,
		resume.OwnerID,
		resume.Title,
		resume.Status,
		resumeJSON,
		resume.CreatedAt,
		resume.UpdatedAt,
	); err != nil {
		return Resume{}, err
	}

	const insertVersion = `
INSERT INTO resume_versions (
    id, resume_id, version_number, source_type, resume_json, change_summary, parent_version_id, created_at
) VALUES ($1, $2, $3, $4, $5, $6, NULL, $7)`
	if _, err := tx.ExecContext(ctx, insertVersion,
		version.ID,
		version.ResumeID,
		version.VersionNumber,
		version.SourceType,
		versionJSON,
		changeSummaryJSON,
		version.CreatedAt,
	); err != nil {
		return Resume{}, err
	}

	const setCurrentVersion = `
UPDATE resumes
SET current_version_id = $1
WHERE id = $2`
	if _, err := tx.ExecContext(ctx, setCurrentVersion, version.ID, resume.ID); err != nil {
		return Resume{}, err
	}

	if err := tx.Commit(); err != nil {
		return Resume{}, err
	}
	resume.CurrentVersionID = version.ID
	return resume, nil
}

func (r *PGRepo) Update(ctx context.Context, ownerID, resumeID, title string, version ResumeVersion) (Resume, error) {
	if !validSourceType(version.SourceType) {
		return Resume{}, ErrInvalidInput
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Resume{}, err
	}
	defer tx.Rollback()

	const selectResume = `
SELECT id, owner_id, title, status, current_version_id, current_resume_json, created_at, updated_at
FROM resumes
WHERE id = $1
FOR UPDATE`
	current, err := scanResume(tx.QueryRowContext(ctx, selectResume, resumeID))
	if err != nil {
		return Resume{}, err
	}
	if current.OwnerID != ownerID {
		return Resume{}, ErrForbidden
	}

	const nextVersion = `
SELECT COALESCE(MAX(version_number), 0) + 1
FROM resume_versions
WHERE resume_id = $1`
	if err := tx.QueryRowContext(ctx, nextVersion, resumeID).Scan(&version.VersionNumber); err != nil {
		return Resume{}, err
	}
	if current.CurrentVersionID != "" {
		parent := current.CurrentVersionID
		version.ParentVersionID = &parent
	}
	if version.CreatedAt.IsZero() {
		version.CreatedAt = time.Now().UTC()
	}

	versionJSON, err := json.Marshal(version.Resume)
	if err != nil {
		return Resume{}, err
	}
	changeSummaryJSON, err := marshalNullableJSON(version.ChangeSummary)
	if err != nil {
		return Resume{}, err
	}

	const insertVersion = `
INSERT INTO resume_versions (
    id, resume_id, version_number, source_type, resume_json, change_summary, parent_version_id, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	if _, err := tx.ExecContext(ctx, insertVersion,
		version.ID,
		resumeID,
		version.VersionNumber,
		version.SourceType,
		versionJSON,
		changeSummaryJSON,
		nullableString(version.ParentVersionID),
		version.CreatedAt,
	); err != nil {
		return Resume{}, err
	}

	const updateResume = `
UPDATE resumes
SET title = $1,
    current_version_id = $2,
    current_resume_json = $3,
    updated_at = $4
WHERE id = $5`
	if _, err := tx.ExecContext(ctx, updateResume, title, version.ID, versionJSON, version.CreatedAt, resumeID); err != nil {
		return Resume{}, err
	}

	if err := tx.Commit(); err != nil {
		return Resume{}, err
	}

	current.Title = title
	current.CurrentVersionID = version.ID
	current.CurrentResume = version.Resume
	current.UpdatedAt = version.CreatedAt
	return current, nil
}

func (r *PGRepo) GetByID(ctx context.Context, ownerID, resumeID string) (Resume, error) {
	const query = `
SELECT id, owner_id, title, status, current_version_id, current_resume_json, created_at, updated_at
FROM resumes
WHERE id = $1
LIMIT 1`
	resume, err := scanResume(r.DB.QueryRowContext(ctx, query, resumeID))
	if err != nil {
		return Resume{}, err
	}
	if resume.OwnerID != ownerID {
		return Resume{}, ErrForbidden
	}
	return resume, nil
}

func (r *PGRepo) ListByOwner(ctx context.Context, ownerID string, limit, offset int) ([]Resume, error) {
	limit, offset = normalizeLimitOffset(limit, offset)
	const query = `
SELECT id, owner_id, title, status, current_version_id, current_resume_json, created_at, updated_at
FROM resumes
WHERE owner_id = $1
ORDER BY updated_at DESC
LIMIT $2 OFFSET $3`
	rows, err := r.DB.QueryContext(ctx, query, ownerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Resume
	for rows.Next() {
		resume, err := scanResumeRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, resume)
	}
	return out, rows.Err()
}

func (r *PGRepo) ListVersions(ctx context.Context, ownerID, resumeID string) ([]ResumeVersion, error) {
	if _, err := r.GetByID(ctx, ownerID, resumeID); err != nil {
		return nil, err
	}
	const query = `
SELECT id, resume_id, version_number, source_type, resume_json, change_summary, parent_version_id, created_at
FROM resume_versions
WHERE resume_id = $1
ORDER BY version_number DESC`
	rows, err := r.DB.QueryContext(ctx, query, resumeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ResumeVersion
	for rows.Next() {
		version, err := scanVersionRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, version)
	}
	return out, rows.Err()
}

func (r *PGRepo) GetVersionByID(ctx context.Context, ownerID, resumeID, versionID string) (ResumeVersion, error) {
	if _, err := r.GetByID(ctx, ownerID, resumeID); err != nil {
		return ResumeVersion{}, err
	}
	const query = `
SELECT id, resume_id, version_number, source_type, resume_json, change_summary, parent_version_id, created_at
FROM resume_versions
WHERE resume_id = $1 AND id = $2
LIMIT 1`
	version, err := scanVersion(r.DB.QueryRowContext(ctx, query, resumeID, versionID))
	if err != nil {
		return ResumeVersion{}, err
	}
	return version, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanResume(row rowScanner) (Resume, error) {
	var resume Resume
	var versionID sql.NullString
	var raw []byte
	err := row.Scan(
		&resume.ID,
		&resume.OwnerID,
		&resume.Title,
		&resume.Status,
		&versionID,
		&raw,
		&resume.CreatedAt,
		&resume.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Resume{}, ErrNotFound
		}
		return Resume{}, err
	}
	if versionID.Valid {
		resume.CurrentVersionID = versionID.String
	}
	if err := json.Unmarshal(raw, &resume.CurrentResume); err != nil {
		return Resume{}, err
	}
	return resume, nil
}

func scanResumeRows(rows *sql.Rows) (Resume, error) {
	return scanResume(rows)
}

func scanVersion(row rowScanner) (ResumeVersion, error) {
	var version ResumeVersion
	var raw []byte
	var changeRaw []byte
	var parent sql.NullString
	err := row.Scan(
		&version.ID,
		&version.ResumeID,
		&version.VersionNumber,
		&version.SourceType,
		&raw,
		&changeRaw,
		&parent,
		&version.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ResumeVersion{}, ErrNotFound
		}
		return ResumeVersion{}, err
	}
	if parent.Valid {
		version.ParentVersionID = &parent.String
	}
	if len(changeRaw) > 0 {
		if err := json.Unmarshal(changeRaw, &version.ChangeSummary); err != nil {
			return ResumeVersion{}, err
		}
	}
	if err := json.Unmarshal(raw, &version.Resume); err != nil {
		return ResumeVersion{}, err
	}
	return version, nil
}

func scanVersionRows(rows *sql.Rows) (ResumeVersion, error) {
	return scanVersion(rows)
}

func marshalNullableJSON(value map[string]any) (any, error) {
	if value == nil {
		return nil, nil
	}
	return json.Marshal(value)
}

func nullableString(value *string) sql.NullString {
	if value == nil || *value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

var _ Repo = (*PGRepo)(nil)
