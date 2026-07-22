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

type EnrollmentRepository struct {
	pool *pgxpool.Pool
}

func NewEnrollmentRepository(pool *pgxpool.Pool) *EnrollmentRepository {
	return &EnrollmentRepository{pool: pool}
}

const enrollmentBaseSelect = `
	SELECT se.id, se.short_id,
	       se.student_id, CONCAT(u.first_name, ' ', u.last_name),
	       se.course_id, c.short_id, c.name,
	       se.batch_id, b.short_id, b.batch_number,
	       se.enrollment_date, se.enrollment_type, se.status::TEXT,
	       se.access_start_date, se.access_end_date, se.is_late_enrollment,
	       se.completion_percentage, se.final_score, se.final_rank,
	       COALESCE(se.created_by::TEXT, ''), se.created_at, se.updated_at,
	       COALESCE(bs.fees_paid, FALSE)
	FROM student_enrollments se
	JOIN users   u ON se.student_id = u.id
	JOIN courses c ON se.course_id  = c.id
	JOIN batches b ON se.batch_id   = b.id
	LEFT JOIN batch_students bs ON bs.batch_id = se.batch_id AND bs.user_id = se.student_id`

func scanEnrollment(row pgx.Row) (models.StudentEnrollment, error) {
	var e models.StudentEnrollment
	err := row.Scan(
		&e.ID, &e.ShortID,
		&e.StudentID, &e.StudentName,
		&e.CourseID, &e.CourseShortID, &e.CourseName,
		&e.BatchID, &e.BatchShortID, &e.BatchNumber,
		&e.EnrollmentDate, &e.EnrollmentType, &e.Status,
		&e.AccessStartDate, &e.AccessEndDate, &e.IsLateEnrollment,
		&e.CompletionPercentage, &e.FinalScore, &e.FinalRank,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
		&e.FeesPaid,
	)
	return e, err
}

// activeEnrollmentStatuses are the statuses that count as "currently occupying
// a seat in this course" for the same-course-exclusivity check — completed,
// dropped, transferred, and removed enrollments don't block a fresh one.
var activeEnrollmentStatuses = []string{
	models.EnrollmentInvited, models.EnrollmentEnrolled, models.EnrollmentActive, models.EnrollmentOnHold,
}

// HasActiveEnrollmentInCourse reports whether studentID already holds an
// active-ish enrollment in any batch of courseID OTHER than excludeBatchID —
// the check behind "a student shouldn't be actively enrolled in two batches
// of the same course at once."
func (r *EnrollmentRepository) HasActiveEnrollmentInCourse(ctx context.Context, studentID, courseID, excludeBatchID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM student_enrollments
			WHERE student_id = $1::UUID AND course_id = $2::UUID AND batch_id <> $3::UUID
			  AND status = ANY($4::enrollment_status[])
		)`,
		studentID, courseID, excludeBatchID, activeEnrollmentStatuses,
	).Scan(&exists)
	return exists, err
}

// HasAnyActiveEnrollment reports whether studentID holds any active-ish
// enrollment at all, in any course/batch — used to decide whether a college
// transfer (PATCH /users/:id/college) can proceed without an explicit
// force override.
func (r *EnrollmentRepository) HasAnyActiveEnrollment(ctx context.Context, studentID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM student_enrollments
			WHERE student_id = $1::UUID AND status = ANY($2::enrollment_status[])
		)`,
		studentID, activeEnrollmentStatuses,
	).Scan(&exists)
	return exists, err
}

// Create records a new enrollment (student in a batch of a course). Retries
// on short_id collision, matching the pattern used across this codebase.
func (r *EnrollmentRepository) Create(
	ctx context.Context,
	studentID, courseID, batchID, createdBy string,
	isLateEnrollment bool,
	accessStartDate *time.Time,
	accessEndDate *time.Time,
) (*models.StudentEnrollment, error) {
	// See the comment on assessment_repo.go's Create for why this reads FROM
	// ins rather than FROM student_enrollments.
	insSelect := strings.Replace(enrollmentBaseSelect, "FROM student_enrollments se", "FROM ins se", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		e, err := scanEnrollment(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO student_enrollments (
					short_id, student_id, course_id, batch_id, enrollment_type, status,
					access_start_date, access_end_date, is_late_enrollment, created_by
				)
				VALUES ($1, $2::UUID, $3::UUID, $4::UUID, 'direct', 'active', $5, $6, $7, $8::UUID)
				ON CONFLICT (student_id, batch_id) DO UPDATE SET
					status = 'active', access_start_date = EXCLUDED.access_start_date,
					access_end_date = EXCLUDED.access_end_date, is_late_enrollment = EXCLUDED.is_late_enrollment,
					updated_at = NOW()
				RETURNING *
			)
			%s`, insSelect),
			shortID, studentID, courseID, batchID, accessStartDate, accessEndDate, isLateEnrollment, createdBy,
		))
		if err == nil {
			return &e, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert enrollment: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// CountActiveStudents returns the number of distinct students currently
// holding an active-ish enrollment (invited/enrolled/active/on_hold) —
// the dashboard's "Active Students" figure. If mentorID is set, only counts
// students in batches that mentor manages.
func (r *EnrollmentRepository) CountActiveStudents(ctx context.Context, mentorID string) (int, error) {
	q := `
		SELECT COUNT(DISTINCT se.student_id) FROM student_enrollments se`
	args := []interface{}{activeEnrollmentStatuses}
	if mentorID != "" {
		q += ` JOIN batches b ON b.id = se.batch_id WHERE se.status = ANY($1::enrollment_status[]) AND (b.batch_manager_id = $2 OR b.additional_manager_id = $2)`
		args = append(args, mentorID)
	} else {
		q += ` WHERE se.status = ANY($1::enrollment_status[])`
	}
	var count int
	err := r.pool.QueryRow(ctx, q, args...).Scan(&count)
	return count, err
}

// GetEnrollmentTrend returns the count of new enrollments per day over the
// last `days` days (including days with zero enrollments), oldest first —
// the dashboard's enrollment trend chart. If mentorID is set, only counts
// enrollments into batches that mentor manages.
func (r *EnrollmentRepository) GetEnrollmentTrend(ctx context.Context, days int, mentorID string) ([]models.EnrollmentTrendPoint, error) {
	q := `
		SELECT d::DATE::TEXT, COUNT(se.id)
		FROM generate_series(CURRENT_DATE - ($1::int - 1), CURRENT_DATE, '1 day') AS d
		LEFT JOIN student_enrollments se ON se.enrollment_date::DATE = d`
	args := []interface{}{days}
	if mentorID != "" {
		q += ` AND se.batch_id IN (SELECT id FROM batches WHERE batch_manager_id = $2 OR additional_manager_id = $2)`
		args = append(args, mentorID)
	}
	q += ` GROUP BY d ORDER BY d`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.EnrollmentTrendPoint
	for rows.Next() {
		var p models.EnrollmentTrendPoint
		if err := rows.Scan(&p.Date, &p.Count); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// FindByStudent returns every enrollment a student has ever had, newest first —
// this is the real, per-course, per-batch history the spec's "student taking
// multiple courses" dashboard is built from.
func (r *EnrollmentRepository) FindByStudent(ctx context.Context, studentID string) ([]models.StudentEnrollment, error) {
	q := enrollmentBaseSelect + " WHERE se.student_id = $1::UUID ORDER BY se.enrollment_date DESC"
	rows, err := r.pool.Query(ctx, q, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.StudentEnrollment
	for rows.Next() {
		e, err := scanEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpdateStatus changes a student's enrollment status within a specific batch
// (put on hold, mark completed/dropped/removed, etc) without touching roster
// membership itself.
func (r *EnrollmentRepository) UpdateStatus(ctx context.Context, studentID, batchID, status string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE student_enrollments SET status = $3::enrollment_status, updated_at = NOW()
		WHERE student_id = $1::UUID AND batch_id = $2::UUID`,
		studentID, batchID, status,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Transfer moves a student's enrollment from one batch to another (same
// course, checked by the caller) — the old enrollment is marked "transferred"
// and kept for history, and a new "active" enrollment is created for the
// destination batch. Runs in a single transaction so a mid-way failure can't
// leave a student enrolled nowhere or in both places.
//
// Rejects the transfer if the student and the destination batch belong to
// different colleges — a student must never be moved into another
// college's batch this way. users.college_id/batches.college_id both
// already exist (not migration-pending), so this check always runs.
func (r *EnrollmentRepository) Transfer(ctx context.Context, studentID, fromBatchID, toBatchID, courseID, actorID string) (*models.StudentEnrollment, error) {
	var sameCollege bool
	if err := r.pool.QueryRow(ctx, `
		SELECT u.college_id = b.college_id
		FROM users u, batches b
		WHERE u.id = $1::UUID AND b.id = $2::UUID`,
		studentID, toBatchID,
	).Scan(&sameCollege); err != nil {
		return nil, fmt.Errorf("check transfer college match: %w", err)
	}
	if !sameCollege {
		return nil, fmt.Errorf("%w: student and destination batch belong to different colleges", ErrBatchCollegeMismatch)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE student_enrollments SET status = 'transferred', updated_at = NOW()
		WHERE student_id = $1::UUID AND batch_id = $2::UUID`,
		studentID, fromBatchID,
	); err != nil {
		return nil, fmt.Errorf("mark old enrollment transferred: %w", err)
	}

	// See the comment on assessment_repo.go's Create for why this reads FROM
	// ins rather than FROM student_enrollments.
	transferInsSelect := strings.Replace(enrollmentBaseSelect, "FROM student_enrollments se", "FROM ins se", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		e, err := scanEnrollment(tx.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO student_enrollments (
					short_id, student_id, course_id, batch_id, enrollment_type, status, created_by
				)
				VALUES ($1, $2::UUID, $3::UUID, $4::UUID, 'transferred', 'active', $5::UUID)
				ON CONFLICT (student_id, batch_id) DO UPDATE SET status = 'active', updated_at = NOW()
				RETURNING *
			)
			%s`, transferInsSelect),
			shortID, studentID, courseID, toBatchID, actorID,
		))
		if err == nil {
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return &e, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert transferred enrollment: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}
