package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type AssignmentRepository struct {
	pool *pgxpool.Pool
}

func NewAssignmentRepository(pool *pgxpool.Pool) *AssignmentRepository {
	return &AssignmentRepository{pool: pool}
}

// assignmentBaseSelect joins in batch/module/session names and submission counts.
const assignmentBaseSelect = `
	SELECT a.id, a.short_id, a.title, COALESCE(a.description,''),
	       a.batch_id, b.short_id, b.batch_number,
	       COALESCE(m.short_id, ''), COALESCE(m.module_name, ''),
	       COALESCE(s.short_id, ''), COALESCE(s.name, ''),
	       a.max_marks, a.deadline,
	       a.allowed_submission_types, COALESCE(a.allowed_file_formats, '{}'),
	       COALESCE(a.max_file_size_mb, 0),
	       a.late_submission_allowed, COALESCE(a.late_penalty_percent, 0),
	       a.status::TEXT, a.created_by, CONCAT(u.first_name, ' ', u.last_name),
	       (SELECT COUNT(*) FROM assignment_submissions asub WHERE asub.assignment_id = a.id),
	       (SELECT COUNT(*) FROM assignment_submissions asub WHERE asub.assignment_id = a.id AND asub.status IN ('submitted', 'late')),
	       a.created_at, a.updated_at
	FROM assignments a
	JOIN batches b ON a.batch_id   = b.id
	JOIN users   u ON a.created_by = u.id AND u.deleted_at IS NULL
	LEFT JOIN modules  m ON a.module_id  = m.id AND m.deleted_at IS NULL
	LEFT JOIN sessions s ON a.session_id = s.id AND s.deleted_at IS NULL`

func scanAssignment(row pgx.Row) (models.Assignment, error) {
	var a models.Assignment
	err := row.Scan(
		&a.ID, &a.ShortID, &a.Title, &a.Description,
		&a.BatchID, &a.BatchShortID, &a.BatchNumber,
		&a.ModuleShortID, &a.ModuleName,
		&a.SessionShortID, &a.SessionName,
		&a.MaxMarks, &a.Deadline,
		&a.AllowedSubmissionTypes, &a.AllowedFileFormats,
		&a.MaxFileSizeMB,
		&a.LateSubmissionAllowed, &a.LatePenaltyPercent,
		&a.Status, &a.CreatedBy, &a.CreatedByName,
		&a.SubmissionCount, &a.PendingReviewCount,
		&a.CreatedAt, &a.UpdatedAt,
	)
	return a, err
}

// Create inserts an assignment and returns the full enriched record.
func (r *AssignmentRepository) Create(ctx context.Context, in models.CreateAssignmentInput, createdBy string) (*models.Assignment, error) {
	status := in.Status
	if status == "" {
		status = "draft"
	}

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		// A data-modifying CTE and the main query share one snapshot, so a
		// plain re-scan of assignments (e.g. "WHERE a.id = (SELECT id FROM
		// ins)") can never see the row ins just inserted — Postgres only
		// guarantees visibility through the CTE's own RETURNING columns. So
		// the outer SELECT reads FROM ins directly instead of FROM
		// assignments a. A brand-new assignment has zero submissions, so the
		// submission-count subqueries collapse to literal 0s.
		a, err := scanAssignment(r.pool.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO assignments (
					short_id, title, description, batch_id, module_id, session_id,
					max_marks, deadline, allowed_submission_types, allowed_file_formats,
					max_file_size_mb, late_submission_allowed, late_penalty_percent,
					status, created_by
				) VALUES (
					$1, $2, NULLIF($3,''),
					(SELECT id FROM batches WHERE short_id = $4 AND deleted_at IS NULL),
					(SELECT id FROM modules WHERE short_id = NULLIF($5,'') AND deleted_at IS NULL),
					(SELECT id FROM sessions WHERE short_id = NULLIF($6,'') AND deleted_at IS NULL),
					$7, $8, $9, NULLIF($10, '{}'::text[]),
					NULLIF($11, 0), $12, NULLIF($13, 0),
					$14::assignment_status, $15
				)
				RETURNING *
			)
			SELECT ins.id, ins.short_id, ins.title, COALESCE(ins.description,''),
			       ins.batch_id, b.short_id, b.batch_number,
			       COALESCE(m.short_id, ''), COALESCE(m.module_name, ''),
			       COALESCE(s.short_id, ''), COALESCE(s.name, ''),
			       ins.max_marks, ins.deadline,
			       ins.allowed_submission_types, COALESCE(ins.allowed_file_formats, '{}'),
			       COALESCE(ins.max_file_size_mb, 0),
			       ins.late_submission_allowed, COALESCE(ins.late_penalty_percent, 0),
			       ins.status::TEXT, ins.created_by, CONCAT(u.first_name, ' ', u.last_name),
			       0, 0,
			       ins.created_at, ins.updated_at
			FROM ins
			JOIN batches b ON ins.batch_id   = b.id
			JOIN users   u ON ins.created_by = u.id AND u.deleted_at IS NULL
			LEFT JOIN modules  m ON ins.module_id  = m.id AND m.deleted_at IS NULL
			LEFT JOIN sessions s ON ins.session_id = s.id AND s.deleted_at IS NULL`,
			shortID, in.Title, in.Description, in.BatchShortID, in.ModuleShortID, in.SessionShortID,
			in.MaxMarks, in.Deadline, in.AllowedSubmissionTypes, in.AllowedFileFormats,
			in.MaxFileSizeMB, in.LateSubmissionAllowed, in.LatePenaltyPercent,
			status, createdBy,
		))
		if err == nil {
			return &a, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
				continue
			}
			if pgErr.Code == "23502" {
				return nil, fmt.Errorf("invalid batch_short_id: batch not found")
			}
		}
		return nil, fmt.Errorf("insert assignment: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func (r *AssignmentRepository) scanAll(ctx context.Context, q string, args ...interface{}) ([]models.Assignment, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Assignment
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FindAll returns every non-deleted assignment (staff view — unscoped).
func (r *AssignmentRepository) FindAll(ctx context.Context, f models.AssignmentFilter) ([]models.Assignment, error) {
	where := []string{"a.deleted_at IS NULL"}
	args := []interface{}{}
	i := 1
	if f.BatchShortID != "" {
		where = append(where, fmt.Sprintf("b.short_id = $%d", i))
		args = append(args, f.BatchShortID)
		i++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("a.status = $%d::assignment_status", i))
		args = append(args, f.Status)
		i++
	}
	q := fmt.Sprintf("%s WHERE %s ORDER BY a.created_at DESC", assignmentBaseSelect, strings.Join(where, " AND "))
	return r.scanAll(ctx, q, args...)
}

// FindAllForMentor returns assignments for batches the given mentor manages.
func (r *AssignmentRepository) FindAllForMentor(ctx context.Context, mentorID string) ([]models.Assignment, error) {
	q := fmt.Sprintf(`%s
		WHERE a.deleted_at IS NULL AND (b.batch_manager_id = $1 OR b.additional_manager_id = $1)
		ORDER BY a.created_at DESC`, assignmentBaseSelect)
	return r.scanAll(ctx, q, mentorID)
}

// FindAllForStudent returns published assignments for batches the student is enrolled in.
func (r *AssignmentRepository) FindAllForStudent(ctx context.Context, studentID string) ([]models.Assignment, error) {
	q := fmt.Sprintf(`%s
		JOIN batch_students bs ON bs.batch_id = a.batch_id AND bs.user_id = $1
		WHERE a.deleted_at IS NULL AND a.status = 'active'
		ORDER BY a.deadline ASC`, assignmentBaseSelect)
	return r.scanAll(ctx, q, studentID)
}

// FindDueForDeadlineReminder returns active assignments whose deadline falls
// within the next 24 hours and haven't been reminded about yet.
func (r *AssignmentRepository) FindDueForDeadlineReminder(ctx context.Context) ([]models.Assignment, error) {
	q := fmt.Sprintf(`%s
		WHERE a.deleted_at IS NULL AND a.status = 'active' AND a.deadline_reminder_sent = FALSE
		  AND a.deadline BETWEEN NOW() AND NOW() + INTERVAL '24 hours'
		ORDER BY a.deadline ASC`, assignmentBaseSelect)
	return r.scanAll(ctx, q)
}

// MarkDeadlineReminderSent flags an assignment so its deadline reminder fires once.
func (r *AssignmentRepository) MarkDeadlineReminderSent(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE assignments SET deadline_reminder_sent = TRUE WHERE id = $1::UUID`, id)
	return err
}

func (r *AssignmentRepository) FindByShortID(ctx context.Context, shortID string) (*models.Assignment, error) {
	q := fmt.Sprintf("%s WHERE a.short_id = $1 AND a.deleted_at IS NULL", assignmentBaseSelect)
	a, err := scanAssignment(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &a, err
}

func (r *AssignmentRepository) Update(ctx context.Context, shortID string, in models.UpdateAssignmentInput) error {
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
	if in.BatchShortID != nil {
		add("batch_id = (SELECT id FROM batches WHERE short_id = $%d AND deleted_at IS NULL)", *in.BatchShortID)
	}
	if in.ModuleShortID != nil {
		add("module_id = (SELECT id FROM modules WHERE short_id = NULLIF($%d,'') AND deleted_at IS NULL)", *in.ModuleShortID)
	}
	if in.SessionShortID != nil {
		add("session_id = (SELECT id FROM sessions WHERE short_id = NULLIF($%d,'') AND deleted_at IS NULL)", *in.SessionShortID)
	}
	if in.MaxMarks != nil {
		add("max_marks = $%d", *in.MaxMarks)
	}
	if in.Deadline != nil {
		add("deadline = $%d", *in.Deadline)
	}
	if in.AllowedSubmissionTypes != nil {
		add("allowed_submission_types = $%d", in.AllowedSubmissionTypes)
	}
	if in.AllowedFileFormats != nil {
		add("allowed_file_formats = $%d", in.AllowedFileFormats)
	}
	if in.MaxFileSizeMB != nil {
		add("max_file_size_mb = NULLIF($%d, 0)", *in.MaxFileSizeMB)
	}
	if in.LateSubmissionAllowed != nil {
		add("late_submission_allowed = $%d", *in.LateSubmissionAllowed)
	}
	if in.LatePenaltyPercent != nil {
		add("late_penalty_percent = NULLIF($%d, 0)", *in.LatePenaltyPercent)
	}
	if in.Status != nil {
		add("status = $%d::assignment_status", *in.Status)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf("UPDATE assignments SET %s WHERE short_id = $1 AND deleted_at IS NULL", strings.Join(setClauses, ", "))
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *AssignmentRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE assignments SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ── Submissions ──────────────────────────────────────────────────────────────

// submissionBaseSelect deliberately does NOT include result_published_at — a
// new column (schema_updates_submission_flow_v1.sql). This backs
// CreateOrResubmit/FindMySubmission/FindAllSubmissions, pre-existing,
// constantly-hit paths that must keep working even if that migration hasn't
// landed yet. Publish-state is read separately (GetResultPublishedAt), only
// where results are actually gated, with a graceful fallback.
const submissionBaseSelect = `
	SELECT asub.id, asub.short_id, a.short_id,
	       asub.student_id, CONCAT(u.first_name, ' ', u.last_name), u.email,
	       asub.submission_type::TEXT, COALESCE(asub.content, ''), COALESCE(asub.file_url, ''),
	       asub.status::TEXT, asub.marks, COALESCE(asub.feedback, ''),
	       asub.submitted_at, asub.evaluated_at, COALESCE(asub.evaluated_by::TEXT, ''),
	       asub.created_at, asub.updated_at
	FROM assignment_submissions asub
	JOIN assignments a ON asub.assignment_id = a.id
	JOIN users       u ON asub.student_id    = u.id AND u.deleted_at IS NULL`

func scanSubmission(row pgx.Row) (models.AssignmentSubmission, error) {
	var s models.AssignmentSubmission
	err := row.Scan(
		&s.ID, &s.ShortID, &s.AssignmentShortID,
		&s.StudentID, &s.StudentName, &s.StudentEmail,
		&s.SubmissionType, &s.Content, &s.FileURL,
		&s.Status, &s.Marks, &s.Feedback,
		&s.SubmittedAt, &s.EvaluatedAt, &s.EvaluatedBy,
		&s.CreatedAt, &s.UpdatedAt,
	)
	return s, err
}

// StudentHasAccess reports whether a student may view or submit to an
// assignment: they must be enrolled in the assignment's batch and the
// assignment must be published (status = 'active'). Mirrors
// ProjectRepository.StudentHasAccess.
func (r *AssignmentRepository) StudentHasAccess(ctx context.Context, assignmentShortID, studentID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM assignments a
			JOIN batch_students bs ON bs.batch_id = a.batch_id AND bs.user_id = $2
			WHERE a.short_id = $1 AND a.deleted_at IS NULL AND a.status = 'active'
		)`, assignmentShortID, studentID).Scan(&exists)
	return exists, err
}

// CreateOrResubmit inserts a student's submission for an assignment, or overwrites
// an existing one that's in "resubmission_required" state. Returns an error if a
// submission already exists in any other state (call this "already submitted"),
// or if there's no submission yet and the deadline has passed with late
// submissions disabled (a mentor-approved resubmission always overrides the
// deadline — that's the whole point of granting one).
func (r *AssignmentRepository) CreateOrResubmit(ctx context.Context, assignmentShortID, studentID string, in models.CreateAssignmentSubmissionInput) (*models.AssignmentSubmission, error) {
	var existingStatus string
	err := r.pool.QueryRow(ctx, `
		SELECT asub.status::TEXT FROM assignment_submissions asub
		JOIN assignments a ON asub.assignment_id = a.id
		WHERE a.short_id = $1 AND asub.student_id = $2`,
		assignmentShortID, studentID,
	).Scan(&existingStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check existing submission: %w", err)
	}
	if err == nil && existingStatus != "resubmission_required" {
		return nil, fmt.Errorf("already submitted")
	}
	if existingStatus != "resubmission_required" {
		var deadline time.Time
		var lateAllowed bool
		if err := r.pool.QueryRow(ctx, `SELECT deadline, late_submission_allowed FROM assignments WHERE short_id = $1 AND deleted_at IS NULL`, assignmentShortID).Scan(&deadline, &lateAllowed); err != nil {
			return nil, fmt.Errorf("check assignment deadline: %w", err)
		}
		if !lateAllowed && time.Now().After(deadline) {
			return nil, fmt.Errorf("the submission deadline has passed and late submissions are not allowed")
		}
	}

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		// A data-modifying CTE and the main query share one snapshot, so a
		// plain re-scan of assignment_submissions (e.g. "WHERE asub.id =
		// (SELECT id FROM ins)") can never see the row ins just
		// inserted/updated — Postgres only guarantees visibility through the
		// CTE's own RETURNING columns. So the outer SELECT reads FROM ins
		// directly instead of FROM assignment_submissions asub.
		s, err := scanSubmission(r.pool.QueryRow(ctx, `
			WITH target AS (
				SELECT id, deadline FROM assignments WHERE short_id = $6 AND deleted_at IS NULL
			), ins AS (
				INSERT INTO assignment_submissions (
					short_id, assignment_id, student_id, submission_type, content, file_url,
					status, submitted_at, marks, feedback, evaluated_at, evaluated_by
				)
				SELECT $1, target.id, $2, $3::assignment_submission_type, NULLIF($4,''), NULLIF($5,''),
				       (CASE WHEN NOW() > target.deadline THEN 'late' ELSE 'submitted' END)::assignment_submission_status,
				       NOW(), NULL, NULL, NULL, NULL
				FROM target
				ON CONFLICT (assignment_id, student_id) DO UPDATE SET
					submission_type = EXCLUDED.submission_type,
					content = EXCLUDED.content,
					file_url = EXCLUDED.file_url,
					status = EXCLUDED.status,
					submitted_at = NOW(),
					marks = NULL,
					feedback = NULL,
					evaluated_at = NULL,
					evaluated_by = NULL,
					updated_at = NOW()
				RETURNING *
			)
			SELECT ins.id, ins.short_id, a.short_id,
			       ins.student_id, CONCAT(u.first_name, ' ', u.last_name), u.email,
			       ins.submission_type::TEXT, COALESCE(ins.content, ''), COALESCE(ins.file_url, ''),
			       ins.status::TEXT, ins.marks, COALESCE(ins.feedback, ''),
			       ins.submitted_at, ins.evaluated_at, COALESCE(ins.evaluated_by::TEXT, ''),
			       ins.created_at, ins.updated_at
			FROM ins
			JOIN assignments a ON ins.assignment_id = a.id
			JOIN users       u ON ins.student_id    = u.id AND u.deleted_at IS NULL`,
			shortID, studentID, in.SubmissionType, in.Content, in.FileURL, assignmentShortID,
		))
		if err == nil {
			return &s, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert submission: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindMySubmission returns the current user's submission for an assignment, if any.
func (r *AssignmentRepository) FindMySubmission(ctx context.Context, assignmentShortID, studentID string) (*models.AssignmentSubmission, error) {
	q := fmt.Sprintf("%s WHERE a.short_id = $1 AND asub.student_id = $2", submissionBaseSelect)
	s, err := scanSubmission(r.pool.QueryRow(ctx, q, assignmentShortID, studentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}

// FindAllSubmissions returns every submission for an assignment (mentor grading view).
func (r *AssignmentRepository) FindAllSubmissions(ctx context.Context, assignmentShortID string) ([]models.AssignmentSubmission, error) {
	q := fmt.Sprintf("%s WHERE a.short_id = $1 ORDER BY asub.submitted_at DESC", submissionBaseSelect)
	rows, err := r.pool.Query(ctx, q, assignmentShortID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AssignmentSubmission
	for rows.Next() {
		s, err := scanSubmission(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FindAllSubmissionsForMentor returns every assignment submission across
// every assignment — or, when mentorID is non-empty, only those in batches
// that mentor manages — newest first. This is the cross-assignment feed
// behind the unified Submissions workspace.
func (r *AssignmentRepository) FindAllSubmissionsForMentor(ctx context.Context, mentorID string) ([]models.AssignmentSubmission, error) {
	q := `
		SELECT asub.id, asub.short_id, a.short_id, a.title, b.short_id, b.batch_number,
		       asub.student_id, CONCAT(u.first_name, ' ', u.last_name), u.email,
		       asub.submission_type::TEXT, COALESCE(asub.content, ''), COALESCE(asub.file_url, ''),
		       asub.status::TEXT, asub.marks, COALESCE(asub.feedback, ''),
		       asub.submitted_at, asub.evaluated_at, COALESCE(asub.evaluated_by::TEXT, ''),
		       asub.created_at, asub.updated_at
		FROM assignment_submissions asub
		JOIN assignments a ON asub.assignment_id = a.id
		JOIN batches     b ON a.batch_id         = b.id
		JOIN users       u ON asub.student_id    = u.id AND u.deleted_at IS NULL`
	args := []interface{}{}
	if mentorID != "" {
		q += ` WHERE b.batch_manager_id = $1 OR b.additional_manager_id = $1`
		args = append(args, mentorID)
	}
	q += ` ORDER BY asub.submitted_at DESC LIMIT 500`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AssignmentSubmission
	for rows.Next() {
		var s models.AssignmentSubmission
		if err := rows.Scan(
			&s.ID, &s.ShortID, &s.AssignmentShortID, &s.AssignmentTitle, &s.BatchShortID, &s.BatchNumber,
			&s.StudentID, &s.StudentName, &s.StudentEmail,
			&s.SubmissionType, &s.Content, &s.FileURL,
			&s.Status, &s.Marks, &s.Feedback,
			&s.SubmittedAt, &s.EvaluatedAt, &s.EvaluatedBy,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Grade records a mentor's evaluation of a submission.
func (r *AssignmentRepository) Grade(ctx context.Context, assignmentShortID, submissionShortID, evaluatedBy string, in models.GradeSubmissionInput) error {
	status := in.Status
	if status == "" {
		status = "evaluated"
	}
	result, err := r.pool.Exec(ctx, `
		UPDATE assignment_submissions SET
			marks = $1, feedback = NULLIF($2,''), status = $3::assignment_submission_status,
			evaluated_at = NOW(), evaluated_by = $4, updated_at = NOW()
		WHERE short_id = $5
		  AND assignment_id = (SELECT id FROM assignments WHERE short_id = $6 AND deleted_at IS NULL)`,
		in.Marks, in.Feedback, status, evaluatedBy, submissionShortID, assignmentShortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetResultPublishedAt reads a submission's result_published_at via an
// isolated query (see submissionBaseSelect's comment). ok=false means the
// migration hasn't been applied yet — callers should treat that as "not
// gated" (fail open to pre-existing behavior), not "unpublished."
func (r *AssignmentRepository) GetResultPublishedAt(ctx context.Context, submissionShortID string) (publishedAt *time.Time, ok bool) {
	err := r.pool.QueryRow(ctx, `SELECT result_published_at FROM assignment_submissions WHERE short_id = $1`, submissionShortID).Scan(&publishedAt)
	return publishedAt, err == nil
}

// PublishResults makes every graded submission for this assignment visible
// to students at once — grading (Grade) never sets this itself.
func (r *AssignmentRepository) PublishResults(ctx context.Context, assignmentShortID string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE assignment_submissions SET result_published_at = NOW(), updated_at = NOW()
		WHERE assignment_id = (SELECT id FROM assignments WHERE short_id = $1 AND deleted_at IS NULL)
		  AND status = 'evaluated' AND result_published_at IS NULL`,
		assignmentShortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
