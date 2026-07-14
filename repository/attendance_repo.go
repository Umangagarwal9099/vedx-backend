package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type AttendanceRepository struct {
	pool *pgxpool.Pool
}

func NewAttendanceRepository(pool *pgxpool.Pool) *AttendanceRepository {
	return &AttendanceRepository{pool: pool}
}

const attendanceBaseSelect = `
	SELECT sa.id, sa.short_id,
	       sa.session_id, s.short_id, s.name, s.session_date::TEXT,
	       sa.batch_id, b.short_id, b.batch_number,
	       sa.student_id, CONCAT(u.first_name, ' ', u.last_name),
	       sa.status::TEXT, COALESCE(sa.marked_by::TEXT, ''), sa.marked_at,
	       COALESCE(sa.notes, ''), sa.created_at, sa.updated_at
	FROM session_attendance sa
	JOIN sessions s ON sa.session_id = s.id
	JOIN batches  b ON sa.batch_id   = b.id
	JOIN users    u ON sa.student_id = u.id`

func scanAttendance(row pgx.Row) (models.SessionAttendance, error) {
	var a models.SessionAttendance
	err := row.Scan(
		&a.ID, &a.ShortID,
		&a.SessionID, &a.SessionShortID, &a.SessionName, &a.SessionDate,
		&a.BatchID, &a.BatchShortID, &a.BatchNumber,
		&a.StudentID, &a.StudentName,
		&a.Status, &a.MarkedBy, &a.MarkedAt,
		&a.Notes, &a.CreatedAt, &a.UpdatedAt,
	)
	return a, err
}

// MarkOne upserts a single student's attendance for one session.
func (r *AttendanceRepository) MarkOne(ctx context.Context, sessionID, batchID, studentID, status, notes, markedBy string) (*models.SessionAttendance, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		a, err := scanAttendance(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO session_attendance (
					short_id, session_id, batch_id, student_id, status, notes, marked_by, marked_at
				)
				VALUES ($1, $2::UUID, $3::UUID, $4::UUID, $5::attendance_status, $6, $7::UUID, NOW())
				ON CONFLICT (session_id, student_id) DO UPDATE SET
					status = EXCLUDED.status, notes = EXCLUDED.notes,
					marked_by = EXCLUDED.marked_by, marked_at = NOW(), updated_at = NOW()
				RETURNING *
			)
			%s WHERE sa.id = (SELECT id FROM ins)`, attendanceBaseSelect),
			shortID, sessionID, batchID, studentID, status, notes, markedBy,
		))
		if err == nil {
			return &a, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("mark attendance: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// BulkMark marks attendance for many students in one session in a single
// transaction — this is what the "take attendance" screen submits.
func (r *AttendanceRepository) BulkMark(ctx context.Context, sessionID, batchID, markedBy string, records []models.AttendanceRecordInput) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, rec := range records {
		for attempt := 0; attempt < 3; attempt++ {
			shortID := util.GenerateShortID()
			_, err := tx.Exec(ctx, `
				INSERT INTO session_attendance (
					short_id, session_id, batch_id, student_id, status, notes, marked_by, marked_at
				)
				VALUES ($1, $2::UUID, $3::UUID, $4::UUID, $5::attendance_status, $6, $7::UUID, NOW())
				ON CONFLICT (session_id, student_id) DO UPDATE SET
					status = EXCLUDED.status, notes = EXCLUDED.notes,
					marked_by = EXCLUDED.marked_by, marked_at = NOW(), updated_at = NOW()`,
				shortID, sessionID, batchID, rec.StudentID, rec.Status, rec.Notes, markedBy,
			)
			if err == nil {
				break
			}
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				continue
			}
			return fmt.Errorf("bulk mark attendance for student %s: %w", rec.StudentID, err)
		}
	}
	return tx.Commit(ctx)
}

// GetForSession returns every attendance record marked so far for one
// session. Students with no record yet simply won't appear here — the
// caller merges this against the batch roster to show "unmarked" defaults.
func (r *AttendanceRepository) GetForSession(ctx context.Context, sessionID string) ([]models.SessionAttendance, error) {
	rows, err := r.pool.Query(ctx, attendanceBaseSelect+" WHERE sa.session_id = $1::UUID", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SessionAttendance
	for rows.Next() {
		a, err := scanAttendance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetHistoryForStudent returns every attendance record ever marked for a
// student, newest first — the session-by-session log behind "My Attendance".
func (r *AttendanceRepository) GetHistoryForStudent(ctx context.Context, studentID string) ([]models.SessionAttendance, error) {
	rows, err := r.pool.Query(ctx,
		attendanceBaseSelect+" WHERE sa.student_id = $1::UUID ORDER BY s.session_date DESC LIMIT 200",
		studentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SessionAttendance
	for rows.Next() {
		a, err := scanAttendance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetBatchSummary rolls up every enrolled student's attendance across every
// held (active, past-or-today) session of the batch.
func (r *AttendanceRepository) GetBatchSummary(ctx context.Context, batchID string) ([]models.StudentAttendanceSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT bs.user_id, CONCAT(u.first_name, ' ', u.last_name),
		       COUNT(DISTINCT s.id) FILTER (WHERE s.is_active AND s.session_date <= CURRENT_DATE) AS total_sessions,
		       COUNT(*) FILTER (WHERE sa.status = 'present') AS present,
		       COUNT(*) FILTER (WHERE sa.status = 'absent')  AS absent,
		       COUNT(*) FILTER (WHERE sa.status = 'late')    AS late,
		       COUNT(*) FILTER (WHERE sa.status = 'excused') AS excused
		FROM batch_students bs
		JOIN users u ON bs.user_id = u.id
		LEFT JOIN sessions s ON s.batch_id = bs.batch_id AND s.is_active AND s.session_date <= CURRENT_DATE
		LEFT JOIN session_attendance sa ON sa.session_id = s.id AND sa.student_id = bs.user_id
		WHERE bs.batch_id = $1::UUID
		GROUP BY bs.user_id, u.first_name, u.last_name
		ORDER BY u.first_name, u.last_name`,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.StudentAttendanceSummary
	for rows.Next() {
		var sm models.StudentAttendanceSummary
		if err := rows.Scan(&sm.StudentID, &sm.StudentName, &sm.TotalSessions, &sm.Present, &sm.Absent, &sm.Late, &sm.Excused); err != nil {
			return nil, err
		}
		sm.BatchID = batchID
		if sm.TotalSessions > 0 {
			sm.AttendancePct = float64(sm.Present+sm.Late) / float64(sm.TotalSessions) * 100
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// GetSummaryForStudent rolls up one student's attendance per batch, across
// every batch they've ever been added to.
func (r *AttendanceRepository) GetSummaryForStudent(ctx context.Context, studentID string) ([]models.StudentAttendanceSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT bs.batch_id, b.short_id, b.batch_number,
		       COUNT(DISTINCT s.id) FILTER (WHERE s.is_active AND s.session_date <= CURRENT_DATE) AS total_sessions,
		       COUNT(*) FILTER (WHERE sa.status = 'present') AS present,
		       COUNT(*) FILTER (WHERE sa.status = 'absent')  AS absent,
		       COUNT(*) FILTER (WHERE sa.status = 'late')    AS late,
		       COUNT(*) FILTER (WHERE sa.status = 'excused') AS excused
		FROM batch_students bs
		JOIN batches b ON bs.batch_id = b.id
		LEFT JOIN sessions s ON s.batch_id = bs.batch_id AND s.is_active AND s.session_date <= CURRENT_DATE
		LEFT JOIN session_attendance sa ON sa.session_id = s.id AND sa.student_id = bs.user_id
		WHERE bs.user_id = $1::UUID
		GROUP BY bs.batch_id, b.short_id, b.batch_number
		ORDER BY b.batch_number`,
		studentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.StudentAttendanceSummary
	for rows.Next() {
		var sm models.StudentAttendanceSummary
		if err := rows.Scan(&sm.BatchID, &sm.BatchShortID, &sm.BatchNumber, &sm.TotalSessions, &sm.Present, &sm.Absent, &sm.Late, &sm.Excused); err != nil {
			return nil, err
		}
		sm.StudentID = studentID
		if sm.TotalSessions > 0 {
			sm.AttendancePct = float64(sm.Present+sm.Late) / float64(sm.TotalSessions) * 100
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// GetSessionReports rolls up attendance per session (across every batch, or —
// when mentorID is non-empty — only batches that mentor manages), newest
// first. Only sessions with at least one marked record appear.
func (r *AttendanceRepository) GetSessionReports(ctx context.Context, mentorID string) ([]models.SessionAttendanceReport, error) {
	q := `
		SELECT s.short_id, s.name, s.session_date::TEXT, b.short_id, b.batch_number,
		       COUNT(*) AS total,
		       COUNT(*) FILTER (WHERE sa.status = 'present') AS present,
		       COUNT(*) FILTER (WHERE sa.status = 'absent')  AS absent,
		       COUNT(*) FILTER (WHERE sa.status = 'late')    AS late,
		       COUNT(*) FILTER (WHERE sa.status = 'excused') AS excused
		FROM session_attendance sa
		JOIN sessions s ON sa.session_id = s.id
		JOIN batches  b ON sa.batch_id   = b.id`
	args := []interface{}{}
	if mentorID != "" {
		q += ` WHERE b.batch_manager_id = $1 OR b.additional_manager_id = $1`
		args = append(args, mentorID)
	}
	q += `
		GROUP BY s.id, s.short_id, s.name, s.session_date, b.short_id, b.batch_number
		ORDER BY s.session_date DESC
		LIMIT 200`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SessionAttendanceReport
	for rows.Next() {
		var rep models.SessionAttendanceReport
		if err := rows.Scan(&rep.SessionShortID, &rep.SessionName, &rep.SessionDate, &rep.BatchShortID, &rep.BatchNumber,
			&rep.Total, &rep.Present, &rep.Absent, &rep.Late, &rep.Excused); err != nil {
			return nil, err
		}
		if rep.Total > 0 {
			rep.AttendancePct = float64(rep.Present+rep.Late) / float64(rep.Total) * 100
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}
