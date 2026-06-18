package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type CodingQuestionRepository struct {
	pool *pgxpool.Pool
}

func NewCodingQuestionRepository(pool *pgxpool.Pool) *CodingQuestionRepository {
	return &CodingQuestionRepository{pool: pool}
}

const selectCodingQuestion = `
	SELECT id, short_id, title, description, difficulty,
	       topics, languages, constraints,
	       examples, starter_code, test_cases,
	       is_active, created_by, created_at, updated_at
	FROM coding_questions`

func (r *CodingQuestionRepository) scanOne(row pgx.Row) (*models.CodingQuestion, error) {
	var q models.CodingQuestion
	var topicsArr, languagesArr, constraintsArr []string
	var examplesRaw, starterRaw, testCasesRaw []byte

	err := row.Scan(
		&q.ID, &q.ShortID, &q.Title, &q.Description, &q.Difficulty,
		&topicsArr, &languagesArr, &constraintsArr,
		&examplesRaw, &starterRaw, &testCasesRaw,
		&q.IsActive, &q.CreatedBy, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	q.Topics = topicsArr
	q.Languages = languagesArr
	q.Constraints = constraintsArr

	if err := json.Unmarshal(examplesRaw, &q.Examples); err != nil {
		q.Examples = []models.CodingExample{}
	}
	if err := json.Unmarshal(starterRaw, &q.StarterCode); err != nil {
		q.StarterCode = models.StarterCode{}
	}
	if err := json.Unmarshal(testCasesRaw, &q.TestCases); err != nil {
		q.TestCases = []models.CodingTestCase{}
	}

	if q.Topics == nil {
		q.Topics = []string{}
	}
	if q.Languages == nil {
		q.Languages = []string{}
	}
	if q.Constraints == nil {
		q.Constraints = []string{}
	}

	return &q, nil
}

func (r *CodingQuestionRepository) scanMany(ctx context.Context, q string, args ...interface{}) ([]models.CodingQuestion, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.CodingQuestion
	for rows.Next() {
		var cq models.CodingQuestion
		var topicsArr, languagesArr, constraintsArr []string
		var examplesRaw, starterRaw, testCasesRaw []byte

		if err := rows.Scan(
			&cq.ID, &cq.ShortID, &cq.Title, &cq.Description, &cq.Difficulty,
			&topicsArr, &languagesArr, &constraintsArr,
			&examplesRaw, &starterRaw, &testCasesRaw,
			&cq.IsActive, &cq.CreatedBy, &cq.CreatedAt, &cq.UpdatedAt,
		); err != nil {
			return nil, err
		}

		cq.Topics = topicsArr
		cq.Languages = languagesArr
		cq.Constraints = constraintsArr

		if err := json.Unmarshal(examplesRaw, &cq.Examples); err != nil {
			cq.Examples = []models.CodingExample{}
		}
		if err := json.Unmarshal(starterRaw, &cq.StarterCode); err != nil {
			cq.StarterCode = models.StarterCode{}
		}
		if err := json.Unmarshal(testCasesRaw, &cq.TestCases); err != nil {
			cq.TestCases = []models.CodingTestCase{}
		}

		if cq.Topics == nil {
			cq.Topics = []string{}
		}
		if cq.Languages == nil {
			cq.Languages = []string{}
		}
		if cq.Constraints == nil {
			cq.Constraints = []string{}
		}

		results = append(results, cq)
	}
	return results, rows.Err()
}

// Create inserts a new coding question, retrying up to 3 times on short_id collision.
func (r *CodingQuestionRepository) Create(ctx context.Context, in models.CreateCodingQuestionInput, createdBy string) (*models.CodingQuestion, error) {
	const q = `
		INSERT INTO coding_questions
		  (short_id, title, description, difficulty, topics, languages, constraints,
		   examples, starter_code, test_cases, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING ` + selectFields

	topics := in.Topics
	if topics == nil {
		topics = []string{}
	}
	constraints := in.Constraints
	if constraints == nil {
		constraints = []string{}
	}
	examples := in.Examples
	if examples == nil {
		examples = []models.CodingExample{}
	}

	examplesJSON, _ := json.Marshal(examples)
	starterJSON, _ := json.Marshal(in.StarterCode)
	testCasesJSON, _ := json.Marshal(in.TestCases)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		row := r.pool.QueryRow(ctx, q,
			shortID, in.Title, in.Description, in.Difficulty,
			topics, in.Languages, constraints,
			examplesJSON, starterJSON, testCasesJSON,
			createdBy,
		)
		result, err := r.scanOne(row)
		if err == nil {
			return result, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert coding question: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted active coding questions.
func (r *CodingQuestionRepository) FindAll(ctx context.Context) ([]models.CodingQuestion, error) {
	q := selectCodingQuestion + `
		WHERE deleted_at IS NULL AND is_active = TRUE
		ORDER BY created_at ASC`
	return r.scanMany(ctx, q)
}

// FindAllAdmin returns all non-deleted questions (active + inactive) for admin.
func (r *CodingQuestionRepository) FindAllAdmin(ctx context.Context) ([]models.CodingQuestion, error) {
	q := selectCodingQuestion + `
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC`
	return r.scanMany(ctx, q)
}

// FindByShortID returns a single non-deleted question.
func (r *CodingQuestionRepository) FindByShortID(ctx context.Context, shortID string) (*models.CodingQuestion, error) {
	q := selectCodingQuestion + `
		WHERE short_id = $1 AND deleted_at IS NULL
		LIMIT 1`
	result, err := r.scanOne(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return result, err
}

// Update applies a partial update.
func (r *CodingQuestionRepository) Update(ctx context.Context, shortID string, in models.UpdateCodingQuestionInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	if in.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", i))
		args = append(args, *in.Title)
		i++
	}
	if in.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", i))
		args = append(args, *in.Description)
		i++
	}
	if in.Difficulty != nil {
		setClauses = append(setClauses, fmt.Sprintf("difficulty = $%d", i))
		args = append(args, *in.Difficulty)
		i++
	}
	if in.Topics != nil {
		setClauses = append(setClauses, fmt.Sprintf("topics = $%d", i))
		args = append(args, in.Topics)
		i++
	}
	if in.Languages != nil {
		setClauses = append(setClauses, fmt.Sprintf("languages = $%d", i))
		args = append(args, in.Languages)
		i++
	}
	if in.Constraints != nil {
		setClauses = append(setClauses, fmt.Sprintf("constraints = $%d", i))
		args = append(args, in.Constraints)
		i++
	}
	if in.Examples != nil {
		b, _ := json.Marshal(in.Examples)
		setClauses = append(setClauses, fmt.Sprintf("examples = $%d", i))
		args = append(args, b)
		i++
	}
	if in.StarterCode != nil {
		b, _ := json.Marshal(*in.StarterCode)
		setClauses = append(setClauses, fmt.Sprintf("starter_code = $%d", i))
		args = append(args, b)
		i++
	}
	if in.TestCases != nil {
		b, _ := json.Marshal(in.TestCases)
		setClauses = append(setClauses, fmt.Sprintf("test_cases = $%d", i))
		args = append(args, b)
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

	query := fmt.Sprintf(
		"UPDATE coding_questions SET %s WHERE short_id = $1 AND deleted_at IS NULL",
		strings.Join(setClauses, ", "),
	)
	result, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Delete soft-deletes a coding question.
func (r *CodingQuestionRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE coding_questions SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`,
		shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

const selectFields = `id, short_id, title, description, difficulty,
	topics, languages, constraints,
	examples, starter_code, test_cases,
	is_active, created_by, created_at, updated_at`
