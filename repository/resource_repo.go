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

type ResourceRepository struct {
	pool *pgxpool.Pool
}

func NewResourceRepository(pool *pgxpool.Pool) *ResourceRepository {
	return &ResourceRepository{pool: pool}
}

// resourceBaseSelect left-joins batch/module/session since a resource may be
// globally visible (no batch) rather than scoped to one.
const resourceBaseSelect = `
	SELECT r.id, r.short_id, r.title, COALESCE(r.description,''),
	       r.url, r.resource_type::TEXT,
	       COALESCE(b.short_id, ''), COALESCE(b.batch_number, ''),
	       COALESCE(m.short_id, ''), COALESCE(m.module_name, ''),
	       COALESCE(s.short_id, ''), COALESCE(s.name, ''),
	       r.is_downloadable, r.expires_at,
	       r.created_by, CONCAT(u.first_name, ' ', u.last_name),
	       r.created_at, r.updated_at
	FROM resources r
	JOIN users u ON r.created_by = u.id AND u.deleted_at IS NULL
	LEFT JOIN batches  b ON r.batch_id   = b.id AND b.deleted_at IS NULL
	LEFT JOIN modules  m ON r.module_id  = m.id AND m.deleted_at IS NULL
	LEFT JOIN sessions s ON r.session_id = s.id AND s.deleted_at IS NULL`

func scanResource(row pgx.Row) (models.Resource, error) {
	var r models.Resource
	err := row.Scan(
		&r.ID, &r.ShortID, &r.Title, &r.Description,
		&r.URL, &r.ResourceType,
		&r.BatchShortID, &r.BatchNumber,
		&r.ModuleShortID, &r.ModuleName,
		&r.SessionShortID, &r.SessionName,
		&r.IsDownloadable, &r.ExpiresAt,
		&r.CreatedBy, &r.CreatedByName,
		&r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
}

func (r *ResourceRepository) Create(ctx context.Context, in models.CreateResourceInput, createdBy string) (*models.Resource, error) {
	// See the comment on assessment_repo.go's Create for why this reads FROM
	// ins rather than FROM resources.
	insSelect := strings.Replace(resourceBaseSelect, "FROM resources r", "FROM ins r", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		res, err := scanResource(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO resources (
					short_id, title, description, url, resource_type,
					batch_id, module_id, session_id, is_downloadable, expires_at, created_by
				) VALUES (
					$1, $2, NULLIF($3,''), $4, $5::resource_type,
					(SELECT id FROM batches  WHERE short_id = NULLIF($6,'') AND deleted_at IS NULL),
					(SELECT id FROM modules  WHERE short_id = NULLIF($7,'') AND deleted_at IS NULL),
					(SELECT id FROM sessions WHERE short_id = NULLIF($8,'') AND deleted_at IS NULL),
					$9, $10, $11
				)
				RETURNING *
			)
			%s`, insSelect),
			shortID, in.Title, in.Description, in.URL, in.ResourceType,
			in.BatchShortID, in.ModuleShortID, in.SessionShortID,
			in.IsDownloadable, in.ExpiresAt, createdBy,
		))
		if err == nil {
			return &res, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert resource: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func (r *ResourceRepository) scanAll(ctx context.Context, q string, args ...interface{}) ([]models.Resource, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Resource
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

// FindAll returns every non-deleted resource (staff view — unscoped).
func (r *ResourceRepository) FindAll(ctx context.Context, f models.ResourceFilter) ([]models.Resource, error) {
	where := []string{"r.deleted_at IS NULL"}
	args := []interface{}{}
	i := 1
	if f.BatchShortID != "" {
		where = append(where, fmt.Sprintf("b.short_id = $%d", i))
		args = append(args, f.BatchShortID)
		i++
	}
	if f.ResourceType != "" {
		where = append(where, fmt.Sprintf("r.resource_type = $%d::resource_type", i))
		args = append(args, f.ResourceType)
		i++
	}
	q := fmt.Sprintf("%s WHERE %s ORDER BY r.created_at DESC", resourceBaseSelect, strings.Join(where, " AND "))
	return r.scanAll(ctx, q, args...)
}

// FindAllForMentor returns global resources plus resources scoped to batches the mentor manages.
func (r *ResourceRepository) FindAllForMentor(ctx context.Context, mentorID string) ([]models.Resource, error) {
	q := fmt.Sprintf(`%s
		WHERE r.deleted_at IS NULL
		  AND (r.batch_id IS NULL OR b.batch_manager_id = $1 OR b.additional_manager_id = $1)
		ORDER BY r.created_at DESC`, resourceBaseSelect)
	return r.scanAll(ctx, q, mentorID)
}

// FindAllForStudent returns global resources plus resources scoped to batches the student is enrolled in.
// Expired resources are excluded.
func (r *ResourceRepository) FindAllForStudent(ctx context.Context, studentID string) ([]models.Resource, error) {
	q := fmt.Sprintf(`%s
		LEFT JOIN batch_students bs ON bs.batch_id = r.batch_id AND bs.user_id = $1
		WHERE r.deleted_at IS NULL
		  AND (r.batch_id IS NULL OR bs.user_id IS NOT NULL)
		  AND (r.expires_at IS NULL OR r.expires_at > NOW())
		ORDER BY r.created_at DESC`, resourceBaseSelect)
	return r.scanAll(ctx, q, studentID)
}

func (r *ResourceRepository) FindByShortID(ctx context.Context, shortID string) (*models.Resource, error) {
	q := fmt.Sprintf("%s WHERE r.short_id = $1 AND r.deleted_at IS NULL", resourceBaseSelect)
	res, err := scanResource(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &res, err
}

func (r *ResourceRepository) Update(ctx context.Context, shortID string, in models.UpdateResourceInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.Title != nil {
		add("title = $%d", *in.Title)
	}
	if in.Description != nil {
		add("description = NULLIF($%d,'')", *in.Description)
	}
	if in.URL != nil {
		add("url = $%d", *in.URL)
	}
	if in.ResourceType != nil {
		add("resource_type = $%d::resource_type", *in.ResourceType)
	}
	if in.BatchShortID != nil {
		add("batch_id = (SELECT id FROM batches WHERE short_id = NULLIF($%d,'') AND deleted_at IS NULL)", *in.BatchShortID)
	}
	if in.ModuleShortID != nil {
		add("module_id = (SELECT id FROM modules WHERE short_id = NULLIF($%d,'') AND deleted_at IS NULL)", *in.ModuleShortID)
	}
	if in.SessionShortID != nil {
		add("session_id = (SELECT id FROM sessions WHERE short_id = NULLIF($%d,'') AND deleted_at IS NULL)", *in.SessionShortID)
	}
	if in.IsDownloadable != nil {
		add("is_downloadable = $%d", *in.IsDownloadable)
	}
	if in.ExpiresAt != nil {
		add("expires_at = $%d", *in.ExpiresAt)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf("UPDATE resources SET %s WHERE short_id = $1 AND deleted_at IS NULL", strings.Join(setClauses, ", "))
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *ResourceRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE resources SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
