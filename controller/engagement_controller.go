package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// Thresholds behind at-risk detection. Not user-configurable (yet) — kept as
// named constants so the reasoning is visible in one place.
const (
	atRiskAttendanceThresholdPct = 60.0
	atRiskScoreThresholdPct      = 40.0
	atRiskInactivityDays         = 7
)

type EngagementController struct {
	activityRepo   *repository.ActivityRepository
	batchRepo      *repository.BatchRepository
	attendanceRepo *repository.AttendanceRepository
	scoreRepo      *repository.ScoreRepository
	userRepo       *repository.UserRepository
}

func NewEngagementController(activityRepo *repository.ActivityRepository, batchRepo *repository.BatchRepository, attendanceRepo *repository.AttendanceRepository, scoreRepo *repository.ScoreRepository, userRepo *repository.UserRepository) *EngagementController {
	return &EngagementController{activityRepo: activityRepo, batchRepo: batchRepo, attendanceRepo: attendanceRepo, scoreRepo: scoreRepo, userRepo: userRepo}
}

// GetStudentStreak godoc
//
//	@Summary		Get a student's activity streak
//	@Description	"Active" means the student submitted an assignment/project, attempted an exam, or attended a session on that calendar day.
//	@Tags			engagement
//	@Produce		json
//	@Param			id	path		string	true	"Student user ID"
//	@Success		200	{object}	models.StudentStreak
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/streak [get]
func (ctrl *EngagementController) GetStudentStreak(c *gin.Context) {
	userID := c.Param("id")

	current, longest, total, lastActive, err := ctrl.activityRepo.GetStreak(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute streak"})
		return
	}

	streak := models.StudentStreak{
		StudentID:       userID,
		CurrentStreak:   current,
		LongestStreak:   longest,
		TotalActiveDays: total,
		LastActiveDate:  lastActive,
	}
	if user, err := ctrl.userRepo.FindByID(c.Request.Context(), userID); err == nil && user != nil {
		streak.StudentName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
	}

	c.JSON(http.StatusOK, streak)
}

// GetBatchAtRisk godoc
//
//	@Summary		Get a batch's at-risk students
//	@Description	Flags students with low attendance (<60%), a low final score (<40%, only once they have graded work), or no activity in 7+ days (only once the batch has been running that long). Only returns students who tripped at least one flag.
//	@Tags			engagement
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		200			{array}		models.AtRiskStudent
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/at-risk [get]
func (ctrl *EngagementController) GetBatchAtRisk(c *gin.Context) {
	shortID := c.Param("short_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, shortID) {
		return
	}

	roster, err := ctrl.batchRepo.GetStudents(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch batch roster"})
		return
	}

	attendance, err := ctrl.attendanceRepo.GetBatchSummary(c.Request.Context(), batch.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch attendance"})
		return
	}
	attendanceByStudent := map[string]models.StudentAttendanceSummary{}
	for _, a := range attendance {
		attendanceByStudent[a.StudentID] = a
	}

	leaderboard, err := ctrl.scoreRepo.GetBatchLeaderboard(
		c.Request.Context(), batch.ID, batch.ShortID, batch.BatchNumber,
		batch.ScoreWeightAssignments, batch.ScoreWeightExams, batch.ScoreWeightProjects,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute scores"})
		return
	}
	scoreByStudent := map[string]models.StudentScoreBreakdown{}
	for _, s := range leaderboard {
		scoreByStudent[s.StudentID] = s
	}

	batchStart := parseBatchDate(batch.StartDate)
	batchOldEnough := batchStart != nil && time.Since(*batchStart) > atRiskInactivityDays*24*time.Hour

	atRisk := []models.AtRiskStudent{}
	for _, student := range roster {
		attendancePct := 0.0
		hasAttendanceData := false
		if a, ok := attendanceByStudent[student.UserID]; ok {
			attendancePct = a.AttendancePct
			hasAttendanceData = a.TotalSessions > 0
		}

		finalScore := 0.0
		hasScoreData := false
		if s, ok := scoreByStudent[student.UserID]; ok {
			finalScore = s.FinalScore
			hasScoreData = s.Assignments.Max > 0 || s.Exams.Max > 0 || s.Projects.Max > 0
		}

		current, _, _, lastActive, err := ctrl.activityRepo.GetStreak(c.Request.Context(), student.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute activity"})
			return
		}
		var daysInactive *int
		if lastActive != nil {
			if lastDate, err := time.Parse("2006-01-02", *lastActive); err == nil {
				d := int(time.Since(lastDate).Hours() / 24)
				daysInactive = &d
			}
		}

		var reasons []string
		if hasAttendanceData && attendancePct < atRiskAttendanceThresholdPct {
			reasons = append(reasons, fmt.Sprintf("Attendance below %.0f%% (currently %.0f%%)", atRiskAttendanceThresholdPct, attendancePct))
		}
		if hasScoreData && finalScore < atRiskScoreThresholdPct {
			reasons = append(reasons, fmt.Sprintf("Score below %.0f%% (currently %.0f%%)", atRiskScoreThresholdPct, finalScore))
		}
		if batchOldEnough && (daysInactive == nil || *daysInactive > atRiskInactivityDays) {
			reasons = append(reasons, fmt.Sprintf("No activity in the last %d+ days", atRiskInactivityDays))
		}

		if len(reasons) > 0 {
			atRisk = append(atRisk, models.AtRiskStudent{
				StudentID:     student.UserID,
				StudentName:   fmt.Sprintf("%s %s", student.FirstName, student.LastName),
				AttendancePct: attendancePct,
				FinalScore:    finalScore,
				CurrentStreak: current,
				DaysInactive:  daysInactive,
				Reasons:       reasons,
			})
		}
	}

	c.JSON(http.StatusOK, atRisk)
}
