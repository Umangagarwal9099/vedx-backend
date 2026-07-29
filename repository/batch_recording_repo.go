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

type BatchRecordingRepository struct {
	pool *pgxpool.Pool
}

func NewBatchRecordingRepository(pool *pgxpool.Pool) *BatchRecordingRepository {
	return &BatchRecordingRepository{pool: pool}
}

const batchRecordingBaseSelect = `
	SELECT r.id, r.short_id, r.batch_id, b.short_id, b.batch_number,
	       r.title, r.url, COALESCE(r.file_size, 0), COALESCE(r.content_type, ''),
	       r.uploaded_by, CONCAT(u.first_name, ' ', u.last_name),
	       r.created_at, r.updated_at
	FROM batch_recordings r
	JOIN batches b ON r.batch_id    = b.id AND b.deleted_at IS NULL
	JOIN users   u ON r.uploaded_by = u.id AND u.deleted_at IS NULL`

func scanBatchRecording(row pgx.Row) (models.BatchRecording, error) {
	var r models.BatchRecording
	err := row.Scan(
		&r.ID, &r.ShortID, &r.BatchID, &r.BatchShortID, &r.BatchNumber,
		&r.Title, &r.URL, &r.FileSize, &r.ContentType,
		&r.UploadedBy, &r.UploadedByName,
		&r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
}

// Create inserts one uploaded recording under batchShortID, retrying up to
// 3 times on short_id collision. Call it once per file for a multi-file
// upload — there is no bulk-insert variant since each file needs its own
// storage upload (and therefore its own error handling) before it can be
// persisted.
func (r *BatchRecordingRepository) Create(ctx context.Context, batchShortID, title, url string, fileSize int64, contentType, uploadedBy string) (*models.BatchRecording, error) {
	insSelect := strings.Replace(batchRecordingBaseSelect, "FROM batch_recordings r", "FROM ins r", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		rec, err := scanBatchRecording(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO batch_recordings (
					short_id, batch_id, title, url, file_size, content_type, uploaded_by
				) VALUES (
					$1,
					(SELECT id FROM batches WHERE short_id = $2 AND deleted_at IS NULL),
					$3, $4, NULLIF($5, 0), NULLIF($6, ''), $7::UUID
				)
				RETURNING *
			)
			%s`, insSelect),
			shortID, batchShortID, title, url, fileSize, contentType, uploadedBy,
		))
		if err == nil {
			return &rec, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert batch recording: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindByBatchShortID returns every non-deleted uploaded recording for a
// batch, newest first.
func (r *BatchRecordingRepository) FindByBatchShortID(ctx context.Context, batchShortID string) ([]models.BatchRecording, error) {
	q := fmt.Sprintf(`%s WHERE b.short_id = $1 AND r.deleted_at IS NULL ORDER BY r.created_at DESC`, batchRecordingBaseSelect)
	rows, err := r.pool.Query(ctx, q, batchShortID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.BatchRecording
	for rows.Next() {
		rec, err := scanBatchRecording(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// FindByShortID returns a single non-deleted uploaded recording.
func (r *BatchRecordingRepository) FindByShortID(ctx context.Context, shortID string) (*models.BatchRecording, error) {
	q := fmt.Sprintf(`%s WHERE r.short_id = $1 AND r.deleted_at IS NULL`, batchRecordingBaseSelect)
	rec, err := scanBatchRecording(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &rec, err
}

// Delete soft-deletes an uploaded recording. Does not remove the underlying
// R2 object — this only hides it from API responses, matching the
// soft-delete convention used elsewhere in this codebase (e.g. resources).
func (r *BatchRecordingRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE batch_recordings SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
