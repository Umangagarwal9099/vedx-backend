package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type EmployeeAttendanceController struct {
	attendanceRepo *repository.EmployeeAttendanceRepository
}

func NewEmployeeAttendanceController(attendanceRepo *repository.EmployeeAttendanceRepository) *EmployeeAttendanceController {
	return &EmployeeAttendanceController{attendanceRepo: attendanceRepo}
}

// CheckIn godoc
//
//	@Summary		Check in for today
//	@Description	Self-service check-in for the calling employee/team_lead — creates today's attendance row if it doesn't exist yet.
//	@Tags			attendance
//	@Produce		json
//	@Success		200	{object}	models.EmployeeAttendance
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/attendance/employee/check-in [post]
func (ctrl *EmployeeAttendanceController) CheckIn(c *gin.Context) {
	employeeID := c.GetString("user_id")
	a, err := ctrl.attendanceRepo.CheckIn(c.Request.Context(), employeeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check in: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, a)
}

// CheckOut godoc
//
//	@Summary		Check out for today
//	@Description	Self-service check-out for the calling employee/team_lead — requires a check-in already recorded today.
//	@Tags			attendance
//	@Produce		json
//	@Success		200	{object}	models.EmployeeAttendance
//	@Failure		400	{object}	map[string]string	"No check-in found for today"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/attendance/employee/check-out [post]
func (ctrl *EmployeeAttendanceController) CheckOut(c *gin.Context) {
	employeeID := c.GetString("user_id")
	a, err := ctrl.attendanceRepo.CheckOut(c.Request.Context(), employeeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, a)
}

// GetMine godoc
//
//	@Summary		My attendance history
//	@Description	Returns the calling employee/team_lead's own attendance history (last 90 rows) plus today's status.
//	@Tags			attendance
//	@Produce		json
//	@Success		200	{array}		models.EmployeeAttendance
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/attendance/employee/me [get]
func (ctrl *EmployeeAttendanceController) GetMine(c *gin.Context) {
	employeeID := c.GetString("user_id")
	history, err := ctrl.attendanceRepo.FindForEmployee(c.Request.Context(), employeeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch attendance"})
		return
	}
	if history == nil {
		history = []models.EmployeeAttendance{}
	}
	c.JSON(http.StatusOK, history)
}

// GetTeam godoc
//
//	@Summary		Team attendance for a date
//	@Description	Returns every employee/team_lead's attendance row for a given date (?date=YYYY-MM-DD, defaults to today). Restricted to super_admin/team_lead.
//	@Tags			attendance
//	@Produce		json
//	@Param			date	query	string	false	"Date (YYYY-MM-DD), defaults to today"
//	@Success		200		{array}	models.EmployeeAttendance
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/attendance/employee/team [get]
func (ctrl *EmployeeAttendanceController) GetTeam(c *gin.Context) {
	date := c.Query("date")
	rows, err := ctrl.attendanceRepo.FindAllForDate(c.Request.Context(), date)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch team attendance"})
		return
	}
	if rows == nil {
		rows = []models.EmployeeAttendance{}
	}
	c.JSON(http.StatusOK, rows)
}
