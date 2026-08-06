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

type LeaveRequestRepository struct {
	pool *pgxpool.Pool
}

func NewLeaveRequestRepository(pool *pgxpool.Pool) *LeaveRequestRepository {
	return &LeaveRequestRepository{pool: pool}
}

const leaveRequestBaseSelect = `
	SELECT l.id, l.short_id, l.employee_id, CONCAT(u.first_name, ' ', u.last_name),
	       l.from_date::TEXT, l.to_date::TEXT, l.leave_type::TEXT,
	       l.leave_category::TEXT, COALESCE(l.certificate_url, ''),
	       l.reason, l.status::TEXT,
	       COALESCE(l.admin_note, ''), COALESCE(l.reviewed_by::TEXT, ''), l.reviewed_at,
	       l.created_at, l.updated_at
	FROM employee_leave_requests l
	JOIN users u ON l.employee_id = u.id AND u.deleted_at IS NULL`

func scanLeaveRequest(row pgx.Row) (models.LeaveRequest, error) {
	var l models.LeaveRequest
	err := row.Scan(
		&l.ID, &l.ShortID, &l.EmployeeID, &l.EmployeeName,
		&l.FromDate, &l.ToDate, &l.LeaveType,
		&l.LeaveCategory, &l.CertificateURL,
		&l.Reason, &l.Status,
		&l.AdminNote, &l.ReviewedBy, &l.ReviewedAt,
		&l.CreatedAt, &l.UpdatedAt,
	)
	return l, err
}

func (r *LeaveRequestRepository) scanAll(ctx context.Context, q string, args ...interface{}) ([]models.LeaveRequest, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.LeaveRequest
	for rows.Next() {
		l, err := scanLeaveRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Apply creates a new leave request in 'pending' status.
func (r *LeaveRequestRepository) Apply(ctx context.Context, employeeID string, in models.CreateLeaveRequestInput) (*models.LeaveRequest, error) {
	leaveType := in.LeaveType
	if leaveType == "" {
		leaveType = "full_day"
	}
	leaveCategory := in.LeaveCategory
	if leaveCategory == "" {
		leaveCategory = "casual"
	}

	// See the comment on assessment_repo.go's Create for why this reads FROM
	// ins rather than FROM employee_leave_requests.
	insSelect := strings.Replace(leaveRequestBaseSelect, "FROM employee_leave_requests l", "FROM ins l", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		l, err := scanLeaveRequest(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO employee_leave_requests (short_id, employee_id, from_date, to_date, leave_type, leave_category, certificate_url, reason)
				VALUES ($1, $2, $3::DATE, $4::DATE, $5::leave_type, $6::leave_category, NULLIF($7,''), $8)
				RETURNING *
			)
			%s`, insSelect),
			shortID, employeeID, in.FromDate, in.ToDate, leaveType, leaveCategory, in.CertificateURL, in.Reason,
		))
		if err == nil {
			return &l, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert leave request: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAllForEmployee returns an employee's own leave requests, newest first.
func (r *LeaveRequestRepository) FindAllForEmployee(ctx context.Context, employeeID string) ([]models.LeaveRequest, error) {
	q := fmt.Sprintf("%s WHERE l.employee_id = $1 ORDER BY l.created_at DESC", leaveRequestBaseSelect)
	return r.scanAll(ctx, q, employeeID)
}

// FindAllPending returns every pending leave request across all employees —
// admin's approval queue.
func (r *LeaveRequestRepository) FindAllPending(ctx context.Context) ([]models.LeaveRequest, error) {
	q := fmt.Sprintf("%s WHERE l.status = 'pending' ORDER BY l.created_at ASC", leaveRequestBaseSelect)
	return r.scanAll(ctx, q)
}

// FindAll returns every leave request across all employees regardless of
// status, newest first — admin's full history view.
func (r *LeaveRequestRepository) FindAll(ctx context.Context) ([]models.LeaveRequest, error) {
	q := fmt.Sprintf("%s ORDER BY l.created_at DESC", leaveRequestBaseSelect)
	return r.scanAll(ctx, q)
}

func (r *LeaveRequestRepository) FindByShortID(ctx context.Context, shortID string) (*models.LeaveRequest, error) {
	q := fmt.Sprintf("%s WHERE l.short_id = $1", leaveRequestBaseSelect)
	l, err := scanLeaveRequest(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &l, err
}

// Review records the admin's approve/reject decision. On approval, the
// caller (controller) is responsible for incrementing the employee's leave
// balance via LeaveBalanceRepository — kept separate so this repo doesn't
// need to know about day-count math.
func (r *LeaveRequestRepository) Review(ctx context.Context, shortID, status, adminNote, reviewedBy string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE employee_leave_requests SET
			status = $2::leave_status, admin_note = NULLIF($3,''),
			reviewed_by = $4, reviewed_at = NOW(), updated_at = NOW()
		WHERE short_id = $1 AND status = 'pending'`,
		shortID, status, adminNote, reviewedBy,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
