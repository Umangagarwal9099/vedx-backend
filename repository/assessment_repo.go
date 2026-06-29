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

type AssessmentRepository struct {
	pool *pgxpool.Pool
}

func NewAssessmentRepository(pool *pgxpool.Pool) *AssessmentRepository {
	return &AssessmentRepository{pool: pool}
}

const assessmentSelectCols = `
	id, short_id, name,
	COALESCE(description, ''),
	COALESCE(thumbnail, ''),
	COALESCE(file_url, ''),
	COALESCE(general_instructions, ''),
	total_marks,
	passing_percentage::FLOAT8,
	result_declaration::TEXT,
	result_display::TEXT,
	allow_attempts_after_passing,
	is_active,
	created_by::TEXT,
	created_at, updated_at`

func scanAssessment(row pgx.Row) (*models.Assessment, error) {
	var a models.Assessment
	err := row.Scan(
		&a.ID, &a.ShortID, &a.Name,
		&a.Description, &a.Thumbnail,
		&a.FileURL,
		&a.GeneralInstructions,
		&a.TotalMarks,
		&a.PassingPercentage,
		&a.ResultDeclaration,
		&a.ResultDisplay,
		&a.AllowAttemptsAfterPassing,
		&a.IsActive,
		&a.CreatedBy,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AssessmentRepository) Create(ctx context.Context, in models.CreateAssessmentInput, createdBy string) (*models.Assessment, error) {
	q := fmt.Sprintf(`
		INSERT INTO assessments (
			short_id, name, description, thumbnail, file_url,
			general_instructions, total_marks, passing_percentage,
			result_declaration, result_display, allow_attempts_after_passing,
			created_by
		) VALUES (
			$1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''),
			NULLIF($6,''), $7, $8,
			$9::assessment_result_declaration, $10::assessment_result_display, $11,
			$12::UUID
		)
		RETURNING %s`, assessmentSelectCols)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		a, err := scanAssessment(r.pool.QueryRow(ctx, q,
			shortID, in.Name, in.Description, in.Thumbnail, in.FileURL,
			in.GeneralInstructions, in.TotalMarks, in.PassingPercentage,
			in.ResultDeclaration, in.ResultDisplay, in.AllowAttemptsAfterPassing,
			createdBy,
		))
		if err == nil {
			return a, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert assessment: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func (r *AssessmentRepository) FindAll(ctx context.Context) ([]models.Assessment, error) {
	q := fmt.Sprintf(`SELECT %s FROM assessments WHERE deleted_at IS NULL ORDER BY created_at DESC`, assessmentSelectCols)
	return r.scanAssessments(ctx, q)
}

func (r *AssessmentRepository) FindByShortID(ctx context.Context, shortID string) (*models.Assessment, error) {
	q := fmt.Sprintf(`SELECT %s FROM assessments WHERE short_id = $1 AND deleted_at IS NULL LIMIT 1`, assessmentSelectCols)
	a, err := scanAssessment(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (r *AssessmentRepository) Search(ctx context.Context, f models.AssessmentFilter) ([]models.Assessment, error) {
	args := []interface{}{}
	where := []string{"deleted_at IS NULL"}
	i := 1

	if f.Name != "" {
		where = append(where, fmt.Sprintf("name ILIKE $%d", i))
		args = append(args, "%"+f.Name+"%")
		i++
	}
	if f.Description != "" {
		where = append(where, fmt.Sprintf("description ILIKE $%d", i))
		args = append(args, "%"+f.Description+"%")
		i++
	}
	if f.IsActive == "true" {
		where = append(where, "is_active = TRUE")
	} else if f.IsActive == "false" {
		where = append(where, "is_active = FALSE")
	}

	q := fmt.Sprintf(
		`SELECT %s FROM assessments WHERE %s ORDER BY created_at DESC`,
		assessmentSelectCols, strings.Join(where, " AND "),
	)
	return r.scanAssessments(ctx, q, args...)
}

func (r *AssessmentRepository) Update(ctx context.Context, shortID string, in models.UpdateAssessmentInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.Name != nil {
		add("name = $%d", *in.Name)
	}
	if in.Description != nil {
		add("description = NULLIF($%d,'')", *in.Description)
	}
	if in.Thumbnail != nil {
		add("thumbnail = NULLIF($%d,'')", *in.Thumbnail)
	}
	if in.FileURL != nil {
		add("file_url = NULLIF($%d,'')", *in.FileURL)
	}
	if in.GeneralInstructions != nil {
		add("general_instructions = NULLIF($%d,'')", *in.GeneralInstructions)
	}
	if in.TotalMarks != nil {
		add("total_marks = $%d", *in.TotalMarks)
	}
	if in.PassingPercentage != nil {
		add("passing_percentage = $%d", *in.PassingPercentage)
	}
	if in.ResultDeclaration != nil {
		add("result_declaration = $%d::assessment_result_declaration", *in.ResultDeclaration)
	}
	if in.ResultDisplay != nil {
		add("result_display = $%d::assessment_result_display", *in.ResultDisplay)
	}
	if in.AllowAttemptsAfterPassing != nil {
		add("allow_attempts_after_passing = $%d", *in.AllowAttemptsAfterPassing)
	}
	if in.IsActive != nil {
		add("is_active = $%d", *in.IsActive)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf(
		"UPDATE assessments SET %s WHERE short_id = $1 AND deleted_at IS NULL",
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

func (r *AssessmentRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE assessments SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *AssessmentRepository) scanAssessments(ctx context.Context, q string, args ...interface{}) ([]models.Assessment, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assessments []models.Assessment
	for rows.Next() {
		var a models.Assessment
		if err := rows.Scan(
			&a.ID, &a.ShortID, &a.Name,
			&a.Description, &a.Thumbnail,
			&a.FileURL,
			&a.GeneralInstructions,
			&a.TotalMarks,
			&a.PassingPercentage,
			&a.ResultDeclaration,
			&a.ResultDisplay,
			&a.AllowAttemptsAfterPassing,
			&a.IsActive,
			&a.CreatedBy,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		assessments = append(assessments, a)
	}
	return assessments, rows.Err()
}
