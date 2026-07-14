-- ============================================================== --
-- Password reset OTPs, for the "forgot password" flow (email OTP,
-- 5-minute expiry). The OTP is bcrypt-hashed before storage, same
-- as passwords — the plaintext code only ever exists in the email.
-- One row per requested OTP; old rows are cheap and harmless, so
-- there's no cleanup job for now.
-- ============================================================== --
CREATE TABLE IF NOT EXISTS password_reset_otps (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    otp_hash   TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_password_reset_otps_user_id ON password_reset_otps(user_id);
-- End of file --
