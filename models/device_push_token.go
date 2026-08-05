package models

import "time"

// DevicePushToken is one mobile device's Expo push token, registered by an
// authenticated user from the VedXlence mobile app. A token is unique across
// the whole table (not per-user) so that re-registering the same device
// under a different account — logout/login as someone else on a shared
// phone — cleanly moves ownership via upsert rather than leaving stale rows.
type DevicePushToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	Platform  string    `json:"platform"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RegisterPushTokenInput struct {
	Token    string `json:"token"    binding:"required"`
	Platform string `json:"platform" binding:"omitempty,oneof=ios android web unknown"`
}

type UnregisterPushTokenInput struct {
	Token string `json:"token" binding:"required"`
}
