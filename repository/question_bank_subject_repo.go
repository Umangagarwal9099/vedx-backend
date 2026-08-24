package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

// ErrSubjectInUse is returned by Delete when questions still reference the subject.
var ErrSubjectInUse = errors.New("subject still has questions assigned to it")

// ErrSubjectNameTaken is returned by Create when a subject with that name already exists.
var ErrSubjectNameTaken = errors.New("a subject with this name already exists")

// QuestionBankSubjectRepository manages the DB-editable subject list that
// drives the question bank's browse tiles (see models.QuestionBankSubject).
type QuestionBankSubjectRepository struct {
	pool *pgxpool.Pool
}

func NewQuestionBankSubjectRepository(pool *pgxpool.Pool) *QuestionBankSubjectRepository {
	return &QuestionBankSubjectRepository{pool: pool}
}

// List returns every subject ordered for display (curated order first, then
// newest-added last).
func (r *QuestionBankSubjectRepository) List(ctx context.Context) ([]models.QuestionBankSubject, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, short_id, name, display_order, created_at
		FROM question_bank_subjects
		ORDER BY display_order, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subjects := []models.QuestionBankSubject{}
	for rows.Next() {
		var s models.QuestionBankSubject
		if err := rows.Scan(&s.ID, &s.ShortID, &s.Name, &s.DisplayOrder, &s.CreatedAt); err != nil {
			return nil, err
		}
		subjects = append(subjects, s)
	}
	return subjects, rows.Err()
}

// Create adds a new subject, appended after every existing one by display
// order. Returns a distinguishable error if the name is already taken so the
// controller can respond 409 instead of a generic 500.
func (r *QuestionBankSubjectRepository) Create(ctx context.Context, name, createdBy string) (*models.QuestionBankSubject, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		var s models.QuestionBankSubject
		err := r.pool.QueryRow(ctx, `
			INSERT INTO question_bank_subjects (short_id, name, display_order, created_by)
			VALUES ($1, $2, (SELECT COALESCE(MAX(display_order), 0) + 1 FROM question_bank_subjects), NULLIF($3,'')::UUID)
			RETURNING id, short_id, name, display_order, created_at`,
			shortID, name, createdBy,
		).Scan(&s.ID, &s.ShortID, &s.Name, &s.DisplayOrder, &s.CreatedAt)
		if err == nil {
			return &s, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "question_bank_subjects_name_key" {
				return nil, ErrSubjectNameTaken
			}
			continue // short_id collision — vanishingly rare, just retry with a fresh one
		}
		return nil, err
	}
	return nil, errors.New("could not generate a unique short_id after 3 attempts")
}

// Delete removes a subject. Refuses if any non-deleted question still uses
// this subject name, so deleting a tile can never silently orphan questions
// from the browse UI.
func (r *QuestionBankSubjectRepository) Delete(ctx context.Context, shortID string) error {
	var name string
	if err := r.pool.QueryRow(ctx,
		`SELECT name FROM question_bank_subjects WHERE short_id = $1`, shortID,
	).Scan(&name); err != nil {
		return err
	}

	var questionCount int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM assessment_questions WHERE subject = $1 AND deleted_at IS NULL`, name,
	).Scan(&questionCount); err != nil {
		return err
	}
	if questionCount > 0 {
		return ErrSubjectInUse
	}

	tag, err := r.pool.Exec(ctx, `DELETE FROM question_bank_subjects WHERE short_id = $1`, shortID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
