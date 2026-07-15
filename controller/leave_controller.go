package controller

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

type LeaveController struct {
	leaveRepo   *repository.LeaveRequestRepository
	balanceRepo *repository.LeaveBalanceRepository
	userRepo    *repository.UserRepository
	emailSvc    *service.EmailService
}

func NewLeaveController(leaveRepo *repository.LeaveRequestRepository, balanceRepo *repository.LeaveBalanceRepository, userRepo *repository.UserRepository, emailSvc *service.EmailService) *LeaveController {
	return &LeaveController{leaveRepo: leaveRepo, balanceRepo: balanceRepo, userRepo: userRepo, emailSvc: emailSvc}
}

// leaveDayCount returns the inclusive day-count between from/to, halved for
// half_day requests.
func leaveDayCount(fromDate, toDate, leaveType string) float64 {
	from, err1 := time.Parse("2006-01-02", fromDate)
	to, err2 := time.Parse("2006-01-02", toDate)
	if err1 != nil || err2 != nil || to.Before(from) {
		return 0
	}
	days := to.Sub(from).Hours()/24 + 1
	if leaveType == "half_day" {
		days = days * 0.5
	}
	return days
}

// Apply godoc
//
//	@Summary		Apply for leave
//	@Description	Submits a leave request in 'pending' status for the calling employee/team_lead.
//	@Tags			leaves
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateLeaveRequestInput	true	"Leave request details"
//	@Success		201		{object}	models.LeaveRequest
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leaves [post]
func (ctrl *LeaveController) Apply(c *gin.Context) {
	var input models.CreateLeaveRequestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	employeeID := c.GetString("user_id")
	leave, err := ctrl.leaveRepo.Apply(c.Request.Context(), employeeID, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not submit leave request: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, leave)
}

// GetMine godoc
//
//	@Summary		My leave requests
//	@Description	Returns the calling employee/team_lead's own leave requests, newest first.
//	@Tags			leaves
//	@Produce		json
//	@Success		200	{array}		models.LeaveRequest
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leaves/me [get]
func (ctrl *LeaveController) GetMine(c *gin.Context) {
	employeeID := c.GetString("user_id")
	leaves, err := ctrl.leaveRepo.FindAllForEmployee(c.Request.Context(), employeeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch leave requests"})
		return
	}
	if leaves == nil {
		leaves = []models.LeaveRequest{}
	}
	c.JSON(http.StatusOK, leaves)
}

// GetMyBalance godoc
//
//	@Summary		My leave balance
//	@Description	Returns the calling employee/team_lead's leave balance for the current year.
//	@Tags			leaves
//	@Produce		json
//	@Success		200	{object}	models.LeaveBalance
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leaves/me/balance [get]
func (ctrl *LeaveController) GetMyBalance(c *gin.Context) {
	employeeID := c.GetString("user_id")
	balance, err := ctrl.balanceRepo.GetOrInit(c.Request.Context(), employeeID, time.Now().Year())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch leave balance"})
		return
	}
	c.JSON(http.StatusOK, balance)
}

// GetAllPending godoc
//
//	@Summary		Pending leave requests
//	@Description	Returns every pending leave request across all employees, oldest first. Restricted to super_admin/team_lead.
//	@Tags			leaves
//	@Produce		json
//	@Success		200	{array}		models.LeaveRequest
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leaves/pending [get]
func (ctrl *LeaveController) GetAllPending(c *gin.Context) {
	leaves, err := ctrl.leaveRepo.FindAllPending(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch pending leave requests"})
		return
	}
	if leaves == nil {
		leaves = []models.LeaveRequest{}
	}
	c.JSON(http.StatusOK, leaves)
}

// GetAll godoc
//
//	@Summary		All leave requests
//	@Description	Returns every leave request across all employees regardless of status, newest first. Restricted to super_admin/team_lead.
//	@Tags			leaves
//	@Produce		json
//	@Success		200	{array}		models.LeaveRequest
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leaves [get]
func (ctrl *LeaveController) GetAll(c *gin.Context) {
	leaves, err := ctrl.leaveRepo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch leave requests"})
		return
	}
	if leaves == nil {
		leaves = []models.LeaveRequest{}
	}
	c.JSON(http.StatusOK, leaves)
}

// Review godoc
//
//	@Summary		Approve or reject a leave request
//	@Description	Records the admin's decision. On approval, increments the employee's used-days balance and emails them the decision. Restricted to super_admin/team_lead.
//	@Tags			leaves
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Leave request short ID"
//	@Param			body		body		models.ReviewLeaveRequestInput	true	"Decision"
//	@Success		200			{object}	models.LeaveRequest
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Leave request not found or already reviewed"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/leaves/{short_id}/review [patch]
func (ctrl *LeaveController) Review(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.ReviewLeaveRequestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	leave, err := ctrl.leaveRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch leave request"})
		return
	}
	if leave == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "leave request not found"})
		return
	}

	reviewedBy := c.GetString("user_id")
	if err := ctrl.leaveRepo.Review(c.Request.Context(), shortID, input.Status, input.AdminNote, reviewedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "leave request not found or already reviewed"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not review leave request"})
		return
	}

	if input.Status == "approved" {
		year, _ := parseYear(leave.FromDate)
		days := leaveDayCount(leave.FromDate, leave.ToDate, leave.LeaveType)
		if err := ctrl.balanceRepo.AddUsedDays(c.Request.Context(), leave.EmployeeID, year, days); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "leave approved, but could not update balance"})
			return
		}
	}

	if ctrl.emailSvc != nil && ctrl.emailSvc.Configured() {
		if employee, err := ctrl.userRepo.FindByID(c.Request.Context(), leave.EmployeeID); err == nil && employee != nil {
			subject, html := service.LeaveDecisionEmail(employee.FirstName, input.Status, leave.FromDate, leave.ToDate, input.AdminNote)
			ctrl.emailSvc.SendAsync(employee.Email, subject, html)
		}
	}

	updated, err := ctrl.leaveRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || updated == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated leave request"})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func parseYear(dateStr string) (int, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Now().Year(), err
	}
	return t.Year(), nil
}
