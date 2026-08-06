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

type WorkReportRepository struct {
	pool *pgxpool.Pool
}

func NewWorkReportRepository(pool *pgxpool.Pool) *WorkReportRepository {
	return &WorkReportRepository{pool: pool}
}

const workReportBaseSelect = `
	SELECT w.id, w.short_id, w.employee_id, CONCAT(u.first_name, ' ', u.last_name),
	       w.report_date::TEXT, w.calls_made, w.leads_contacted, w.follow_ups_done, w.admissions,
	       COALESCE(w.summary, ''), w.created_at, w.updated_at
	FROM employee_work_reports w
	JOIN users u ON w.employee_id = u.id AND u.deleted_at IS NULL`

func scanWorkReport(row pgx.Row) (models.WorkReport, error) {
	var w models.WorkReport
	err := row.Scan(
		&w.ID, &w.ShortID, &w.EmployeeID, &w.EmployeeName,
		&w.ReportDate, &w.CallsMade, &w.LeadsContacted, &w.FollowUpsDone, &w.Admissions,
		&w.Summary, &w.CreatedAt, &w.UpdatedAt,
	)
	return w, err
}

// Submit upserts today's report for employeeID — re-submitting the same day
// updates the existing row (report_date is always CURRENT_DATE, never
// client-supplied).
func (r *WorkReportRepository) Submit(ctx context.Context, employeeID string, in models.SubmitWorkReportInput) (*models.WorkReport, error) {
	// See the comment on assessment_repo.go's Create for why this reads FROM
	// ins rather than FROM employee_work_reports.
	insSelect := strings.Replace(workReportBaseSelect, "FROM employee_work_reports w", "FROM ins w", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		w, err := scanWorkReport(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO employee_work_reports
				  (short_id, employee_id, report_date, calls_made, leads_contacted, follow_ups_done, admissions, summary)
				VALUES ($1, $2, CURRENT_DATE, $3, $4, $5, $6, NULLIF($7,''))
				ON CONFLICT (employee_id, report_date) DO UPDATE SET
					calls_made = EXCLUDED.calls_made,
					leads_contacted = EXCLUDED.leads_contacted,
					follow_ups_done = EXCLUDED.follow_ups_done,
					admissions = EXCLUDED.admissions,
					summary = EXCLUDED.summary,
					updated_at = NOW()
				RETURNING *
			)
			%s`, insSelect),
			shortID, employeeID, in.CallsMade, in.LeadsContacted, in.FollowUpsDone, in.Admissions, in.Summary,
		))
		if err == nil {
			return &w, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("submit work report: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAllForEmployee returns an employee's last 30 days of reports, newest first.
func (r *WorkReportRepository) FindAllForEmployee(ctx context.Context, employeeID string) ([]models.WorkReport, error) {
	q := fmt.Sprintf("%s WHERE w.employee_id = $1 ORDER BY w.report_date DESC LIMIT 30", workReportBaseSelect)
	rows, err := r.pool.Query(ctx, q, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.WorkReport
	for rows.Next() {
		w, err := scanWorkReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// FindAllForDate returns every employee's work report for a given date
// (defaults to today if date is empty) — admin's team roster view.
func (r *WorkReportRepository) FindAllForDate(ctx context.Context, date string) ([]models.WorkReport, error) {
	q := fmt.Sprintf("%s WHERE w.report_date = COALESCE(NULLIF($1,'')::DATE, CURRENT_DATE) ORDER BY u.first_name, u.last_name", workReportBaseSelect)
	rows, err := r.pool.Query(ctx, q, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.WorkReport
	for rows.Next() {
		w, err := scanWorkReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
