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
	"github.com/umangagarwal/vedx-backend/service"
	"github.com/umangagarwal/vedx-backend/util"
)

type NotificationRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

// allRoles lists every valid user role — expanded from the "all" target.
var allRoles = []string{
	string(models.RoleStudent), string(models.RoleMentor), string(models.RoleEmployee),
	string(models.RoleTeamLead), string(models.RoleSuperAdmin),
}

func isValidRole(role string) bool {
	for _, r := range allRoles {
		if r == role {
			return true
		}
	}
	return false
}

// resolveRoles expands a caller-supplied role list (which may include "all")
// into the concrete set of role strings to query for.
func resolveRoles(roles []string) ([]string, error) {
	for _, r := range roles {
		if r == "all" {
			return allRoles, nil
		}
		if !isValidRole(r) {
			return nil, fmt.Errorf("invalid role: %s", r)
		}
	}
	return roles, nil
}

// Create broadcasts a manually-authored notification to one or more roles (or "all").
// Used by POST /notifications.
func (r *NotificationRepository) Create(ctx context.Context, in models.CreateNotificationInput, createdBy string) (*models.Notification, error) {
	roles, err := resolveRoles(in.Roles)
	if err != nil {
		return nil, err
	}

	notifType := in.Type
	if notifType == "" {
		notifType = "general"
	}

	return r.notify(ctx, in.Title, in.Message, notifType, "", "", createdBy, roles, nil)
}

// NotifyRoles sends a system-generated notification to every user with one of the given roles.
// Used internally whenever another entity (course, batch, event, ...) is created.
func (r *NotificationRepository) NotifyRoles(ctx context.Context, title, message, notifType, refType, refShortID, createdBy string, roles []string) error {
	_, err := r.notify(ctx, title, message, notifType, refType, refShortID, createdBy, roles, nil)
	return err
}

// NotifyUsers sends a system-generated notification to a specific set of users.
// Used internally, e.g. when members are added to a community.
func (r *NotificationRepository) NotifyUsers(ctx context.Context, title, message, notifType, refType, refShortID, createdBy string, userIDs []string) error {
	_, err := r.notify(ctx, title, message, notifType, refType, refShortID, createdBy, nil, userIDs)
	return err
}

// notify creates the shared notification row, resolves the recipient set from
// roles and/or explicit user IDs (excluding the actor), and fans it out to
// notification_recipients.
func (r *NotificationRepository) notify(
	ctx context.Context,
	title, message, notifType, refType, refShortID, createdBy string,
	roles []string, userIDs []string,
) (*models.Notification, error) {
	recipients := map[string]struct{}{}

	if len(roles) > 0 {
		rows, err := r.pool.Query(ctx,
			`SELECT id FROM users WHERE deleted_at IS NULL AND id <> $1 AND role::TEXT = ANY($2::text[])`,
			createdBy, roles,
		)
		if err != nil {
			return nil, fmt.Errorf("resolve role recipients: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			recipients[id] = struct{}{}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}

	if len(userIDs) > 0 {
		rows, err := r.pool.Query(ctx,
			`SELECT id FROM users WHERE deleted_at IS NULL AND id <> $1 AND id = ANY($2::uuid[])`,
			createdBy, userIDs,
		)
		if err != nil {
			return nil, fmt.Errorf("resolve user recipients: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			recipients[id] = struct{}{}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}

	notif, err := r.insertNotification(ctx, title, message, notifType, refType, refShortID, createdBy)
	if err != nil {
		return nil, err
	}

	if len(recipients) > 0 {
		ids := make([]string, 0, len(recipients))
		for id := range recipients {
			ids = append(ids, id)
		}
		_, err := r.pool.Exec(ctx, `
			INSERT INTO notification_recipients (notification_id, user_id)
			SELECT $1, uid FROM unnest($2::uuid[]) AS uid
			ON CONFLICT (notification_id, user_id) DO NOTHING`,
			notif.ID, ids,
		)
		if err != nil {
			return nil, fmt.Errorf("insert recipients: %w", err)
		}
		r.pushToRecipients(ctx, ids, notif)
	}

	return notif, nil
}

// pushToRecipients best-effort fans the notification out to every
// recipient's registered mobile device(s) via Expo push, alongside the
// in-app notification_recipients row just written. A recipient with no
// registered device is the common case (web-only user), not an error.
func (r *NotificationRepository) pushToRecipients(ctx context.Context, userIDs []string, notif *models.Notification) {
	tokens, err := r.pool.Query(ctx, `SELECT token FROM device_push_tokens WHERE user_id = ANY($1::uuid[])`, userIDs)
	if err != nil {
		return
	}
	defer tokens.Close()
	var toks []string
	for tokens.Next() {
		var t string
		if tokens.Scan(&t) == nil {
			toks = append(toks, t)
		}
	}
	if len(toks) == 0 {
		return
	}
	service.SendExpoPushAsync(toks, notif.Title, notif.Message, map[string]interface{}{
		"type":            notif.Type,
		"ref_type":        notif.RefType,
		"ref_short_id":    notif.RefShortID,
		"notification_id": notif.ShortID,
	})
}

// insertNotification inserts the shared notification row, retrying on short_id collision.
func (r *NotificationRepository) insertNotification(ctx context.Context, title, message, notifType, refType, refShortID, createdBy string) (*models.Notification, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		var n models.Notification
		err := r.pool.QueryRow(ctx, `
			INSERT INTO notifications (short_id, title, message, type, ref_type, ref_short_id, created_by)
			VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7)
			RETURNING id, short_id, title, message, type, COALESCE(ref_type,''), COALESCE(ref_short_id,''), created_by, created_at, updated_at`,
			shortID, title, message, notifType, refType, refShortID, createdBy,
		).Scan(
			&n.ID, &n.ShortID, &n.Title, &n.Message, &n.Type,
			&n.RefType, &n.RefShortID, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt,
		)
		if err == nil {
			return &n, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert notification: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindByShortID returns a single non-deleted notification (content only, not per-recipient state).
func (r *NotificationRepository) FindByShortID(ctx context.Context, shortID string) (*models.Notification, error) {
	var n models.Notification
	err := r.pool.QueryRow(ctx, `
		SELECT id, short_id, title, message, type, COALESCE(ref_type,''), COALESCE(ref_short_id,''), created_by, created_at, updated_at
		FROM notifications WHERE short_id = $1 AND deleted_at IS NULL LIMIT 1`,
		shortID,
	).Scan(&n.ID, &n.ShortID, &n.Title, &n.Message, &n.Type, &n.RefType, &n.RefShortID, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// GetInbox returns every notification addressed to userID, newest first, with read state.
func (r *NotificationRepository) GetInbox(ctx context.Context, userID string) ([]models.NotificationView, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT n.short_id, n.title, n.message, n.type, COALESCE(n.ref_type,''), COALESCE(n.ref_short_id,''),
		       nr.is_read, nr.read_at, n.created_at
		FROM notification_recipients nr
		JOIN notifications n ON n.id = nr.notification_id AND n.deleted_at IS NULL
		WHERE nr.user_id = $1
		ORDER BY n.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []models.NotificationView
	for rows.Next() {
		var v models.NotificationView
		if err := rows.Scan(&v.ShortID, &v.Title, &v.Message, &v.Type, &v.RefType, &v.RefShortID, &v.IsRead, &v.ReadAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		views = append(views, v)
	}
	return views, rows.Err()
}

// Update edits the shared notification content — only non-nil fields are changed.
func (r *NotificationRepository) Update(ctx context.Context, shortID string, in models.UpdateNotificationInput) error {
	args := []interface{}{shortID}
	setClauses := []string{"updated_at = NOW()"}
	i := 2

	if in.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", i))
		args = append(args, *in.Title)
		i++
	}
	if in.Message != nil {
		setClauses = append(setClauses, fmt.Sprintf("message = $%d", i))
		args = append(args, *in.Message)
		i++
	}

	if len(setClauses) == 1 {
		return fmt.Errorf("no fields to update")
	}

	q := fmt.Sprintf(
		"UPDATE notifications SET %s WHERE short_id = $1 AND deleted_at IS NULL",
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

// MarkRead sets the read state of a notification for a single recipient.
func (r *NotificationRepository) MarkRead(ctx context.Context, notificationShortID, userID string, isRead bool) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE notification_recipients nr
		SET is_read = $3, read_at = CASE WHEN $3 THEN NOW() ELSE NULL END
		FROM notifications n
		WHERE nr.notification_id = n.id
		  AND n.short_id = $1 AND n.deleted_at IS NULL
		  AND nr.user_id = $2::uuid`,
		notificationShortID, userID, isRead,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Delete soft-deletes a notification for every recipient.
func (r *NotificationRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE notifications SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
