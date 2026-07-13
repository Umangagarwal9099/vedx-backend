package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type ExamAttemptRepository struct {
	pool *pgxpool.Pool
}

func NewExamAttemptRepository(pool *pgxpool.Pool) *ExamAttemptRepository {
	return &ExamAttemptRepository{pool: pool}
}

const attemptBaseSelect = `
	SELECT ea.id, ea.short_id, a.short_id,
	       ea.student_id, CONCAT(u.first_name, ' ', u.last_name),
	       ea.attempt_number, ea.started_at, ea.ends_at, ea.submitted_at, ea.auto_submitted,
	       ea.status::TEXT, ea.total_score, ea.max_score, ea.passed
	FROM exam_attempts ea
	JOIN assessments a ON ea.assessment_id = a.id
	JOIN users       u ON ea.student_id    = u.id AND u.deleted_at IS NULL`

func scanAttempt(row pgx.Row) (models.ExamAttempt, error) {
	var a models.ExamAttempt
	err := row.Scan(
		&a.ID, &a.ShortID, &a.AssessmentShortID,
		&a.StudentID, &a.StudentName,
		&a.AttemptNumber, &a.StartedAt, &a.EndsAt, &a.SubmittedAt, &a.AutoSubmitted,
		&a.Status, &a.TotalScore, &a.MaxScore, &a.Passed,
	)
	return a, err
}

func (r *ExamAttemptRepository) FindInProgressAttempt(ctx context.Context, assessmentShortID, studentID string) (*models.ExamAttempt, error) {
	q := attemptBaseSelect + " WHERE a.short_id = $1 AND ea.student_id = $2 AND ea.status = 'in_progress' ORDER BY ea.started_at DESC LIMIT 1"
	a, err := scanAttempt(r.pool.QueryRow(ctx, q, assessmentShortID, studentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &a, err
}

func (r *ExamAttemptRepository) CreateAttempt(ctx context.Context, assessmentShortID, studentID string, attemptNumber, maxScore int, questionOrder []string, startedAt time.Time, endsAt *time.Time) (*models.ExamAttempt, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		a, err := scanAttempt(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO exam_attempts (
					short_id, assessment_id, student_id, attempt_number,
					started_at, ends_at, status, max_score, question_order
				)
				SELECT $1, a.id, $2, $3, $4, $5, 'in_progress', $6, $7
				FROM assessments a WHERE a.short_id = $8 AND a.deleted_at IS NULL
				RETURNING *
			)
			%s WHERE ea.id = (SELECT id FROM ins)`, attemptBaseSelect),
			shortID, studentID, attemptNumber, startedAt, endsAt, maxScore, questionOrder, assessmentShortID,
		))
		if err == nil {
			return &a, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert attempt: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func (r *ExamAttemptRepository) FindAttemptByShortID(ctx context.Context, attemptShortID string) (*models.ExamAttempt, error) {
	q := attemptBaseSelect + " WHERE ea.short_id = $1"
	a, err := scanAttempt(r.pool.QueryRow(ctx, q, attemptShortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &a, err
}

// GetQuestionOrder returns the stored (possibly randomized) question short_id
// order captured when the attempt was started.
func (r *ExamAttemptRepository) GetQuestionOrder(ctx context.Context, attemptShortID string) ([]string, error) {
	var order []string
	err := r.pool.QueryRow(ctx, `SELECT question_order FROM exam_attempts WHERE short_id = $1`, attemptShortID).Scan(&order)
	if err != nil {
		return nil, err
	}
	if order == nil {
		order = []string{}
	}
	return order, nil
}

func (r *ExamAttemptRepository) FindMyAttempts(ctx context.Context, assessmentShortID, studentID string) ([]models.ExamAttempt, error) {
	q := attemptBaseSelect + " WHERE a.short_id = $1 AND ea.student_id = $2 ORDER BY ea.attempt_number DESC"
	rows, err := r.pool.Query(ctx, q, assessmentShortID, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ExamAttempt
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *ExamAttemptRepository) FindAllAttempts(ctx context.Context, assessmentShortID string) ([]models.ExamAttempt, error) {
	q := attemptBaseSelect + " WHERE a.short_id = $1 ORDER BY ea.started_at DESC"
	rows, err := r.pool.Query(ctx, q, assessmentShortID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ExamAttempt
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FindExpiredInProgress returns every attempt still "in_progress" whose
// computed deadline has already passed — the set the auto-submit sweep acts on.
func (r *ExamAttemptRepository) FindExpiredInProgress(ctx context.Context) ([]models.ExamAttempt, error) {
	q := attemptBaseSelect + " WHERE ea.status = 'in_progress' AND ea.ends_at IS NOT NULL AND ea.ends_at < NOW()"
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ExamAttempt
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CancelAssessmentAttempts flips every in-progress attempt at an assessment to
// "cancelled" — used when an admin/mentor cancels the assessment itself.
func (r *ExamAttemptRepository) CancelAssessmentAttempts(ctx context.Context, assessmentShortID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE exam_attempts SET status = 'cancelled', updated_at = NOW()
		WHERE status = 'in_progress'
		  AND assessment_id = (SELECT id FROM assessments WHERE short_id = $1)`,
		assessmentShortID,
	)
	return err
}

// CountReattemptGrants returns how many extra attempts have been granted to a
// student for an assessment — added on top of the assessment's max_attempts.
func (r *ExamAttemptRepository) CountReattemptGrants(ctx context.Context, assessmentShortID, studentID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM exam_reattempt_grants g
		JOIN assessments a ON g.assessment_id = a.id
		WHERE a.short_id = $1 AND g.student_id = $2`,
		assessmentShortID, studentID,
	).Scan(&n)
	return n, err
}

// CreateReattemptGrant records a granted reattempt. Append-only — it never
// touches the student's past attempt rows.
func (r *ExamAttemptRepository) CreateReattemptGrant(ctx context.Context, assessmentShortID, studentID, grantedBy, reason string, newAttemptNumber int) (*models.ReattemptGrant, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		var g models.ReattemptGrant
		err := r.pool.QueryRow(ctx, `
			INSERT INTO exam_reattempt_grants (
				short_id, assessment_id, student_id, granted_by, reason, new_attempt_number
			)
			SELECT $1, a.id, $2, $3, NULLIF($4,''), $5
			FROM assessments a WHERE a.short_id = $6 AND a.deleted_at IS NULL
			RETURNING short_id, $6, student_id, granted_by, COALESCE(reason,''), new_attempt_number, created_at`,
			shortID, studentID, grantedBy, reason, newAttemptNumber, assessmentShortID,
		).Scan(&g.ShortID, &g.AssessmentShortID, &g.StudentID, &g.GrantedBy, &g.Reason, &g.NewAttemptNumber, &g.CreatedAt)
		if err == nil {
			return &g, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert reattempt grant: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func (r *ExamAttemptRepository) MarkSubmitted(ctx context.Context, attemptShortID string, autoSubmitted bool) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE exam_attempts SET status = 'submitted', submitted_at = NOW(), auto_submitted = $2, updated_at = NOW()
		WHERE short_id = $1 AND status = 'in_progress'`,
		attemptShortID, autoSubmitted,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *ExamAttemptRepository) FinalizeAttempt(ctx context.Context, attemptShortID string, totalScore int, passed bool) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE exam_attempts SET status = 'evaluated', total_score = $2, passed = $3, updated_at = NOW()
		WHERE short_id = $1`,
		attemptShortID, totalScore, passed,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ── Answers ──────────────────────────────────────────────────────────────────

const answerBaseSelect = `
	SELECT q.short_id, ea.selected_option_ids, COALESCE(ea.text_answer,''),
	       ea.is_correct, ea.marks_awarded, COALESCE(ea.feedback,'')
	FROM exam_answers ea
	JOIN assessment_questions q ON ea.question_id = q.id`

func scanAnswer(row pgx.Row) (models.StudentAnswer, error) {
	var a models.StudentAnswer
	err := row.Scan(&a.QuestionShortID, &a.SelectedOptionIDs, &a.TextAnswer, &a.IsCorrect, &a.MarksAwarded, &a.Feedback)
	if a.SelectedOptionIDs == nil {
		a.SelectedOptionIDs = []string{}
	}
	return a, err
}

// UpsertAnswer autosaves a student's answer for one question within an in-progress attempt.
func (r *ExamAttemptRepository) UpsertAnswer(ctx context.Context, attemptShortID, questionShortID string, selectedOptionIDs []string, textAnswer string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO exam_answers (attempt_id, question_id, selected_option_ids, text_answer)
		SELECT ea.id, q.id, $3, NULLIF($4,'')
		FROM exam_attempts ea, assessment_questions q
		WHERE ea.short_id = $1 AND ea.status = 'in_progress'
		  AND q.short_id = $2 AND q.deleted_at IS NULL
		ON CONFLICT (attempt_id, question_id) DO UPDATE SET
			selected_option_ids = EXCLUDED.selected_option_ids,
			text_answer = EXCLUDED.text_answer,
			updated_at = NOW()`,
		attemptShortID, questionShortID, selectedOptionIDs, textAnswer,
	)
	return err
}

func (r *ExamAttemptRepository) GetAnswers(ctx context.Context, attemptShortID string) ([]models.StudentAnswer, error) {
	q := answerBaseSelect + " WHERE ea.attempt_id = (SELECT id FROM exam_attempts WHERE short_id = $1)"
	rows, err := r.pool.Query(ctx, q, attemptShortID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.StudentAnswer
	for rows.Next() {
		a, err := scanAnswer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetAnswerGrade records the grade for one answer — used both by auto-grading
// at submit time and by a mentor's manual grade for descriptive/coding answers.
// Upserts so it works even for questions the student never answered at all.
func (r *ExamAttemptRepository) SetAnswerGrade(ctx context.Context, attemptShortID, questionShortID string, isCorrect *bool, marksAwarded int, feedback string) error {
	result, err := r.pool.Exec(ctx, `
		INSERT INTO exam_answers (attempt_id, question_id, is_correct, marks_awarded, feedback)
		SELECT ea.id, q.id, $3, $4, NULLIF($5,'')
		FROM exam_attempts ea, assessment_questions q
		WHERE ea.short_id = $1 AND q.short_id = $2
		ON CONFLICT (attempt_id, question_id) DO UPDATE SET
			is_correct = EXCLUDED.is_correct,
			marks_awarded = EXCLUDED.marks_awarded,
			feedback = EXCLUDED.feedback,
			updated_at = NOW()`,
		attemptShortID, questionShortID, isCorrect, marksAwarded, feedback,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// CountUngradedAnswers returns how many answers in the attempt still have no marks_awarded.
func (r *ExamAttemptRepository) CountUngradedAnswers(ctx context.Context, attemptShortID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM exam_answers
		WHERE attempt_id = (SELECT id FROM exam_attempts WHERE short_id = $1) AND marks_awarded IS NULL`,
		attemptShortID,
	).Scan(&n)
	return n, err
}

// SumMarks totals marks_awarded across every answer in the attempt (NULLs count as 0).
func (r *ExamAttemptRepository) SumMarks(ctx context.Context, attemptShortID string) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(COALESCE(marks_awarded, 0)), 0) FROM exam_answers
		WHERE attempt_id = (SELECT id FROM exam_attempts WHERE short_id = $1)`,
		attemptShortID,
	).Scan(&total)
	return total, err
}
