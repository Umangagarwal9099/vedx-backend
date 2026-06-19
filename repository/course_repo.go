package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type CourseRepository struct {
	pool *pgxpool.Pool
}

func NewCourseRepository(pool *pgxpool.Pool) *CourseRepository {
	return &CourseRepository{pool: pool}
}

// Create inserts a new course, retrying up to 3 times on short_id collision.
func (r *CourseRepository) Create(ctx context.Context, in models.CreateCourseInput, createdBy string) (*models.Course, error) {
	const q = `
		INSERT INTO courses (short_id, name, description, thumbnail, created_by)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5)
		RETURNING id, short_id, name, COALESCE(description,''), COALESCE(thumbnail,''),
		          is_active, created_by, created_at, updated_at`

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		var c models.Course
		err := r.pool.QueryRow(ctx, q, shortID, in.Name, in.Description, in.Thumbnail, createdBy).Scan(
			&c.ID, &c.ShortID, &c.Name, &c.Description, &c.Thumbnail,
			&c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		)
		if err == nil {
			return &c, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue // short_id collision — retry
		}
		return nil, fmt.Errorf("insert course: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted courses ordered newest first.
func (r *CourseRepository) FindAll(ctx context.Context) ([]models.Course, error) {
	const q = `
		SELECT id, short_id, name, COALESCE(description,''), COALESCE(thumbnail,''),
		       is_active, created_by, created_at, updated_at
		FROM courses
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC`
	return r.scanCourses(ctx, q)
}

// FindByShortID returns a single non-deleted course by its short_id.
func (r *CourseRepository) FindByShortID(ctx context.Context, shortID string) (*models.Course, error) {
	const q = `
		SELECT id, short_id, name, COALESCE(description,''), COALESCE(thumbnail,''),
		       is_active, created_by, created_at, updated_at
		FROM courses
		WHERE short_id = $1 AND deleted_at IS NULL
		LIMIT 1`

	var c models.Course
	err := r.pool.QueryRow(ctx, q, shortID).Scan(
		&c.ID, &c.ShortID, &c.Name, &c.Description, &c.Thumbnail,
		&c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Search returns non-deleted courses whose name or description matches the query.
func (r *CourseRepository) Search(ctx context.Context, query string) ([]models.Course, error) {
	const q = `
		SELECT id, short_id, name, COALESCE(description,''), COALESCE(thumbnail,''),
		       is_active, created_by, created_at, updated_at
		FROM courses
		WHERE deleted_at IS NULL
		  AND (name ILIKE $1 OR description ILIKE $1)
		ORDER BY created_at DESC`
	return r.scanCourses(ctx, q, "%"+query+"%")
}

// Update applies a partial update — only non-nil fields are changed.
func (r *CourseRepository) Update(ctx context.Context, shortID string, in models.UpdateCourseInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	if in.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", i))
		args = append(args, *in.Name)
		i++
	}
	if in.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = NULLIF($%d,'')", i))
		args = append(args, *in.Description)
		i++
	}
	if in.Thumbnail != nil {
		setClauses = append(setClauses, fmt.Sprintf("thumbnail = NULLIF($%d,'')", i))
		args = append(args, *in.Thumbnail)
		i++
	}
	if in.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", i))
		args = append(args, *in.IsActive)
		i++
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}

	q := fmt.Sprintf(
		"UPDATE courses SET %s WHERE short_id = $1 AND deleted_at IS NULL",
		strings.Join(setClauses, ", "),
	)
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Delete soft-deletes a course by its short_id.
func (r *CourseRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE courses SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *CourseRepository) scanCourses(ctx context.Context, q string, args ...interface{}) ([]models.Course, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var courses []models.Course
	for rows.Next() {
		var c models.Course
		if err := rows.Scan(
			&c.ID, &c.ShortID, &c.Name, &c.Description, &c.Thumbnail,
			&c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}
