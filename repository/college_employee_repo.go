package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

// ErrLastCollegeMembership is returned by RemoveMembership when asked to
// remove an employee's only remaining college — every employee must always
// belong to at least one.
var ErrLastCollegeMembership = errors.New("cannot remove an employee's only remaining college")

// CollegeEmployeeRepository manages which colleges a mentor/employee/
// team_lead currently serves — a real many-to-many, on top of the single
// users.college_id "default" column that everything else in the app still
// reads.
type CollegeEmployeeRepository struct {
	pool *pgxpool.Pool
}

func NewCollegeEmployeeRepository(pool *pgxpool.Pool) *CollegeEmployeeRepository {
	return &CollegeEmployeeRepository{pool: pool}
}

// List returns every college a user is currently linked to, default first.
func (r *CollegeEmployeeRepository) List(ctx context.Context, userID string) ([]models.CollegeMembership, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.short_id, c.name, ce.is_default, ce.assigned_at
		FROM college_employees ce
		JOIN colleges c ON c.id = ce.college_id
		WHERE ce.user_id = $1::UUID
		ORDER BY ce.is_default DESC, ce.assigned_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memberships := []models.CollegeMembership{}
	for rows.Next() {
		var m models.CollegeMembership
		if err := rows.Scan(&m.CollegeID, &m.CollegeShortID, &m.CollegeName, &m.IsDefault, &m.AssignedAt); err != nil {
			return nil, err
		}
		memberships = append(memberships, m)
	}
	return memberships, rows.Err()
}

// AddMembership links a user to another college. The very first membership
// for a user is automatically their default; later ones are additional.
func (r *CollegeEmployeeRepository) AddMembership(ctx context.Context, userID, collegeID, assignedBy string) error {
	var existing int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM college_employees WHERE user_id = $1::UUID`, userID).Scan(&existing); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO college_employees (college_id, user_id, is_default, assigned_by)
		VALUES ($1::UUID, $2::UUID, $3, NULLIF($4,'')::UUID)
		ON CONFLICT (college_id, user_id) DO NOTHING`,
		collegeID, userID, existing == 0, assignedBy,
	)
	return err
}

// RemoveMembership unlinks a user from a college. Refuses to remove the
// user's only remaining membership — SetDefault (reassign a different one
// first) or delete the user instead. Removing the current default hands
// default status to whichever remaining membership is oldest.
func (r *CollegeEmployeeRepository) RemoveMembership(ctx context.Context, userID, collegeID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM college_employees WHERE user_id = $1::UUID`, userID).Scan(&total); err != nil {
		return err
	}
	if total <= 1 {
		return ErrLastCollegeMembership
	}

	var wasDefault bool
	err = tx.QueryRow(ctx, `
		DELETE FROM college_employees WHERE user_id = $1::UUID AND college_id = $2::UUID
		RETURNING is_default`,
		userID, collegeID,
	).Scan(&wasDefault)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}
		return err
	}

	if wasDefault {
		var newDefaultCollegeID string
		if err := tx.QueryRow(ctx, `
			SELECT college_id FROM college_employees WHERE user_id = $1::UUID
			ORDER BY assigned_at LIMIT 1`, userID,
		).Scan(&newDefaultCollegeID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE college_employees SET is_default = true WHERE user_id = $1::UUID AND college_id = $2::UUID`,
			userID, newDefaultCollegeID,
		); err != nil {
			return err
		}
		// users.college_id is the single source every other query still
		// reads — keep it pointed at whichever membership is now default.
		if _, err := tx.Exec(ctx, `UPDATE users SET college_id = $2::UUID, updated_at = NOW() WHERE id = $1::UUID`,
			userID, newDefaultCollegeID,
		); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
