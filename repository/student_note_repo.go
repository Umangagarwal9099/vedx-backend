package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type StudentNoteRepository struct {
	pool *pgxpool.Pool
}

func NewStudentNoteRepository(pool *pgxpool.Pool) *StudentNoteRepository {
	return &StudentNoteRepository{pool: pool}
}

// Create adds a new note against a student.
func (r *StudentNoteRepository) Create(ctx context.Context, studentUserID, note, createdBy string) (*models.StudentNote, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		var n models.StudentNote
		err := r.pool.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO student_notes (short_id, student_user_id, note, created_by)
				VALUES ($1, $2, $3, $4)
				RETURNING *
			)
			SELECT ins.id, ins.short_id, ins.student_user_id, ins.note, ins.created_by,
			       CONCAT(u.first_name, ' ', u.last_name), ins.created_at
			FROM ins JOIN users u ON ins.created_by = u.id AND u.deleted_at IS NULL`,
			shortID, studentUserID, note, createdBy,
		).Scan(&n.ID, &n.ShortID, &n.StudentUserID, &n.Note, &n.CreatedBy, &n.CreatedByName, &n.CreatedAt)
		if err == nil {
			return &n, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert student note: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAllForStudent returns every note for a student, newest first.
func (r *StudentNoteRepository) FindAllForStudent(ctx context.Context, studentUserID string) ([]models.StudentNote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT n.id, n.short_id, n.student_user_id, n.note, n.created_by,
		       CONCAT(u.first_name, ' ', u.last_name), n.created_at
		FROM student_notes n
		JOIN users u ON n.created_by = u.id AND u.deleted_at IS NULL
		WHERE n.student_user_id = $1
		ORDER BY n.created_at DESC`,
		studentUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.StudentNote
	for rows.Next() {
		var n models.StudentNote
		if err := rows.Scan(&n.ID, &n.ShortID, &n.StudentUserID, &n.Note, &n.CreatedBy, &n.CreatedByName, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
