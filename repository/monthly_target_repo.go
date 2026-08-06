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

type MonthlyTargetRepository struct {
	pool *pgxpool.Pool
}

func NewMonthlyTargetRepository(pool *pgxpool.Pool) *MonthlyTargetRepository {
	return &MonthlyTargetRepository{pool: pool}
}

// achievedSubquery counts an employee's conversions within the given
// year/month, computed fresh every time rather than stored.
const achievedSubquery = `(
	SELECT COUNT(*) FROM leads
	WHERE leads.assigned_to = t.employee_id AND leads.status = 'converted'
	  AND leads.deleted_at IS NULL
	  AND EXTRACT(YEAR FROM leads.converted_at) = t.year
	  AND EXTRACT(MONTH FROM leads.converted_at) = t.month
)`

const monthlyTargetBaseSelect = `
	SELECT t.id, t.short_id, t.employee_id, CONCAT(u.first_name, ' ', u.last_name),
	       t.year, t.month, t.target_conversions, ` + achievedSubquery + `,
	       t.created_at, t.updated_at
	FROM employee_monthly_targets t
	JOIN users u ON t.employee_id = u.id AND u.deleted_at IS NULL`

func scanMonthlyTarget(row pgx.Row) (models.MonthlyTarget, error) {
	var m models.MonthlyTarget
	err := row.Scan(
		&m.ID, &m.ShortID, &m.EmployeeID, &m.EmployeeName,
		&m.Year, &m.Month, &m.TargetConversions, &m.Achieved,
		&m.CreatedAt, &m.UpdatedAt,
	)
	return m, err
}

// Set upserts an employee's target for a given year/month.
func (r *MonthlyTargetRepository) Set(ctx context.Context, employeeID string, in models.SetMonthlyTargetInput, setBy string) (*models.MonthlyTarget, error) {
	// See the comment on assessment_repo.go's Create for why this reads FROM
	// ins rather than FROM employee_monthly_targets.
	insSelect := strings.Replace(monthlyTargetBaseSelect, "FROM employee_monthly_targets t", "FROM ins t", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		m, err := scanMonthlyTarget(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO employee_monthly_targets (short_id, employee_id, year, month, target_conversions, set_by)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (employee_id, year, month) DO UPDATE SET
					target_conversions = EXCLUDED.target_conversions,
					set_by = EXCLUDED.set_by,
					updated_at = NOW()
				RETURNING *
			)
			%s`, insSelect),
			shortID, employeeID, in.Year, in.Month, in.TargetConversions, setBy,
		))
		if err == nil {
			return &m, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("set monthly target: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// GetForEmployee returns one employee's target (+ computed achieved) for a
// given year/month, or nil if no target has been set.
func (r *MonthlyTargetRepository) GetForEmployee(ctx context.Context, employeeID string, year, month int) (*models.MonthlyTarget, error) {
	q := fmt.Sprintf("%s WHERE t.employee_id = $1 AND t.year = $2 AND t.month = $3", monthlyTargetBaseSelect)
	m, err := scanMonthlyTarget(r.pool.QueryRow(ctx, q, employeeID, year, month))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if wd := util.WorkingDaysInMonth(year, month); wd > 0 {
		m.DailyTarget = float64(m.TargetConversions) / float64(wd)
	}
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM leads
		WHERE assigned_to = $1 AND status = 'converted' AND deleted_at IS NULL
		  AND converted_at::DATE = CURRENT_DATE`,
		employeeID,
	).Scan(&m.AchievedToday); err != nil {
		return nil, fmt.Errorf("count today's conversions: %w", err)
	}

	return &m, nil
}

// GetTeamSummary returns every employee/team_lead's target (+ achieved) for
// a given year/month — every eligible user appears even if no target has
// been set yet for this month (target_conversions defaults to 0), so admin
// can set a first target for someone new.
func (r *MonthlyTargetRepository) GetTeamSummary(ctx context.Context, year, month int) ([]models.MonthlyTarget, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, CONCAT(u.first_name, ' ', u.last_name),
		       COALESCE(t.target_conversions, 0),
		       (SELECT COUNT(*) FROM leads
		        WHERE leads.assigned_to = u.id AND leads.status = 'converted'
		          AND leads.deleted_at IS NULL
		          AND EXTRACT(YEAR FROM leads.converted_at) = $1
		          AND EXTRACT(MONTH FROM leads.converted_at) = $2) AS achieved
		FROM users u
		LEFT JOIN employee_monthly_targets t
		  ON t.employee_id = u.id AND t.year = $1 AND t.month = $2
		WHERE u.role IN ('employee', 'team_lead') AND u.deleted_at IS NULL
		ORDER BY u.first_name, u.last_name`,
		year, month,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.MonthlyTarget
	for rows.Next() {
		var m models.MonthlyTarget
		if err := rows.Scan(&m.EmployeeID, &m.EmployeeName, &m.TargetConversions, &m.Achieved); err != nil {
			return nil, err
		}
		m.Year = year
		m.Month = month
		out = append(out, m)
	}
	return out, rows.Err()
}
