package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DevicePushTokenRepository struct {
	pool *pgxpool.Pool
}

func NewDevicePushTokenRepository(pool *pgxpool.Pool) *DevicePushTokenRepository {
	return &DevicePushTokenRepository{pool: pool}
}

// Upsert registers (or re-registers) a device token against a user. The
// token column is globally unique, so if the same device was previously
// registered under a different account (e.g. a shared phone, or a re-login
// as someone else), this simply reassigns it rather than erroring or
// leaving a duplicate/stale row behind.
func (r *DevicePushTokenRepository) Upsert(ctx context.Context, userID, token, platform string) error {
	if platform == "" {
		platform = "unknown"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO device_push_tokens (user_id, token, platform)
		VALUES ($1::UUID, $2, $3)
		ON CONFLICT (token) DO UPDATE
		SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform, updated_at = NOW()`,
		userID, token, platform,
	)
	return err
}

// Delete unregisters a token (e.g. on logout) so it stops receiving pushes.
func (r *DevicePushTokenRepository) Delete(ctx context.Context, token string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM device_push_tokens WHERE token = $1`, token)
	return err
}

// FindTokensByUserIDs returns every registered device token for the given
// users — used to fan a notification out to Expo push alongside the normal
// in-app notification_recipients row.
func (r *DevicePushTokenRepository) FindTokensByUserIDs(ctx context.Context, userIDs []string) ([]string, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT token FROM device_push_tokens WHERE user_id = ANY($1::uuid[])`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}
