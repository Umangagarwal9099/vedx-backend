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

type EventRepository struct {
	pool *pgxpool.Pool
}

func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

// eventBaseSelect joins users (mentor) to surface manager name alongside ID.
const eventBaseSelect = `
	SELECT e.id, e.short_id, e.name,
	       TO_CHAR(e.event_date, 'YYYY-MM-DD'),
	       TO_CHAR(e.start_time, 'HH24:MI'),
	       TO_CHAR(e.end_time,   'HH24:MI'),
	       COALESCE(e.image_url, ''),
	       COALESCE(e.description, ''),
	       e.status::TEXT,
	       e.mode::TEXT,
	       e.guest_access,
	       COALESCE(e.event_manager::TEXT, ''),
	       COALESCE(CONCAT(em.first_name, ' ', em.last_name), ''),
	       COALESCE(e.categories, '{}'),
	       e.is_active,
	       e.created_by::TEXT,
	       e.created_at, e.updated_at
	FROM events e
	LEFT JOIN users em ON e.event_manager = em.id AND em.deleted_at IS NULL`

func scanEvent(row pgx.Row) (*models.Event, error) {
	var e models.Event
	err := row.Scan(
		&e.ID, &e.ShortID, &e.Name,
		&e.EventDate, &e.StartTime, &e.EndTime,
		&e.ImageURL, &e.Description,
		&e.Status, &e.Mode,
		&e.GuestAccess,
		&e.EventManagerID,
		&e.EventManagerName,
		&e.Categories,
		&e.IsActive,
		&e.CreatedBy,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// Create inserts a new event, retrying up to 3 times on short_id collision.
func (r *EventRepository) Create(ctx context.Context, in models.CreateEventInput, createdBy string) (*models.Event, error) {
	cats := in.Categories
	if cats == nil {
		cats = []string{}
	}

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		row := r.pool.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO events
				  (short_id, name, event_date, start_time, end_time, image_url, description,
				   status, mode, guest_access, event_manager, categories, created_by)
				VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),
				        $8::event_status,$9::event_mode,$10,
				        NULLIF($11,'')::UUID, $12, $13::UUID)
				RETURNING *
			)
			SELECT ins.id, ins.short_id, ins.name,
			       TO_CHAR(ins.event_date, 'YYYY-MM-DD'),
			       TO_CHAR(ins.start_time, 'HH24:MI'),
			       TO_CHAR(ins.end_time,   'HH24:MI'),
			       COALESCE(ins.image_url, ''),
			       COALESCE(ins.description, ''),
			       ins.status::TEXT,
			       ins.mode::TEXT,
			       ins.guest_access,
			       COALESCE(ins.event_manager::TEXT, ''),
			       COALESCE(CONCAT(em.first_name, ' ', em.last_name), ''),
			       COALESCE(ins.categories, '{}'),
			       ins.is_active,
			       ins.created_by::TEXT,
			       ins.created_at, ins.updated_at
			FROM ins
			LEFT JOIN users em ON ins.event_manager = em.id AND em.deleted_at IS NULL`,
			shortID, in.Name, in.EventDate, in.StartTime, in.EndTime,
			in.ImageURL, in.Description,
			in.Status, in.Mode, in.GuestAccess,
			in.EventManager, cats, createdBy,
		)

		e, err := scanEvent(row)
		if err == nil {
			return e, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert event: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted events ordered newest first.
func (r *EventRepository) FindAll(ctx context.Context) ([]models.Event, error) {
	q := eventBaseSelect + ` WHERE e.deleted_at IS NULL ORDER BY e.created_at DESC`
	return r.scanEvents(ctx, q)
}

// FindByShortID returns a single non-deleted event.
func (r *EventRepository) FindByShortID(ctx context.Context, shortID string) (*models.Event, error) {
	q := eventBaseSelect + ` WHERE e.short_id = $1 AND e.deleted_at IS NULL LIMIT 1`
	e, err := scanEvent(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

// Search filters non-deleted events by name (ILIKE) and/or exact status.
func (r *EventRepository) Search(ctx context.Context, f models.EventFilter) ([]models.Event, error) {
	args := []interface{}{}
	where := []string{"e.deleted_at IS NULL"}
	i := 1

	if f.Name != "" {
		where = append(where, fmt.Sprintf("e.name ILIKE $%d", i))
		args = append(args, "%"+f.Name+"%")
		i++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("e.status = $%d::event_status", i))
		args = append(args, f.Status)
		i++
	}

	q := eventBaseSelect + ` WHERE ` + strings.Join(where, " AND ") + ` ORDER BY e.created_at DESC`
	return r.scanEvents(ctx, q, args...)
}

// Update applies a partial update — only non-nil fields are changed.
func (r *EventRepository) Update(ctx context.Context, shortID string, in models.UpdateEventInput) error {
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
	if in.EventDate != nil {
		add("event_date = $%d", *in.EventDate)
	}
	if in.StartTime != nil {
		add("start_time = $%d", *in.StartTime)
	}
	if in.EndTime != nil {
		add("end_time = $%d", *in.EndTime)
	}
	if in.ImageURL != nil {
		add("image_url = NULLIF($%d,'')", *in.ImageURL)
	}
	if in.Description != nil {
		add("description = NULLIF($%d,'')", *in.Description)
	}
	if in.Status != nil {
		add("status = $%d::event_status", *in.Status)
	}
	if in.Mode != nil {
		add("mode = $%d::event_mode", *in.Mode)
	}
	if in.GuestAccess != nil {
		add("guest_access = $%d", *in.GuestAccess)
	}
	if in.EventManager != nil {
		add("event_manager = NULLIF($%d,'')::UUID", *in.EventManager)
	}
	if in.Categories != nil {
		add("categories = $%d", in.Categories)
	}
	if in.IsActive != nil {
		add("is_active = $%d", *in.IsActive)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf(
		"UPDATE events SET %s WHERE short_id = $1 AND deleted_at IS NULL",
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

// Delete soft-deletes an event by its short_id.
func (r *EventRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE events SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *EventRepository) scanEvents(ctx context.Context, q string, args ...interface{}) ([]models.Event, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(
			&e.ID, &e.ShortID, &e.Name,
			&e.EventDate, &e.StartTime, &e.EndTime,
			&e.ImageURL, &e.Description,
			&e.Status, &e.Mode,
			&e.GuestAccess,
			&e.EventManagerID,
			&e.EventManagerName,
			&e.Categories,
			&e.IsActive,
			&e.CreatedBy,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
