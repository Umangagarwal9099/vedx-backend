package controller

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

type BatchController struct {
	batchRepo        *repository.BatchRepository
	notificationRepo *repository.NotificationRepository
	userRepo         *repository.UserRepository
	emailSvc         *service.EmailService
}

func NewBatchController(batchRepo *repository.BatchRepository, notificationRepo *repository.NotificationRepository, userRepo *repository.UserRepository, emailSvc *service.EmailService) *BatchController {
	return &BatchController{batchRepo: batchRepo, notificationRepo: notificationRepo, userRepo: userRepo, emailSvc: emailSvc}
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

	batch, err := ctrl.batchRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create batch: " + err.Error()})
		return
	}

	subject, html := service.BatchCreatedEmail(batch.BatchNumber, batch.CourseName, batch.StartDate, batch.EndDate)

	if len(input.StudentIDs) > 0 {
		added, err := ctrl.batchRepo.AddStudents(c.Request.Context(), batch.ShortID, input.StudentIDs, createdBy)
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
	batches, err := ctrl.batchRepo.FindAll(c.Request.Context())
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

	c.JSON(http.StatusOK, batch)
}

// AddBatchStudents godoc
//
//	@Summary		Add students to batch
//	@Description	Enroll one or more students into a batch by user ID. Only users with role=student are matched; students already enrolled are left unchanged. Restricted to super_admin / team_lead.
//	@Tags			batches
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Batch short ID"
//	@Param			body		body		models.AddBatchStudentsInput	true	"Student user IDs to add"
//	@Success		200			{object}	map[string]int	"Number of students added"
//	@Failure		400			{object}	map[string]string	"Validation error"
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

	addedBy := c.GetString("user_id")

	added, err := ctrl.batchRepo.AddStudents(c.Request.Context(), shortID, input.StudentIDs, addedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add students: " + err.Error()})
		return
	}

	if len(added) > 0 {
		batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
		if err == nil && batch != nil {
			if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(),
				"Added to batch: "+batch.BatchNumber,
				fmt.Sprintf("You've been enrolled in batch %q.", batch.BatchNumber),
				"batch", "batch", shortID, addedBy, added,
			); err != nil {
				log.Printf("notify batch add students: %v", err)
			}
		}
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

	if err := ctrl.batchRepo.RemoveStudent(c.Request.Context(), shortID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "student not enrolled in batch"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not remove student"})
		return
	}

	c.Status(http.StatusNoContent)
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

	if err := ctrl.batchRepo.SetFeesPaid(c.Request.Context(), shortID, userID, *input.FeesPaid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "student not enrolled in batch"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update fee status"})
		return
	}

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

	c.Status(http.StatusNoContent)
}
