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

type LeadAssignmentHistoryRepository struct {
	pool *pgxpool.Pool
}

func NewLeadAssignmentHistoryRepository(pool *pgxpool.Pool) *LeadAssignmentHistoryRepository {
	return &LeadAssignmentHistoryRepository{pool: pool}
}

const leadAssignmentHistoryBaseSelect = `
	SELECT h.id, h.short_id, l.short_id, h.action::TEXT,
	       COALESCE(h.from_employee_id::TEXT, ''), COALESCE(CONCAT(fu.first_name, ' ', fu.last_name), ''),
	       COALESCE(h.to_employee_id::TEXT, ''), COALESCE(CONCAT(tu.first_name, ' ', tu.last_name), ''),
	       COALESCE(h.priority_set::TEXT, ''), h.follow_up_set_at,
	       h.performed_by, h.created_at
	FROM lead_assignment_history h
	JOIN leads l ON h.lead_id = l.id
	LEFT JOIN users fu ON h.from_employee_id = fu.id AND fu.deleted_at IS NULL
	LEFT JOIN users tu ON h.to_employee_id = tu.id AND tu.deleted_at IS NULL`

func scanLeadAssignmentHistory(row pgx.Row) (models.LeadAssignmentHistoryEntry, error) {
	var h models.LeadAssignmentHistoryEntry
	err := row.Scan(
		&h.ID, &h.ShortID, &h.LeadShortID, &h.Action,
		&h.FromEmployeeID, &h.FromEmployeeName,
		&h.ToEmployeeID, &h.ToEmployeeName,
		&h.PrioritySet, &h.FollowUpSetAt,
		&h.PerformedBy, &h.CreatedAt,
	)
	return h, err
}

// Create writes one assignment-history event. leadID is the internal UUID
// (already resolved by the caller), not the short_id.
func (r *LeadAssignmentHistoryRepository) Create(ctx context.Context, leadID, action, fromEmployeeID, toEmployeeID, priority string, followUpAt interface{}, performedBy string) error {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		_, err := r.pool.Exec(ctx, `
			INSERT INTO lead_assignment_history
			  (short_id, lead_id, action, from_employee_id, to_employee_id, priority_set, follow_up_set_at, performed_by)
			VALUES ($1, $2, $3::lead_assignment_action, NULLIF($4,'')::UUID, NULLIF($5,'')::UUID, NULLIF($6,'')::lead_priority, $7, $8)`,
			shortID, leadID, action, fromEmployeeID, toEmployeeID, priority, followUpAt, performedBy,
		)
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return fmt.Errorf("insert assignment history: %w", err)
	}
	return fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAllForLead returns every assignment-history event for one lead, newest first.
func (r *LeadAssignmentHistoryRepository) FindAllForLead(ctx context.Context, leadShortID string) ([]models.LeadAssignmentHistoryEntry, error) {
	q := fmt.Sprintf("%s WHERE l.short_id = $1 ORDER BY h.created_at DESC", leadAssignmentHistoryBaseSelect)
	rows, err := r.pool.Query(ctx, q, leadShortID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.LeadAssignmentHistoryEntry
	for rows.Next() {
		h, err := scanLeadAssignmentHistory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
