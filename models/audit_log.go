package models

import "time"

// AuditEntry is what a controller hands to AuditRepository.Log — one record
// of an actor doing something to some entity, optionally scoped to a batch.
type AuditEntry struct {
	ActorID       string
	Action        string // create | update | delete | grade | issue | revoke | transfer | schedule | unschedule | cancel | grant_reattempt | enroll | remove
	EntityType    string // batch | session | attendance | assignment | assignment_submission | project | project_submission | assessment | exam_attempt | exam_answer | resource | certificate | module_schedule | score_weights | enrollment | fees
	EntityID      string
	EntityShortID string
	EntityLabel   string
	BatchShortID  string // optional — empty when the action isn't batch-scoped
	Metadata      map[string]interface{}
}

// AuditLogEntry is the persisted, denormalized record returned by the
// read-only audit log endpoints.
type AuditLogEntry struct {
	ID            string                 `json:"id"`
	ShortID       string                 `json:"short_id"`
	ActorID       string                 `json:"actor_id"`
	ActorName     string                 `json:"actor_name,omitempty"`
	Action        string                 `json:"action"`
	EntityType    string                 `json:"entity_type"`
	EntityID      string                 `json:"entity_id,omitempty"`
	EntityShortID string                 `json:"entity_short_id,omitempty"`
	EntityLabel   string                 `json:"entity_label,omitempty"`
	BatchID       string                 `json:"batch_id,omitempty"`
	BatchShortID  string                 `json:"batch_short_id,omitempty"`
	BatchNumber   string                 `json:"batch_number,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

// AuditLogFilter holds query params for GET /audit-logs.
type AuditLogFilter struct {
	BatchShortID string `form:"batch_short_id"`
	EntityType   string `form:"entity_type"`
	Action       string `form:"action"`
	ActorID      string `form:"actor_id"`
}
