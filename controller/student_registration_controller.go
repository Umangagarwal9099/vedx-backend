package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type StudentRegistrationController struct {
	registrationRepo *repository.StudentRegistrationRepository
	noteRepo         *repository.StudentNoteRepository
	auditLogRepo     *repository.AuditLogRepository
}

func NewStudentRegistrationController(registrationRepo *repository.StudentRegistrationRepository, noteRepo *repository.StudentNoteRepository, auditLogRepo *repository.AuditLogRepository) *StudentRegistrationController {
	return &StudentRegistrationController{registrationRepo: registrationRepo, noteRepo: noteRepo, auditLogRepo: auditLogRepo}
}

// registrationUpdateDiff builds a { field: {from, to} } metadata map for only
// the fields actually present in the request, mirroring userUpdateDiff.
func registrationUpdateDiff(before *models.StudentRegistrationDetails, input models.UpdateStudentRegistrationInput) map[string]interface{} {
	diff := map[string]interface{}{}
	strField := func(name string, from string, to *string) {
		if to != nil && *to != from {
			diff[name] = map[string]string{"from": from, "to": *to}
		}
	}
	boolField := func(name string, from bool, to *bool) {
		if to != nil && *to != from {
			diff[name] = map[string]interface{}{"from": from, "to": *to}
		}
	}
	strField("enrollment_no", before.EnrollmentNo, input.EnrollmentNo)
	strField("gender", before.Gender, input.Gender)
	strField("alternate_contact", before.AlternateContact, input.AlternateContact)
	strField("student_source", before.StudentSource, input.StudentSource)
	strField("religion", before.Religion, input.Religion)
	strField("standard", before.Standard, input.Standard)
	strField("occupation", before.Occupation, input.Occupation)
	strField("timezone", before.Timezone, input.Timezone)
	strField("parent_name", before.ParentName, input.ParentName)
	strField("parent_contact", before.ParentContact, input.ParentContact)
	strField("parent_email", before.ParentEmail, input.ParentEmail)
	strField("area", before.Area, input.Area)
	strField("school_college_name", before.SchoolCollegeName, input.SchoolCollegeName)
	strField("residential_address", before.ResidentialAddress, input.ResidentialAddress)
	strField("permanent_address", before.PermanentAddress, input.PermanentAddress)
	strField("city", before.City, input.City)
	strField("state", before.State, input.State)
	strField("pincode", before.Pincode, input.Pincode)
	boolField("opt_whatsapp", before.OptWhatsapp, input.OptWhatsapp)
	boolField("opt_email", before.OptEmail, input.OptEmail)
	boolField("opt_sms", before.OptSMS, input.OptSMS)
	boolField("opt_push", before.OptPush, input.OptPush)
	return diff
}

// GetDetails godoc
//
//	@Summary		Get student registration details
//	@Description	Returns the extended demographic/registration fields (parent info, addresses, etc.) for a student. Self, or super_admin/team_lead.
//	@Tags			students
//	@Produce		json
//	@Param			id	path		string	true	"Student user ID (UUID)"
//	@Success		200	{object}	models.StudentRegistrationDetails
//	@Failure		404	{object}	map[string]string	"User not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/registration-details [get]
func (ctrl *StudentRegistrationController) GetDetails(c *gin.Context) {
	id := c.Param("id")

	details, err := ctrl.registrationRepo.GetByUserID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch registration details"})
		return
	}
	if details == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, details)
}

// UpdateDetails godoc
//
//	@Summary		Update student registration details
//	@Description	Partially update a student's demographic/registration fields. Self, or super_admin/team_lead.
//	@Tags			students
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string									true	"Student user ID (UUID)"
//	@Param			body	body		models.UpdateStudentRegistrationInput	true	"Fields to update (all optional)"
//	@Success		200		{object}	models.StudentRegistrationDetails
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/registration-details [patch]
func (ctrl *StudentRegistrationController) UpdateDetails(c *gin.Context) {
	id := c.Param("id")

	var input models.UpdateStudentRegistrationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	before, err := ctrl.registrationRepo.GetByUserID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch registration details"})
		return
	}

	if err := ctrl.registrationRepo.Upsert(c.Request.Context(), id, input); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update registration details: " + err.Error()})
		return
	}

	details, err := ctrl.registrationRepo.GetByUserID(c.Request.Context(), id)
	if err != nil || details == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated registration details"})
		return
	}

	if before != nil {
		logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
			Action: "update", EntityType: "user",
			EntityID: id,
			Metadata: registrationUpdateDiff(before, input),
		})
	}

	c.JSON(http.StatusOK, details)
}

// UpdateStatus godoc
//
//	@Summary		Update a student's status
//	@Description	Changes a student's lifecycle status and records it in their history. Restricted to super_admin/team_lead.
//	@Tags			students
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string								true	"Student user ID (UUID)"
//	@Param			body	body		models.UpdateStudentStatusInput	true	"New status + optional notes"
//	@Success		200		{object}	map[string]string	"status updated"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		404		{object}	map[string]string	"Student not found"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/status [post]
func (ctrl *StudentRegistrationController) UpdateStatus(c *gin.Context) {
	id := c.Param("id")

	var input models.UpdateStudentStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	changedBy := c.GetString("user_id")
	if err := ctrl.registrationRepo.UpdateStatus(c.Request.Context(), id, input.Status, input.Notes, changedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "student not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update status: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "status updated"})
}

// GetStatusHistory godoc
//
//	@Summary		Get a student's status history
//	@Description	Returns every status-change event for a student, newest first. Restricted to super_admin/team_lead.
//	@Tags			students
//	@Produce		json
//	@Param			id	path		string	true	"Student user ID (UUID)"
//	@Success		200	{array}		models.StudentStatusHistoryEntry
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/status-history [get]
func (ctrl *StudentRegistrationController) GetStatusHistory(c *gin.Context) {
	id := c.Param("id")

	history, err := ctrl.registrationRepo.GetStatusHistory(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch status history"})
		return
	}
	if history == nil {
		history = []models.StudentStatusHistoryEntry{}
	}
	c.JSON(http.StatusOK, history)
}

// AddNote godoc
//
//	@Summary		Add a learner note
//	@Description	Adds a freeform staff note about a student. Restricted to super_admin/team_lead/mentor.
//	@Tags			students
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string							true	"Student user ID (UUID)"
//	@Param			body	body		models.CreateStudentNoteInput	true	"Note text"
//	@Success		201		{object}	models.StudentNote
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/notes [post]
func (ctrl *StudentRegistrationController) AddNote(c *gin.Context) {
	id := c.Param("id")

	var input models.CreateStudentNoteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdBy := c.GetString("user_id")
	note, err := ctrl.noteRepo.Create(c.Request.Context(), id, input.Note, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add note: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, note)
}

// GetNotes godoc
//
//	@Summary		List learner notes
//	@Description	Returns every note for a student, newest first. Restricted to super_admin/team_lead/mentor.
//	@Tags			students
//	@Produce		json
//	@Param			id	path		string	true	"Student user ID (UUID)"
//	@Success		200	{array}		models.StudentNote
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/notes [get]
func (ctrl *StudentRegistrationController) GetNotes(c *gin.Context) {
	id := c.Param("id")

	notes, err := ctrl.noteRepo.FindAllForStudent(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch notes"})
		return
	}
	if notes == nil {
		notes = []models.StudentNote{}
	}
	c.JSON(http.StatusOK, notes)
}
