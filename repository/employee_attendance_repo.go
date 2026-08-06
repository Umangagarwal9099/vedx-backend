package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type EmployeeAttendanceRepository struct {
	pool *pgxpool.Pool
}

func NewEmployeeAttendanceRepository(pool *pgxpool.Pool) *EmployeeAttendanceRepository {
	return &EmployeeAttendanceRepository{pool: pool}
}

const employeeAttendanceBaseSelect = `
	SELECT a.id, a.short_id, a.employee_id, CONCAT(u.first_name, ' ', u.last_name),
	       a.date::TEXT, a.status::TEXT,
	       a.check_in_at, a.check_in_lat, a.check_in_lng, COALESCE(a.check_in_selfie_url, ''),
	       a.check_out_at, a.check_out_lat, a.check_out_lng, COALESCE(a.check_out_selfie_url, ''),
	       COALESCE(a.notes, ''),
	       a.created_at, a.updated_at
	FROM employee_attendance a
	JOIN users u ON a.employee_id = u.id AND u.deleted_at IS NULL`

func scanEmployeeAttendance(row pgx.Row) (models.EmployeeAttendance, error) {
	var a models.EmployeeAttendance
	err := row.Scan(
		&a.ID, &a.ShortID, &a.EmployeeID, &a.EmployeeName,
		&a.Date, &a.Status,
		&a.CheckInAt, &a.CheckInLat, &a.CheckInLng, &a.CheckInSelfieURL,
		&a.CheckOutAt, &a.CheckOutLat, &a.CheckOutLng, &a.CheckOutSelfieURL,
		&a.Notes,
		&a.CreatedAt, &a.UpdatedAt,
	)
	return a, err
}

// CheckIn creates (or reuses) today's row for the employee and stamps
// check_in_at + the captured location/selfie, unless they've already
// checked in today (COALESCE guards every column so a second call is a
// true no-op, not an overwrite).
func (r *EmployeeAttendanceRepository) CheckIn(ctx context.Context, employeeID string, in models.CheckInInput) (*models.EmployeeAttendance, error) {
	// See the comment on assessment_repo.go's Create for why this reads FROM
	// ins rather than FROM employee_attendance.
	insSelect := strings.Replace(employeeAttendanceBaseSelect, "FROM employee_attendance a", "FROM ins a", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		a, err := scanEmployeeAttendance(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO employee_attendance (short_id, employee_id, date, status, check_in_at, check_in_lat, check_in_lng, check_in_selfie_url)
				VALUES ($1, $2, CURRENT_DATE, 'present', NOW(), $3, $4, $5)
				ON CONFLICT (employee_id, date) DO UPDATE SET
					check_in_at = COALESCE(employee_attendance.check_in_at, EXCLUDED.check_in_at),
					check_in_lat = COALESCE(employee_attendance.check_in_lat, EXCLUDED.check_in_lat),
					check_in_lng = COALESCE(employee_attendance.check_in_lng, EXCLUDED.check_in_lng),
					check_in_selfie_url = COALESCE(employee_attendance.check_in_selfie_url, EXCLUDED.check_in_selfie_url),
					updated_at = NOW()
				RETURNING *
			)
			%s`, insSelect),
			shortID, employeeID, in.Latitude, in.Longitude, in.SelfieURL,
		))
		if err == nil {
			return &a, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("check in: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// CheckOut stamps check_out_at + the captured location/selfie on today's row.
func (r *EmployeeAttendanceRepository) CheckOut(ctx context.Context, employeeID string, in models.CheckOutInput) (*models.EmployeeAttendance, error) {
	result, err := r.pool.Exec(ctx, `
		UPDATE employee_attendance SET
			check_out_at = NOW(), check_out_lat = $2, check_out_lng = $3, check_out_selfie_url = $4,
			updated_at = NOW()
		WHERE employee_id = $1 AND date = CURRENT_DATE`,
		employeeID, in.Latitude, in.Longitude, in.SelfieURL,
	)
	if err != nil {
		return nil, fmt.Errorf("check out: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil, fmt.Errorf("no check-in found for today")
	}
	return r.FindToday(ctx, employeeID)
}

// FindToday returns the employee's row for today, if any.
func (r *EmployeeAttendanceRepository) FindToday(ctx context.Context, employeeID string) (*models.EmployeeAttendance, error) {
	q := fmt.Sprintf("%s WHERE a.employee_id = $1 AND a.date = CURRENT_DATE", employeeAttendanceBaseSelect)
	a, err := scanEmployeeAttendance(r.pool.QueryRow(ctx, q, employeeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &a, err
}

// FindForEmployee returns an employee's attendance history, newest first.
func (r *EmployeeAttendanceRepository) FindForEmployee(ctx context.Context, employeeID string) ([]models.EmployeeAttendance, error) {
	q := fmt.Sprintf("%s WHERE a.employee_id = $1 ORDER BY a.date DESC LIMIT 90", employeeAttendanceBaseSelect)
	rows, err := r.pool.Query(ctx, q, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.EmployeeAttendance
	for rows.Next() {
		a, err := scanEmployeeAttendance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FindAllForDate returns every employee's attendance row for a given date
// (defaults to today if date is empty) — admin's team roster view.
func (r *EmployeeAttendanceRepository) FindAllForDate(ctx context.Context, date string) ([]models.EmployeeAttendance, error) {
	q := fmt.Sprintf("%s WHERE a.date = COALESCE(NULLIF($1,'')::DATE, CURRENT_DATE) ORDER BY u.first_name, u.last_name", employeeAttendanceBaseSelect)
	rows, err := r.pool.Query(ctx, q, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.EmployeeAttendance
	for rows.Next() {
		a, err := scanEmployeeAttendance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
