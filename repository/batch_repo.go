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

type BatchRepository struct {
	pool *pgxpool.Pool
}

func NewBatchRepository(pool *pgxpool.Pool) *BatchRepository {
	return &BatchRepository{pool: pool}
}

// batchBaseSelect joins courses and users to return names alongside IDs.
const batchBaseSelect = `
	SELECT b.id, b.short_id,
	       COALESCE(b.college_id::TEXT, ''), COALESCE(col.short_id, ''),
	       b.batch_number,
	       b.course_id, c.name, c.short_id,
	       b.batch_manager_id,
	       CONCAT(bm.first_name, ' ', bm.last_name),
	       COALESCE(b.additional_manager_id::TEXT, ''),
	       COALESCE(CONCAT(am.first_name, ' ', am.last_name), ''),
	       COALESCE(b.module, ''),
	       b.start_date::TEXT, b.end_date::TEXT,
	       b.is_active, b.status::TEXT, b.max_students,
	       (SELECT COUNT(*) FROM batch_students bs WHERE bs.batch_id = b.id),
	       b.score_weight_assignments, b.score_weight_exams, b.score_weight_projects,
	       b.created_by, b.created_at, b.updated_at
	FROM batches b
	JOIN  courses c  ON b.course_id             = c.id  AND c.deleted_at  IS NULL
	JOIN  users   bm ON b.batch_manager_id       = bm.id AND bm.deleted_at IS NULL
	LEFT JOIN users am ON b.additional_manager_id = am.id AND am.deleted_at IS NULL
	LEFT JOIN colleges col ON b.college_id = col.id`

// Create inserts a batch and returns the full record. Retries on short_id
// collision. collegeID must never be empty — resolved by the caller (see
// controller.resolveTargetCollege) so no newly created batch is ever left
// with a NULL college_id.
func (r *BatchRepository) Create(ctx context.Context, in models.CreateBatchInput, createdBy, collegeID string) (*models.Batch, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		var b models.Batch
		err := r.pool.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO batches
				  (short_id, college_id, batch_number, course_id, batch_manager_id,
				   additional_manager_id, module, start_date, end_date, status, max_students, created_by)
				VALUES (
				  $1, $2::UUID, $3,
				  (SELECT id FROM courses WHERE short_id = $4 AND deleted_at IS NULL),
				  $5::UUID,
				  NULLIF($6,'')::UUID,
				  NULLIF($7,''),
				  $8::DATE, $9::DATE,
				  COALESCE(NULLIF($10,'')::batch_status, 'draft'), $11, $12
				)
				RETURNING *
			)
			SELECT ins.id, ins.short_id,
			       COALESCE(ins.college_id::TEXT, ''), COALESCE(col.short_id, ''),
			       ins.batch_number,
			       ins.course_id, c.name, c.short_id,
			       ins.batch_manager_id,
			       CONCAT(bm.first_name, ' ', bm.last_name),
			       COALESCE(ins.additional_manager_id::TEXT, ''),
			       COALESCE(CONCAT(am.first_name, ' ', am.last_name), ''),
			       COALESCE(ins.module, ''),
			       ins.start_date::TEXT, ins.end_date::TEXT,
			       ins.is_active, ins.status::TEXT, ins.max_students, 0,
			       ins.score_weight_assignments, ins.score_weight_exams, ins.score_weight_projects,
			       ins.created_by, ins.created_at, ins.updated_at
			FROM ins
			JOIN  courses c  ON ins.course_id             = c.id
			JOIN  users   bm ON ins.batch_manager_id       = bm.id
			LEFT JOIN users am ON ins.additional_manager_id = am.id
			LEFT JOIN colleges col ON ins.college_id = col.id`,
			shortID, collegeID, in.BatchNumber, in.CourseShortID,
			in.BatchManagerID, in.AdditionalManagerID, in.Module,
			in.StartDate, in.EndDate, in.Status, in.MaxStudents, createdBy,
		).Scan(
			&b.ID, &b.ShortID,
			&b.CollegeID, &b.CollegeShortID,
			&b.BatchNumber,
			&b.CourseID, &b.CourseName, &b.CourseShortID,
			&b.BatchManagerID, &b.BatchManagerName,
			&b.AdditionalManagerID, &b.AdditionalManagerName,
			&b.Module, &b.StartDate, &b.EndDate,
			&b.IsActive, &b.Status, &b.MaxStudents, &b.StudentCount,
			&b.ScoreWeightAssignments, &b.ScoreWeightExams, &b.ScoreWeightProjects,
			&b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
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

// FindAll returns all non-deleted batches ordered newest first. collegeID
// scopes the list for non-super-admin callers (empty = unscoped) — see
// repository.CollegeFilter.
func (r *BatchRepository) FindAll(ctx context.Context, collegeID string) ([]models.Batch, error) {
	q := batchBaseSelect + ` WHERE b.deleted_at IS NULL`
	args := []interface{}{}
	if collegeID != "" {
		q += ` AND b.college_id = $1::UUID`
		args = append(args, collegeID)
	}
	q += ` ORDER BY b.created_at DESC`
	return r.scanBatches(ctx, q, args...)
}

// FindAllForMentor returns non-deleted batches the given mentor manages
// (batch_manager_id or additional_manager_id) — the row-level-scoped view
// used for mentor/employee callers instead of FindAll. collegeID scopes
// further for non-super-admin callers (empty = unscoped); in practice a
// mentor's own college_id should already match every batch they manage, but
// this is defense-in-depth against that ever drifting.
func (r *BatchRepository) FindAllForMentor(ctx context.Context, mentorID, collegeID string) ([]models.Batch, error) {
	q := batchBaseSelect + `
		WHERE b.deleted_at IS NULL AND (b.batch_manager_id = $1 OR b.additional_manager_id = $1)`
	args := []interface{}{mentorID}
	if collegeID != "" {
		q += ` AND b.college_id = $2::UUID`
		args = append(args, collegeID)
	}
	q += ` ORDER BY b.created_at DESC`
	return r.scanBatches(ctx, q, args...)
}

// FindByShortID returns a single non-deleted batch.
//
// NOTE: deliberately NOT college-scoped, per an explicit scope decision —
// list-level scoping (FindAll/FindAllForMentor) is this pass's target;
// detail/update/delete ownership hardening across every batch-detail call
// site (certificates, curriculum, attendance, analytics, audit log, score,
// authz_helpers — 20 call sites as of this writing) is a distinct, larger
// follow-up phase.
func (r *BatchRepository) FindByShortID(ctx context.Context, shortID string) (*models.Batch, error) {
	q := batchBaseSelect + ` WHERE b.short_id = $1 AND b.deleted_at IS NULL LIMIT 1`

	var b models.Batch
	err := r.pool.QueryRow(ctx, q, shortID).Scan(
		&b.ID, &b.ShortID,
		&b.CollegeID, &b.CollegeShortID,
		&b.BatchNumber,
		&b.CourseID, &b.CourseName, &b.CourseShortID,
		&b.BatchManagerID, &b.BatchManagerName,
		&b.AdditionalManagerID, &b.AdditionalManagerName,
		&b.Module, &b.StartDate, &b.EndDate,
		&b.IsActive, &b.Status, &b.MaxStudents, &b.StudentCount,
		&b.ScoreWeightAssignments, &b.ScoreWeightExams, &b.ScoreWeightProjects,
		&b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
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
	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("b.status = $%d::batch_status", i))
		args = append(args, f.Status)
		i++
	}

	q := batchBaseSelect + " WHERE " + strings.Join(conditions, " AND ") + " ORDER BY b.created_at DESC"
	return r.scanBatches(ctx, q, args...)
}

// FindByStudentID returns every non-deleted batch userID is enrolled in as a
// student, most recently joined first.
func (r *BatchRepository) FindByStudentID(ctx context.Context, userID string) ([]models.Batch, error) {
	q := batchBaseSelect + `
		JOIN batch_students bs ON bs.batch_id = b.id AND bs.user_id = $1::UUID
		WHERE b.deleted_at IS NULL
		ORDER BY bs.joined_at DESC`
	return r.scanBatches(ctx, q, userID)
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
	if in.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d::batch_status", i))
		args = append(args, *in.Status)
		i++
	}
	if in.MaxStudents != nil {
		setClauses = append(setClauses, fmt.Sprintf("max_students = $%d", i))
		args = append(args, *in.MaxStudents)
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

// UpdateScoreWeights sets how much each category (assignments/exams/projects)
// contributes to this batch's final score / leaderboard ranking.
func (r *BatchRepository) UpdateScoreWeights(ctx context.Context, shortID string, in models.UpdateScoreWeightsInput) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE batches SET
			score_weight_assignments = $2, score_weight_exams = $3, score_weight_projects = $4
		WHERE short_id = $1 AND deleted_at IS NULL`,
		shortID, in.AssignmentsWeight, in.ExamsWeight, in.ProjectsWeight,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// AddStudents bulk-enrolls students into a batch by user ID. Only users with
// role='student' AND the same college_id as the batch are matched — a
// student from another college is silently excluded from the "added" list,
// the same way an already-enrolled or non-student ID already is; students
// already enrolled are left unchanged. Returns the IDs of students actually
// added.
func (r *BatchRepository) AddStudents(ctx context.Context, batchShortID string, studentIDs []string, addedBy string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		INSERT INTO batch_students (batch_id, user_id, added_by)
		SELECT b.id, u.id, $3
		FROM batches b
		CROSS JOIN unnest($2::uuid[]) AS uid(user_id)
		JOIN users u ON u.id = uid.user_id AND u.deleted_at IS NULL AND u.role = 'student' AND u.college_id = b.college_id
		WHERE b.short_id = $1 AND b.deleted_at IS NULL
		ON CONFLICT (batch_id, user_id) DO NOTHING
		RETURNING user_id`,
		batchShortID, studentIDs, addedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("add students: %w", err)
	}
	defer rows.Close()

	var added []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		added = append(added, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// See the identical comment in AddStudentsWithEnrollment — a student's
	// status badge should reflect that they're actually enrolled somewhere.
	if len(added) > 0 {
		if _, err := r.pool.Exec(ctx, `UPDATE students SET status = 'enrolled' WHERE user_id = ANY($1::uuid[]) AND status = 'registered'`, added); err != nil {
			return nil, fmt.Errorf("update student status on enrollment: %w", err)
		}
	}
	return added, nil
}

// AddStudentsWithEnrollment bulk-enrolls students into a batch AND creates
// their student_enrollments rows in a single transaction — batch_students and
// student_enrollments are two hand-synced tables, and doing these as two
// separate statements (as this used to work) meant a mid-way failure could
// leave a student on the roster but invisible to enrollment/scoring/at-risk
// queries, silently. Returns the IDs of students actually added to the roster.
func (r *BatchRepository) AddStudentsWithEnrollment(
	ctx context.Context,
	batchShortID string,
	studentIDs []string,
	addedBy string,
	courseID, batchID string,
	isLateEnrollment bool,
	accessStartDate *time.Time,
	accessEndDate *time.Time,
) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Only same-college students are matched — a cross-college student ID is
	// silently excluded from "added," identical to AddStudents.
	rows, err := tx.Query(ctx, `
		INSERT INTO batch_students (batch_id, user_id, added_by)
		SELECT b.id, u.id, $3
		FROM batches b
		CROSS JOIN unnest($2::uuid[]) AS uid(user_id)
		JOIN users u ON u.id = uid.user_id AND u.deleted_at IS NULL AND u.role = 'student' AND u.college_id = b.college_id
		WHERE b.short_id = $1 AND b.deleted_at IS NULL
		ON CONFLICT (batch_id, user_id) DO NOTHING
		RETURNING user_id`,
		batchShortID, studentIDs, addedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("add students: %w", err)
	}
	var added []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		added = append(added, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	// A student's status badge should reflect that they're actually enrolled
	// somewhere, not sit on "Registered" forever until an admin manually
	// changes it — this only ever moves registered -> enrolled, never
	// touches completed/on_leave/archived, which stay under explicit control.
	if len(added) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE students SET status = 'enrolled' WHERE user_id = ANY($1::uuid[]) AND status = 'registered'`, added); err != nil {
			return nil, fmt.Errorf("update student status on enrollment: %w", err)
		}
	}

	for _, studentID := range added {
		enrolled := false
		for attempt := 0; attempt < 3 && !enrolled; attempt++ {
			shortID := util.GenerateShortID()
			_, err := tx.Exec(ctx, `
				INSERT INTO student_enrollments (
					short_id, student_id, course_id, batch_id, enrollment_type, status,
					access_start_date, access_end_date, is_late_enrollment, created_by
				)
				VALUES ($1, $2::UUID, $3::UUID, $4::UUID, 'direct', 'active', $5, $6, $7, $8::UUID)
				ON CONFLICT (student_id, batch_id) DO UPDATE SET
					status = 'active', access_start_date = EXCLUDED.access_start_date,
					access_end_date = EXCLUDED.access_end_date, is_late_enrollment = EXCLUDED.is_late_enrollment,
					updated_at = NOW()`,
				shortID, studentID, courseID, batchID, accessStartDate, accessEndDate, isLateEnrollment, addedBy,
			)
			if err == nil {
				enrolled = true
				continue
			}
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				continue
			}
			return nil, fmt.Errorf("insert enrollment for %s: %w", studentID, err)
		}
		if !enrolled {
			return nil, fmt.Errorf("could not generate a unique short ID for %s after 3 attempts", studentID)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return added, nil
}

// RemoveStudent removes a single student from a batch.
func (r *BatchRepository) RemoveStudent(ctx context.Context, batchShortID, userID string) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM batch_students
		WHERE batch_id = (SELECT id FROM batches WHERE short_id = $1 AND deleted_at IS NULL)
		  AND user_id = $2::uuid`,
		batchShortID, userID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetStudents returns every student enrolled in a batch, newest enrollment first.
func (r *BatchRepository) GetStudents(ctx context.Context, batchShortID string) ([]models.BatchStudent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.first_name, u.last_name, u.email, bs.fees_paid, bs.joined_at
		FROM batch_students bs
		JOIN users u ON u.id = bs.user_id AND u.deleted_at IS NULL
		WHERE bs.batch_id = (SELECT id FROM batches WHERE short_id = $1 AND deleted_at IS NULL)
		ORDER BY bs.joined_at DESC`,
		batchShortID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var students []models.BatchStudent
	for rows.Next() {
		var s models.BatchStudent
		if err := rows.Scan(&s.UserID, &s.FirstName, &s.LastName, &s.Email, &s.FeesPaid, &s.JoinedAt); err != nil {
			return nil, err
		}
		students = append(students, s)
	}
	return students, rows.Err()
}

// SetFeesPaid updates a single student's fee-payment status for a batch —
// used to grant or revoke access to that batch's session recordings.
func (r *BatchRepository) SetFeesPaid(ctx context.Context, batchShortID, userID string, feesPaid bool) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE batch_students bs
		SET fees_paid = $3
		FROM batches b
		WHERE bs.batch_id = b.id
		  AND b.short_id = $1 AND b.deleted_at IS NULL
		  AND bs.user_id = $2::uuid`,
		batchShortID, userID, feesPaid,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// IsFeesPaid reports whether userID is marked as fully paid for the batch
// identified by its internal UUID (not short_id — callers already have this
// from session.BatchID). Returns false, not an error, if the user isn't
// enrolled in the batch at all.
func (r *BatchRepository) IsFeesPaid(ctx context.Context, batchID, userID string) (bool, error) {
	var paid bool
	err := r.pool.QueryRow(ctx,
		`SELECT fees_paid FROM batch_students WHERE batch_id = $1::uuid AND user_id = $2::uuid`,
		batchID, userID,
	).Scan(&paid)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return paid, err
}

// IsFeesPaidByBatchShortID reports whether userID is marked as fully paid for
// the batch identified by its short_id — for callers (e.g. student-facing
// endpoints) that only have the short_id on hand, not the internal batch UUID.
// Returns false, not an error, if the user isn't enrolled in the batch at all.
func (r *BatchRepository) IsFeesPaidByBatchShortID(ctx context.Context, batchShortID, userID string) (bool, error) {
	var paid bool
	err := r.pool.QueryRow(ctx, `
		SELECT bs.fees_paid
		FROM batch_students bs
		JOIN batches b ON b.id = bs.batch_id
		WHERE b.short_id = $1 AND b.deleted_at IS NULL AND bs.user_id = $2::uuid`,
		batchShortID, userID,
	).Scan(&paid)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return paid, err
}

// IsStudentEnrolled reports whether userID is a member (any fee status) of
// the batch identified by its short_id — used to let an enrolled student
// view batch-wide read-only content (e.g. the leaderboard) that's otherwise
// staff-only.
func (r *BatchRepository) IsStudentEnrolled(ctx context.Context, batchShortID, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM batch_students bs
			JOIN batches b ON b.id = bs.batch_id
			WHERE b.short_id = $1 AND b.deleted_at IS NULL AND bs.user_id = $2::uuid
		)`,
		batchShortID, userID,
	).Scan(&exists)
	return exists, err
}

// FindDueForStartReminder returns active batches starting on startDate
// ("YYYY-MM-DD") that haven't had a start reminder sent yet.
func (r *BatchRepository) FindDueForStartReminder(ctx context.Context, startDate string) ([]models.Batch, error) {
	q := batchBaseSelect + `
		WHERE b.deleted_at IS NULL
		  AND b.is_active
		  AND b.start_date = $1::DATE
		  AND b.start_reminder_sent_at IS NULL`
	return r.scanBatches(ctx, q, startDate)
}

// MarkStartReminderSent records that the start-date reminder has been sent,
// so the reminder worker doesn't send it again on the next poll.
func (r *BatchRepository) MarkStartReminderSent(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE batches SET start_reminder_sent_at = NOW() WHERE id = $1::UUID`, id)
	return err
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
			&b.ID, &b.ShortID,
			&b.CollegeID, &b.CollegeShortID,
			&b.BatchNumber,
			&b.CourseID, &b.CourseName, &b.CourseShortID,
			&b.BatchManagerID, &b.BatchManagerName,
			&b.AdditionalManagerID, &b.AdditionalManagerName,
			&b.Module, &b.StartDate, &b.EndDate,
			&b.IsActive, &b.Status, &b.MaxStudents, &b.StudentCount,
			&b.ScoreWeightAssignments, &b.ScoreWeightExams, &b.ScoreWeightProjects,
			&b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, err
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}
