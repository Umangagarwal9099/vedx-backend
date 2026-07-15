package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

type LeaveBalanceRepository struct {
	pool *pgxpool.Pool
}

func NewLeaveBalanceRepository(pool *pgxpool.Pool) *LeaveBalanceRepository {
	return &LeaveBalanceRepository{pool: pool}
}

// GetOrInit returns the employee's balance row for the given year, creating
// a zero-total row on first access (an admin can PATCH total_days separately
// if this org grants leave upfront — not built here, keep scope to what the
// employee dashboard needs: total/used/remaining).
func (r *LeaveBalanceRepository) GetOrInit(ctx context.Context, employeeID string, year int) (*models.LeaveBalance, error) {
	var b models.LeaveBalance
	err := r.pool.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO employee_leave_balances (employee_id, year)
			VALUES ($1, $2)
			ON CONFLICT (employee_id, year) DO NOTHING
			RETURNING employee_id, year, total_days, used_days
		)
		SELECT employee_id, year, total_days, used_days FROM ins
		UNION ALL
		SELECT employee_id, year, total_days, used_days FROM employee_leave_balances
		WHERE employee_id = $1 AND year = $2
		LIMIT 1`,
		employeeID, year,
	).Scan(&b.EmployeeID, &b.Year, &b.TotalDays, &b.UsedDays)
	if err != nil {
		return nil, err
	}
	b.Remaining = b.TotalDays - b.UsedDays
	return &b, nil
}

// AddUsedDays increments used_days by the given amount (e.g. day-count of an
// approved leave request) for the given employee/year, initializing the row
// first if it doesn't exist.
func (r *LeaveBalanceRepository) AddUsedDays(ctx context.Context, employeeID string, year int, days float64) error {
	if _, err := r.GetOrInit(ctx, employeeID, year); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE employee_leave_balances SET used_days = used_days + $3, updated_at = NOW()
		WHERE employee_id = $1 AND year = $2`,
		employeeID, year, days,
	)
	return err
}
