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

type FeedbackFormRepository struct {
	pool *pgxpool.Pool
}

func NewFeedbackFormRepository(pool *pgxpool.Pool) *FeedbackFormRepository {
	return &FeedbackFormRepository{pool: pool}
}

const feedbackFormCols = `
	id, short_id, title,
	COALESCE(description, ''),
	form_type, template,
	is_active,
	created_by::TEXT,
	created_at, updated_at`

func scanForm(row pgx.Row) (*models.FeedbackForm, error) {
	var f models.FeedbackForm
	err := row.Scan(
		&f.ID, &f.ShortID, &f.Title,
		&f.Description,
		&f.FormType, &f.Template,
		&f.IsActive,
		&f.CreatedBy,
		&f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// Create inserts a new feedback form, retrying up to 3 times on short_id collision.
// Title is auto-derived from the chosen template.
func (r *FeedbackFormRepository) Create(ctx context.Context, in models.CreateFeedbackFormInput, createdBy string) (*models.FeedbackForm, error) {
	title := models.TemplateTitles[in.Template]

	q := fmt.Sprintf(`
		INSERT INTO feedback_forms (short_id, title, form_type, template, created_by)
		VALUES ($1, $2, $3, $4, $5::UUID)
		RETURNING %s`, feedbackFormCols)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		f, err := scanForm(r.pool.QueryRow(ctx, q,
			shortID, title, in.FormType, in.Template, createdBy,
		))
		if err == nil {
			return f, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert feedback_form: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted feedback forms ordered newest first.
func (r *FeedbackFormRepository) FindAll(ctx context.Context) ([]models.FeedbackForm, error) {
	q := fmt.Sprintf(`SELECT %s FROM feedback_forms WHERE deleted_at IS NULL ORDER BY created_at DESC`, feedbackFormCols)
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var forms []models.FeedbackForm
	for rows.Next() {
		var f models.FeedbackForm
		if err := rows.Scan(
			&f.ID, &f.ShortID, &f.Title,
			&f.Description,
			&f.FormType, &f.Template,
			&f.IsActive,
			&f.CreatedBy,
			&f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, err
		}
		forms = append(forms, f)
	}
	return forms, rows.Err()
}

// FindFormByShortID returns a single non-deleted form (without questions).
func (r *FeedbackFormRepository) FindFormByShortID(ctx context.Context, shortID string) (*models.FeedbackForm, error) {
	q := fmt.Sprintf(`SELECT %s FROM feedback_forms WHERE short_id = $1 AND deleted_at IS NULL LIMIT 1`, feedbackFormCols)
	f, err := scanForm(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return f, err
}

// FindByShortID returns a form with all its questions.
func (r *FeedbackFormRepository) FindByShortID(ctx context.Context, shortID string) (*models.FeedbackFormWithQuestions, error) {
	f, err := r.FindFormByShortID(ctx, shortID)
	if err != nil || f == nil {
		return nil, err
	}
	questions, err := r.findQuestions(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	return &models.FeedbackFormWithQuestions{FeedbackForm: *f, Questions: questions}, nil
}

// Update applies a partial update — only non-nil fields are changed.
func (r *FeedbackFormRepository) Update(ctx context.Context, shortID string, in models.UpdateFeedbackFormInput) error {
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
	if in.IsActive != nil {
		add("is_active = $%d", *in.IsActive)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf(
		"UPDATE feedback_forms SET %s WHERE short_id = $1 AND deleted_at IS NULL",
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

// Delete soft-deletes a feedback form by its short_id.
func (r *FeedbackFormRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE feedback_forms SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ── Questions ─────────────────────────────────────────────────────────────────

const questionCols = `
	id, short_id, form_id::TEXT,
	question_type, question_text,
	is_required, order_index,
	scale_min, scale_max,
	start_label, end_label,
	options::TEXT,
	created_at, updated_at`

// jsonbArg marshals []string to a JSON string for use with ::jsonb in SQL.
// Returns nil (→ NULL) when the slice is empty.
func jsonbArg(vals []string) interface{} {
	if len(vals) == 0 {
		return nil
	}
	b, _ := json.Marshal(vals)
	return string(b)
}

func scanQuestion(row pgx.Row) (*models.FeedbackFormQuestion, error) {
	var q models.FeedbackFormQuestion
	var optsStr *string
	err := row.Scan(
		&q.ID, &q.ShortID, &q.FormID,
		&q.QuestionType, &q.QuestionText,
		&q.IsRequired, &q.OrderIndex,
		&q.ScaleMin, &q.ScaleMax,
		&q.StartLabel, &q.EndLabel,
		&optsStr,
		&q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if optsStr != nil {
		_ = json.Unmarshal([]byte(*optsStr), &q.Options)
	}
	return &q, nil
}

// AddQuestion inserts a new question into a form identified by its short_id.
func (r *FeedbackFormRepository) AddQuestion(ctx context.Context, formShortID string, in models.CreateFeedbackFormQuestionInput) (*models.FeedbackFormQuestion, error) {
	var formID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM feedback_forms WHERE short_id = $1 AND deleted_at IS NULL`, formShortID,
	).Scan(&formID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	q := fmt.Sprintf(`
		INSERT INTO feedback_form_questions
			(short_id, form_id, question_type, question_text, is_required, order_index,
			 scale_min, scale_max, start_label, end_label, options)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
		RETURNING %s`, questionCols)

	for attempt := 0; attempt < 3; attempt++ {
		sid := util.GenerateShortID()
		question, err := scanQuestion(r.pool.QueryRow(ctx, q,
			sid, formID, in.QuestionType, in.QuestionText, in.IsRequired, in.OrderIndex,
			in.ScaleMin, in.ScaleMax, in.StartLabel, in.EndLabel, jsonbArg(in.Options),
		))
		if err == nil {
			return question, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert feedback_form_question: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// UpdateQuestion partially updates a question and returns the updated row.
func (r *FeedbackFormRepository) UpdateQuestion(ctx context.Context, formShortID, questionShortID string, in models.UpdateFeedbackFormQuestionInput) (*models.FeedbackFormQuestion, error) {
	var formID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM feedback_forms WHERE short_id = $1 AND deleted_at IS NULL`, formShortID,
	).Scan(&formID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	args := []interface{}{questionShortID, formID}
	setClauses := []string{}
	i := 3

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.QuestionText != nil {
		add("question_text = $%d", *in.QuestionText)
	}
	if in.IsRequired != nil {
		add("is_required = $%d", *in.IsRequired)
	}
	if in.OrderIndex != nil {
		add("order_index = $%d", *in.OrderIndex)
	}
	if in.ScaleMin != nil {
		add("scale_min = $%d", *in.ScaleMin)
	}
	if in.ScaleMax != nil {
		add("scale_max = $%d", *in.ScaleMax)
	}
	if in.StartLabel != nil {
		add("start_label = NULLIF($%d,'')", *in.StartLabel)
	}
	if in.EndLabel != nil {
		add("end_label = NULLIF($%d,'')", *in.EndLabel)
	}
	// nil slice = field not sent (skip); non-nil empty slice = clear options (NULL)
	if in.Options != nil {
		add("options = $%d::jsonb", jsonbArg(in.Options))
	}

	if len(setClauses) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf(
		`UPDATE feedback_form_questions SET %s WHERE short_id = $1 AND form_id = $2 RETURNING %s`,
		strings.Join(setClauses, ", "),
		questionCols,
	)
	question, err := scanQuestion(r.pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return question, err
}

// DeleteQuestion removes a question from a form.
func (r *FeedbackFormRepository) DeleteQuestion(ctx context.Context, formShortID, questionShortID string) error {
	var formID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM feedback_forms WHERE short_id = $1 AND deleted_at IS NULL`, formShortID,
	).Scan(&formID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.ErrNoRows
	}
	if err != nil {
		return err
	}

	result, err := r.pool.Exec(ctx,
		`DELETE FROM feedback_form_questions WHERE short_id = $1 AND form_id = $2`,
		questionShortID, formID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// CreateResponse stores a student's feedback submission inside a transaction.
// It inserts one row into feedback_form_responses and one row per answer into
// feedback_form_answers, resolving question short_ids to UUIDs within the same tx.
func (r *FeedbackFormRepository) CreateResponse(ctx context.Context, formShortID string, userID string, in models.SubmitFeedbackFormInput) (*models.FeedbackFormResponse, error) {
	// Resolve form → UUID
	var formID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM feedback_forms WHERE short_id = $1 AND deleted_at IS NULL AND is_active = TRUE`, formShortID,
	).Scan(&formID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("fetch form: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Insert response header
	var resp models.FeedbackFormResponse
	shortID := util.GenerateShortID()
	err = tx.QueryRow(ctx,
		`INSERT INTO feedback_form_responses (short_id, form_id, submitted_by)
		 VALUES ($1, $2, $3::UUID)
		 RETURNING id, short_id, form_id::TEXT, submitted_by::TEXT, submitted_at`,
		shortID, formID, userID,
	).Scan(&resp.ID, &resp.ShortID, &resp.FormID, &resp.SubmittedBy, &resp.SubmittedAt)
	if err != nil {
		return nil, fmt.Errorf("insert response: %w", err)
	}

	// Insert each answer
	for _, a := range in.Answers {
		var questionID string
		err = tx.QueryRow(ctx,
			`SELECT id FROM feedback_form_questions WHERE short_id = $1 AND form_id = $2`, a.QuestionShortID, formID,
		).Scan(&questionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("question %s not found in this form", a.QuestionShortID)
		}
		if err != nil {
			return nil, fmt.Errorf("fetch question: %w", err)
		}

		var arrJSON interface{}
		if len(a.Array) > 0 {
			b, _ := json.Marshal(a.Array)
			arrJSON = string(b)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO feedback_form_answers (response_id, question_id, answer_text, answer_number, answer_array)
			 VALUES ($1, $2, $3, $4, $5::jsonb)`,
			resp.ID, questionID, a.Text, a.Number, arrJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("insert answer: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &resp, nil
}

func (r *FeedbackFormRepository) findQuestions(ctx context.Context, formID string) ([]models.FeedbackFormQuestion, error) {
	q := fmt.Sprintf(`
		SELECT %s FROM feedback_form_questions
		WHERE form_id = $1
		ORDER BY order_index ASC, created_at ASC`, questionCols)

	rows, err := r.pool.Query(ctx, q, formID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	questions := []models.FeedbackFormQuestion{}
	for rows.Next() {
		var ques models.FeedbackFormQuestion
		var optsStr *string
		if err := rows.Scan(
			&ques.ID, &ques.ShortID, &ques.FormID,
			&ques.QuestionType, &ques.QuestionText,
			&ques.IsRequired, &ques.OrderIndex,
			&ques.ScaleMin, &ques.ScaleMax,
			&ques.StartLabel, &ques.EndLabel,
			&optsStr,
			&ques.CreatedAt, &ques.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if optsStr != nil {
			_ = json.Unmarshal([]byte(*optsStr), &ques.Options)
		}
		questions = append(questions, ques)
	}
	return questions, rows.Err()
}
