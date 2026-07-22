package controller

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type DashboardController struct {
	batchRepo       *repository.BatchRepository
	enrollmentRepo  *repository.EnrollmentRepository
	sessionRepo     *repository.SessionRepository
	attendanceRepo  *repository.AttendanceRepository
	certificateRepo *repository.CertificateRepository
}

func NewDashboardController(
	batchRepo *repository.BatchRepository,
	enrollmentRepo *repository.EnrollmentRepository,
	sessionRepo *repository.SessionRepository,
	attendanceRepo *repository.AttendanceRepository,
	certificateRepo *repository.CertificateRepository,
) *DashboardController {
	return &DashboardController{
		batchRepo: batchRepo, enrollmentRepo: enrollmentRepo, sessionRepo: sessionRepo,
		attendanceRepo: attendanceRepo, certificateRepo: certificateRepo,
	}
}

// GetStats godoc
//
//	@Summary		Admin dashboard stats
//	@Description	Real, computed-from-live-data summary for the admin home screen — active batches/students, today's sessions with attendance, certificates issued in the last 30 days, average attendance this week, and a 30-day enrollment trend. Restricted to super_admin / team_lead.
//	@Tags			dashboard
//	@Produce		json
//	@Success		200	{object}	models.DashboardStats
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/dashboard/stats [get]
func (ctrl *DashboardController) GetStats(c *gin.Context) {
	ctx := c.Request.Context()
	stats := models.DashboardStats{}

	// Mentors/employees only see stats for batches they manage — same scoping
	// used everywhere else in this app (batch list, submissions, leaderboard).
	mentorID := ""
	role := c.GetString("role")
	if role == string(models.RoleMentor) || role == string(models.RoleEmployee) {
		mentorID = c.GetString("user_id")
	}

	collegeID, err := repository.CollegeFilter(role, c.GetString("college_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
		return
	}

	var batches []models.Batch
	if mentorID != "" {
		batches, err = ctrl.batchRepo.FindAllForMentor(ctx, mentorID, collegeID)
	} else {
		batches, err = ctrl.batchRepo.FindAll(ctx, collegeID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch batches"})
		return
	}
	for _, b := range batches {
		if b.IsActive {
			stats.ActiveBatches++
		}
	}

	if count, err := ctrl.enrollmentRepo.CountActiveStudents(ctx, mentorID); err == nil {
		stats.ActiveStudents = count
	}

	today := time.Now().Format("2006-01-02")
	var sessionsToday []models.Session
	if mentorID != "" {
		if all, ferr := ctrl.sessionRepo.FindAllForMentor(ctx, mentorID); ferr == nil {
			for _, s := range all {
				if s.SessionDate == today {
					sessionsToday = append(sessionsToday, s)
				}
			}
		}
	} else {
		sessionsToday, _ = ctrl.sessionRepo.Filter(ctx, models.SessionFilter{Date: today})
	}
	{
		stats.SessionsToday = make([]models.DashboardSession, 0, len(sessionsToday))
		for _, s := range sessionsToday {
			ds := models.DashboardSession{
				ShortID: s.ShortID, Name: s.Name, StartTime: s.StartTime, EndTime: s.EndTime,
				BatchNumber: s.BatchNumber, MentorName: s.MentorName,
			}
			if records, err := ctrl.attendanceRepo.GetForSession(ctx, s.ID); err == nil {
				ds.TotalCount = len(records)
				for _, r := range records {
					if r.Status == "present" || r.Status == "late" {
						ds.PresentCount++
					}
				}
			}
			stats.SessionsToday = append(stats.SessionsToday, ds)
		}
	}

	if reports, err := ctrl.attendanceRepo.GetSessionReports(ctx, mentorID); err == nil {
		cutoff := time.Now().AddDate(0, 0, -7)
		var sum float64
		var n int
		for _, rep := range reports {
			d, err := time.Parse("2006-01-02", rep.SessionDate)
			if err != nil || d.Before(cutoff) {
				continue
			}
			sum += rep.AttendancePct
			n++
		}
		if n > 0 {
			stats.AvgAttendanceRate7d = sum / float64(n)
		}
	}

	if certs, err := ctrl.certificateRepo.GetAll(ctx, mentorID); err == nil {
		cutoff := time.Now().AddDate(0, 0, -30)
		for _, cert := range certs {
			if cert.IssuedAt.After(cutoff) {
				stats.CertificatesIssued30d++
			}
		}
	}

	if trend, err := ctrl.enrollmentRepo.GetEnrollmentTrend(ctx, 30, mentorID); err == nil {
		stats.EnrollmentTrend30d = trend
	}
	if stats.SessionsToday == nil {
		stats.SessionsToday = []models.DashboardSession{}
	}
	if stats.EnrollmentTrend30d == nil {
		stats.EnrollmentTrend30d = []models.EnrollmentTrendPoint{}
	}

	c.JSON(http.StatusOK, stats)
}
