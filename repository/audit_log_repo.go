package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type AuditLogRepository struct {
	pool *pgxpool.Pool
}

func NewAuditLogRepository(pool *pgxpool.Pool) *AuditLogRepository {
	return &AuditLogRepository{pool: pool}
}

// Log appends one audit record. Best-effort by convention — callers log a
// failure and continue rather than fail the request over it (same pattern
// already used for notifications throughout this codebase).
func (r *AuditLogRepository) Log(ctx context.Context, e models.AuditEntry) error {
	metadataJSON, err := json.Marshal(e.Metadata)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO audit_logs (
			short_id, actor_id, action, entity_type, entity_id, entity_short_id,
			entity_label, batch_id, metadata
		)
		VALUES (
			$1, $2::UUID, $3, $4, NULLIF($5,'')::UUID, NULLIF($6,''), NULLIF($7,''),
			(SELECT id FROM batches WHERE short_id = NULLIF($8,'') AND deleted_at IS NULL),
			$9::JSONB
		)`,
		// metadataJSON must go in as a string, not []byte — pgx sends a
		// []byte query param in binary (bytea) format, and Postgres can't
		// reinterpret that as a JSONB text cast. This was silently failing
		// every single audit-log write in the app (0 rows ever landed in
		// audit_logs) until this fix — logAudit() only ever logs failures
		// to stdout, never surfaces them, so nothing user-visible caught it.
		util.GenerateShortID(), e.ActorID, e.Action, e.EntityType, e.EntityID, e.EntityShortID,
		e.EntityLabel, e.BatchShortID, string(metadataJSON),
	)
	return err
}

const auditLogBaseSelect = `
	SELECT al.id, al.short_id, al.actor_id, CONCAT(u.first_name, ' ', u.last_name),
	       al.action, al.entity_type, COALESCE(al.entity_id::TEXT, ''), COALESCE(al.entity_short_id, ''),
	       COALESCE(al.entity_label, ''),
	       COALESCE(al.batch_id::TEXT, ''), COALESCE(b.short_id, ''), COALESCE(b.batch_number, ''),
	       al.metadata, al.created_at
	FROM audit_logs al
	JOIN users   u ON al.actor_id = u.id
	LEFT JOIN batches b ON al.batch_id = b.id`

func scanAuditLog(rows pgx.Row) (models.AuditLogEntry, error) {
	var e models.AuditLogEntry
	var metadataRaw []byte
	err := rows.Scan(
		&e.ID, &e.ShortID, &e.ActorID, &e.ActorName,
		&e.Action, &e.EntityType, &e.EntityID, &e.EntityShortID,
		&e.EntityLabel,
		&e.BatchID, &e.BatchShortID, &e.BatchNumber,
		&metadataRaw, &e.CreatedAt,
	)
	if err != nil {
		return e, err
	}
	if len(metadataRaw) > 0 {
		if err := json.Unmarshal(metadataRaw, &e.Metadata); err != nil {
			log.Printf("unmarshal audit log metadata: %v", err)
		}
	}
	return e, nil
}

// GetAll returns audit log entries matching the filter, newest first, capped
// at 500 rows — this is a browse view, not a full export.
func (r *AuditLogRepository) GetAll(ctx context.Context, f models.AuditLogFilter) ([]models.AuditLogEntry, error) {
	conditions := []string{}
	args := []interface{}{}
	i := 1

	if f.BatchShortID != "" {
		conditions = append(conditions, fmt.Sprintf("b.short_id = $%d", i))
		args = append(args, f.BatchShortID)
		i++
	}
	if f.EntityType != "" {
		conditions = append(conditions, fmt.Sprintf("al.entity_type = $%d", i))
		args = append(args, f.EntityType)
		i++
	}
	if f.Action != "" {
		conditions = append(conditions, fmt.Sprintf("al.action = $%d", i))
		args = append(args, f.Action)
		i++
	}
	if f.ActorID != "" {
		conditions = append(conditions, fmt.Sprintf("al.actor_id = $%d::UUID", i))
		args = append(args, f.ActorID)
		i++
	}

	q := auditLogBaseSelect
	if len(conditions) > 0 {
		q += " WHERE " + strings.Join(conditions, " AND ")
	}
	q += " ORDER BY al.created_at DESC LIMIT 500"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AuditLogEntry
	for rows.Next() {
		e, err := scanAuditLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetForBatch returns a single batch's audit trail, newest first.
func (r *AuditLogRepository) GetForBatch(ctx context.Context, batchID string) ([]models.AuditLogEntry, error) {
	rows, err := r.pool.Query(ctx,
		auditLogBaseSelect+" WHERE al.batch_id = $1::UUID ORDER BY al.created_at DESC LIMIT 500",
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AuditLogEntry
	for rows.Next() {
		e, err := scanAuditLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
