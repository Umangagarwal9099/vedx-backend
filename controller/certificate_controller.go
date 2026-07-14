package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type CertificateController struct {
	certificateRepo *repository.CertificateRepository
	batchRepo       *repository.BatchRepository
	auditLogRepo    *repository.AuditLogRepository
}

func NewCertificateController(certificateRepo *repository.CertificateRepository, batchRepo *repository.BatchRepository, auditLogRepo *repository.AuditLogRepository) *CertificateController {
	return &CertificateController{certificateRepo: certificateRepo, batchRepo: batchRepo, auditLogRepo: auditLogRepo}
}

// IssueCertificate godoc
//
//	@Summary		Issue a certificate
//	@Description	Issues (or re-issues) a completion certificate for a student in a batch, snapshotting their final score/rank at issuance time. Requires the student to have an enrollment record in this batch. Restricted to super_admin / team_lead / mentor.
//	@Tags			certificates
//	@Produce		json
//	@Param			short_id	path		string	true	"Batch short ID"
//	@Param			user_id		path		string	true	"Student user ID"
//	@Success		201			{object}	models.Certificate
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error, or no enrollment found"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/students/{user_id}/certificate [post]
func (ctrl *CertificateController) IssueCertificate(c *gin.Context) {
	batchShortID := c.Param("short_id")
	userID := c.Param("user_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), batchShortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, batchShortID) {
		return
	}

	issuedBy := c.GetString("user_id")
	cert, err := ctrl.certificateRepo.Issue(c.Request.Context(), userID, batch.ID, batch.CourseID, issuedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "issue", EntityType: "certificate",
		EntityID: cert.ID, EntityShortID: cert.ShortID, EntityLabel: cert.CertificateNumber,
		BatchShortID: batchShortID,
		Metadata: map[string]interface{}{"student_id": userID},
	})

	c.JSON(http.StatusCreated, cert)
}

// GetBatchCertificates godoc
//
//	@Summary		List a batch's issued certificates
//	@Tags			certificates
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		200			{array}		models.Certificate
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/certificates [get]
func (ctrl *CertificateController) GetBatchCertificates(c *gin.Context) {
	batchShortID := c.Param("short_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), batchShortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, batchShortID) {
		return
	}

	certs, err := ctrl.certificateRepo.GetForBatch(c.Request.Context(), batch.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch certificates"})
		return
	}
	if certs == nil {
		certs = []models.Certificate{}
	}

	c.JSON(http.StatusOK, certs)
}

// GetAllCertificates godoc
//
//	@Summary		List every issued certificate
//	@Description	Returns every issued certificate, newest first — mentors see only certificates for batches they manage; team_lead/super_admin see everything.
//	@Tags			certificates
//	@Produce		json
//	@Success		200	{array}		models.Certificate
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/certificates [get]
func (ctrl *CertificateController) GetAll(c *gin.Context) {
	mentorID := ""
	role := c.GetString("role")
	if role == string(models.RoleMentor) || role == string(models.RoleEmployee) {
		mentorID = c.GetString("user_id")
	}

	certs, err := ctrl.certificateRepo.GetAll(c.Request.Context(), mentorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch certificates"})
		return
	}
	if certs == nil {
		certs = []models.Certificate{}
	}

	c.JSON(http.StatusOK, certs)
}

// GetStudentCertificates godoc
//
//	@Summary		List a student's certificates
//	@Tags			certificates
//	@Produce		json
//	@Param			id	path		string	true	"Student user ID"
//	@Success		200	{array}		models.Certificate
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/certificates [get]
func (ctrl *CertificateController) GetStudentCertificates(c *gin.Context) {
	userID := c.Param("id")

	certs, err := ctrl.certificateRepo.GetForStudent(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch certificates"})
		return
	}
	if certs == nil {
		certs = []models.Certificate{}
	}

	c.JSON(http.StatusOK, certs)
}

// VerifyCertificate godoc
//
//	@Summary		Verify a certificate (public)
//	@Description	Unauthenticated lookup by certificate number, for third parties to verify authenticity. Always returns 200 — check the "valid" field.
//	@Tags			certificates
//	@Produce		json
//	@Param			certificate_number	path	string	true	"Certificate number"
//	@Success		200					{object}	models.CertificateVerification
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Router			/certificates/verify/{certificate_number} [get]
func (ctrl *CertificateController) VerifyCertificate(c *gin.Context) {
	certNumber := c.Param("certificate_number")

	cert, err := ctrl.certificateRepo.VerifyByNumber(c.Request.Context(), certNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify certificate"})
		return
	}
	if cert == nil {
		c.JSON(http.StatusOK, models.CertificateVerification{Valid: false})
		return
	}

	c.JSON(http.StatusOK, models.CertificateVerification{
		Valid:             true,
		CertificateNumber: cert.CertificateNumber,
		StudentName:       cert.StudentName,
		CourseName:        cert.CourseName,
		BatchNumber:       cert.BatchNumber,
		FinalScore:        cert.FinalScore,
		FinalRank:         cert.FinalRank,
		IssuedAt:          cert.IssuedAt,
	})
}

// RevokeCertificate godoc
//
//	@Summary		Revoke a certificate
//	@Description	Soft-revokes a certificate (kept for history; fails verification afterward). Restricted to super_admin / team_lead.
//	@Tags			certificates
//	@Produce		json
//	@Param			short_id	path	string	true	"Certificate short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Certificate not found or already revoked"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/certificates/{short_id} [delete]
func (ctrl *CertificateController) RevokeCertificate(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.certificateRepo.Revoke(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "certificate not found or already revoked"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not revoke certificate"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "revoke", EntityType: "certificate", EntityShortID: shortID,
	})

	c.Status(http.StatusNoContent)
}
