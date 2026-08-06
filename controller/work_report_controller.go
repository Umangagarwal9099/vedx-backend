package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type WorkReportController struct {
	reportRepo  *repository.WorkReportRepository
	leadRepo    *repository.LeadRepository
	callLogRepo *repository.LeadCallLogRepository
}

func NewWorkReportController(reportRepo *repository.WorkReportRepository, leadRepo *repository.LeadRepository, callLogRepo *repository.LeadCallLogRepository) *WorkReportController {
	return &WorkReportController{reportRepo: reportRepo, leadRepo: leadRepo, callLogRepo: callLogRepo}
}

// GetSuggestions godoc
//
//	@Summary		Today's work report suggestions
//	@Description	Returns today's real activity counts (calls made, leads contacted, follow-ups done, admissions) as starting values for the daily work report — the employee can confirm or edit them before submitting.
//	@Tags			work-reports
//	@Produce		json
//	@Success		200	{object}	models.WorkReportSuggestions
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/work-reports/suggestions [get]
func (ctrl *WorkReportController) GetSuggestions(c *gin.Context) {
	employeeID := c.GetString("user_id")
	ctx := c.Request.Context()

	callsMade, leadsContacted, followUpsDone, err := ctrl.callLogRepo.GetTodaySummary(ctx, employeeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute suggestions"})
		return
	}
	admissions, err := ctrl.leadRepo.CountConvertedToday(ctx, employeeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute suggestions"})
		return
	}

	c.JSON(http.StatusOK, models.WorkReportSuggestions{
		CallsMade:      callsMade,
		LeadsContacted: leadsContacted,
		FollowUpsDone:  followUpsDone,
		Admissions:     admissions,
	})
}

// Submit godoc
//
//	@Summary		Submit today's work report
//	@Description	Submits (or updates) the calling employee's work report for today. report_date is always the server's current date — re-submitting the same day updates the existing report.
//	@Tags			work-reports
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.SubmitWorkReportInput	true	"Report details"
//	@Success		200		{object}	models.WorkReport
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/work-reports [post]
func (ctrl *WorkReportController) Submit(c *gin.Context) {
	var input models.SubmitWorkReportInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	employeeID := c.GetString("user_id")
	report, err := ctrl.reportRepo.Submit(c.Request.Context(), employeeID, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not submit work report: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

// GetMine godoc
//
//	@Summary		My work reports
//	@Description	Returns the calling employee's last 30 days of work reports, newest first.
//	@Tags			work-reports
//	@Produce		json
//	@Success		200	{array}		models.WorkReport
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/work-reports/me [get]
func (ctrl *WorkReportController) GetMine(c *gin.Context) {
	employeeID := c.GetString("user_id")
	reports, err := ctrl.reportRepo.FindAllForEmployee(c.Request.Context(), employeeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch work reports"})
		return
	}
	if reports == nil {
		reports = []models.WorkReport{}
	}
	c.JSON(http.StatusOK, reports)
}
