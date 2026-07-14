package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

type PasswordResetRepository struct {
	pool *pgxpool.Pool
}

func NewPasswordResetRepository(pool *pgxpool.Pool) *PasswordResetRepository {
	return &PasswordResetRepository{pool: pool}
}

// CreateOTP discards any OTPs still outstanding for this user and inserts a
// fresh one, so only the most recently requested code is ever valid.
func (r *PasswordResetRepository) CreateOTP(ctx context.Context, userID, otpHash string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM password_reset_otps WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO password_reset_otps (user_id, otp_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, otpHash, expiresAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FindLatestOTP returns the most recently issued OTP for a user, regardless
// of whether it's expired or already used — the caller decides how to react.
func (r *PasswordResetRepository) FindLatestOTP(ctx context.Context, userID string) (*models.PasswordResetOTP, error) {
	const q = `
		SELECT id, user_id, otp_hash, expires_at, used_at, created_at
		FROM password_reset_otps
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	var o models.PasswordResetOTP
	err := r.pool.QueryRow(ctx, q, userID).Scan(
		&o.ID, &o.UserID, &o.OTPHash, &o.ExpiresAt, &o.UsedAt, &o.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// MarkOTPUsed flags an OTP row as consumed so it can't be replayed.
func (r *PasswordResetRepository) MarkOTPUsed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE password_reset_otps SET used_at = NOW() WHERE id = $1`, id)
	return err
}
