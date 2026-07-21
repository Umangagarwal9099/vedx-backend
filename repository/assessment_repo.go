package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

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

// assessmentBaseSelect deliberately does NOT include the newer security/
// result-publication columns (result_published_at, close_on_tab_switch,
// etc. — schema_updates_submission_flow_v1.sql). This query backs
// GetAll/GetByShortID/Create/Update, which are pre-existing, constantly-hit
// endpoints — if that migration hasn't been applied yet, they must keep
// working exactly as before. The new columns are read separately, via
// GetSecurityConfig, only by the new code paths that need them, with a
// graceful fallback if that migration is pending.
const assessmentBaseSelect = `
	SELECT a.id, a.short_id, a.name,
	       COALESCE(a.description, ''), COALESCE(a.thumbnail, ''), COALESCE(a.file_url, ''),
	       COALESCE(a.general_instructions, ''),
	       a.total_marks, a.passing_percentage::FLOAT8,
	       a.result_declaration::TEXT, a.result_display::TEXT,
	       a.allow_attempts_after_passing,
	       COALESCE(b.short_id, ''), COALESCE(b.batch_number, ''),
	       a.start_at, a.end_at, a.duration_minutes,
	       a.max_attempts, a.negative_marking, a.randomize_questions, a.randomize_options,
	       a.auto_submit, a.show_correct_answers, a.requires_proctoring,
	       (SELECT COUNT(*) FROM assessment_question_links aql WHERE aql.assessment_id = a.id),
	       a.is_active, a.cancelled_at, COALESCE(a.cancelled_by::TEXT, ''), a.created_by::TEXT,
	       a.created_at, a.updated_at
	FROM assessments a
	LEFT JOIN batches b ON a.batch_id = b.id AND b.deleted_at IS NULL`

func scanAssessment(row pgx.Row) (*models.Assessment, error) {
	var a models.Assessment
	err := row.Scan(
		&a.ID, &a.ShortID, &a.Name,
		&a.Description, &a.Thumbnail, &a.FileURL,
		&a.GeneralInstructions,
		&a.TotalMarks, &a.PassingPercentage,
		&a.ResultDeclaration, &a.ResultDisplay,
		&a.AllowAttemptsAfterPassing,
		&a.BatchShortID, &a.BatchNumber,
		&a.StartAt, &a.EndAt, &a.DurationMinutes,
		&a.MaxAttempts, &a.NegativeMarking, &a.RandomizeQuestions, &a.RandomizeOptions,
		&a.AutoSubmit, &a.ShowCorrectAnswers, &a.RequiresProctoring,
		&a.QuestionCount,
		&a.IsActive, &a.CancelledAt, &a.CancelledBy, &a.CreatedBy,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AssessmentSecurityConfig holds the exam-security/result-publication columns
// that live outside assessmentBaseSelect (see the comment above it).
type AssessmentSecurityConfig struct {
	ResultPublishedAt     *time.Time
	CloseOnTabSwitch      bool
	CloseOnWindowBlur     bool
	CloseOnFullscreenExit bool
	AllowedWarningCount   int
	AutoSubmitOnViolation bool
}

// GetSecurityConfig reads the exam-security/result-publication columns for an
// assessment via a query isolated from assessmentBaseSelect. Returns ok=false
// if schema_updates_submission_flow_v1.sql hasn't been applied yet (or any
// other error) — callers must treat that as "config unknown," not "config is
// all zero values," and fall back to pre-existing (ungated) behavior.
func (r *AssessmentRepository) GetSecurityConfig(ctx context.Context, shortID string) (AssessmentSecurityConfig, bool) {
	var cfg AssessmentSecurityConfig
	err := r.pool.QueryRow(ctx, `
		SELECT result_published_at, close_on_tab_switch, close_on_window_blur, close_on_fullscreen_exit,
		       allowed_warning_count, auto_submit_on_violation
		FROM assessments WHERE short_id = $1 AND deleted_at IS NULL`,
		shortID,
	).Scan(&cfg.ResultPublishedAt, &cfg.CloseOnTabSwitch, &cfg.CloseOnWindowBlur, &cfg.CloseOnFullscreenExit,
		&cfg.AllowedWarningCount, &cfg.AutoSubmitOnViolation)
	return cfg, err == nil
}

func (r *AssessmentRepository) Create(ctx context.Context, in models.CreateAssessmentInput, createdBy string) (*models.Assessment, error) {
	// The final SELECT reads FROM ins (not FROM assessments) because a
	// data-modifying CTE's effects are only visible via its own RETURNING —
	// a later part of the same statement re-querying the base table runs
	// against the pre-insert snapshot and would always find zero rows.
	insSelect := strings.Replace(assessmentBaseSelect, "FROM assessments a", "FROM ins a", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		// A data-modifying CTE and the main query share one snapshot, so a
		// plain re-scan of assessments (e.g. "WHERE a.id = (SELECT id FROM
		// ins)") can never see the row ins just inserted — Postgres only
		// guarantees visibility through the CTE's own RETURNING columns. So
		// the outer SELECT reads FROM ins directly instead of FROM
		// assessments a.
		a, err := scanAssessment(r.pool.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO assessments (
					short_id, name, description, thumbnail, file_url,
					general_instructions, total_marks, passing_percentage,
					result_declaration, result_display, allow_attempts_after_passing,
					batch_id, start_at, end_at, duration_minutes,
					max_attempts, negative_marking, randomize_questions, randomize_options,
					auto_submit, show_correct_answers, requires_proctoring,
					created_by
				) VALUES (
					$1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''),
					NULLIF($6,''), $7, $8,
					$9::assessment_result_declaration, $10::assessment_result_display, $11,
					(SELECT id FROM batches WHERE short_id = NULLIF($12,'') AND deleted_at IS NULL),
					$13, $14, $15,
					$16, $17, $18, $19,
					$20, $21, $22,
					$23::UUID
				)
				RETURNING *
			)
			SELECT ins.id, ins.short_id, ins.name,
			       COALESCE(ins.description, ''), COALESCE(ins.thumbnail, ''), COALESCE(ins.file_url, ''),
			       COALESCE(ins.general_instructions, ''),
			       ins.total_marks, ins.passing_percentage::FLOAT8,
			       ins.result_declaration::TEXT, ins.result_display::TEXT,
			       ins.allow_attempts_after_passing,
			       COALESCE(b.short_id, ''), COALESCE(b.batch_number, ''),
			       ins.start_at, ins.end_at, ins.duration_minutes,
			       ins.max_attempts, ins.negative_marking, ins.randomize_questions, ins.randomize_options,
			       ins.auto_submit, ins.show_correct_answers, ins.requires_proctoring,
			       0,
			       ins.is_active, ins.cancelled_at, COALESCE(ins.cancelled_by::TEXT, ''), ins.created_by::TEXT,
			       ins.created_at, ins.updated_at
			FROM ins
			LEFT JOIN batches b ON ins.batch_id = b.id AND b.deleted_at IS NULL`,
			shortID, in.Name, in.Description, in.Thumbnail, in.FileURL,
			in.GeneralInstructions, in.TotalMarks, in.PassingPercentage,
			in.ResultDeclaration, in.ResultDisplay, in.AllowAttemptsAfterPassing,
			in.BatchShortID, in.StartAt, in.EndAt, in.DurationMinutes,
			maxAttemptsOrDefault(in.MaxAttempts), in.NegativeMarking, in.RandomizeQuestions, in.RandomizeOptions,
			in.AutoSubmit, in.ShowCorrectAnswers, in.RequiresProctoring,
			createdBy,
		))
		if err == nil {
			// Best-effort — the exam-security columns are new
			// (schema_updates_submission_flow_v1.sql); if it's applied, set
			// whatever the caller passed. If not, silently skip rather than
			// fail the whole assessment creation over an optional feature.
			if _, secErr := r.pool.Exec(ctx, `
				UPDATE assessments SET close_on_tab_switch = $2, close_on_window_blur = $3,
				       close_on_fullscreen_exit = $4, allowed_warning_count = $5, auto_submit_on_violation = $6
				WHERE id = $1`,
				a.ID, in.CloseOnTabSwitch, in.CloseOnWindowBlur, in.CloseOnFullscreenExit,
				allowedWarningCountOrDefault(in.AllowedWarningCount), in.AutoSubmitOnViolation,
			); secErr != nil {
				log.Printf("set exam-security config for new assessment %s (migration pending?): %v", a.ShortID, secErr)
			}
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

func maxAttemptsOrDefault(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func allowedWarningCountOrDefault(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func (r *AssessmentRepository) scanAll(ctx context.Context, q string, args ...interface{}) ([]models.Assessment, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Assessment
	for rows.Next() {
		a, err := scanAssessment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *AssessmentRepository) FindAll(ctx context.Context) ([]models.Assessment, error) {
	q := fmt.Sprintf("%s WHERE a.deleted_at IS NULL ORDER BY a.created_at DESC", assessmentBaseSelect)
	return r.scanAll(ctx, q)
}

// FindAllForMentor returns global assessments plus assessments scoped to batches the mentor manages.
func (r *AssessmentRepository) FindAllForMentor(ctx context.Context, mentorID string) ([]models.Assessment, error) {
	q := fmt.Sprintf(`%s
		WHERE a.deleted_at IS NULL
		  AND (a.batch_id IS NULL OR b.batch_manager_id = $1 OR b.additional_manager_id = $1)
		ORDER BY a.created_at DESC`, assessmentBaseSelect)
	return r.scanAll(ctx, q, mentorID)
}

// FindAllForStudent returns active global assessments plus active assessments scoped to
// batches the student is enrolled in.
func (r *AssessmentRepository) FindAllForStudent(ctx context.Context, studentID string) ([]models.Assessment, error) {
	q := fmt.Sprintf(`%s
		LEFT JOIN batch_students bs ON bs.batch_id = a.batch_id AND bs.user_id = $1
		WHERE a.deleted_at IS NULL AND a.is_active = TRUE
		  AND (a.batch_id IS NULL OR bs.user_id IS NOT NULL)
		ORDER BY a.created_at DESC`, assessmentBaseSelect)
	return r.scanAll(ctx, q, studentID)
}

func (r *AssessmentRepository) FindByShortID(ctx context.Context, shortID string) (*models.Assessment, error) {
	q := fmt.Sprintf("%s WHERE a.short_id = $1 AND a.deleted_at IS NULL LIMIT 1", assessmentBaseSelect)
	a, err := scanAssessment(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (r *AssessmentRepository) Search(ctx context.Context, f models.AssessmentFilter) ([]models.Assessment, error) {
	args := []interface{}{}
	where := []string{"a.deleted_at IS NULL"}
	i := 1

	if f.Name != "" {
		where = append(where, fmt.Sprintf("a.name ILIKE $%d", i))
		args = append(args, "%"+f.Name+"%")
		i++
	}
	if f.Description != "" {
		where = append(where, fmt.Sprintf("a.description ILIKE $%d", i))
		args = append(args, "%"+f.Description+"%")
		i++
	}
	if f.IsActive == "true" {
		where = append(where, "a.is_active = TRUE")
	} else if f.IsActive == "false" {
		where = append(where, "a.is_active = FALSE")
	}
	if f.BatchShortID != "" {
		where = append(where, fmt.Sprintf("b.short_id = $%d", i))
		args = append(args, f.BatchShortID)
		i++
	}

	q := fmt.Sprintf("%s WHERE %s ORDER BY a.created_at DESC", assessmentBaseSelect, strings.Join(where, " AND "))
	return r.scanAll(ctx, q, args...)
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
	if in.BatchShortID != nil {
		add("batch_id = (SELECT id FROM batches WHERE short_id = NULLIF($%d,'') AND deleted_at IS NULL)", *in.BatchShortID)
	}
	if in.StartAt != nil {
		add("start_at = $%d", *in.StartAt)
	}
	if in.EndAt != nil {
		add("end_at = $%d", *in.EndAt)
	}
	if in.DurationMinutes != nil {
		add("duration_minutes = NULLIF($%d, 0)", *in.DurationMinutes)
	}
	if in.MaxAttempts != nil {
		add("max_attempts = $%d", *in.MaxAttempts)
	}
	if in.NegativeMarking != nil {
		add("negative_marking = $%d", *in.NegativeMarking)
	}
	if in.RandomizeQuestions != nil {
		add("randomize_questions = $%d", *in.RandomizeQuestions)
	}
	if in.RandomizeOptions != nil {
		add("randomize_options = $%d", *in.RandomizeOptions)
	}
	if in.AutoSubmit != nil {
		add("auto_submit = $%d", *in.AutoSubmit)
	}
	if in.ShowCorrectAnswers != nil {
		add("show_correct_answers = $%d", *in.ShowCorrectAnswers)
	}
	if in.RequiresProctoring != nil {
		add("requires_proctoring = $%d", *in.RequiresProctoring)
	}
	if in.IsActive != nil {
		add("is_active = $%d", *in.IsActive)
	}
	if in.CloseOnTabSwitch != nil {
		add("close_on_tab_switch = $%d", *in.CloseOnTabSwitch)
	}
	if in.CloseOnWindowBlur != nil {
		add("close_on_window_blur = $%d", *in.CloseOnWindowBlur)
	}
	if in.CloseOnFullscreenExit != nil {
		add("close_on_fullscreen_exit = $%d", *in.CloseOnFullscreenExit)
	}
	if in.AllowedWarningCount != nil {
		add("allowed_warning_count = $%d", *in.AllowedWarningCount)
	}
	if in.AutoSubmitOnViolation != nil {
		add("auto_submit_on_violation = $%d", *in.AutoSubmitOnViolation)
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

// Cancel marks an assessment as cancelled — it blocks new attempts (see
// ExamAttemptController.StartAttempt) but does not touch existing rows.
// Cascading in-progress attempts to "cancelled" is a separate step, done by
// ExamAttemptRepository.CancelAssessmentAttempts.
func (r *AssessmentRepository) Cancel(ctx context.Context, shortID, cancelledBy string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE assessments SET cancelled_at = NOW(), cancelled_by = $2::UUID, updated_at = NOW()
		WHERE short_id = $1 AND deleted_at IS NULL AND cancelled_at IS NULL`,
		shortID, cancelledBy,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// PublishResults marks every graded attempt's results visible at once —
// grading itself (FinalizeAttempt) never sets this; it's a deliberate,
// separate action so a mentor can grade over time and reveal to the whole
// class together.
func (r *AssessmentRepository) PublishResults(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE assessments SET result_published_at = NOW(), updated_at = NOW()
		WHERE short_id = $1 AND deleted_at IS NULL`,
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

// FindDueForStartReminder returns non-cancelled assessments starting within
// the next 15 minutes that haven't had a start reminder sent yet.
func (r *AssessmentRepository) FindDueForStartReminder(ctx context.Context) ([]models.Assessment, error) {
	q := fmt.Sprintf(`%s
		WHERE a.deleted_at IS NULL AND a.cancelled_at IS NULL
		  AND a.start_reminder_sent = FALSE
		  AND a.start_at IS NOT NULL
		  AND a.start_at > NOW() AND a.start_at <= NOW() + INTERVAL '15 minutes'`, assessmentBaseSelect)
	return r.scanAll(ctx, q)
}

// MarkStartReminderSent flags an assessment so its start reminder isn't sent twice.
func (r *AssessmentRepository) MarkStartReminderSent(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE assessments SET start_reminder_sent = TRUE WHERE id = $1`, id)
	return err
}

// FindIDByShortID resolves an assessment's internal UUID and a couple of
// fields needed by other repos (exam attempts) without pulling the full row.
func (r *AssessmentRepository) FindIDByShortID(ctx context.Context, shortID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id FROM assessments WHERE short_id = $1 AND deleted_at IS NULL`, shortID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}
