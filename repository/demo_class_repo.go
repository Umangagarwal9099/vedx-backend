package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

type DemoClassRepository struct {
	pool *pgxpool.Pool
}

func NewDemoClassRepository(pool *pgxpool.Pool) *DemoClassRepository {
	return &DemoClassRepository{pool: pool}
}

// ListDemoBatches returns every non-deleted batch that has at least one
// demo-marked session recording or uploaded recording, newest course first,
// along with how many demo videos each one has (across both sources).
func (r *DemoClassRepository) ListDemoBatches(ctx context.Context) ([]models.DemoBatchSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT b.short_id, b.batch_number, c.name, c.short_id,
		       (SELECT COUNT(*) FROM sessions s WHERE s.batch_id = b.id AND s.is_demo AND s.deleted_at IS NULL)
		     + (SELECT COUNT(*) FROM batch_recordings r WHERE r.batch_id = b.id AND r.is_demo AND r.deleted_at IS NULL) AS demo_count
		FROM batches b
		JOIN courses c ON b.course_id = c.id AND c.deleted_at IS NULL
		WHERE b.deleted_at IS NULL
		  AND (
		    EXISTS (SELECT 1 FROM sessions s WHERE s.batch_id = b.id AND s.is_demo AND s.deleted_at IS NULL)
		    OR EXISTS (SELECT 1 FROM batch_recordings r WHERE r.batch_id = b.id AND r.is_demo AND r.deleted_at IS NULL)
		  )
		ORDER BY c.name ASC, b.batch_number ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.DemoBatchSummary{}
	for rows.Next() {
		var s models.DemoBatchSummary
		if err := rows.Scan(&s.BatchShortID, &s.BatchNumber, &s.CourseName, &s.CourseShortID, &s.DemoVideoCount); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
