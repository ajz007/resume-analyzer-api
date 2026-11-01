package workspace

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type PGStore struct{ DB *sql.DB }

func (s *PGStore) Primary(ctx context.Context, user string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT document_id FROM document_preferences WHERE user_id=$1`, user).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
func (s *PGStore) SetPrimary(ctx context.Context, user, id string) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO document_preferences(user_id,document_id)
 SELECT $1,id FROM documents WHERE user_id=$1 AND id=$2 AND deleted_at IS NULL
 ON CONFLICT(user_id) DO UPDATE SET document_id=EXCLUDED.document_id`, user, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *PGStore) GetLabels(ctx context.Context, id string) (Labels, error) {
	var l Labels
	err := s.DB.QueryRowContext(ctx, `SELECT company_name,target_role FROM application_labels WHERE analysis_id=$1`, id).Scan(&l.CompanyName, &l.TargetRole)
	if errors.Is(err, sql.ErrNoRows) {
		return l, nil
	}
	return l, err
}
func (s *PGStore) SaveLabels(ctx context.Context, id string, l Labels) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO application_labels(analysis_id,company_name,target_role) VALUES($1,$2,$3)
 ON CONFLICT(analysis_id) DO UPDATE SET company_name=EXCLUDED.company_name,target_role=EXCLUDED.target_role,updated_at=now()`, id, l.CompanyName, l.TargetRole)
	return err
}
func (s *PGStore) List(ctx context.Context, user, q string, limit, offset int) ([]Application, error) {
	// Treat search as literal text, including %, _ and backslash, rather than SQL patterns.
	pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
	rows, err := s.DB.QueryContext(ctx, `SELECT a.id,a.document_id,COALESCE(d.file_name,''),LEFT(COALESCE(a.job_description,''),240),
 COALESCE(a.mode,'job_match'),a.status,a.created_at,COALESCE(l.company_name,''),COALESCE(l.target_role,'')
 FROM analyses a
 LEFT JOIN documents d ON d.id=a.document_id AND d.user_id=a.user_id AND d.deleted_at IS NULL
 LEFT JOIN application_labels l ON l.analysis_id=a.id
 WHERE a.user_id=$1 AND a.deleted_at IS NULL AND ($2='' OR
 COALESCE(l.company_name,'') ILIKE $3 OR COALESCE(l.target_role,'') ILIKE $3 OR
 COALESCE(d.file_name,'') ILIKE $3 OR LEFT(COALESCE(a.job_description,''),240) ILIKE $3)
 ORDER BY a.created_at DESC,a.id DESC LIMIT $4 OFFSET $5`, user, q, pattern, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Application{}
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.AnalysisID, &a.DocumentID, &a.ResumeFileName, &a.JobExcerpt, &a.Mode, &a.Status, &a.CreatedAt, &a.CompanyName, &a.TargetRole); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
