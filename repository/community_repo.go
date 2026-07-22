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

type CommunityRepository struct {
	pool *pgxpool.Pool
}

func NewCommunityRepository(pool *pgxpool.Pool) *CommunityRepository {
	return &CommunityRepository{pool: pool}
}

// communityBaseSelect joins the batch and creator name in, plus a live member count.
const communityBaseSelect = `
	SELECT c.id, c.short_id, c.name, COALESCE(c.description,''),
	       c.batch_id, b.short_id, b.batch_number,
	       c.is_active, c.created_by, CONCAT(u.first_name, ' ', u.last_name),
	       (SELECT COUNT(*) FROM community_members cm WHERE cm.community_id = c.id),
	       c.created_at, c.updated_at
	FROM communities c
	JOIN batches b ON c.batch_id   = b.id AND b.deleted_at IS NULL
	JOIN users   u ON c.created_by = u.id AND u.deleted_at IS NULL`

func scanCommunity(row pgx.Row) (models.Community, error) {
	var c models.Community
	err := row.Scan(
		&c.ID, &c.ShortID, &c.Name, &c.Description,
		&c.BatchID, &c.BatchShortID, &c.BatchNumber,
		&c.IsActive, &c.CreatedBy, &c.CreatedByName,
		&c.MemberCount, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

// Create inserts a community and returns the full record. Retries on short_id collision.
func (r *CommunityRepository) Create(ctx context.Context, in models.CreateCommunityInput, createdBy string) (*models.Community, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		c, err := scanCommunity(r.pool.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO communities (short_id, name, description, batch_id, created_by)
				VALUES (
					$1, $2, NULLIF($3,''),
					(SELECT id FROM batches WHERE short_id = $4 AND deleted_at IS NULL),
					$5
				)
				RETURNING *
			)
			SELECT ins.id, ins.short_id, ins.name, COALESCE(ins.description,''),
			       ins.batch_id, b.short_id, b.batch_number,
			       ins.is_active, ins.created_by, CONCAT(u.first_name, ' ', u.last_name),
			       0,
			       ins.created_at, ins.updated_at
			FROM ins
			JOIN batches b ON ins.batch_id   = b.id
			JOIN users   u ON ins.created_by = u.id`,
			shortID, in.Name, in.Description, in.BatchShortID, createdBy,
		))
		if err == nil {
			return &c, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
				continue
			}
			if pgErr.Code == "23502" {
				return nil, fmt.Errorf("invalid batch_short_id: batch not found")
			}
		}
		return nil, fmt.Errorf("insert community: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted communities ordered newest first.
func (r *CommunityRepository) FindAll(ctx context.Context) ([]models.Community, error) {
	q := communityBaseSelect + ` WHERE c.deleted_at IS NULL ORDER BY c.created_at DESC`
	return r.scanCommunities(ctx, q)
}

// FindAllForUser returns the non-deleted, active communities the given user
// is a member of — used to scope a student's Community page to their own
// batch(es) instead of every community on the platform.
func (r *CommunityRepository) FindAllForUser(ctx context.Context, userID string) ([]models.Community, error) {
	q := communityBaseSelect + `
		JOIN community_members cm ON cm.community_id = c.id AND cm.user_id = $1
		WHERE c.deleted_at IS NULL AND c.is_active = TRUE
		ORDER BY c.created_at DESC`
	return r.scanCommunities(ctx, q, userID)
}

// IsMember reports whether userID belongs to the community identified by
// communityShortID — used to gate posting/commenting/liking to members.
func (r *CommunityRepository) IsMember(ctx context.Context, communityShortID, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM community_members cm
			JOIN communities c ON c.id = cm.community_id
			WHERE c.short_id = $1 AND cm.user_id = $2
		)`, communityShortID, userID).Scan(&exists)
	return exists, err
}

// FindByShortID returns a single non-deleted community.
func (r *CommunityRepository) FindByShortID(ctx context.Context, shortID string) (*models.Community, error) {
	q := communityBaseSelect + ` WHERE c.short_id = $1 AND c.deleted_at IS NULL LIMIT 1`

	c, err := scanCommunity(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Update applies a partial update — only non-nil fields are changed.
func (r *CommunityRepository) Update(ctx context.Context, shortID string, in models.UpdateCommunityInput) error {
	args := []interface{}{shortID}
	setClauses := []string{"updated_at = NOW()"}
	i := 2

	if in.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", i))
		args = append(args, *in.Name)
		i++
	}
	if in.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = NULLIF($%d,'')", i))
		args = append(args, *in.Description)
		i++
	}
	if in.BatchShortID != nil {
		setClauses = append(setClauses, fmt.Sprintf(
			"batch_id = (SELECT id FROM batches WHERE short_id = $%d AND deleted_at IS NULL)", i,
		))
		args = append(args, *in.BatchShortID)
		i++
	}
	if in.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", i))
		args = append(args, *in.IsActive)
		i++
	}

	if len(setClauses) == 1 {
		return fmt.Errorf("no fields to update")
	}

	q := fmt.Sprintf(
		"UPDATE communities SET %s WHERE short_id = $1 AND deleted_at IS NULL",
		strings.Join(setClauses, ", "),
	)
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Delete soft-deletes a community.
func (r *CommunityRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE communities SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// AddMembers bulk-adds users to a community by user ID. Users already in the
// community are left unchanged. Returns the IDs of members actually added
// (excludes IDs that were already members).
func (r *CommunityRepository) AddMembers(ctx context.Context, communityShortID string, userIDs []string, addedBy string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		INSERT INTO community_members (community_id, user_id, added_by)
		SELECT c.id, u.id, $3
		FROM communities c
		CROSS JOIN unnest($2::uuid[]) AS uid(user_id)
		JOIN users u ON u.id = uid.user_id AND u.deleted_at IS NULL
		WHERE c.short_id = $1 AND c.deleted_at IS NULL
		ON CONFLICT (community_id, user_id) DO NOTHING
		RETURNING user_id`,
		communityShortID, userIDs, addedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("add members: %w", err)
	}
	defer rows.Close()

	var added []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		added = append(added, id)
	}
	return added, rows.Err()
}

// AddMembersByBatchShortID adds the given users to every community tied to
// batchShortID (normally exactly one — a batch's auto-created community).
// Used to keep community membership in sync when students are added to a
// batch, so admins don't have to separately re-add them in Community. A
// batch with no community is a no-op, not an error — communities are an
// optional feature per college.
func (r *CommunityRepository) AddMembersByBatchShortID(ctx context.Context, batchShortID string, userIDs []string, addedBy string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO community_members (community_id, user_id, added_by)
		SELECT c.id, u.id, $3
		FROM communities c
		JOIN batches b ON c.batch_id = b.id
		CROSS JOIN unnest($2::uuid[]) AS uid(user_id)
		JOIN users u ON u.id = uid.user_id AND u.deleted_at IS NULL
		WHERE b.short_id = $1 AND c.deleted_at IS NULL AND b.deleted_at IS NULL
		ON CONFLICT (community_id, user_id) DO NOTHING`,
		batchShortID, userIDs, addedBy,
	)
	return err
}

// RemoveMemberByBatchShortID removes a user from every community tied to
// batchShortID — the counterpart to AddMembersByBatchShortID, used when a
// student leaves/transfers out of a batch. No-op if there's no community.
func (r *CommunityRepository) RemoveMemberByBatchShortID(ctx context.Context, batchShortID, userID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM community_members
		WHERE user_id = $2::uuid
		  AND community_id IN (
		      SELECT c.id FROM communities c
		      JOIN batches b ON c.batch_id = b.id
		      WHERE b.short_id = $1 AND c.deleted_at IS NULL
		  )`,
		batchShortID, userID,
	)
	return err
}

// RemoveMember removes a single user from a community.
func (r *CommunityRepository) RemoveMember(ctx context.Context, communityShortID, userID string) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM community_members
		WHERE community_id = (SELECT id FROM communities WHERE short_id = $1 AND deleted_at IS NULL)
		  AND user_id = $2::uuid`,
		communityShortID, userID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetMembers returns every member of a community with their user details.
func (r *CommunityRepository) GetMembers(ctx context.Context, communityShortID string) ([]models.CommunityMember, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.first_name, u.last_name, u.email, u.role, cm.joined_at
		FROM community_members cm
		JOIN users u ON u.id = cm.user_id AND u.deleted_at IS NULL
		WHERE cm.community_id = (SELECT id FROM communities WHERE short_id = $1 AND deleted_at IS NULL)
		ORDER BY cm.joined_at DESC`,
		communityShortID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.CommunityMember
	for rows.Next() {
		var m models.CommunityMember
		if err := rows.Scan(&m.UserID, &m.FirstName, &m.LastName, &m.Email, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *CommunityRepository) scanCommunities(ctx context.Context, q string, args ...interface{}) ([]models.Community, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var communities []models.Community
	for rows.Next() {
		var c models.Community
		if err := rows.Scan(
			&c.ID, &c.ShortID, &c.Name, &c.Description,
			&c.BatchID, &c.BatchShortID, &c.BatchNumber,
			&c.IsActive, &c.CreatedBy, &c.CreatedByName,
			&c.MemberCount, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		communities = append(communities, c)
	}
	return communities, rows.Err()
}
