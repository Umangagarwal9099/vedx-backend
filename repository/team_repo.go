package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

// TeamRepository manages the team hierarchy — which Operations employee
// reports to which Team Lead. A member belongs to at most one Team Lead at
// a time; reassigning them is an upsert, never a second row.
type TeamRepository struct {
	pool *pgxpool.Pool
}

func NewTeamRepository(pool *pgxpool.Pool) *TeamRepository {
	return &TeamRepository{pool: pool}
}

// AddMember assigns memberID to teamLeadID's team, moving them off any team
// they were previously on.
func (r *TeamRepository) AddMember(ctx context.Context, teamLeadID, memberID, assignedBy string) error {
	shortID := util.GenerateShortID()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO team_memberships (short_id, team_lead_id, member_id, assigned_by)
		VALUES ($1, $2::UUID, $3::UUID, $4::UUID)
		ON CONFLICT (member_id) DO UPDATE
			SET team_lead_id = EXCLUDED.team_lead_id,
			    assigned_by  = EXCLUDED.assigned_by,
			    assigned_at  = NOW()`,
		shortID, teamLeadID, memberID, assignedBy,
	)
	if err != nil {
		return fmt.Errorf("add team member: %w", err)
	}
	return nil
}

// RemoveMember takes memberID off whatever team they're currently on.
func (r *TeamRepository) RemoveMember(ctx context.Context, memberID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM team_memberships WHERE member_id = $1::UUID`, memberID)
	if err != nil {
		return fmt.Errorf("remove team member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetMembers returns everyone currently on teamLeadID's team, name-sorted.
func (r *TeamRepository) GetMembers(ctx context.Context, teamLeadID string) ([]models.TeamMember, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.first_name, u.last_name, u.email, COALESCE(u.phone, ''), tm.assigned_at
		FROM team_memberships tm
		JOIN users u ON u.id = tm.member_id AND u.deleted_at IS NULL
		WHERE tm.team_lead_id = $1::UUID
		ORDER BY u.first_name, u.last_name`,
		teamLeadID,
	)
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	defer rows.Close()

	var out []models.TeamMember
	for rows.Next() {
		var m models.TeamMember
		if err := rows.Scan(&m.UserID, &m.FirstName, &m.LastName, &m.Email, &m.Phone, &m.AssignedAt); err != nil {
			return nil, fmt.Errorf("scan team member: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// IsMember reports whether memberID is currently on teamLeadID's team — the
// backend enforcement point for "a Team Lead can only assign leads to their
// own team."
func (r *TeamRepository) IsMember(ctx context.Context, teamLeadID, memberID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM team_memberships WHERE team_lead_id = $1::UUID AND member_id = $2::UUID)`,
		teamLeadID, memberID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check team membership: %w", err)
	}
	return exists, nil
}

// GetStaffByDepartment returns every active employee tagged with the given
// department, name-sorted — the narrow, department-scoped picker list a
// Manager needs (department="team_lead" or "operations") without exposing
// the general user directory to them.
func (r *TeamRepository) GetStaffByDepartment(ctx context.Context, department string) ([]models.StaffOption, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.first_name, u.last_name, u.email
		FROM users u
		JOIN employees e ON e.user_id = u.id
		WHERE u.role = 'employee' AND e.department = $1 AND u.deleted_at IS NULL
		ORDER BY u.first_name, u.last_name`,
		department,
	)
	if err != nil {
		return nil, fmt.Errorf("list staff by department: %w", err)
	}
	defer rows.Close()

	var out []models.StaffOption
	for rows.Next() {
		var s models.StaffOption
		if err := rows.Scan(&s.UserID, &s.FirstName, &s.LastName, &s.Email); err != nil {
			return nil, fmt.Errorf("scan staff option: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetAllTeams returns every Team Lead who currently has at least one member,
// each with their full member list — the Manager/Admin overview.
func (r *TeamRepository) GetAllTeams(ctx context.Context) ([]models.Team, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT tl.id, CONCAT(tl.first_name, ' ', tl.last_name),
		       u.id, u.first_name, u.last_name, u.email, COALESCE(u.phone, ''), tm.assigned_at
		FROM team_memberships tm
		JOIN users tl ON tl.id = tm.team_lead_id AND tl.deleted_at IS NULL
		JOIN users u  ON u.id  = tm.member_id    AND u.deleted_at IS NULL
		ORDER BY tl.first_name, tl.last_name, u.first_name, u.last_name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	byLead := map[string]*models.Team{}
	var order []string
	for rows.Next() {
		var leadID, leadName string
		var m models.TeamMember
		if err := rows.Scan(&leadID, &leadName, &m.UserID, &m.FirstName, &m.LastName, &m.Email, &m.Phone, &m.AssignedAt); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		t, ok := byLead[leadID]
		if !ok {
			t = &models.Team{TeamLeadID: leadID, TeamLeadName: leadName}
			byLead[leadID] = t
			order = append(order, leadID)
		}
		t.Members = append(t.Members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]models.Team, 0, len(order))
	for _, id := range order {
		out = append(out, *byLead[id])
	}
	return out, nil
}
