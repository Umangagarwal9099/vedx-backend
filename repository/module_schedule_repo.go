package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

type ModuleScheduleRepository struct {
	pool *pgxpool.Pool
}

func NewModuleScheduleRepository(pool *pgxpool.Pool) *ModuleScheduleRepository {
	return &ModuleScheduleRepository{pool: pool}
}

// Upsert schedules (or reschedules) when moduleID becomes visible to batchID's students.
func (r *ModuleScheduleRepository) Upsert(ctx context.Context, batchID, moduleID, releaseDate string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO batch_module_schedule (batch_id, module_id, release_date)
		VALUES ($1::UUID, $2::UUID, $3::DATE)
		ON CONFLICT (batch_id, module_id) DO UPDATE SET release_date = EXCLUDED.release_date, updated_at = NOW()`,
		batchID, moduleID, releaseDate,
	)
	return err
}

// Delete removes a module's schedule for a batch, reverting it to
// "released from day one".
func (r *ModuleScheduleRepository) Delete(ctx context.Context, batchID, moduleID string) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM batch_module_schedule WHERE batch_id = $1::UUID AND module_id = $2::UUID`,
		batchID, moduleID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetForBatch returns module_short_id -> release_date for every module
// scheduled in a batch — used to annotate the batch-scoped curriculum view.
func (r *ModuleScheduleRepository) GetForBatch(ctx context.Context, batchID string) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.short_id, bms.release_date::TEXT
		FROM batch_module_schedule bms
		JOIN modules m ON bms.module_id = m.id
		WHERE bms.batch_id = $1::UUID`,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var shortID, releaseDate string
		if err := rows.Scan(&shortID, &releaseDate); err != nil {
			return nil, err
		}
		out[shortID] = releaseDate
	}
	return out, rows.Err()
}

// ListForBatch returns every configured module-schedule row for a batch,
// with module names, for the admin scheduling UI.
func (r *ModuleScheduleRepository) ListForBatch(ctx context.Context, batchID string) ([]models.ModuleScheduleEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.short_id, m.module_name, bms.release_date::TEXT
		FROM batch_module_schedule bms
		JOIN modules m ON bms.module_id = m.id
		WHERE bms.batch_id = $1::UUID
		ORDER BY bms.release_date`,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ModuleScheduleEntry
	for rows.Next() {
		var e models.ModuleScheduleEntry
		if err := rows.Scan(&e.ModuleShortID, &e.ModuleName, &e.ReleaseDate); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
