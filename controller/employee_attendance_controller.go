package controller

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/util"
)

type EmployeeAttendanceController struct {
	attendanceRepo     *repository.EmployeeAttendanceRepository
	officeLocationRepo *repository.OfficeLocationRepository
}

func NewEmployeeAttendanceController(attendanceRepo *repository.EmployeeAttendanceRepository, officeLocationRepo *repository.OfficeLocationRepository) *EmployeeAttendanceController {
	return &EmployeeAttendanceController{attendanceRepo: attendanceRepo, officeLocationRepo: officeLocationRepo}
}

// checkWithinOfficeRadius rejects the request if there's no active office
// location configured, or if (lat,lng) doesn't fall within any active
// location's radius. Returns the HTTP status + error message to write, or
// ("", "") if the location is fine to proceed.
func (ctrl *EmployeeAttendanceController) checkWithinOfficeRadius(ctx context.Context, lat, lng float64) (int, string) {
	if lat == 0 && lng == 0 {
		return http.StatusBadRequest, "location is required"
	}

	locations, err := ctrl.officeLocationRepo.GetAllActive(ctx)
	if err != nil {
		return http.StatusInternalServerError, "could not verify office location"
	}
	if len(locations) == 0 {
		return http.StatusBadRequest, "no office location configured yet"
	}

	nearestName := locations[0].Name
	nearestDist := util.HaversineMeters(lat, lng, locations[0].Latitude, locations[0].Longitude)
	for _, loc := range locations {
		dist := util.HaversineMeters(lat, lng, loc.Latitude, loc.Longitude)
		if dist <= float64(loc.RadiusMeters) {
			return 0, ""
		}
		if dist < nearestDist {
			nearestDist = dist
			nearestName = loc.Name
		}
	}
	return http.StatusForbidden, fmt.Sprintf("you're too far from %s to check in", nearestName)
}

// CheckIn godoc
//
//	@Summary		Check in for today
//	@Description	Self-service check-in for the calling employee/team_lead, verified against a registered office location's GPS radius plus a selfie. Creates today's attendance row if it doesn't exist yet.
//	@Tags			attendance
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CheckInInput	true	"Captured location + selfie"
//	@Success		200		{object}	models.EmployeeAttendance
//	@Failure		400		{object}	map[string]string	"Validation error, or no office location configured"
//	@Failure		403		{object}	map[string]string	"Outside every office location's radius"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/attendance/employee/check-in [post]
func (ctrl *EmployeeAttendanceController) CheckIn(c *gin.Context) {
	var input models.CheckInInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	if status, msg := ctrl.checkWithinOfficeRadius(ctx, input.Latitude, input.Longitude); status != 0 {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	employeeID := c.GetString("user_id")
	a, err := ctrl.attendanceRepo.CheckIn(ctx, employeeID, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not check in: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, a)
}

// CheckOut godoc
//
//	@Summary		Check out for today
//	@Description	Self-service check-out for the calling employee/team_lead, verified against a registered office location's GPS radius plus a selfie. Requires a check-in already recorded today.
//	@Tags			attendance
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CheckOutInput	true	"Captured location + selfie"
//	@Success		200		{object}	models.EmployeeAttendance
//	@Failure		400		{object}	map[string]string	"No check-in found for today, validation error, or no office location configured"
//	@Failure		403		{object}	map[string]string	"Outside every office location's radius"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/attendance/employee/check-out [post]
func (ctrl *EmployeeAttendanceController) CheckOut(c *gin.Context) {
	var input models.CheckOutInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	if status, msg := ctrl.checkWithinOfficeRadius(ctx, input.Latitude, input.Longitude); status != 0 {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	employeeID := c.GetString("user_id")
	a, err := ctrl.attendanceRepo.CheckOut(ctx, employeeID, input)
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
