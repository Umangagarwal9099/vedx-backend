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

type LeadCallLogRepository struct {
	pool *pgxpool.Pool
}

func NewLeadCallLogRepository(pool *pgxpool.Pool) *LeadCallLogRepository {
	return &LeadCallLogRepository{pool: pool}
}

const leadCallLogBaseSelect = `
	SELECT cl.id, cl.short_id, l.short_id, cl.employee_id, CONCAT(u.first_name, ' ', u.last_name),
	       cl.outcome::TEXT, cl.status_after::TEXT, COALESCE(cl.notes, ''),
	       cl.follow_up_set_at, cl.called_at, cl.created_at
	FROM lead_call_logs cl
	JOIN leads l  ON cl.lead_id = l.id
	JOIN users  u ON cl.employee_id = u.id AND u.deleted_at IS NULL`

func scanLeadCallLog(row pgx.Row) (models.LeadCallLog, error) {
	var cl models.LeadCallLog
	err := row.Scan(
		&cl.ID, &cl.ShortID, &cl.LeadShortID, &cl.EmployeeID, &cl.EmployeeName,
		&cl.Outcome, &cl.StatusAfter, &cl.Notes,
		&cl.FollowUpSetAt, &cl.CalledAt, &cl.CreatedAt,
	)
	return cl, err
}

// Create logs a call against a lead. This is purely the log-write; the
// caller (controller) is responsible for also updating the lead's own
// status/last_contacted_at/next_follow_up_at fields.
func (r *LeadCallLogRepository) Create(ctx context.Context, leadShortID, employeeID string, in models.CreateCallLogInput) (*models.LeadCallLog, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		cl, err := scanLeadCallLog(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO lead_call_logs (short_id, lead_id, employee_id, outcome, status_after, notes, follow_up_set_at)
				SELECT $1, l.id, $2, $3::call_outcome, $4::lead_status, NULLIF($5,''), $6
				FROM leads l WHERE l.short_id = $7 AND l.deleted_at IS NULL
				RETURNING *
			)
			%s WHERE cl.id = (SELECT id FROM ins)`, leadCallLogBaseSelect),
			shortID, employeeID, in.Outcome, in.Status, in.Notes, in.NextFollowUpAt, leadShortID,
		))
		if err == nil {
			return &cl, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert call log: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAllForLead returns every call log for one lead, newest first.
func (r *LeadCallLogRepository) FindAllForLead(ctx context.Context, leadShortID string) ([]models.LeadCallLog, error) {
	q := fmt.Sprintf("%s WHERE l.short_id = $1 ORDER BY cl.called_at DESC", leadCallLogBaseSelect)
	rows, err := r.pool.Query(ctx, q, leadShortID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.LeadCallLog
	for rows.Next() {
		cl, err := scanLeadCallLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cl)
	}
	return out, rows.Err()
}
