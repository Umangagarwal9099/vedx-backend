package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

// GuestAccessRepository manages batch_guest_access — a student viewing a
// batch's live sessions and recordings without being enrolled there. See
// db/schema_updates_guest_access_v1.sql for why this is a separate table
// from batch_students/student_enrollments.
type GuestAccessRepository struct {
	pool *pgxpool.Pool
}

func NewGuestAccessRepository(pool *pgxpool.Pool) *GuestAccessRepository {
	return &GuestAccessRepository{pool: pool}
}

// Grant gives each of studentIDs guest access to the batch identified by
// batchShortID. Mirrors AddStudentsWithEnrollment's guards: only matches
// same-college students with role='student', and is a no-op (not an error)
// for a student who already has guest access to this batch. Deliberately
// does not check for an existing active enrollment elsewhere — that's the
// whole point of guest access.
func (r *GuestAccessRepository) Grant(ctx context.Context, batchShortID string, studentIDs []string, grantedBy string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		INSERT INTO batch_guest_access (batch_id, user_id, granted_by)
		SELECT b.id, u.id, $3::uuid
		FROM batches b
		CROSS JOIN unnest($2::uuid[]) AS uid(user_id)
		JOIN users u ON u.id = uid.user_id AND u.deleted_at IS NULL AND u.role = 'student' AND u.college_id = b.college_id
		WHERE b.short_id = $1 AND b.deleted_at IS NULL
		ON CONFLICT (batch_id, user_id) DO NOTHING
		RETURNING user_id`,
		batchShortID, studentIDs, grantedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("grant guest access: %w", err)
	}
	defer rows.Close()
	var granted []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		granted = append(granted, id)
	}
	return granted, rows.Err()
}

// Revoke removes one student's guest access to one batch. Not an error if
// they didn't have it.
func (r *GuestAccessRepository) Revoke(ctx context.Context, batchShortID, userID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM batch_guest_access
		WHERE user_id = $2::uuid
		  AND batch_id = (SELECT id FROM batches WHERE short_id = $1 AND deleted_at IS NULL)`,
		batchShortID, userID,
	)
	return err
}

// HasAccess reports whether userID holds guest access to the batch
// identified by its internal UUID — for callers that already have it
// (mirrors BatchRepository.IsFeesPaid's split between UUID and short_id
// variants).
func (r *GuestAccessRepository) HasAccess(ctx context.Context, batchID, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM batch_guest_access WHERE batch_id = $1::uuid AND user_id = $2::uuid)`,
		batchID, userID,
	).Scan(&exists)
	return exists, err
}

// HasAccessByShortID is HasAccess for callers that only have the batch's
// short_id on hand.
func (r *GuestAccessRepository) HasAccessByShortID(ctx context.Context, batchShortID, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM batch_guest_access ga
			JOIN batches b ON b.id = ga.batch_id
			WHERE b.short_id = $1 AND b.deleted_at IS NULL AND ga.user_id = $2::uuid
		)`,
		batchShortID, userID,
	).Scan(&exists)
	return exists, err
}

// ListForBatch returns everyone currently holding guest access to a batch —
// the admin-facing "who's on the guest list" view.
func (r *GuestAccessRepository) ListForBatch(ctx context.Context, batchShortID string) ([]models.GuestAccessStudent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.first_name || ' ' || u.last_name, u.email, ga.created_at
		FROM batch_guest_access ga
		JOIN batches b ON b.id = ga.batch_id
		JOIN users u ON u.id = ga.user_id
		WHERE b.short_id = $1 AND b.deleted_at IS NULL
		ORDER BY ga.created_at DESC`,
		batchShortID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []models.GuestAccessStudent{}
	for rows.Next() {
		var s models.GuestAccessStudent
		if err := rows.Scan(&s.UserID, &s.Name, &s.Email, &s.GrantedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// ListForStudent returns every batch userID has been granted guest access
// to — the student-facing "batches I can also follow" list, kept separate
// from their real enrollments (GET /users/{id}/enrollments) so the
// frontend never has to guess which is which.
func (r *GuestAccessRepository) ListForStudent(ctx context.Context, userID string) ([]models.GuestBatchAccess, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT b.short_id, b.batch_number, c.name, b.start_date, b.end_date, ga.created_at
		FROM batch_guest_access ga
		JOIN batches b ON b.id = ga.batch_id AND b.deleted_at IS NULL
		JOIN courses c ON c.id = b.course_id
		WHERE ga.user_id = $1::uuid
		ORDER BY ga.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []models.GuestBatchAccess{}
	for rows.Next() {
		var g models.GuestBatchAccess
		var startDate, endDate *string
		if err := rows.Scan(&g.BatchShortID, &g.BatchNumber, &g.CourseName, &startDate, &endDate, &g.GrantedAt); err != nil {
			return nil, err
		}
		if startDate != nil {
			g.StartDate = *startDate
		}
		if endDate != nil {
			g.EndDate = *endDate
		}
		list = append(list, g)
	}
	return list, rows.Err()
}
