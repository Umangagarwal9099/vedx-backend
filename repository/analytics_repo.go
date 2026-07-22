package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

type AnalyticsRepository struct {
	pool *pgxpool.Pool
}

func NewAnalyticsRepository(pool *pgxpool.Pool) *AnalyticsRepository {
	return &AnalyticsRepository{pool: pool}
}

// GetBatchAnalytics returns one row per batch — student count, average
// completion %, average attendance rate, an attendance-based at-risk count,
// and certificates issued — built from the same real tables already used
// per-batch elsewhere (student_enrollments, session_attendance, certificates).
// mentorID scopes to batches that mentor manages (empty = every batch).
// collegeID additionally scopes to one college's batches (empty = unscoped,
// per repository.CollegeFilter — never pass a caller-supplied value directly).
func (r *AnalyticsRepository) GetBatchAnalytics(ctx context.Context, mentorID, collegeID string) ([]models.BatchAnalyticsRow, error) {
	q := `
		WITH student_attendance AS (
			SELECT sa.batch_id, sa.student_id,
			       AVG(CASE WHEN sa.status IN ('present','late') THEN 100.0 ELSE 0 END) AS pct
			FROM session_attendance sa
			GROUP BY sa.batch_id, sa.student_id
		),
		batch_attendance AS (
			SELECT batch_id, AVG(pct) AS avg_attendance, COUNT(*) FILTER (WHERE pct < 60) AS at_risk_count
			FROM student_attendance GROUP BY batch_id
		),
		batch_completion AS (
			SELECT batch_id, AVG(completion_percentage) AS avg_completion
			FROM student_enrollments WHERE completion_percentage IS NOT NULL GROUP BY batch_id
		),
		batch_certs AS (
			SELECT batch_id, COUNT(*) AS cert_count FROM certificates WHERE revoked_at IS NULL GROUP BY batch_id
		)
		SELECT b.short_id, b.batch_number, c.name,
		       (SELECT COUNT(*) FROM batch_students bs WHERE bs.batch_id = b.id),
		       COALESCE(bcomp.avg_completion, 0), COALESCE(batt.avg_attendance, 0),
		       COALESCE(batt.at_risk_count, 0), COALESCE(bcert.cert_count, 0)
		FROM batches b
		JOIN courses c ON b.course_id = c.id
		LEFT JOIN batch_attendance batt ON batt.batch_id = b.id
		LEFT JOIN batch_completion bcomp ON bcomp.batch_id = b.id
		LEFT JOIN batch_certs bcert ON bcert.batch_id = b.id
		WHERE b.deleted_at IS NULL`
	args := []interface{}{}
	if mentorID != "" {
		args = append(args, mentorID)
		q += fmt.Sprintf(` AND (b.batch_manager_id = $%d OR b.additional_manager_id = $%d)`, len(args), len(args))
	}
	if collegeID != "" {
		args = append(args, collegeID)
		q += fmt.Sprintf(` AND b.college_id = $%d::UUID`, len(args))
	}
	q += ` ORDER BY b.created_at DESC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.BatchAnalyticsRow
	for rows.Next() {
		var row models.BatchAnalyticsRow
		if err := rows.Scan(
			&row.BatchShortID, &row.BatchNumber, &row.CourseName,
			&row.StudentCount, &row.AvgCompletionPercent, &row.AvgAttendanceRate,
			&row.AtRiskCount, &row.CertificatesIssued,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetBatchAttendanceTrend returns average attendance % per week for one
// batch, oldest first — reuses session_attendance the same way GetSessionReports does.
func (r *AnalyticsRepository) GetBatchAttendanceTrend(ctx context.Context, batchID string) ([]models.BatchAttendanceTrendPoint, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT date_trunc('week', s.session_date)::DATE::TEXT AS week_start,
		       AVG(CASE WHEN sa.status IN ('present','late') THEN 100.0 ELSE 0 END) AS attendance_rate
		FROM session_attendance sa
		JOIN sessions s ON sa.session_id = s.id
		WHERE sa.batch_id = $1::UUID
		GROUP BY week_start
		ORDER BY week_start`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.BatchAttendanceTrendPoint
	for rows.Next() {
		var p models.BatchAttendanceTrendPoint
		if err := rows.Scan(&p.WeekStart, &p.Attendance); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
