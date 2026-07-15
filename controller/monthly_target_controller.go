package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type MonthlyTargetController struct {
	targetRepo *repository.MonthlyTargetRepository
}

func NewMonthlyTargetController(targetRepo *repository.MonthlyTargetRepository) *MonthlyTargetController {
	return &MonthlyTargetController{targetRepo: targetRepo}
}

// currentYearMonth parses ?year=&month= query params, defaulting to the
// current calendar month if either is missing/invalid.
func currentYearMonth(c *gin.Context) (int, int) {
	now := time.Now()
	year, err := strconv.Atoi(c.Query("year"))
	if err != nil {
		year = now.Year()
	}
	month, err := strconv.Atoi(c.Query("month"))
	if err != nil {
		month = int(now.Month())
	}
	return year, month
}

// Set godoc
//
//	@Summary		Set an employee's monthly target
//	@Description	Sets (or updates) an employee's conversion target for a given year/month. Restricted to super_admin/team_lead.
//	@Tags			targets
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path		string							true	"Employee user ID"
//	@Param			body	body		models.SetMonthlyTargetInput	true	"Target details"
//	@Success		200		{object}	models.MonthlyTarget
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/targets/employee/{user_id} [patch]
func (ctrl *MonthlyTargetController) Set(c *gin.Context) {
	employeeID := c.Param("user_id")

	var input models.SetMonthlyTargetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	setBy := c.GetString("user_id")
	target, err := ctrl.targetRepo.Set(c.Request.Context(), employeeID, input, setBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not set target: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, target)
}

// GetMine godoc
//
//	@Summary		My monthly target
//	@Description	Returns the calling employee/team_lead's target + achieved conversions for a year/month (?year=&month=, defaults to current month).
//	@Tags			targets
//	@Produce		json
//	@Param			year	query	int	false	"Year, defaults to current"
//	@Param			month	query	int	false	"Month (1-12), defaults to current"
//	@Success		200		{object}	models.MonthlyTarget
//	@Failure		404		{object}	map[string]string	"No target set for this month"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/targets/employee/me [get]
func (ctrl *MonthlyTargetController) GetMine(c *gin.Context) {
	employeeID := c.GetString("user_id")
	year, month := currentYearMonth(c)

	target, err := ctrl.targetRepo.GetForEmployee(c.Request.Context(), employeeID, year, month)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch target"})
		return
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no target set for this month"})
		return
	}
	c.JSON(http.StatusOK, target)
}

// GetTeam godoc
//
//	@Summary		Team monthly targets
//	@Description	Returns every employee's target + achieved conversions for a year/month (?year=&month=, defaults to current month). Restricted to super_admin/team_lead.
//	@Tags			targets
//	@Produce		json
//	@Param			year	query	int	false	"Year, defaults to current"
//	@Param			month	query	int	false	"Month (1-12), defaults to current"
//	@Success		200		{array}		models.MonthlyTarget
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/targets/employee/team [get]
func (ctrl *MonthlyTargetController) GetTeam(c *gin.Context) {
	year, month := currentYearMonth(c)

	targets, err := ctrl.targetRepo.GetTeamSummary(c.Request.Context(), year, month)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch team targets"})
		return
	}
	if targets == nil {
		targets = []models.MonthlyTarget{}
	}
	c.JSON(http.StatusOK, targets)
}
