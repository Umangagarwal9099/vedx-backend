package controller

import (
	"encoding/csv"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
	"github.com/umangagarwal/vedx-backend/util"
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
)

type StudentRegistrationController struct {
	registrationRepo *repository.StudentRegistrationRepository
	noteRepo         *repository.StudentNoteRepository
	auditLogRepo     *repository.AuditLogRepository
	userRepo         *repository.UserRepository
	collegeRepo      *repository.CollegeRepository
	emailSvc         *service.EmailService
	publicURL        string
}

func NewStudentRegistrationController(registrationRepo *repository.StudentRegistrationRepository, noteRepo *repository.StudentNoteRepository, auditLogRepo *repository.AuditLogRepository, userRepo *repository.UserRepository, collegeRepo *repository.CollegeRepository, emailSvc *service.EmailService, publicURL string) *StudentRegistrationController {
	return &StudentRegistrationController{registrationRepo: registrationRepo, noteRepo: noteRepo, auditLogRepo: auditLogRepo, userRepo: userRepo, collegeRepo: collegeRepo, emailSvc: emailSvc, publicURL: publicURL}
}

// validateRegistrationInput checks the admin-editable demographic fields and
// trims the free-text ones in place. Every field here is optional, so empty
// values pass — only malformed or over-long ones are rejected. Only fields
// present in the request (non-nil pointers) are touched.
func validateRegistrationInput(input *models.UpdateStudentRegistrationInput) error {
	// Enums must match the <select> options the admin portal renders, so a
	// hand-crafted request can't store a value the UI can never display back.
	enums := []struct {
		field   string
		value   *string
		allowed []string
	}{
		{"gender", input.Gender, []string{"male", "female", "other"}},
		{"student_source", input.StudentSource, []string{"database", "referral", "website", "social", "walk-in"}},
	}
	for _, e := range enums {
		if e.value == nil {
			continue
		}
		if err := util.ValidateOneOf(e.field, *e.value, e.allowed...); err != nil {
			return err
		}
	}

	phones := []struct {
		field string
		value *string
	}{
		{"alternate_contact", input.AlternateContact},
		{"parent_contact", input.ParentContact},
	}
	for _, p := range phones {
		if p.value == nil {
			continue
		}
		if err := util.ValidatePhone(p.field, *p.value); err != nil {
			return err
		}
	}

	if input.ParentEmail != nil {
		if err := util.ValidateEmailAddr("parent_email", *input.ParentEmail); err != nil {
			return err
		}
	}
	if input.Pincode != nil {
		if err := util.ValidatePincode("pincode", *input.Pincode); err != nil {
			return err
		}
	}
	if input.ParentName != nil {
		if strings.TrimSpace(*input.ParentName) != "" {
			if err := util.ValidateName("parent_name", *input.ParentName); err != nil {
				return err
			}
		}
	}

	texts := []struct {
		field string
		value **string
		max   int
	}{
		{"enrollment_no", &input.EnrollmentNo, util.MaxShortTextLen},
		{"religion", &input.Religion, util.MaxShortTextLen},
		{"standard", &input.Standard, util.MaxShortTextLen},
		{"occupation", &input.Occupation, util.MaxShortTextLen},
		{"timezone", &input.Timezone, util.MaxShortTextLen},
		{"area", &input.Area, util.MaxShortTextLen},
		{"school_college_name", &input.SchoolCollegeName, util.MaxShortTextLen},
		{"city", &input.City, util.MaxShortTextLen},
		{"state", &input.State, util.MaxShortTextLen},
		{"parent_name", &input.ParentName, util.MaxNameLen},
		{"parent_email", &input.ParentEmail, util.MaxShortTextLen},
		{"alternate_contact", &input.AlternateContact, util.MaxShortTextLen},
		{"parent_contact", &input.ParentContact, util.MaxShortTextLen},
		{"pincode", &input.Pincode, util.MaxShortTextLen},
		{"residential_address", &input.ResidentialAddress, util.MaxAddressLen},
		{"permanent_address", &input.PermanentAddress, util.MaxAddressLen},
	}
	for _, t := range texts {
		if *t.value == nil {
			continue
		}
		if err := util.ValidateText(t.field, **t.value, t.max); err != nil {
			return err
		}
		trimmed := strings.TrimSpace(**t.value)
		*t.value = &trimmed
	}

	return nil
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

	if err := validateRegistrationInput(&input); err != nil {
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

// GetAllStatuses godoc
//
//	@Summary		Bulk-fetch every student's lifecycle status
//	@Description	Returns a map of user_id -> status (registered/enrolled/completed/on_leave/archived) for every student — backs the Learners list's status column/filters without one round-trip per row. Restricted to super_admin/team_lead/mentor.
//	@Tags			students
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/students/statuses [get]
func (ctrl *StudentRegistrationController) GetAllStatuses(c *gin.Context) {
	statuses, err := ctrl.registrationRepo.GetAllStatuses(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch student statuses"})
		return
	}
	if statuses == nil {
		statuses = map[string]string{}
	}
	c.JSON(http.StatusOK, statuses)
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

// studentImportColumns maps a lowercased header cell to the StudentImportRow
// field it feeds — flexible so the College Admin's spreadsheet doesn't need
// exact column names. Mirrors the identical pattern in lead_controller.go.
var studentImportColumns = map[string]string{
	"first name": "first_name", "firstname": "first_name",
	"last name": "last_name", "lastname": "last_name",
	"email": "email", "email address": "email",
	"phone": "phone", "phone number": "phone", "mobile": "phone", "mobile number": "phone",
	"roll number": "roll_number", "roll no": "roll_number", "rollnumber": "roll_number", "admission number": "roll_number",
}

func parseStudentRows(header []string, records [][]string) []models.StudentImportRow {
	colIndex := map[string]int{}
	for i, h := range header {
		if field, ok := studentImportColumns[strings.ToLower(strings.TrimSpace(h))]; ok {
			colIndex[field] = i
		}
	}

	get := func(row []string, field string) string {
		i, ok := colIndex[field]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	rows := make([]models.StudentImportRow, 0, len(records))
	for i, rec := range records {
		rows = append(rows, models.StudentImportRow{
			RowNumber:  i + 2, // +1 for header row, +1 for 1-indexing
			FirstName:  get(rec, "first_name"),
			LastName:   get(rec, "last_name"),
			Email:      get(rec, "email"),
			Phone:      get(rec, "phone"),
			RollNumber: get(rec, "roll_number"),
		})
	}
	return rows
}

// BulkImportStudents godoc
//
//	@Summary		Import students from Excel/CSV
//	@Description	Uploads a .xlsx/.xls/.csv file of students (columns: first name/last name/email/phone/roll number, header names flexible) and bulk-creates accounts. The sheet never carries a college — every imported student lands under the authenticated caller's own college (or, for super_admin, an optional college_short_id form field). Duplicate emails and duplicate roll numbers within the same college are skipped with a reason, not aborted.
//	@Tags			students
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Students spreadsheet"
//	@Success		200		{object}	models.StudentImportResult
//	@Failure		400		{object}	map[string]string	"Missing/unreadable file"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/students/import [post]
func (ctrl *StudentRegistrationController) BulkImportStudents(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not open file"})
		return
	}
	defer file.Close()

	var header []string
	var records [][]string

	name := strings.ToLower(fileHeader.Filename)
	if strings.HasSuffix(name, ".csv") {
		reader := csv.NewReader(file)
		reader.FieldsPerRecord = -1
		all, err := reader.ReadAll()
		if err != nil || len(all) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not parse CSV"})
			return
		}
		header = all[0]
		records = all[1:]
	} else {
		xl, err := excelize.OpenReader(file)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not parse spreadsheet"})
			return
		}
		defer xl.Close()

		sheet := xl.GetSheetName(0)
		rows, err := xl.GetRows(sheet)
		if err != nil || len(rows) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "spreadsheet is empty"})
			return
		}
		header = rows[0]
		records = rows[1:]
	}

	// The import sheet itself never carries a college — super_admin may
	// optionally pass one as a form field (defaulting to the Internal EdTech
	// Platform); every other caller is always forced onto their own college.
	collegeID, err := resolveTargetCollege(c.Request.Context(), ctrl.collegeRepo, c.GetString("role"), c.GetString("college_id"), c.PostForm("college_short_id"))
	if err != nil {
		if errors.Is(err, repository.ErrCollegeScopeRequired) {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope to import students under"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not resolve target college: " + err.Error()})
		return
	}

	rows := parseStudentRows(header, records)
	result := &models.StudentImportResult{Skipped: []models.StudentImportRowError{}}

	for _, row := range rows {
		if strings.TrimSpace(row.FirstName) == "" || strings.TrimSpace(row.Email) == "" {
			result.Skipped = append(result.Skipped, models.StudentImportRowError{RowNumber: row.RowNumber, Reason: "missing first name or email"})
			continue
		}

		exists, err := ctrl.userRepo.EmailExists(c.Request.Context(), row.Email)
		if err != nil {
			result.Skipped = append(result.Skipped, models.StudentImportRowError{RowNumber: row.RowNumber, Reason: "could not verify email"})
			continue
		}
		if exists {
			result.Skipped = append(result.Skipped, models.StudentImportRowError{RowNumber: row.RowNumber, Reason: "email already in use"})
			continue
		}

		if row.RollNumber != "" {
			dup, err := ctrl.registrationRepo.EnrollmentNoExistsInCollege(c.Request.Context(), collegeID, row.RollNumber)
			if err != nil {
				result.Skipped = append(result.Skipped, models.StudentImportRowError{RowNumber: row.RowNumber, Reason: "could not verify roll number"})
				continue
			}
			if dup {
				result.Skipped = append(result.Skipped, models.StudentImportRowError{RowNumber: row.RowNumber, Reason: "roll number already in use in this college"})
				continue
			}
		}

		tempPassword := util.GenerateTemporaryPassword()
		hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcrypt.DefaultCost)
		if err != nil {
			result.Skipped = append(result.Skipped, models.StudentImportRowError{RowNumber: row.RowNumber, Reason: "could not generate credentials"})
			continue
		}

		var registrationNo *int
		if n, ok := ctrl.collegeRepo.NextRegistrationNo(c.Request.Context(), collegeID); ok {
			registrationNo = &n
		}

		userID, err := ctrl.userRepo.Register(c.Request.Context(), models.User{
			Email: row.Email, PasswordHash: string(hash),
			FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone,
		}, collegeID, registrationNo)
		if err != nil {
			reason := "could not create account"
			if errors.Is(err, repository.ErrEmailAlreadyExists) {
				reason = "email already in use"
			} else {
				log.Printf("bulk import row %d: create account: %v", row.RowNumber, err)
			}
			result.Skipped = append(result.Skipped, models.StudentImportRowError{RowNumber: row.RowNumber, Reason: reason})
			continue
		}

		if row.RollNumber != "" {
			enrollmentNo := row.RollNumber
			if err := ctrl.registrationRepo.Upsert(c.Request.Context(), userID, models.UpdateStudentRegistrationInput{EnrollmentNo: &enrollmentNo}); err != nil {
				log.Printf("set roll number for imported student %s: %v", userID, err)
			}
		}

		if ctrl.emailSvc != nil && ctrl.emailSvc.Configured() {
			subject, html := service.StaffWelcomeEmail(row.FirstName, roleLabels[models.RoleStudent], row.Email, tempPassword, util.LoginURLForRole(models.RoleStudent))
			ctrl.emailSvc.SendAsync(row.Email, subject, html)
		}

		result.Imported++
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "bulk_import", EntityType: "student",
		EntityLabel: "bulk import",
		Metadata:    map[string]interface{}{"imported": result.Imported, "skipped": len(result.Skipped)},
	})

	c.JSON(http.StatusOK, result)
}
