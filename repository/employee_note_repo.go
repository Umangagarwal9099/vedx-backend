package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

// EmployeeNoteRepository manages Manager-authored notes on an employee's
// profile — a running record, not a one-time review.
type EmployeeNoteRepository struct {
	pool *pgxpool.Pool
}

func NewEmployeeNoteRepository(pool *pgxpool.Pool) *EmployeeNoteRepository {
	return &EmployeeNoteRepository{pool: pool}
}

// Create adds a note to an employee's profile.
func (r *EmployeeNoteRepository) Create(ctx context.Context, employeeUserID, authorUserID, noteText string) (*models.EmployeeNote, error) {
	shortID := util.GenerateShortID()
	var n models.EmployeeNote
	err := r.pool.QueryRow(ctx, `
		INSERT INTO employee_notes (short_id, employee_user_id, author_user_id, note_text)
		VALUES ($1, $2::UUID, $3::UUID, $4)
		RETURNING short_id, note_text, created_at`,
		shortID, employeeUserID, authorUserID, noteText,
	).Scan(&n.ShortID, &n.NoteText, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	n.EmployeeUserID = employeeUserID
	n.AuthorUserID = authorUserID
	return &n, nil
}

// ListForEmployee returns every note on an employee's profile, newest first.
func (r *EmployeeNoteRepository) ListForEmployee(ctx context.Context, employeeUserID string) ([]models.EmployeeNote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT n.short_id, n.employee_user_id, n.author_user_id,
		       CONCAT(u.first_name, ' ', u.last_name), n.note_text, n.created_at
		FROM employee_notes n
		JOIN users u ON u.id = n.author_user_id
		WHERE n.employee_user_id = $1::UUID
		ORDER BY n.created_at DESC`,
		employeeUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := []models.EmployeeNote{}
	for rows.Next() {
		var n models.EmployeeNote
		if err := rows.Scan(&n.ShortID, &n.EmployeeUserID, &n.AuthorUserID, &n.AuthorName, &n.NoteText, &n.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}
