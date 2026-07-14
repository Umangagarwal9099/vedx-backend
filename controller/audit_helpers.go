package controller

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// logAudit records one audit log entry, filling ActorID from the request's
// JWT context. Best-effort — same convention as notifications elsewhere in
// this codebase: a logging failure is logged to stderr, never fails the
// request. auditRepo may be nil (defensive no-op) so this never panics if a
// controller wasn't wired with one.
func logAudit(c *gin.Context, auditRepo *repository.AuditLogRepository, e models.AuditEntry) {
	if auditRepo == nil {
		return
	}
	e.ActorID = c.GetString("user_id")
	if err := auditRepo.Log(c.Request.Context(), e); err != nil {
		log.Printf("audit log write failed (action=%s entity_type=%s): %v", e.Action, e.EntityType, err)
	}
}
