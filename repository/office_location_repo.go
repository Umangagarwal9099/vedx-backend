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

type OfficeLocationRepository struct {
	pool *pgxpool.Pool
}

func NewOfficeLocationRepository(pool *pgxpool.Pool) *OfficeLocationRepository {
	return &OfficeLocationRepository{pool: pool}
}

const officeLocationBaseSelect = `
	SELECT id, short_id, name, COALESCE(address, ''), latitude, longitude, radius_meters, is_active, created_at, updated_at
	FROM office_locations`

func scanOfficeLocation(row pgx.Row) (models.OfficeLocation, error) {
	var o models.OfficeLocation
	err := row.Scan(
		&o.ID, &o.ShortID, &o.Name, &o.Address, &o.Latitude, &o.Longitude,
		&o.RadiusMeters, &o.IsActive, &o.CreatedAt, &o.UpdatedAt,
	)
	return o, err
}

func (r *OfficeLocationRepository) scanAll(ctx context.Context, q string, args ...interface{}) ([]models.OfficeLocation, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.OfficeLocation
	for rows.Next() {
		o, err := scanOfficeLocation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Create inserts a new office location.
func (r *OfficeLocationRepository) Create(ctx context.Context, in models.CreateOfficeLocationInput) (*models.OfficeLocation, error) {
	insSelect := strings.Replace(officeLocationBaseSelect, "FROM office_locations", "FROM ins", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		o, err := scanOfficeLocation(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO office_locations (short_id, name, address, latitude, longitude, radius_meters)
				VALUES ($1, $2, NULLIF($3,''), $4, $5, $6)
				RETURNING *
			)
			%s`, insSelect),
			shortID, in.Name, in.Address, in.Latitude, in.Longitude, in.RadiusMeters,
		))
		if err == nil {
			return &o, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert office location: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// GetAll returns every office location, active or not — admin management view.
func (r *OfficeLocationRepository) GetAll(ctx context.Context) ([]models.OfficeLocation, error) {
	q := officeLocationBaseSelect + " ORDER BY name"
	return r.scanAll(ctx, q)
}

// GetAllActive returns only active office locations — used by the
// check-in/out geofence check.
func (r *OfficeLocationRepository) GetAllActive(ctx context.Context) ([]models.OfficeLocation, error) {
	q := officeLocationBaseSelect + " WHERE is_active = TRUE ORDER BY name"
	return r.scanAll(ctx, q)
}

// Update applies a partial update.
func (r *OfficeLocationRepository) Update(ctx context.Context, shortID string, in models.UpdateOfficeLocationInput) (*models.OfficeLocation, error) {
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
	if in.Address != nil {
		add("address = NULLIF($%d,'')", *in.Address)
	}
	if in.Latitude != nil {
		add("latitude = $%d", *in.Latitude)
	}
	if in.Longitude != nil {
		add("longitude = $%d", *in.Longitude)
	}
	if in.RadiusMeters != nil {
		add("radius_meters = $%d", *in.RadiusMeters)
	}
	if in.IsActive != nil {
		add("is_active = $%d", *in.IsActive)
	}

	if len(setClauses) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf("UPDATE office_locations SET %s WHERE short_id = $1 RETURNING short_id", strings.Join(setClauses, ", "))
	var returnedShortID string
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&returnedShortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}

	q2 := fmt.Sprintf("%s WHERE short_id = $1", officeLocationBaseSelect)
	o, err := scanOfficeLocation(r.pool.QueryRow(ctx, q2, shortID))
	return &o, err
}

// Delete permanently removes an office location.
func (r *OfficeLocationRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM office_locations WHERE short_id = $1`, shortID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
