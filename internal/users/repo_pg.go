package users

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type PGRepo struct {
	DB *sql.DB
}

func (r *PGRepo) Upsert(ctx context.Context, user User) error {
	const query = `
INSERT INTO users (id, email, full_name, given_name, family_name, picture_url, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now(), now())
ON CONFLICT (id) DO UPDATE SET
  email = EXCLUDED.email,
  full_name = EXCLUDED.full_name,
  given_name = EXCLUDED.given_name,
  family_name = EXCLUDED.family_name,
  picture_url = EXCLUDED.picture_url,
  updated_at = now()`
	_, err := r.DB.ExecContext(ctx, query,
		user.ID,
		user.Email,
		nullableString(user.FullName),
		nullableString(user.GivenName),
		nullableString(user.FamilyName),
		nullableString(user.PictureURL),
	)
	return err
}

func (r *PGRepo) GetByID(ctx context.Context, userID string) (User, error) {
	const query = `
SELECT id, email, full_name, given_name, family_name, picture_url, created_at, updated_at
FROM users
WHERE id = $1
LIMIT 1`
	var user User
	var fullName sql.NullString
	var givenName sql.NullString
	var familyName sql.NullString
	var pictureURL sql.NullString
	var updatedAt sql.NullTime
	err := r.DB.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&fullName,
		&givenName,
		&familyName,
		&pictureURL,
		&user.CreatedAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	if fullName.Valid {
		user.FullName = fullName.String
	}
	if givenName.Valid {
		user.GivenName = givenName.String
	}
	if familyName.Valid {
		user.FamilyName = familyName.String
	}
	if pictureURL.Valid {
		user.PictureURL = pictureURL.String
	}
	if updatedAt.Valid {
		user.UpdatedAt = updatedAt.Time
	} else {
		user.UpdatedAt = time.Now().UTC()
	}
	return user, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
