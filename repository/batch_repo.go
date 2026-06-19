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

type BatchRepository struct {
	pool *pgxpool.Pool
}

func NewBatchRepository(pool *pgxpool.Pool) *BatchRepository {
	return &BatchRepository{pool: pool}
}

// batchBaseSelect joins courses and users to return names alongside IDs.
const batchBaseSelect = `
	SELECT b.id, b.short_id, b.batch_number,
	       b.course_id, c.name, c.short_id,
	       b.batch_manager_id,
	       CONCAT(bm.first_name, ' ', bm.last_name),
	       COALESCE(b.additional_manager_id::TEXT, ''),
	       COALESCE(CONCAT(am.first_name, ' ', am.last_name), ''),
	       COALESCE(b.module, ''),
	       b.start_date::TEXT, b.end_date::TEXT,
	       b.is_active, b.created_by, b.created_at, b.updated_at
	FROM batches b
	JOIN  courses c  ON b.course_id             = c.id  AND c.deleted_at  IS NULL
	JOIN  users   bm ON b.batch_manager_id       = bm.id AND bm.deleted_at IS NULL
	LEFT JOIN users am ON b.additional_manager_id = am.id AND am.deleted_at IS NULL`

// Create inserts a batch and returns the full record. Retries on short_id collision.
func (r *BatchRepository) Create(ctx context.Context, in models.CreateBatchInput, createdBy string) (*models.Batch, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		var b models.Batch
		err := r.pool.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO batches
				  (short_id, batch_number, course_id, batch_manager_id,
				   additional_manager_id, module, start_date, end_date, created_by)
				VALUES (
				  $1, $2,
				  (SELECT id FROM courses WHERE short_id = $3 AND deleted_at IS NULL),
				  $4::UUID,
				  NULLIF($5,'')::UUID,
				  NULLIF($6,''),
				  $7::DATE, $8::DATE, $9
				)
				RETURNING *
			)
			SELECT ins.id, ins.short_id, ins.batch_number,
			       ins.course_id, c.name, c.short_id,
			       ins.batch_manager_id,
			       CONCAT(bm.first_name, ' ', bm.last_name),
			       COALESCE(ins.additional_manager_id::TEXT, ''),
			       COALESCE(CONCAT(am.first_name, ' ', am.last_name), ''),
			       COALESCE(ins.module, ''),
			       ins.start_date::TEXT, ins.end_date::TEXT,
			       ins.is_active, ins.created_by, ins.created_at, ins.updated_at
			FROM ins
			JOIN  courses c  ON ins.course_id             = c.id
			JOIN  users   bm ON ins.batch_manager_id       = bm.id
			LEFT JOIN users am ON ins.additional_manager_id = am.id`,
			shortID, in.BatchNumber, in.CourseShortID,
			in.BatchManagerID, in.AdditionalManagerID, in.Module,
			in.StartDate, in.EndDate, createdBy,
		).Scan(
			&b.ID, &b.ShortID, &b.BatchNumber,
			&b.CourseID, &b.CourseName, &b.CourseShortID,
			&b.BatchManagerID, &b.BatchManagerName,
			&b.AdditionalManagerID, &b.AdditionalManagerName,
			&b.Module, &b.StartDate, &b.EndDate,
			&b.IsActive, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
		)
		if err == nil {
			return &b, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert batch: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted batches ordered newest first.
func (r *BatchRepository) FindAll(ctx context.Context) ([]models.Batch, error) {
	q := batchBaseSelect + ` WHERE b.deleted_at IS NULL ORDER BY b.created_at DESC`
	return r.scanBatches(ctx, q)
}

// FindByShortID returns a single non-deleted batch.
func (r *BatchRepository) FindByShortID(ctx context.Context, shortID string) (*models.Batch, error) {
	q := batchBaseSelect + ` WHERE b.short_id = $1 AND b.deleted_at IS NULL LIMIT 1`

	var b models.Batch
	err := r.pool.QueryRow(ctx, q, shortID).Scan(
		&b.ID, &b.ShortID, &b.BatchNumber,
		&b.CourseID, &b.CourseName, &b.CourseShortID,
		&b.BatchManagerID, &b.BatchManagerName,
		&b.AdditionalManagerID, &b.AdditionalManagerName,
		&b.Module, &b.StartDate, &b.EndDate,
		&b.IsActive, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// Filter returns non-deleted batches matching the provided filter.
func (r *BatchRepository) Filter(ctx context.Context, f models.BatchFilter) ([]models.Batch, error) {
	conditions := []string{"b.deleted_at IS NULL"}
	args := []interface{}{}
	i := 1

	if f.BatchNumber != "" {
		conditions = append(conditions, fmt.Sprintf("b.batch_number ILIKE $%d", i))
		args = append(args, "%"+f.BatchNumber+"%")
		i++
	}
	if f.CourseShortID != "" {
		conditions = append(conditions, fmt.Sprintf("c.short_id = $%d", i))
		args = append(args, f.CourseShortID)
		i++
	}
	if f.ManagerID != "" {
		conditions = append(conditions, fmt.Sprintf("b.batch_manager_id = $%d::UUID", i))
		args = append(args, f.ManagerID)
		i++
	}
	if f.Module != "" {
		conditions = append(conditions, fmt.Sprintf("b.module ILIKE $%d", i))
		args = append(args, "%"+f.Module+"%")
		i++
	}
	if f.StartDate != "" {
		conditions = append(conditions, fmt.Sprintf("b.start_date = $%d::DATE", i))
		args = append(args, f.StartDate)
		i++
	}
	if f.EndDate != "" {
		conditions = append(conditions, fmt.Sprintf("b.end_date = $%d::DATE", i))
		args = append(args, f.EndDate)
		i++
	}
	if f.IsActive == "true" {
		conditions = append(conditions, "b.is_active = TRUE")
	} else if f.IsActive == "false" {
		conditions = append(conditions, "b.is_active = FALSE")
	}

	q := batchBaseSelect + " WHERE " + strings.Join(conditions, " AND ") + " ORDER BY b.created_at DESC"
	return r.scanBatches(ctx, q, args...)
}

// Update applies a partial update — only non-nil fields are changed.
func (r *BatchRepository) Update(ctx context.Context, shortID string, in models.UpdateBatchInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	if in.BatchNumber != nil {
		setClauses = append(setClauses, fmt.Sprintf("batch_number = $%d", i))
		args = append(args, *in.BatchNumber)
		i++
	}
	if in.CourseShortID != nil {
		setClauses = append(setClauses, fmt.Sprintf(
			"course_id = (SELECT id FROM courses WHERE short_id = $%d AND deleted_at IS NULL)", i,
		))
		args = append(args, *in.CourseShortID)
		i++
	}
	if in.BatchManagerID != nil {
		setClauses = append(setClauses, fmt.Sprintf("batch_manager_id = $%d::UUID", i))
		args = append(args, *in.BatchManagerID)
		i++
	}
	if in.AdditionalManagerID != nil {
		setClauses = append(setClauses, fmt.Sprintf("additional_manager_id = NULLIF($%d::TEXT,'')::UUID", i))
		args = append(args, *in.AdditionalManagerID)
		i++
	}
	if in.Module != nil {
		setClauses = append(setClauses, fmt.Sprintf("module = NULLIF($%d,'')", i))
		args = append(args, *in.Module)
		i++
	}
	if in.StartDate != nil {
		setClauses = append(setClauses, fmt.Sprintf("start_date = $%d::DATE", i))
		args = append(args, *in.StartDate)
		i++
	}
	if in.EndDate != nil {
		setClauses = append(setClauses, fmt.Sprintf("end_date = $%d::DATE", i))
		args = append(args, *in.EndDate)
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
		"UPDATE batches SET %s WHERE short_id = $1 AND deleted_at IS NULL",
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

// Delete soft-deletes a batch.
func (r *BatchRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE batches SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *BatchRepository) scanBatches(ctx context.Context, q string, args ...interface{}) ([]models.Batch, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var batches []models.Batch
	for rows.Next() {
		var b models.Batch
		if err := rows.Scan(
			&b.ID, &b.ShortID, &b.BatchNumber,
			&b.CourseID, &b.CourseName, &b.CourseShortID,
			&b.BatchManagerID, &b.BatchManagerName,
			&b.AdditionalManagerID, &b.AdditionalManagerName,
			&b.Module, &b.StartDate, &b.EndDate,
			&b.IsActive, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, err
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}
