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

type AnnouncementRepository struct {
	pool *pgxpool.Pool
}

func NewAnnouncementRepository(pool *pgxpool.Pool) *AnnouncementRepository {
	return &AnnouncementRepository{pool: pool}
}

const announcementSelectCols = `
	id, short_id, name,
	COALESCE(description, ''),
	COALESCE(image_url, ''),
	urgency::TEXT,
	visibility::TEXT,
	is_active,
	created_by::TEXT,
	created_at, updated_at`

func scanAnnouncement(row pgx.Row) (*models.Announcement, error) {
	var a models.Announcement
	err := row.Scan(
		&a.ID, &a.ShortID, &a.Name,
		&a.Description, &a.ImageURL,
		&a.Urgency, &a.Visibility,
		&a.IsActive,
		&a.CreatedBy,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Create inserts a new announcement, retrying up to 3 times on short_id collision.
func (r *AnnouncementRepository) Create(ctx context.Context, in models.CreateAnnouncementInput, createdBy string) (*models.Announcement, error) {
	q := fmt.Sprintf(`
		INSERT INTO announcements (short_id, name, description, image_url, urgency, visibility, created_by)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5::announcement_urgency, $6::announcement_visibility, $7::UUID)
		RETURNING %s`, announcementSelectCols)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		a, err := scanAnnouncement(r.pool.QueryRow(ctx, q,
			shortID, in.Name, in.Description, in.ImageURL,
			in.Urgency, in.Visibility, createdBy,
		))
		if err == nil {
			return a, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert announcement: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted announcements ordered newest first.
func (r *AnnouncementRepository) FindAll(ctx context.Context) ([]models.Announcement, error) {
	q := fmt.Sprintf(`SELECT %s FROM announcements WHERE deleted_at IS NULL ORDER BY created_at DESC`, announcementSelectCols)
	return r.scanAnnouncements(ctx, q)
}

// FindByShortID returns a single non-deleted announcement.
func (r *AnnouncementRepository) FindByShortID(ctx context.Context, shortID string) (*models.Announcement, error) {
	q := fmt.Sprintf(`SELECT %s FROM announcements WHERE short_id = $1 AND deleted_at IS NULL LIMIT 1`, announcementSelectCols)
	a, err := scanAnnouncement(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// Search filters non-deleted announcements by name, urgency, and/or is_active.
func (r *AnnouncementRepository) Search(ctx context.Context, f models.AnnouncementFilter) ([]models.Announcement, error) {
	args := []interface{}{}
	where := []string{"deleted_at IS NULL"}
	i := 1

	if f.Name != "" {
		where = append(where, fmt.Sprintf("name ILIKE $%d", i))
		args = append(args, "%"+f.Name+"%")
		i++
	}
	if f.Urgency != "" {
		where = append(where, fmt.Sprintf("urgency = $%d::announcement_urgency", i))
		args = append(args, f.Urgency)
		i++
	}
	if f.IsActive == "true" {
		where = append(where, "is_active = TRUE")
	} else if f.IsActive == "false" {
		where = append(where, "is_active = FALSE")
	}

	q := fmt.Sprintf(
		`SELECT %s FROM announcements WHERE %s ORDER BY created_at DESC`,
		announcementSelectCols, strings.Join(where, " AND "),
	)
	return r.scanAnnouncements(ctx, q, args...)
}

// Update applies a partial update — only non-nil fields are changed.
func (r *AnnouncementRepository) Update(ctx context.Context, shortID string, in models.UpdateAnnouncementInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.Name != nil {
		add("name = $%d", *in.Name)
	}
	if in.Description != nil {
		add("description = NULLIF($%d,'')", *in.Description)
	}
	if in.ImageURL != nil {
		add("image_url = NULLIF($%d,'')", *in.ImageURL)
	}
	if in.Urgency != nil {
		add("urgency = $%d::announcement_urgency", *in.Urgency)
	}
	if in.Visibility != nil {
		add("visibility = $%d::announcement_visibility", *in.Visibility)
	}
	if in.IsActive != nil {
		add("is_active = $%d", *in.IsActive)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf(
		"UPDATE announcements SET %s WHERE short_id = $1 AND deleted_at IS NULL",
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

// Delete soft-deletes an announcement by its short_id.
func (r *AnnouncementRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE announcements SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *AnnouncementRepository) scanAnnouncements(ctx context.Context, q string, args ...interface{}) ([]models.Announcement, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var announcements []models.Announcement
	for rows.Next() {
		var a models.Announcement
		if err := rows.Scan(
			&a.ID, &a.ShortID, &a.Name,
			&a.Description, &a.ImageURL,
			&a.Urgency, &a.Visibility,
			&a.IsActive,
			&a.CreatedBy,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		announcements = append(announcements, a)
	}
	return announcements, rows.Err()
}
