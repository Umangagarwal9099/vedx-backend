package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type LoginActivityRepository struct {
	pool *pgxpool.Pool
}

func NewLoginActivityRepository(pool *pgxpool.Pool) *LoginActivityRepository {
	return &LoginActivityRepository{pool: pool}
}

// Upsert records a login from the given device — inserting a new row on
// first sight, or incrementing login_count/bumping last_login_at on repeat
// logins from the same (user_id, device_id) pair. Best-effort: the caller
// should not fail the login itself if this errors.
func (r *LoginActivityRepository) Upsert(ctx context.Context, userID, deviceID string, ua util.ParsedUserAgent) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO login_activity (user_id, device_id, device_type, os_name, browser_name, browser_version, login_count, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, 1, NOW())
		ON CONFLICT (user_id, device_id) DO UPDATE SET
			device_type = EXCLUDED.device_type,
			os_name = EXCLUDED.os_name,
			browser_name = EXCLUDED.browser_name,
			browser_version = EXCLUDED.browser_version,
			login_count = login_activity.login_count + 1,
			last_login_at = NOW(),
			removed_at = NULL`,
		userID, deviceID, ua.DeviceType, ua.OSName, ua.BrowserName, ua.BrowserVersion,
	)
	return err
}

// FindAllForUser returns every device (including removed ones) for a user,
// most recently active first.
func (r *LoginActivityRepository) FindAllForUser(ctx context.Context, userID string) ([]models.LoginActivityDevice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, device_id, device_type, os_name, browser_name, browser_version,
		       login_count, last_login_at, removed_at, created_at
		FROM login_activity
		WHERE user_id = $1
		ORDER BY last_login_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.LoginActivityDevice
	for rows.Next() {
		var d models.LoginActivityDevice
		if err := rows.Scan(&d.ID, &d.DeviceID, &d.DeviceType, &d.OSName, &d.BrowserName, &d.BrowserVersion,
			&d.LoginCount, &d.LastLoginAt, &d.RemovedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RemoveDevice soft-removes a device from a user's activity list.
func (r *LoginActivityRepository) RemoveDevice(ctx context.Context, userID, deviceID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE login_activity SET removed_at = NOW()
		WHERE user_id = $1 AND device_id = $2 AND removed_at IS NULL`,
		userID, deviceID,
	)
	return err
}
