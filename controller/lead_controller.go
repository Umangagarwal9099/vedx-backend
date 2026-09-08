package controller

import (
	"context"
	"encoding/csv"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/xuri/excelize/v2"
)

type LeadController struct {
	leadRepo                  *repository.LeadRepository
	leadCallLogRepo           *repository.LeadCallLogRepository
	leadAssignmentHistoryRepo *repository.LeadAssignmentHistoryRepository
	notificationRepo          *repository.NotificationRepository
	auditLogRepo              *repository.AuditLogRepository
	collegeRepo               *repository.CollegeRepository
	userRepo                  *repository.UserRepository

	// cachedIntakeCreator memoizes the system user id used as created_by for
	// public website leads (the first super_admin). Resolved lazily on the
	// first /leads/intake call and cached for the process lifetime.
	intakeCreatorMu     sync.Mutex
	cachedIntakeCreator string
}

func NewLeadController(leadRepo *repository.LeadRepository, leadCallLogRepo *repository.LeadCallLogRepository, leadAssignmentHistoryRepo *repository.LeadAssignmentHistoryRepository, notificationRepo *repository.NotificationRepository, auditLogRepo *repository.AuditLogRepository, collegeRepo *repository.CollegeRepository, userRepo *repository.UserRepository) *LeadController {
	return &LeadController{leadRepo: leadRepo, leadCallLogRepo: leadCallLogRepo, leadAssignmentHistoryRepo: leadAssignmentHistoryRepo, notificationRepo: notificationRepo, auditLogRepo: auditLogRepo, collegeRepo: collegeRepo, userRepo: userRepo}
}

// intakeCreatorID resolves (and caches) the user id credited as created_by
// for unauthenticated website leads — the first super_admin account. Returns
// an error if none exists so the caller can fail closed.
func (ctrl *LeadController) intakeCreatorID(ctx context.Context) (string, error) {
	ctrl.intakeCreatorMu.Lock()
	defer ctrl.intakeCreatorMu.Unlock()
	if ctrl.cachedIntakeCreator != "" {
		return ctrl.cachedIntakeCreator, nil
	}
	admins, err := ctrl.userRepo.FindByRole(ctx, models.RoleSuperAdmin, "")
	if err != nil {
		return "", err
	}
	if len(admins) == 0 {
		return "", errors.New("no super_admin account to own website leads")
	}
	ctrl.cachedIntakeCreator = admins[0].ID
	return ctrl.cachedIntakeCreator, nil
}

// PublicIntake godoc
//
//	@Summary		Submit a website lead (public)
//	@Description	Unauthenticated endpoint for marketing-website forms (Contact Us, Course Enquiry, etc). Creates a lead on the Internal EdTech Platform with source=website, status=new. Accidental resubmissions within 6h (same email or phone) are collapsed into the existing lead. Always returns a generic success — never leaks whether a lead already existed.
//	@Tags			leads
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.PublicLeadIntakeInput	true	"Lead submission"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		503		{object}	map[string]string	"Intake temporarily unavailable"
//	@Router			/leads/intake [post]
func (ctrl *LeadController) PublicIntake(c *gin.Context) {
	var in models.PublicLeadIntakeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and phone are required"})
		return
	}

	// Honeypot — bots fill hidden fields. Accept silently, create nothing.
	if strings.TrimSpace(in.Website) != "" {
		c.JSON(http.StatusOK, gin.H{"message": "received"})
		return
	}

	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.Phone = strings.TrimSpace(in.Phone)
	if in.Name == "" || in.Phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and phone are required"})
		return
	}
	if len(in.Name) > 200 || len(in.Email) > 200 || len(in.Phone) > 40 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "one or more fields exceed the allowed length"})
		return
	}
	if len(in.Message) > 2000 {
		in.Message = in.Message[:2000]
	}

	ctx := c.Request.Context()

	collegeID, err := ctrl.collegeRepo.DefaultCollegeID(ctx)
	if err != nil {
		log.Printf("lead intake: resolve default college: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "lead intake temporarily unavailable"})
		return
	}
	creatorID, err := ctrl.intakeCreatorID(ctx)
	if err != nil {
		log.Printf("lead intake: resolve system creator: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "lead intake temporarily unavailable"})
		return
	}

	// Collapse accidental resubmissions — but always answer success so the
	// caller can't probe which emails/phones are already in the CRM.
	if dup, dupErr := ctrl.leadRepo.ExistsRecentDuplicate(ctx, in.Email, in.Phone, 6*time.Hour); dupErr == nil && dup {
		c.JSON(http.StatusOK, gin.H{"message": "lead received"})
		return
	}

	courseInterest := strings.TrimSpace(in.InterestedIn)
	if courseInterest == "" {
		courseInterest = "General Enquiry"
	}
	notes := strings.TrimSpace(in.Message)
	if form := strings.TrimSpace(in.Source); form != "" {
		if notes != "" {
			notes = "Form: " + form + "\n" + notes
		} else {
			notes = "Form: " + form
		}
	}

	lead, err := ctrl.leadRepo.Create(ctx, models.CreateLeadInput{
		Name:           in.Name,
		Phone:          in.Phone,
		Email:          in.Email,
		City:           strings.TrimSpace(in.City),
		CourseInterest: courseInterest,
		Source:         "website",
		Notes:          notes,
	}, creatorID, collegeID)
	if err != nil {
		log.Printf("lead intake: create lead: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "lead intake temporarily unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "lead received", "id": lead.ShortID})
}

// isEmployeeOnly reports whether the caller is a plain employee (not
// team_lead/super_admin) — used to restrict which fields Update accepts.
func isEmployeeOnly(c *gin.Context) bool {
	return c.GetString("role") == string(models.RoleEmployee)
}

// Create godoc
//
//	@Summary		Create lead
//	@Description	Manually create a single sales lead. Restricted to super_admin/team_lead.
//	@Tags			leads
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateLeadInput	true	"Lead details"
//	@Success		201		{object}	models.Lead
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads [post]
func (ctrl *LeadController) Create(c *gin.Context) {
	var input models.CreateLeadInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdBy := c.GetString("user_id")

	collegeID, err := resolveTargetCollege(c.Request.Context(), ctrl.collegeRepo, c.GetString("role"), c.GetString("college_id"), input.CollegeShortID)
	if err != nil {
		if errors.Is(err, repository.ErrCollegeScopeRequired) {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope to create a lead under"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not resolve target college: " + err.Error()})
		return
	}

	lead, err := ctrl.leadRepo.Create(c.Request.Context(), input, createdBy, collegeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create lead: " + err.Error()})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "create", EntityType: "lead",
		EntityID: lead.ID, EntityShortID: lead.ShortID, EntityLabel: lead.Name,
	})

	c.JSON(http.StatusCreated, lead)
}

// GetAll godoc
//
//	@Summary		List leads
//	@Description	Returns leads scoped to the caller — employees see only leads assigned to them; team_lead/super_admin see (and can filter by employee_id) all leads. Supports status/priority/course/city/employee_id/today/follow_up_due/overdue/date_from/date_to filters.
//	@Tags			leads
//	@Produce		json
//	@Success		200	{array}		models.Lead
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads [get]
func (ctrl *LeadController) GetAll(c *gin.Context) {
	var filter models.LeadFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := c.GetString("role")
	userID := c.GetString("user_id")

	collegeID, err := repository.CollegeFilter(role, c.GetString("college_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
		return
	}

	var leads []models.Lead

	if role == string(models.RoleEmployee) {
		leads, err = ctrl.leadRepo.FindAllForEmployee(c.Request.Context(), userID, filter, collegeID)
	} else {
		leads, err = ctrl.leadRepo.FindAll(c.Request.Context(), filter, collegeID)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch leads"})
		return
	}
	if leads == nil {
		leads = []models.Lead{}
	}
	c.JSON(http.StatusOK, leads)
}

// GetByShortID godoc
//
//	@Summary		Get lead
//	@Description	Returns a single lead by its short_id. Employees may only fetch their own assigned lead.
//	@Tags			leads
//	@Produce		json
//	@Param			short_id	path		string	true	"Lead short ID"
//	@Success		200			{object}	models.Lead
//	@Failure		403			{object}	map[string]string	"Not your lead"
//	@Failure		404			{object}	map[string]string	"Lead not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/{short_id} [get]
func (ctrl *LeadController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")
	if !checkLeadAccess(c, ctrl.leadRepo, shortID) {
		return
	}

	lead, err := ctrl.leadRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch lead"})
		return
	}
	if lead == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "lead not found"})
		return
	}
	c.JSON(http.StatusOK, lead)
}

// Update godoc
//
//	@Summary		Update lead
//	@Description	Partially update a lead. Employees may only change status/priority/next_follow_up_at/notes on their own assigned lead; team_lead/super_admin can change everything.
//	@Tags			leads
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string					true	"Lead short ID"
//	@Param			body		body		models.UpdateLeadInput	true	"Fields to update"
//	@Success		200			{object}	models.Lead
//	@Failure		400			{object}	map[string]string	"Validation error or no fields provided"
//	@Failure		403			{object}	map[string]string	"Not your lead"
//	@Failure		404			{object}	map[string]string	"Lead not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/{short_id} [patch]
func (ctrl *LeadController) Update(c *gin.Context) {
	shortID := c.Param("short_id")
	if !checkLeadAccess(c, ctrl.leadRepo, shortID) {
		return
	}

	var input models.UpdateLeadInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if isEmployeeOnly(c) {
		input.Name = nil
		input.Phone = nil
		input.Email = nil
		input.City = nil
		input.CourseInterest = nil
	}

	if err := ctrl.leadRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "lead not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update lead"})
		return
	}

	lead, err := ctrl.leadRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || lead == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated lead"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "update", EntityType: "lead",
		EntityID: lead.ID, EntityShortID: lead.ShortID, EntityLabel: lead.Name,
	})

	c.JSON(http.StatusOK, lead)
}

// Delete godoc
//
//	@Summary		Delete lead
//	@Description	Soft-deletes a lead. Restricted to super_admin/team_lead.
//	@Tags			leads
//	@Produce		json
//	@Param			short_id	path	string	true	"Lead short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Lead not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/{short_id} [delete]
func (ctrl *LeadController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	existing, err := ctrl.leadRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch lead"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "lead not found"})
		return
	}

	if err := ctrl.leadRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "lead not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete lead"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "delete", EntityType: "lead",
		EntityID: existing.ID, EntityShortID: existing.ShortID, EntityLabel: existing.Name,
	})

	c.Status(http.StatusNoContent)
}

// AddCallLog godoc
//
//	@Summary		Log a call against a lead
//	@Description	Records a call outcome + status + notes (and optionally a new next-follow-up date) against a lead, updating the lead's own status/last_contacted_at/next_follow_up_at. Employees may only log calls on their own assigned lead.
//	@Tags			leads
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Lead short ID"
//	@Param			body		body		models.CreateCallLogInput	true	"Call details"
//	@Success		201			{object}	models.LeadCallLog
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		403			{object}	map[string]string	"Not your lead"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/{short_id}/calls [post]
func (ctrl *LeadController) AddCallLog(c *gin.Context) {
	shortID := c.Param("short_id")
	if !checkLeadAccess(c, ctrl.leadRepo, shortID) {
		return
	}

	var input models.CreateCallLogInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	employeeID := c.GetString("user_id")
	callLog, err := ctrl.leadCallLogRepo.Create(c.Request.Context(), shortID, employeeID, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not log call: " + err.Error()})
		return
	}

	if err := ctrl.leadRepo.RecordContact(c.Request.Context(), shortID, input.Status, input.NextFollowUpAt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "call logged, but could not update lead status"})
		return
	}

	c.JSON(http.StatusCreated, callLog)
}

// GetCallLogs godoc
//
//	@Summary		List call logs for a lead
//	@Description	Returns every call log for one lead, newest first. Employees may only view their own assigned lead's logs.
//	@Tags			leads
//	@Produce		json
//	@Param			short_id	path	string	true	"Lead short ID"
//	@Success		200			{array}	models.LeadCallLog
//	@Failure		403			{object}	map[string]string	"Not your lead"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/{short_id}/calls [get]
func (ctrl *LeadController) GetCallLogs(c *gin.Context) {
	shortID := c.Param("short_id")
	if !checkLeadAccess(c, ctrl.leadRepo, shortID) {
		return
	}

	logs, err := ctrl.leadCallLogRepo.FindAllForLead(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch call logs"})
		return
	}
	if logs == nil {
		logs = []models.LeadCallLog{}
	}
	c.JSON(http.StatusOK, logs)
}

// leadImportColumns maps a lowercased header cell to the LeadImportRow field
// it feeds — flexible so admin's spreadsheet doesn't need exact column names.
var leadImportColumns = map[string]string{
	"name": "name", "full name": "name", "lead name": "name",
	"phone": "phone", "phone number": "phone", "mobile": "phone", "mobile number": "phone",
	"email": "email", "email address": "email",
	"city":   "city",
	"course": "course", "course interest": "course", "interested course": "course",
}

func parseLeadRows(header []string, records [][]string) []models.LeadImportRow {
	colIndex := map[string]int{}
	for i, h := range header {
		field, ok := leadImportColumns[strings.ToLower(strings.TrimSpace(h))]
		if ok {
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

	rows := make([]models.LeadImportRow, 0, len(records))
	for i, rec := range records {
		rows = append(rows, models.LeadImportRow{
			RowNumber:      i + 2, // +1 for header row, +1 for 1-indexing
			Name:           get(rec, "name"),
			Phone:          get(rec, "phone"),
			Email:          get(rec, "email"),
			City:           get(rec, "city"),
			CourseInterest: get(rec, "course"),
		})
	}
	return rows
}

// BulkImport godoc
//
//	@Summary		Import leads from Excel/CSV
//	@Description	Uploads a .xlsx/.xls/.csv file of leads (columns: name/phone/email/city/course, header names flexible) and bulk-inserts them as source=excel_import. Restricted to super_admin/team_lead.
//	@Tags			leads
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Leads spreadsheet"
//	@Success		200		{object}	models.LeadImportResult
//	@Failure		400		{object}	map[string]string	"Missing/unreadable file"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/import [post]
func (ctrl *LeadController) BulkImport(c *gin.Context) {
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

	parsedRows := parseLeadRows(header, records)
	createdBy := c.GetString("user_id")

	// The import sheet itself never carries a college — super_admin may
	// optionally pass one as a form field (defaulting to the Internal EdTech
	// Platform); every other caller is always forced onto their own college.
	collegeID, err := resolveTargetCollege(c.Request.Context(), ctrl.collegeRepo, c.GetString("role"), c.GetString("college_id"), c.PostForm("college_short_id"))
	if err != nil {
		if errors.Is(err, repository.ErrCollegeScopeRequired) {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope to import leads under"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not resolve target college: " + err.Error()})
		return
	}

	result, err := ctrl.leadRepo.BulkImport(c.Request.Context(), parsedRows, createdBy, collegeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not import leads: " + err.Error()})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "create", EntityType: "lead",
		EntityLabel: "bulk import",
		Metadata:    map[string]interface{}{"imported": result.Imported, "skipped": len(result.Skipped)},
	})

	c.JSON(http.StatusOK, result)
}

// AssignBulk godoc
//
//	@Summary		Assign leads to an employee
//	@Description	Assigns one or many leads to an employee with an optional priority + first follow-up date. Restricted to super_admin/team_lead.
//	@Tags			leads
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.AssignLeadsInput	true	"Assignment details"
//	@Success		200		{object}	map[string]int	"assigned count"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/assign [post]
func (ctrl *LeadController) AssignBulk(c *gin.Context) {
	var input models.AssignLeadsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	performedBy := c.GetString("user_id")
	count, err := ctrl.leadRepo.AssignBulk(c.Request.Context(), input.LeadShortIDs, input.EmployeeID, input.Priority, input.NextFollowUpAt, performedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not assign leads: " + err.Error()})
		return
	}

	ctrl.notifyAssignment(c, input.EmployeeID, performedBy, count)
	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "assign", EntityType: "lead",
		Metadata: map[string]interface{}{"lead_short_ids": input.LeadShortIDs, "employee_id": input.EmployeeID},
	})

	c.JSON(http.StatusOK, gin.H{"assigned": count})
}

// Reassign godoc
//
//	@Summary		Reassign leads to a different employee
//	@Description	Moves already-assigned leads to a different employee. Restricted to super_admin/team_lead.
//	@Tags			leads
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.ReassignLeadsInput	true	"Reassignment details"
//	@Success		200		{object}	map[string]int	"assigned count"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/reassign [post]
func (ctrl *LeadController) Reassign(c *gin.Context) {
	var input models.ReassignLeadsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	performedBy := c.GetString("user_id")
	count, err := ctrl.leadRepo.Reassign(c.Request.Context(), input.LeadShortIDs, input.EmployeeID, input.Priority, input.NextFollowUpAt, performedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reassign leads: " + err.Error()})
		return
	}

	ctrl.notifyAssignment(c, input.EmployeeID, performedBy, count)
	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "reassign", EntityType: "lead",
		Metadata: map[string]interface{}{"lead_short_ids": input.LeadShortIDs, "employee_id": input.EmployeeID},
	})

	c.JSON(http.StatusOK, gin.H{"assigned": count})
}

// Unassign godoc
//
//	@Summary		Unassign leads
//	@Description	Clears assignment on the given leads. Restricted to super_admin/team_lead.
//	@Tags			leads
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.UnassignLeadsInput	true	"Lead short IDs to unassign"
//	@Success		200		{object}	map[string]int	"unassigned count"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/unassign [post]
func (ctrl *LeadController) Unassign(c *gin.Context) {
	var input models.UnassignLeadsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	performedBy := c.GetString("user_id")
	count, err := ctrl.leadRepo.Unassign(c.Request.Context(), input.LeadShortIDs, performedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not unassign leads: " + err.Error()})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "unassign", EntityType: "lead",
		Metadata: map[string]interface{}{"lead_short_ids": input.LeadShortIDs},
	})

	c.JSON(http.StatusOK, gin.H{"unassigned": count})
}

// AutoAssign godoc
//
//	@Summary		Auto-assign leads (least-loaded round robin)
//	@Description	Assigns each given lead to whichever eligible employee (role employee/team_lead) currently has the fewest active leads, rebalancing after each assignment. Restricted to super_admin/team_lead.
//	@Tags			leads
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.AutoAssignInput	true	"Lead short IDs to auto-assign"
//	@Success		200		{object}	map[string]int	"assigned count"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/auto-assign [post]
func (ctrl *LeadController) AutoAssign(c *gin.Context) {
	var input models.AutoAssignInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	performedBy := c.GetString("user_id")
	count, err := ctrl.leadRepo.AutoAssignRoundRobin(c.Request.Context(), input.LeadShortIDs, performedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not auto-assign leads: " + err.Error()})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "assign", EntityType: "lead",
		Metadata: map[string]interface{}{"lead_short_ids": input.LeadShortIDs, "auto": true},
	})

	c.JSON(http.StatusOK, gin.H{"assigned": count})
}

// notifyAssignment tells the target employee they've received new lead(s).
func (ctrl *LeadController) notifyAssignment(c *gin.Context, employeeID, performedBy string, count int) {
	if count == 0 {
		return
	}
	title := "New lead assigned"
	message := "You've been assigned a new lead to follow up on."
	if count > 1 {
		message = "You've been assigned new leads to follow up on."
	}
	_ = ctrl.notificationRepo.NotifyUsers(c.Request.Context(), title, message, "lead", "lead", "", performedBy, []string{employeeID})
}

// GetAssignmentHistory godoc
//
//	@Summary		Lead assignment history
//	@Description	Returns every assign/reassign/unassign event for one lead, newest first. Restricted to super_admin/team_lead.
//	@Tags			leads
//	@Produce		json
//	@Param			short_id	path	string	true	"Lead short ID"
//	@Success		200			{array}	models.LeadAssignmentHistoryEntry
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/{short_id}/history [get]
func (ctrl *LeadController) GetAssignmentHistory(c *gin.Context) {
	shortID := c.Param("short_id")
	history, err := ctrl.leadAssignmentHistoryRepo.FindAllForLead(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch assignment history"})
		return
	}
	if history == nil {
		history = []models.LeadAssignmentHistoryEntry{}
	}
	c.JSON(http.StatusOK, history)
}

// GetMyDashboard godoc
//
//	@Summary		My lead dashboard
//	@Description	Returns the calling employee's (or team_lead's personal-quota) daily lead-work summary cards.
//	@Tags			leads
//	@Produce		json
//	@Success		200	{object}	models.LeadDashboard
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/dashboard/me [get]
func (ctrl *LeadController) GetMyDashboard(c *gin.Context) {
	userID := c.GetString("user_id")
	dashboard, err := ctrl.leadRepo.GetDashboard(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute dashboard"})
		return
	}
	c.JSON(http.StatusOK, dashboard)
}

// GetTeamDashboard godoc
//
//	@Summary		Team lead-monitoring dashboard
//	@Description	Returns one row per employee/team_lead — active leads, calls made today, conversions. Restricted to super_admin/team_lead.
//	@Tags			leads
//	@Produce		json
//	@Success		200	{array}		models.EmployeeLeadSummary
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leads/dashboard/team [get]
func (ctrl *LeadController) GetTeamDashboard(c *gin.Context) {
	summary, err := ctrl.leadRepo.GetTeamSummary(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute team dashboard"})
		return
	}
	if summary == nil {
		summary = []models.EmployeeLeadSummary{}
	}
	c.JSON(http.StatusOK, summary)
}
