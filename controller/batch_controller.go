package controller

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

type BatchController struct {
	batchRepo      *repository.BatchRepository
	enrollmentRepo *repository.EnrollmentRepository
	notificationRepo *repository.NotificationRepository
	userRepo         *repository.UserRepository
	collegeRepo      *repository.CollegeRepository
	courseRepo       *repository.CourseRepository
	emailSvc         *service.EmailService
	auditLogRepo     *repository.AuditLogRepository
	communityRepo    *repository.CommunityRepository
}

func NewBatchController(batchRepo *repository.BatchRepository, enrollmentRepo *repository.EnrollmentRepository, notificationRepo *repository.NotificationRepository, userRepo *repository.UserRepository, collegeRepo *repository.CollegeRepository, courseRepo *repository.CourseRepository, emailSvc *service.EmailService, auditLogRepo *repository.AuditLogRepository, communityRepo *repository.CommunityRepository) *BatchController {
	return &BatchController{batchRepo: batchRepo, enrollmentRepo: enrollmentRepo, notificationRepo: notificationRepo, userRepo: userRepo, collegeRepo: collegeRepo, courseRepo: courseRepo, emailSvc: emailSvc, auditLogRepo: auditLogRepo, communityRepo: communityRepo}
}

// batchDateLayout matches how Batch.StartDate/EndDate are stored — plain
// "YYYY-MM-DD" strings (see batchBaseSelect's start_date::TEXT/end_date::TEXT).
const batchDateLayout = "2006-01-02"

func parseBatchDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(batchDateLayout, s)
	if err != nil {
		return nil
	}
	return &t
}

// emailBatchManagers emails the batch manager and additional manager (if set)
// about a newly created batch.
func (ctrl *BatchController) emailBatchManagers(ctx context.Context, batch *models.Batch, subject, html string) {
	if manager, err := ctrl.userRepo.FindByID(ctx, batch.BatchManagerID); err != nil {
		log.Printf("fetch batch manager for email: %v", err)
	} else if manager != nil && manager.Email != "" {
		ctrl.emailSvc.SendAsync(manager.Email, subject, html)
	}
	if batch.AdditionalManagerID == "" {
		return
	}
	if am, err := ctrl.userRepo.FindByID(ctx, batch.AdditionalManagerID); err != nil {
		log.Printf("fetch additional manager for email: %v", err)
	} else if am != nil && am.Email != "" {
		ctrl.emailSvc.SendAsync(am.Email, subject, html)
	}
}

// CreateBatch godoc
//
//	@Summary		Create batch
//	@Description	Create a new batch. Restricted to super_admin. A short unique ID is generated automatically. Provide course_short_id to link to a course. Optionally pass student_ids to enroll students immediately — students can also be added later via POST /batches/{short_id}/students.
//	@Tags			batches
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateBatchInput	true	"Batch details"
//	@Success		201		{object}	models.Batch
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		403		{object}	map[string]string	"Forbidden"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches [post]
func (ctrl *BatchController) Create(c *gin.Context) {
	var input models.CreateBatchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdBy := c.GetString("user_id")

	collegeID, err := resolveTargetCollege(c.Request.Context(), ctrl.collegeRepo, c.GetString("role"), c.GetString("college_id"), input.CollegeShortID)
	if err != nil {
		if errors.Is(err, repository.ErrCollegeScopeRequired) {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope to create a batch under"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not resolve target college: " + err.Error()})
		return
	}

	// The course must actually be available to this college — either
	// organization-owned by it, or a global course explicitly assigned to
	// it. Fails open (skips the check) if Stage 5's course_scope/
	// college_courses columns aren't applied yet.
	if course, cErr := ctrl.courseRepo.FindByShortID(c.Request.Context(), input.CourseShortID); cErr == nil && course != nil {
		if available, ok := ctrl.courseRepo.IsAvailableToCollege(c.Request.Context(), course.ID, collegeID); ok && !available {
			c.JSON(http.StatusBadRequest, gin.H{"code": "COURSE_NOT_ASSIGNED", "error": "this course is not assigned to your college"})
			return
		}
	}

	// The batch manager must belong to the same college as the batch —
	// users.college_id already exists (not migration-pending), so this is
	// always enforced, not fail-open.
	if manager, mErr := ctrl.userRepo.FindByID(c.Request.Context(), input.BatchManagerID); mErr == nil && manager != nil {
		if manager.CollegeID != "" && manager.CollegeID != collegeID {
			c.JSON(http.StatusBadRequest, gin.H{"code": "BATCH_COLLEGE_MISMATCH", "error": "the batch manager does not belong to this college"})
			return
		}
	}

	batch, err := ctrl.batchRepo.Create(c.Request.Context(), input, createdBy, collegeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create batch: " + err.Error()})
		return
	}

	subject, html := service.BatchCreatedEmail(batch.BatchNumber, batch.CourseName, batch.StartDate, batch.EndDate)

	if len(input.StudentIDs) > 0 {
		now := time.Now()
		startDate := parseBatchDate(batch.StartDate)
		isLate := startDate != nil && now.After(*startDate)
		accessEnd := parseBatchDate(batch.EndDate)
		added, err := ctrl.batchRepo.AddStudentsWithEnrollment(
			c.Request.Context(), batch.ShortID, input.StudentIDs, createdBy,
			batch.CourseID, batch.ID, isLate, &now, accessEnd,
		)
		if err != nil {
			log.Printf("add students on batch create: %v", err)
		} else if len(added) > 0 {
			if refreshed, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), batch.ShortID); err == nil && refreshed != nil {
				batch = refreshed
			}
			if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(),
				"Added to batch: "+batch.BatchNumber,
				fmt.Sprintf("You've been enrolled in batch %q.", batch.BatchNumber),
				"batch", "batch", batch.ShortID, createdBy, added,
			); err != nil {
				log.Printf("notify batch add students: %v", err)
			}

			if ctrl.emailSvc.Configured() {
				for _, studentID := range added {
					student, err := ctrl.userRepo.FindByID(c.Request.Context(), studentID)
					if err != nil {
						log.Printf("fetch student for batch created email: %v", err)
						continue
					}
					if student != nil && student.Email != "" {
						ctrl.emailSvc.SendAsync(student.Email, subject, html)
					}
				}
			}
		}
	}

	if err := ctrl.notificationRepo.NotifyRoles(c.Request.Context(),
		"New batch: "+batch.BatchNumber,
		fmt.Sprintf("A new batch %q has been created.", batch.BatchNumber),
		"batch", "batch", batch.ShortID, createdBy,
		[]string{"mentor", "team_lead"},
	); err != nil {
		log.Printf("notify batch create: %v", err)
	}

	if ctrl.emailSvc.Configured() {
		ctrl.emailBatchManagers(c.Request.Context(), batch, subject, html)
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "create", EntityType: "batch",
		EntityID: batch.ID, EntityShortID: batch.ShortID, EntityLabel: batch.BatchNumber,
		BatchShortID: batch.ShortID,
	})

	c.JSON(http.StatusCreated, batch)
}

// GetAllBatches godoc
//
//	@Summary		List batches
//	@Description	Returns all non-deleted batches with full course and manager details.
//	@Tags			batches
//	@Produce		json
//	@Success		200	{array}		models.Batch
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches [get]
func (ctrl *BatchController) GetAll(c *gin.Context) {
	role := c.GetString("role")

	collegeID, err := repository.CollegeFilter(role, c.GetString("college_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
		return
	}

	var batches []models.Batch
	if role == string(models.RoleMentor) || role == string(models.RoleEmployee) {
		batches, err = ctrl.batchRepo.FindAllForMentor(c.Request.Context(), c.GetString("user_id"), collegeID)
	} else {
		batches, err = ctrl.batchRepo.FindAll(c.Request.Context(), collegeID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch batches"})
		return
	}
	if batches == nil {
		batches = []models.Batch{}
	}
	c.JSON(http.StatusOK, batches)
}

// GetBatch godoc
//
//	@Summary		Get batch
//	@Description	Returns a single batch by its short_id, with full course and manager details.
//	@Tags			batches
//	@Produce		json
//	@Param			short_id	path		string	true	"Batch short ID"
//	@Success		200			{object}	models.Batch
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id} [get]
func (ctrl *BatchController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")
	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch batch"})
		return
	}
	if batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	c.JSON(http.StatusOK, batch)
}

// FilterBatches godoc
//
//	@Summary		Filter batches
//	@Description	Filter non-deleted batches by any combination of fields. All query params are optional.
//	@Tags			batches
//	@Produce		json
//	@Param			batch_number		query		string	false	"Partial batch number"
//	@Param			course_short_id		query		string	false	"Exact course short ID"
//	@Param			batch_manager_id	query		string	false	"Exact batch manager user ID (UUID)"
//	@Param			module				query		string	false	"Partial module name"
//	@Param			start_date			query		string	false	"Exact start date (YYYY-MM-DD)"
//	@Param			end_date			query		string	false	"Exact end date (YYYY-MM-DD)"
//	@Param			is_active			query		string	false	"true or false"
//	@Success		200	{array}		models.Batch
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/filter [get]
func (ctrl *BatchController) Filter(c *gin.Context) {
	var f models.BatchFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	batches, err := ctrl.batchRepo.Filter(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not filter batches"})
		return
	}

	role := c.GetString("role")
	if role == string(models.RoleMentor) || role == string(models.RoleEmployee) {
		userID := c.GetString("user_id")
		scoped := make([]models.Batch, 0, len(batches))
		for _, b := range batches {
			if b.BatchManagerID == userID || (b.AdditionalManagerID != "" && b.AdditionalManagerID == userID) {
				scoped = append(scoped, b)
			}
		}
		batches = scoped
	}

	if batches == nil {
		batches = []models.Batch{}
	}
	c.JSON(http.StatusOK, batches)
}

// GetMyBatches godoc
//
//	@Summary		List my batches
//	@Description	Returns every non-deleted batch the logged-in user is enrolled in as a student, most recently joined first.
//	@Tags			batches
//	@Produce		json
//	@Success		200	{array}		models.Batch
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/mine [get]
func (ctrl *BatchController) GetMine(c *gin.Context) {
	userID := c.GetString("user_id")

	batches, err := ctrl.batchRepo.FindByStudentID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch batches"})
		return
	}
	if batches == nil {
		batches = []models.Batch{}
	}
	c.JSON(http.StatusOK, batches)
}

// GetByStudentID godoc
//
//	@Summary		List a student's batches
//	@Description	Returns every batch a given student is enrolled in. Restricted to super_admin / team_lead / mentor — for a mentor/employee to view a student's real enrollment on their profile page.
//	@Tags			batches
//	@Produce		json
//	@Param			id	path		string	true	"Student user ID"
//	@Success		200	{array}		models.Batch
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/batches [get]
func (ctrl *BatchController) GetByStudentID(c *gin.Context) {
	userID := c.Param("id")

	batches, err := ctrl.batchRepo.FindByStudentID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch batches"})
		return
	}
	if batches == nil {
		batches = []models.Batch{}
	}
	c.JSON(http.StatusOK, batches)
}

// UpdateBatch godoc
//
//	@Summary		Update batch
//	@Description	Partially update a batch by its short_id. Send only the fields you want to change. Use is_active to activate or deactivate. Set additional_manager_id to "" to clear it.
//	@Tags			batches
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string					true	"Batch short ID"
//	@Param			body		body		models.UpdateBatchInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.Batch
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id} [patch]
func (ctrl *BatchController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateBatchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.batchRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update batch"})
		return
	}

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated batch"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "update", EntityType: "batch",
		EntityID: batch.ID, EntityShortID: batch.ShortID, EntityLabel: batch.BatchNumber,
		BatchShortID: batch.ShortID,
	})

	c.JSON(http.StatusOK, batch)
}

// AddBatchStudents godoc
//
//	@Summary		Add students to batch
//	@Description	Enroll one or more students into a batch by user ID. Only users with role=student are matched; students already enrolled are left unchanged. Rejected if the batch is at capacity, or if any student already holds an active enrollment in another batch of the same course. Restricted to super_admin / team_lead.
//	@Tags			batches
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Batch short ID"
//	@Param			body		body		models.AddBatchStudentsInput	true	"Student user IDs to add"
//	@Success		200			{object}	map[string]int	"Number of students added"
//	@Failure		400			{object}	map[string]string	"Validation error, batch full, or already active in another batch of this course"
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/students [post]
func (ctrl *BatchController) AddStudents(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.AddBatchStudentsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	if batch.MaxStudents != nil && batch.StudentCount+len(input.StudentIDs) > *batch.MaxStudents {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf(
			"this batch allows at most %d students (currently %d enrolled)", *batch.MaxStudents, batch.StudentCount,
		)})
		return
	}

	for _, studentID := range input.StudentIDs {
		conflict, err := ctrl.enrollmentRepo.HasActiveEnrollmentInCourse(c.Request.Context(), studentID, batch.CourseID, batch.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify existing enrollment"})
			return
		}
		if conflict {
			c.JSON(http.StatusBadRequest, gin.H{"error": "one or more students already have an active enrollment in another batch of this course — remove or transfer them from that batch first"})
			return
		}
	}

	addedBy := c.GetString("user_id")

	now := time.Now()
	startDate := parseBatchDate(batch.StartDate)
	isLate := startDate != nil && now.After(*startDate)
	accessEnd := parseBatchDate(batch.EndDate)

	added, err := ctrl.batchRepo.AddStudentsWithEnrollment(
		c.Request.Context(), shortID, input.StudentIDs, addedBy,
		batch.CourseID, batch.ID, isLate, &now, accessEnd,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add students: " + err.Error()})
		return
	}

	if len(added) > 0 {
		if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(),
			"Added to batch: "+batch.BatchNumber,
			fmt.Sprintf("You've been enrolled in batch %q.", batch.BatchNumber),
			"batch", "batch", shortID, addedBy, added,
		); err != nil {
			log.Printf("notify batch add students: %v", err)
		}

		// Best-effort — a batch without a community is normal (communities
		// are optional), and this must never block the core enrollment.
		if err := ctrl.communityRepo.AddMembersByBatchShortID(c.Request.Context(), shortID, added, addedBy); err != nil {
			log.Printf("sync community membership for batch add students: %v", err)
		}

		logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
			Action: "enroll", EntityType: "enrollment",
			EntityShortID: shortID, EntityLabel: batch.BatchNumber,
			BatchShortID: batch.ShortID,
			Metadata: map[string]interface{}{"student_ids": added, "count": len(added)},
		})
	}

	c.JSON(http.StatusOK, gin.H{"added": len(added)})
}

// GetBatchStudents godoc
//
//	@Summary		List batch students
//	@Description	Returns every student enrolled in a batch along with the total student count.
//	@Tags			batches
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		200			{object}	map[string]interface{}	"total_students and students[]"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/students [get]
func (ctrl *BatchController) GetStudents(c *gin.Context) {
	shortID := c.Param("short_id")

	if !checkBatchAccess(c, ctrl.batchRepo, shortID) {
		return
	}

	students, err := ctrl.batchRepo.GetStudents(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch students"})
		return
	}
	if students == nil {
		students = []models.BatchStudent{}
	}
	c.JSON(http.StatusOK, gin.H{"total_students": len(students), "students": students})
}

// GetMyFeesStatus godoc
//
//	@Summary		Get my fee-payment status for a batch
//	@Description	Returns the logged-in user's own fees_paid status for a batch — never exposes other students' data. Use this to decide whether to show session recordings.
//	@Tags			batches
//	@Produce		json
//	@Param			short_id	path		string	true	"Batch short ID"
//	@Success		200			{object}	map[string]bool	"fees_paid"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/me/fees [get]
func (ctrl *BatchController) GetMyFeesStatus(c *gin.Context) {
	shortID := c.Param("short_id")
	userID := c.GetString("user_id")

	paid, err := ctrl.batchRepo.IsFeesPaidByBatchShortID(c.Request.Context(), shortID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch fee status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"fees_paid": paid})
}

// RemoveBatchStudent godoc
//
//	@Summary		Remove student from batch
//	@Description	Removes a single student from a batch by user ID. Restricted to super_admin / team_lead.
//	@Tags			batches
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Param			user_id		path	string	true	"Student user ID (UUID)"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Student not enrolled in batch"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/students/{user_id} [delete]
func (ctrl *BatchController) RemoveStudent(c *gin.Context) {
	shortID := c.Param("short_id")
	userID := c.Param("user_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	if err := ctrl.batchRepo.RemoveStudent(c.Request.Context(), shortID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "student not enrolled in batch"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not remove student"})
		return
	}

	if err := ctrl.enrollmentRepo.UpdateStatus(c.Request.Context(), userID, batch.ID, models.EnrollmentRemoved); err != nil {
		log.Printf("mark enrollment removed for %s in batch %s: %v", userID, shortID, err)
	}
	if err := ctrl.communityRepo.RemoveMemberByBatchShortID(c.Request.Context(), shortID, userID); err != nil {
		log.Printf("remove community membership for %s in batch %s: %v", userID, shortID, err)
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "remove", EntityType: "enrollment",
		EntityShortID: userID, EntityLabel: batch.BatchNumber,
		BatchShortID: batch.ShortID,
	})

	c.Status(http.StatusNoContent)
}

// TransferStudent godoc
//
//	@Summary		Transfer a student to another batch
//	@Description	Moves a student from this batch to a different batch of the SAME course (schedule conflict, batch merge, etc). Their old enrollment is marked "transferred" and kept for history; a new "active" enrollment is created for the destination batch. Restricted to super_admin / team_lead.
//	@Tags			batches
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string							true	"Source batch short ID"
//	@Param			user_id		path	string							true	"Student user ID (UUID)"
//	@Param			body		body	models.TransferStudentInput	true	"Destination batch"
//	@Success		204			"No Content"
//	@Failure		400			{object}	map[string]string	"Validation error, different course, or destination batch full"
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/students/{user_id}/transfer [post]
func (ctrl *BatchController) TransferStudent(c *gin.Context) {
	fromShortID := c.Param("short_id")
	userID := c.Param("user_id")

	var input models.TransferStudentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fromBatch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), fromShortID)
	if err != nil || fromBatch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source batch not found"})
		return
	}
	toBatch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), input.ToBatchShortID)
	if err != nil || toBatch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "destination batch not found"})
		return
	}
	if toBatch.CourseID != fromBatch.CourseID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "can only transfer a student to another batch of the same course"})
		return
	}
	if toBatch.MaxStudents != nil && toBatch.StudentCount+1 > *toBatch.MaxStudents {
		c.JSON(http.StatusBadRequest, gin.H{"error": "destination batch is full"})
		return
	}

	actorID := c.GetString("user_id")

	if _, err := ctrl.enrollmentRepo.Transfer(c.Request.Context(), userID, fromBatch.ID, toBatch.ID, toBatch.CourseID, actorID); err != nil {
		if errors.Is(err, repository.ErrBatchCollegeMismatch) {
			c.JSON(http.StatusBadRequest, gin.H{"code": "BATCH_COLLEGE_MISMATCH", "error": "the student and destination batch belong to different colleges"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not transfer student: " + err.Error()})
		return
	}

	if err := ctrl.batchRepo.RemoveStudent(c.Request.Context(), fromShortID, userID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("remove from source batch during transfer: %v", err)
	}
	if _, err := ctrl.batchRepo.AddStudents(c.Request.Context(), toBatch.ShortID, []string{userID}, actorID); err != nil {
		log.Printf("add to destination batch during transfer: %v", err)
	}
	if err := ctrl.communityRepo.RemoveMemberByBatchShortID(c.Request.Context(), fromShortID, userID); err != nil {
		log.Printf("remove community membership during transfer: %v", err)
	}
	if err := ctrl.communityRepo.AddMembersByBatchShortID(c.Request.Context(), toBatch.ShortID, []string{userID}, actorID); err != nil {
		log.Printf("sync community membership during transfer: %v", err)
	}

	if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(),
		"Moved to a new batch: "+toBatch.BatchNumber,
		fmt.Sprintf("You've been moved from batch %q to batch %q.", fromBatch.BatchNumber, toBatch.BatchNumber),
		"batch", "batch", toBatch.ShortID, actorID, []string{userID},
	); err != nil {
		log.Printf("notify transfer: %v", err)
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "transfer", EntityType: "enrollment",
		EntityShortID: userID, EntityLabel: fmt.Sprintf("%s -> %s", fromBatch.BatchNumber, toBatch.BatchNumber),
		BatchShortID: toBatch.ShortID,
		Metadata: map[string]interface{}{"from_batch_short_id": fromShortID, "to_batch_short_id": toBatch.ShortID},
	})

	c.Status(http.StatusNoContent)
}

// UpdateEnrollmentStatus godoc
//
//	@Summary		Update a student's enrollment status
//	@Description	Changes a student's enrollment status within a batch (e.g. on_hold, completed, dropped) without touching their roster membership — use DELETE .../students/{user_id} to actually remove them. Restricted to super_admin / team_lead / mentor.
//	@Tags			batches
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string								true	"Batch short ID"
//	@Param			user_id		path	string								true	"Student user ID (UUID)"
//	@Param			body		body	models.UpdateEnrollmentStatusInput	true	"New status"
//	@Success		204			"No Content"
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Batch or enrollment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/students/{user_id}/enrollment [patch]
func (ctrl *BatchController) UpdateEnrollmentStatus(c *gin.Context) {
	shortID := c.Param("short_id")
	userID := c.Param("user_id")

	var input models.UpdateEnrollmentStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !checkBatchAccess(c, ctrl.batchRepo, shortID) {
		return
	}

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	if err := ctrl.enrollmentRepo.UpdateStatus(c.Request.Context(), userID, batch.ID, input.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "enrollment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update enrollment status"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "update", EntityType: "enrollment",
		EntityShortID: userID, EntityLabel: batch.BatchNumber,
		BatchShortID: batch.ShortID,
		Metadata: map[string]interface{}{"status": input.Status},
	})

	c.Status(http.StatusNoContent)
}

// GetStudentEnrollments godoc
//
//	@Summary		List a student's enrollments
//	@Description	Returns every enrollment a student has ever had — one per (course, batch) — newest first. This is the real, per-course enrollment history backing a student's multi-course dashboard. Restricted to super_admin / team_lead / mentor, or the student themselves.
//	@Tags			batches
//	@Produce		json
//	@Param			id	path		string	true	"Student user ID"
//	@Success		200	{array}		models.StudentEnrollment
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/enrollments [get]
func (ctrl *BatchController) GetStudentEnrollments(c *gin.Context) {
	userID := c.Param("id")

	enrollments, err := ctrl.enrollmentRepo.FindByStudent(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch enrollments"})
		return
	}
	if enrollments == nil {
		enrollments = []models.StudentEnrollment{}
	}
	c.JSON(http.StatusOK, enrollments)
}

// SetStudentFeesPaid godoc
//
//	@Summary		Set a student's fee-payment status
//	@Description	Marks whether a student has fully paid fees for a batch. Live sessions stay accessible to every enrolled student regardless of this flag — it only gates access to session recordings. Restricted to super_admin / team_lead / mentor.
//	@Tags			batches
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Batch short ID"
//	@Param			user_id		path		string							true	"Student user ID (UUID)"
//	@Param			body		body		models.UpdateFeesPaidInput		true	"Fee-payment status"
//	@Success		200			{object}	map[string]string
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Student not enrolled in batch"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/students/{user_id}/fees [patch]
func (ctrl *BatchController) SetStudentFeesPaid(c *gin.Context) {
	shortID := c.Param("short_id")
	userID := c.Param("user_id")

	var input models.UpdateFeesPaidInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !checkBatchAccess(c, ctrl.batchRepo, shortID) {
		return
	}

	if err := ctrl.batchRepo.SetFeesPaid(c.Request.Context(), shortID, userID, *input.FeesPaid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "student not enrolled in batch"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update fee status"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "update", EntityType: "fees",
		EntityShortID: userID,
		Metadata: map[string]interface{}{"fees_paid": *input.FeesPaid},
	})

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// DeleteBatch godoc
//
//	@Summary		Delete batch
//	@Description	Soft-delete a batch by its short_id.
//	@Tags			batches
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id} [delete]
func (ctrl *BatchController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.batchRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete batch"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "delete", EntityType: "batch", EntityShortID: shortID,
	})

	c.Status(http.StatusNoContent)
}
