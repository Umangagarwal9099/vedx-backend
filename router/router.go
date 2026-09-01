package router

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/umangagarwal/vedx-backend/config"
	"github.com/umangagarwal/vedx-backend/controller"
	_ "github.com/umangagarwal/vedx-backend/docs"
	"github.com/umangagarwal/vedx-backend/middleware"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

func New(pool *pgxpool.Pool, cfg *config.Config) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.CORS(cfg.App.AllowedOrigins))

	// Swagger UI — http://localhost:8080/swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Repositories
	userRepo := repository.NewUserRepository(pool)
	courseRepo := repository.NewCourseRepository(pool)
	batchRepo := repository.NewBatchRepository(pool)
	batchRecordingRepo := repository.NewBatchRecordingRepository(pool)
	enrollmentRepo := repository.NewEnrollmentRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	announcementRepo := repository.NewAnnouncementRepository(pool)
	blogRepo := repository.NewBlogRepository(pool)
	bannerRepo := repository.NewBannerRepository(pool)
	codingQuestionRepo := repository.NewCodingQuestionRepository(pool)
	codingQuestionDraftRepo := repository.NewCodingQuestionDraftRepository(pool)
	submissionRepo := repository.NewSubmissionRepository(pool)
	feedbackFormRepo := repository.NewFeedbackFormRepository(pool)
	moduleRepo := repository.NewModuleRepository(pool)
	assessmentRepo := repository.NewAssessmentRepository(pool)
	communityRepo := repository.NewCommunityRepository(pool)
	communityPostRepo := repository.NewCommunityPostRepository(pool)
	collegeRepo := repository.NewCollegeRepository(pool)
	notificationRepo := repository.NewNotificationRepository(pool)
	devicePushTokenRepo := repository.NewDevicePushTokenRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	assignmentRepo := repository.NewAssignmentRepository(pool)
	resourceRepo := repository.NewResourceRepository(pool)
	projectRepo := repository.NewProjectRepository(pool)
	questionBankRepo := repository.NewQuestionBankRepository(pool)
	questionBankSubjectRepo := repository.NewQuestionBankSubjectRepository(pool)
	examAttemptRepo := repository.NewExamAttemptRepository(pool)
	attendanceRepo := repository.NewAttendanceRepository(pool)
	scoreRepo := repository.NewScoreRepository(pool)
	moduleScheduleRepo := repository.NewModuleScheduleRepository(pool)
	activityRepo := repository.NewActivityRepository(pool)
	certificateRepo := repository.NewCertificateRepository(pool)
	auditLogRepo := repository.NewAuditLogRepository(pool)
	helpSupportRepo := repository.NewHelpSupportRepository(pool)
	profileRepo := repository.NewProfileRepository(pool)
	passwordResetRepo := repository.NewPasswordResetRepository(pool)
	loginOtpRepo := repository.NewLoginOTPRepository(pool)
	studentRegistrationRepo := repository.NewStudentRegistrationRepository(pool)
	studentNoteRepo := repository.NewStudentNoteRepository(pool)
	loginActivityRepo := repository.NewLoginActivityRepository(pool)
	analyticsRepo := repository.NewAnalyticsRepository(pool)
	leadRepo := repository.NewLeadRepository(pool)
	leadAssignmentHistoryRepo := repository.NewLeadAssignmentHistoryRepository(pool)
	employeeAttendanceRepo := repository.NewEmployeeAttendanceRepository(pool)
	leaveRequestRepo := repository.NewLeaveRequestRepository(pool)
	leaveBalanceRepo := repository.NewLeaveBalanceRepository(pool)
	monthlyTargetRepo := repository.NewMonthlyTargetRepository(pool)
	leadCallLogRepo := repository.NewLeadCallLogRepository(pool)
	workReportRepo := repository.NewWorkReportRepository(pool)
	officeLocationRepo := repository.NewOfficeLocationRepository(pool)

	// Services
	storageSvc := service.NewStorageService(cfg.Storage)
	zoomSvc := service.NewZoomService(cfg.Zoom)
	emailSvc := service.NewEmailService(cfg.Resend)

	// Controllers
	authCtrl := controller.NewAuthController(userRepo, passwordResetRepo, loginOtpRepo, loginActivityRepo, collegeRepo, emailSvc, cfg.JWT.Secret)
	collegeEmployeeRepo := repository.NewCollegeEmployeeRepository(pool)
	userCtrl := controller.NewUserController(userRepo, collegeRepo, collegeEmployeeRepo, enrollmentRepo, emailSvc, cfg.App.PublicURL, auditLogRepo)
	employeeNoteRepo := repository.NewEmployeeNoteRepository(pool)
	employeeNoteCtrl := controller.NewEmployeeNoteController(employeeNoteRepo, userRepo)
	studentRegistrationCtrl := controller.NewStudentRegistrationController(studentRegistrationRepo, studentNoteRepo, auditLogRepo, userRepo, collegeRepo, emailSvc, cfg.App.PublicURL)
	loginActivityCtrl := controller.NewLoginActivityController(loginActivityRepo)
	courseCtrl := controller.NewCourseController(courseRepo, collegeRepo, auditLogRepo)
	batchCtrl := controller.NewBatchController(batchRepo, enrollmentRepo, notificationRepo, userRepo, collegeRepo, courseRepo, emailSvc, auditLogRepo, communityRepo)
	eventCtrl := controller.NewEventController(eventRepo, notificationRepo)
	announcementCtrl := controller.NewAnnouncementController(announcementRepo, notificationRepo)
	uploadCtrl := controller.NewUploadController(storageSvc)
	codingQuestionCtrl := controller.NewCodingQuestionController(codingQuestionRepo, notificationRepo)
	codingQuestionDraftCtrl := controller.NewCodingQuestionDraftController(codingQuestionDraftRepo, codingQuestionRepo)
	submissionCtrl := controller.NewSubmissionController(submissionRepo)
	feedbackFormCtrl := controller.NewFeedbackFormController(feedbackFormRepo, notificationRepo)
	moduleCtrl := controller.NewModuleController(moduleRepo)
	blogCtrl := controller.NewBlogController(blogRepo, notificationRepo)
	bannerCtrl := controller.NewBannerController(bannerRepo, notificationRepo)
	assessmentCtrl := controller.NewAssessmentController(assessmentRepo, questionBankRepo, batchRepo, notificationRepo, auditLogRepo)
	communityCtrl := controller.NewCommunityController(communityRepo, notificationRepo, userRepo, batchRepo, emailSvc)
	collegeCtrl := controller.NewCollegeController(collegeRepo, auditLogRepo)
	communityPostCtrl := controller.NewCommunityPostController(communityPostRepo, communityRepo)
	notificationCtrl := controller.NewNotificationController(notificationRepo)
	devicePushTokenCtrl := controller.NewDevicePushTokenController(devicePushTokenRepo)
	sessionCtrl := controller.NewSessionController(sessionRepo, batchRepo, notificationRepo, userRepo, zoomSvc, emailSvc, storageSvc, cfg.App.PublicURL, cfg.App.Timezone, auditLogRepo, feedbackFormRepo, batchRecordingRepo)
	zoomWebhookCtrl := controller.NewZoomWebhookController(zoomSvc, storageSvc, sessionRepo, cfg.App.Timezone)
	assignmentCtrl := controller.NewAssignmentController(assignmentRepo, batchRepo, notificationRepo, auditLogRepo)
	resourceCtrl := controller.NewResourceController(resourceRepo, batchRepo, auditLogRepo)
	projectCtrl := controller.NewProjectController(projectRepo, batchRepo, notificationRepo, auditLogRepo)
	questionBankCtrl := controller.NewQuestionBankController(questionBankRepo, questionBankSubjectRepo, auditLogRepo)
	examAttemptCtrl := controller.NewExamAttemptController(examAttemptRepo, assessmentRepo, questionBankRepo, batchRepo, notificationRepo, auditLogRepo)
	attendanceCtrl := controller.NewAttendanceController(attendanceRepo, sessionRepo, batchRepo, auditLogRepo)
	scoreCtrl := controller.NewScoreController(scoreRepo, batchRepo, auditLogRepo)
	curriculumCtrl := controller.NewCurriculumController(courseRepo, batchRepo, moduleRepo, moduleScheduleRepo, auditLogRepo)
	certificateCtrl := controller.NewCertificateController(certificateRepo, batchRepo, auditLogRepo)
	engagementCtrl := controller.NewEngagementController(activityRepo, batchRepo, attendanceRepo, scoreRepo, userRepo)
	auditLogCtrl := controller.NewAuditLogController(auditLogRepo, batchRepo)
	helpSupportCtrl := controller.NewHelpSupportController(helpSupportRepo)
	profileCtrl := controller.NewProfileController(profileRepo, userRepo)
	dashboardCtrl := controller.NewDashboardController(batchRepo, enrollmentRepo, sessionRepo, attendanceRepo, certificateRepo)
	analyticsCtrl := controller.NewAnalyticsController(analyticsRepo, batchRepo)
	leadCtrl := controller.NewLeadController(leadRepo, leadCallLogRepo, leadAssignmentHistoryRepo, notificationRepo, auditLogRepo, collegeRepo)
	employeeAttendanceCtrl := controller.NewEmployeeAttendanceController(employeeAttendanceRepo, officeLocationRepo)
	leaveCtrl := controller.NewLeaveController(leaveRequestRepo, leaveBalanceRepo, userRepo, emailSvc)
	monthlyTargetCtrl := controller.NewMonthlyTargetController(monthlyTargetRepo)
	workReportCtrl := controller.NewWorkReportController(workReportRepo, leadRepo, leadCallLogRepo)
	officeLocationCtrl := controller.NewOfficeLocationController(officeLocationRepo)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", controller.Health)

		// ── Public auth routes ────────────────────────────────────────────────
		auth := v1.Group("/auth")
		{
			auth.POST("/login", authCtrl.Login)
			auth.POST("/login/verify-otp", authCtrl.VerifyLoginOTP)
			auth.POST("/register", authCtrl.Register)
			auth.POST("/forgot-password", authCtrl.ForgotPassword)
			auth.POST("/reset-password", authCtrl.ResetPassword)
			auth.POST("/logout", authCtrl.Logout)
		}

		// Recording streaming — hit directly by <video src>, which can't send a
		// custom Authorization header, so these authenticate via the httpOnly
		// session cookie set at login instead (falling back to a Bearer token
		// for non-browser callers). Kept out of the `protected` group below
		// because it uses a different auth middleware.
		stream := v1.Group("/stream")
		stream.Use(middleware.JWTAuthCookieOrHeader(cfg.JWT.Secret))
		{
			stream.GET("/sessions/:short_id", sessionCtrl.StreamSessionRecording)
			stream.GET("/batch-recordings/:short_id", sessionCtrl.StreamBatchRecording)
		}

		// Public session join-link resolution — the token itself is the credential
		// (like a magic link), so this stays outside the JWT-protected group.
		v1.GET("/sessions/by-token/:token", sessionCtrl.JoinByToken)
		// Same public join gateway, keyed by short_id — this is what the
		// student app's /join-session/[shortId] page actually calls, and
		// what session emails link to (see withShareLink).
		v1.GET("/sessions/:short_id/join", sessionCtrl.JoinByShortID)
		v1.GET("/certificates/verify/:certificate_number", certificateCtrl.VerifyCertificate)

		// Zoom calls this directly (no JWT) — authenticity is instead verified via
		// the x-zm-signature header against the Event Subscriptions secret token.
		v1.POST("/zoom/webhook", zoomWebhookCtrl.HandleWebhook)

		// ── Protected routes — require a valid JWT ────────────────────────────
		protected := v1.Group("/")
		protected.Use(middleware.JWTAuth(cfg.JWT.Secret))
		{
			// Role sets used across multiple route groups
			adminOrAbove := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead)
			staffOrAbove := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor)
			// Lead-CRM routes: team_lead carries a personal lead quota alongside employees.
			leadOrAbove := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleEmployee)
			// College-scoped roles (college_admin/college_staff) are deliberately
			// NEVER added to adminOrAbove/staffOrAbove above — those groups carry
			// platform-wide power. This dedicated group is only for the specific
			// student-list/creation routes where their own data is already
			// isolated at the repository layer via CollegeFilter/
			// resolveTargetCollege, so widening just these routes is safe.
			studentProvisionOrAbove := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleCollegeAdmin, models.RoleCollegeStaff)
			studentListOrAbove := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor, models.RoleCollegeAdmin, models.RoleCollegeStaff)
			// Same rationale as above — read-only dashboard/analytics views are
			// already scoped by CollegeFilter at the repository layer, so a
			// College Admin/Staff seeing only their own college's numbers is safe.
			collegeReadOrAbove := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor, models.RoleCollegeAdmin, models.RoleCollegeStaff)
			// Content-authoring routes for assignments/assessments/projects —
			// create/update/delete/grade/publish. Safe to widen to
			// college_admin/college_staff specifically because every one of
			// these handlers calls checkBatchAccess, which now enforces that a
			// college-scoped caller can only touch batches belonging to their
			// own college (mirroring the mentor/employee ownership check it
			// already did). Deliberately not folded into staffOrAbove — other
			// staffOrAbove routes (resources, coding questions, etc.) haven't
			// been audited for this and would leak cross-tenant power.
			collegeContentOrAbove := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor, models.RoleCollegeAdmin, models.RoleCollegeStaff)

			// Colleges — multi-tenancy configuration. Managing colleges/feature
			// toggles is super_admin only; any authenticated user can read their
			// OWN resolved features (used to drive every frontend's sidebar).
			protected.GET("/colleges/me/features", collegeCtrl.GetMyFeatures)
			colleges := protected.Group("/colleges", middleware.RequireRole(models.RoleSuperAdmin))
			{
				colleges.POST("", collegeCtrl.Create)
				colleges.GET("", collegeCtrl.GetAll)
				colleges.GET("/:short_id", collegeCtrl.GetByShortID)
				colleges.PATCH("/:short_id", collegeCtrl.Update)
				colleges.PATCH("/:short_id/features", collegeCtrl.UpdateFeatures)
				colleges.DELETE("/:short_id", collegeCtrl.Delete)
				colleges.POST("/:short_id/subscriptions", collegeCtrl.CreateSubscription)
			}

			// Change password — logged-in user, requires the current password.
			protected.POST("/auth/change-password", authCtrl.ChangePassword)

			// Users — static paths registered before /:id so Gin matches them first.
			// GetAll/GetDeleted/Search return full PII (email, phone, DOB) across every
			// user, so they're staff-only, not just "any authenticated user."
			protected.GET("/users", studentListOrAbove, userCtrl.GetAll)
			protected.GET("/users/deleted", staffOrAbove, userCtrl.GetDeleted)
			protected.GET("/users/search", studentListOrAbove, userCtrl.Search)
			// Staff (mentor/employee/team_lead) creation stays internal-only.
			// Student creation additionally admits College Admin/College Staff —
			// their own college_id is force-resolved server-side (see
			// resolveTargetCollege), never trusting a college_short_id they send.
			protected.POST("/users/staff", adminOrAbove, userCtrl.CreateStaffUser)
			protected.POST("/users/student", studentProvisionOrAbove, userCtrl.CreateStudent)
			protected.PATCH("/users/:id", userCtrl.Update)
			protected.PATCH("/users/:id/role", middleware.RequireRole(models.RoleSuperAdmin), userCtrl.ChangeRole)
			protected.PATCH("/users/:id/college", middleware.RequireRole(models.RoleSuperAdmin), userCtrl.TransferCollege)
			protected.PATCH("/users/:id/department", adminOrAbove, userCtrl.UpdateDepartment)
			protected.GET("/users/:id/colleges", adminOrAbove, userCtrl.ListColleges)
			protected.POST("/users/:id/colleges", middleware.RequireRole(models.RoleSuperAdmin), userCtrl.AddCollege)
			protected.DELETE("/users/:id/colleges/:college_short_id", middleware.RequireRole(models.RoleSuperAdmin), userCtrl.RemoveCollege)
			// Manager-department employee notes — a running record on an
			// employee's profile. managerOrAbove mirrors hrOrAbove's pattern.
			managerOrAbove := middleware.RequireRoleOrDepartment("manager", models.RoleSuperAdmin, models.RoleTeamLead)
			protected.POST("/users/:id/employee-notes", managerOrAbove, employeeNoteCtrl.AddNote)
			protected.GET("/users/:id/employee-notes", managerOrAbove, employeeNoteCtrl.GetNotes)
			protected.PATCH("/users/:id/email", adminOrAbove, userCtrl.ChangeEmail)
			// Deleting an account is destructive and irreversible from the API's
			// perspective (soft-delete, but still removes access) — super_admin only.
			protected.DELETE("/users/:id", middleware.RequireRole(models.RoleSuperAdmin), userCtrl.Delete)

			// Mentors list — for batch manager dropdown
			protected.GET("/mentors", userCtrl.GetMentors)

			protected.GET("/dashboard/stats", collegeReadOrAbove, dashboardCtrl.GetStats)

			// Batch & Progress analytics — mentors/employees see only their own batches
			analytics := protected.Group("/analytics")
			{
				analytics.GET("/batches", collegeReadOrAbove, analyticsCtrl.GetBatchAnalytics)
				analytics.GET("/batches/:short_id/attendance-trend", collegeReadOrAbove, analyticsCtrl.GetBatchAttendanceTrend)
			}

			// Lead CRM — admin imports/assigns/monitors; employees (and team_lead's
			// own quota) work only their assigned leads. Static paths registered
			// before /:short_id so Gin matches them first.
			leads := protected.Group("/leads", middleware.RequireActiveSubscription(collegeRepo), middleware.RequireFeature(collegeRepo, models.FeatureCRM))
			{
				leads.POST("", adminOrAbove, leadCtrl.Create)
				leads.POST("/import", adminOrAbove, leadCtrl.BulkImport)
				leads.POST("/assign", adminOrAbove, leadCtrl.AssignBulk)
				leads.POST("/reassign", adminOrAbove, leadCtrl.Reassign)
				leads.POST("/unassign", adminOrAbove, leadCtrl.Unassign)
				leads.POST("/auto-assign", adminOrAbove, leadCtrl.AutoAssign)
				leads.GET("", leadOrAbove, leadCtrl.GetAll)
				leads.GET("/dashboard/me", leadOrAbove, leadCtrl.GetMyDashboard)
				leads.GET("/dashboard/team", adminOrAbove, leadCtrl.GetTeamDashboard)
				leads.GET("/:short_id", leadOrAbove, leadCtrl.GetByShortID)
				leads.PATCH("/:short_id", leadOrAbove, leadCtrl.Update)
				leads.DELETE("/:short_id", adminOrAbove, leadCtrl.Delete)
				leads.POST("/:short_id/calls", leadOrAbove, leadCtrl.AddCallLog)
				leads.GET("/:short_id/calls", leadOrAbove, leadCtrl.GetCallLogs)
				leads.GET("/:short_id/history", adminOrAbove, leadCtrl.GetAssignmentHistory)
			}

			// Employee attendance — self check-in/out only, admin views the team roster.
			employeeAttendance := protected.Group("/attendance/employee")
			{
				employeeAttendance.POST("/check-in", leadOrAbove, employeeAttendanceCtrl.CheckIn)
				employeeAttendance.POST("/check-out", leadOrAbove, employeeAttendanceCtrl.CheckOut)
				employeeAttendance.GET("/me", leadOrAbove, employeeAttendanceCtrl.GetMine)
				employeeAttendance.GET("/team", adminOrAbove, employeeAttendanceCtrl.GetTeam)
			}

			// Office locations — registered physical offices that employee
			// check-in/out is geofenced against.
			officeLocations := protected.Group("/office-locations")
			{
				officeLocations.POST("", adminOrAbove, officeLocationCtrl.Create)
				officeLocations.GET("", adminOrAbove, officeLocationCtrl.GetAll)
				officeLocations.PATCH("/:short_id", adminOrAbove, officeLocationCtrl.Update)
				officeLocations.DELETE("/:short_id", adminOrAbove, officeLocationCtrl.Delete)
				officeLocations.POST("/resolve-link", adminOrAbove, officeLocationCtrl.ResolveLink)
			}

			// Employee leave requests — apply/view own, admin OR the HR
			// department approves/rejects (RequireRoleOrDepartment lets an
			// employee whose department is "hr" through without granting
			// them every other adminOrAbove route).
			hrOrAbove := middleware.RequireRoleOrDepartment("hr", models.RoleSuperAdmin, models.RoleTeamLead)
			leaves := protected.Group("/leaves")
			{
				leaves.POST("", leadOrAbove, leaveCtrl.Apply)
				leaves.GET("", hrOrAbove, leaveCtrl.GetAll)
				leaves.GET("/me", leadOrAbove, leaveCtrl.GetMine)
				leaves.GET("/me/balance", leadOrAbove, leaveCtrl.GetMyBalance)
				leaves.GET("/pending", hrOrAbove, leaveCtrl.GetAllPending)
				leaves.PATCH("/:short_id/review", hrOrAbove, leaveCtrl.Review)
			}

			// Employee monthly conversion targets.
			targets := protected.Group("/targets/employee")
			{
				targets.PATCH("/:user_id", adminOrAbove, monthlyTargetCtrl.Set)
				targets.GET("/me", leadOrAbove, monthlyTargetCtrl.GetMine)
				targets.GET("/team", adminOrAbove, monthlyTargetCtrl.GetTeam)
			}

			// Employee daily work reports.
			workReports := protected.Group("/work-reports")
			{
				workReports.GET("/suggestions", leadOrAbove, workReportCtrl.GetSuggestions)
				workReports.POST("", leadOrAbove, workReportCtrl.Submit)
				workReports.GET("/me", leadOrAbove, workReportCtrl.GetMine)
				workReports.GET("/team", adminOrAbove, workReportCtrl.GetTeam)
			}

			// Courses — super_admin/team_lead manage any course; college_admin/
			// college_staff may create their own college's courses and edit/
			// delete only the ones they own (checkCourseAccess) — mentor is
			// deliberately excluded here (courses aren't batch-scoped the way
			// assignments/projects are, so there's no per-mentor ownership).
			courses := protected.Group("/courses")
			{
				courses.POST("", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleCollegeAdmin, models.RoleCollegeStaff), courseCtrl.Create)
				courses.GET("", courseCtrl.GetAll)
				courses.GET("/search", courseCtrl.Search)
				courses.GET("/:short_id", courseCtrl.GetByShortID)
				courses.PATCH("/:short_id", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleCollegeAdmin, models.RoleCollegeStaff), courseCtrl.Update)
				courses.DELETE("/:short_id", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleCollegeAdmin, models.RoleCollegeStaff), courseCtrl.Delete)
				// Assigning a global course to a college is a cross-tenant
				// operation — super_admin only, never team_lead.
				courses.POST("/:short_id/assign", middleware.RequireRole(models.RoleSuperAdmin), courseCtrl.AssignToCollege)
				// Curriculum — modules assigned to this course
				courses.GET("/:short_id/curriculum", courseCtrl.GetCurriculum)
				courses.POST("/:short_id/modules", adminOrAbove, courseCtrl.AssignModule)
				courses.DELETE("/:short_id/modules/:module_short_id", adminOrAbove, courseCtrl.UnassignModule)
			}

			// Batches — only super_admin / team_lead may create, edit, or delete
			batches := protected.Group("/batches")
			{
				batches.POST("", adminOrAbove, batchCtrl.Create)
				batches.GET("", batchCtrl.GetAll)
				batches.GET("/filter", batchCtrl.Filter)
				batches.GET("/mine", batchCtrl.GetMine)
				batches.GET("/:short_id", batchCtrl.GetByShortID)
				batches.PATCH("/:short_id", adminOrAbove, batchCtrl.Update)
				batches.DELETE("/:short_id", adminOrAbove, batchCtrl.Delete)

				// Students — enrolled members of a batch. Full roster is staff-only —
				// it includes every student's name/email/fee status; students use
				// GET /:short_id/me/fees for their own status instead.
				batches.GET("/:short_id/students", staffOrAbove, batchCtrl.GetStudents)
				batches.POST("/:short_id/students", adminOrAbove, batchCtrl.AddStudents)
				batches.DELETE("/:short_id/students/:user_id", adminOrAbove, batchCtrl.RemoveStudent)
				batches.PATCH("/:short_id/students/:user_id/fees", staffOrAbove, batchCtrl.SetStudentFeesPaid)
				batches.POST("/:short_id/students/:user_id/transfer", adminOrAbove, batchCtrl.TransferStudent)
				batches.PATCH("/:short_id/students/:user_id/enrollment", staffOrAbove, batchCtrl.UpdateEnrollmentStatus)
				batches.GET("/:short_id/me/fees", batchCtrl.GetMyFeesStatus)

				// Sessions — scoped to this batch
				batches.GET("/:short_id/sessions", sessionCtrl.GetByBatch)
				batches.GET("/:short_id/recordings", sessionCtrl.GetBatchRecordings)
				batches.PATCH("/:short_id/recordings/:recording_short_id/order", staffOrAbove, sessionCtrl.UpdateBatchRecordingOrder)
				batches.POST("/:short_id/recordings/presign", staffOrAbove, sessionCtrl.PresignBatchRecordingUpload)
				batches.POST("/:short_id/recordings/complete", staffOrAbove, sessionCtrl.CompleteBatchRecordingUpload)

				// Attendance rollup — every enrolled student's present/absent/late/
				// excused counts and percentage across the batch's held sessions.
				batches.GET("/:short_id/attendance-summary", staffOrAbove, attendanceCtrl.GetBatchAttendanceSummary)

				// Scoring — weight config (assignments/exams/projects) and the
				// computed leaderboard. adminOrAbove owns the weights; staff (incl.
				// mentor) or an enrolled student can read the leaderboard — scoping
				// is done inside the handler since it differs by role.
				batches.PATCH("/:short_id/score-weights", adminOrAbove, scoreCtrl.UpdateScoreWeights)
				batches.GET("/:short_id/leaderboard", scoreCtrl.GetBatchLeaderboard)

				// Curriculum — batch-scoped view of the course's modules,
				// annotated with release status, plus the schedule that drives it.
				batches.GET("/:short_id/curriculum", curriculumCtrl.GetBatchCurriculum)
				batches.GET("/:short_id/modules/schedule", staffOrAbove, curriculumCtrl.GetModuleSchedule)
				batches.PATCH("/:short_id/modules/:module_short_id/schedule", staffOrAbove, curriculumCtrl.SetModuleSchedule)
				batches.DELETE("/:short_id/modules/:module_short_id/schedule", staffOrAbove, curriculumCtrl.DeleteModuleSchedule)

				// Certificates + at-risk detection.
				batches.POST("/:short_id/students/:user_id/certificate", staffOrAbove, certificateCtrl.IssueCertificate)
				batches.GET("/:short_id/certificates", staffOrAbove, certificateCtrl.GetBatchCertificates)
				batches.GET("/:short_id/at-risk", staffOrAbove, engagementCtrl.GetBatchAtRisk)

				// Audit trail for this batch — mentor scoped to batches they manage.
				batches.GET("/:short_id/audit-log", staffOrAbove, auditLogCtrl.GetForBatch)
			}

			// A student's real batch enrollment, for their admin profile page.
			protected.GET("/users/:id/batches", staffOrAbove, batchCtrl.GetByStudentID)
			// These five are also how a student reads their OWN data (e.g. "My
			// Certificates"), so self-access is allowed alongside staff.
			protected.GET("/users/:id/enrollments", middleware.RequireSelfOrRole("id", models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor), batchCtrl.GetStudentEnrollments)
			protected.GET("/users/:id/attendance-summary", middleware.RequireSelfOrRole("id", models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor), attendanceCtrl.GetStudentAttendanceSummary)
			protected.GET("/users/:id/attendance-history", middleware.RequireSelfOrRole("id", models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor), attendanceCtrl.GetStudentAttendanceHistory)
			protected.GET("/users/:id/score-breakdown", middleware.RequireSelfOrRole("id", models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor), scoreCtrl.GetStudentScoreBreakdown)
			protected.GET("/users/:id/certificates", middleware.RequireSelfOrRole("id", models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor), certificateCtrl.GetStudentCertificates)
			protected.GET("/users/:id/streak", middleware.RequireSelfOrRole("id", models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor), engagementCtrl.GetStudentStreak)
			protected.GET("/users/:id/profile-details", middleware.RequireSelfOrRole("id", models.RoleSuperAdmin, models.RoleTeamLead), profileCtrl.GetDetails)
			protected.PATCH("/users/:id/profile-details", middleware.RequireSelfOrRole("id", models.RoleSuperAdmin, models.RoleTeamLead), profileCtrl.UpdateDetails)

			// Rich learner-detail page support — registration/demographic profile,
			// status lifecycle, learner notes, admin password reset, login activity.
			protected.GET("/users/:id/registration-details", middleware.RequireSelfOrRole("id", models.RoleSuperAdmin, models.RoleTeamLead), studentRegistrationCtrl.GetDetails)
			protected.PATCH("/users/:id/registration-details", middleware.RequireSelfOrRole("id", models.RoleSuperAdmin, models.RoleTeamLead), studentRegistrationCtrl.UpdateDetails)
			protected.POST("/users/:id/status", adminOrAbove, studentRegistrationCtrl.UpdateStatus)
			protected.GET("/users/:id/status-history", adminOrAbove, studentRegistrationCtrl.GetStatusHistory)
			protected.GET("/students/statuses", adminOrAbove, studentRegistrationCtrl.GetAllStatuses)
			protected.POST("/users/students/import", studentProvisionOrAbove, studentRegistrationCtrl.BulkImportStudents)
			// Notes and login/device activity are administrative/counsellor-facing
			// information — admin only, never mentor (see Mentor role lockdown).
			protected.POST("/users/:id/notes", adminOrAbove, studentRegistrationCtrl.AddNote)
			protected.GET("/users/:id/notes", adminOrAbove, studentRegistrationCtrl.GetNotes)
			// college_admin/college_staff may reset passwords too, but only for
			// their own college's users — enforced inside ResetPassword itself.
			protected.POST("/users/:id/reset-password", middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleCollegeAdmin, models.RoleCollegeStaff), userCtrl.ResetPassword)
			protected.GET("/users/:id/login-activity", adminOrAbove, loginActivityCtrl.GetForUser)
			protected.DELETE("/users/:id/login-activity/:deviceId", adminOrAbove, loginActivityCtrl.RemoveDevice)

			// Global audit log browse — admin only.
			protected.GET("/audit-logs", adminOrAbove, auditLogCtrl.GetAll)

			// Cross-batch views — staffOrAbove, role-scoped inside the handler
			// (mentors see only their own batches' data).
			protected.GET("/certificates", staffOrAbove, certificateCtrl.GetAll)
			protected.GET("/attendance/reports", staffOrAbove, attendanceCtrl.GetSessionReports)

			// Certificate revocation — admin-only, lives outside the batches group
			// since it's addressed by the certificate's own short_id.
			protected.DELETE("/certificates/:short_id", adminOrAbove, certificateCtrl.RevokeCertificate)

			// Events — super_admin / team_lead / mentor may create, edit, or delete
			events := protected.Group("/events")
			{
				events.POST("", staffOrAbove, eventCtrl.Create)
				events.GET("", eventCtrl.GetAll)
				events.PATCH("/:short_id", staffOrAbove, eventCtrl.Update)
				events.DELETE("/:short_id", staffOrAbove, eventCtrl.Delete)
			}

			// Announcements — super_admin / team_lead / mentor may create, edit, or delete
			announcements := protected.Group("/announcements")
			{
				announcements.POST("", staffOrAbove, announcementCtrl.Create)
				announcements.GET("", announcementCtrl.GetAll)
				announcements.PATCH("/:short_id", staffOrAbove, announcementCtrl.Update)
				announcements.DELETE("/:short_id", staffOrAbove, announcementCtrl.Delete)
			}

			// Coding Questions — super_admin / team_lead / mentor may create, edit, or delete
			cq := protected.Group("/coding-questions")
			{
				cq.POST("", staffOrAbove, codingQuestionCtrl.Create)
				cq.GET("", middleware.RequireActiveSubscription(collegeRepo), middleware.RequireFeature(collegeRepo, models.FeatureCoding), codingQuestionCtrl.GetAll)
				cq.GET("/admin", collegeReadOrAbove, codingQuestionCtrl.GetAllAdmin)
				cq.GET("/:short_id", codingQuestionCtrl.GetByShortID)
				cq.PATCH("/:short_id", staffOrAbove, codingQuestionCtrl.Update)
				cq.DELETE("/:short_id", staffOrAbove, codingQuestionCtrl.Delete)

				// Autosave — any authenticated caller may save/read their
				// own draft, scoped by their own user_id from the JWT, never
				// user-supplied, so there's no cross-user access to guard.
				cq.PUT("/:short_id/draft", codingQuestionDraftCtrl.SaveDraft)
				cq.GET("/:short_id/draft", codingQuestionDraftCtrl.GetDrafts)
			}

			// Submissions
			subs := protected.Group("/submissions")
			{
				subs.POST("", submissionCtrl.Create)
				subs.GET("/me", submissionCtrl.GetMySubmissions)
				subs.GET("/question/:short_id", submissionCtrl.GetByQuestion)
				// Admin/mentor — see all students' submissions
				subs.GET("/admin", staffOrAbove, submissionCtrl.GetAllAdmin)
				subs.GET("/user/:user_id", staffOrAbove, submissionCtrl.GetByUserAdmin)

				// Cross-entity feeds behind the unified Submissions workspace —
				// role-scoped inside each handler (mentors see only their
				// batches; college_admin/college_staff see only their college's).
				subs.GET("/assignments", collegeContentOrAbove, assignmentCtrl.GetAllSubmissionsGlobal)
				subs.GET("/projects", collegeContentOrAbove, projectCtrl.GetAllSubmissionsGlobal)
				subs.GET("/assessments", collegeContentOrAbove, examAttemptCtrl.GetAllAttemptsGlobal)

				// Coding-practice leaderboard — student-facing, wires up the
				// previously-inert "coding_leaderboard" feature flag. Scoped to
				// the caller's own college inside GetLeaderboard.
				subs.GET("/coding-leaderboard", middleware.RequireActiveSubscription(collegeRepo), middleware.RequireFeature(collegeRepo, models.FeatureCodingLeaderboard), submissionCtrl.GetLeaderboard)
			}

			// Modules — only super_admin / team_lead may create, edit, or delete
			modules := protected.Group("/modules")
			{
				modules.POST("", adminOrAbove, moduleCtrl.Create)
				modules.GET("", moduleCtrl.GetAll)
				modules.GET("/filter", moduleCtrl.Filter)
				modules.PATCH("/:short_id", adminOrAbove, moduleCtrl.Update)
				modules.DELETE("/:short_id", adminOrAbove, moduleCtrl.Delete)

				// Sections — nested under a module
				modules.POST("/:short_id/sections", adminOrAbove, moduleCtrl.AddSection)
				modules.GET("/:short_id/sections", moduleCtrl.GetSections)
				modules.PATCH("/:short_id/sections/:section_short_id", adminOrAbove, moduleCtrl.UpdateSection)
				modules.DELETE("/:short_id/sections/:section_short_id", adminOrAbove, moduleCtrl.DeleteSection)

				// Materials — nested under a section
				modules.POST("/:short_id/sections/:section_short_id/materials", adminOrAbove, moduleCtrl.AddMaterial)
				modules.GET("/:short_id/sections/:section_short_id/materials", moduleCtrl.GetMaterials)
				modules.PATCH("/:short_id/sections/:section_short_id/materials/:material_short_id", adminOrAbove, moduleCtrl.UpdateMaterial)
				modules.DELETE("/:short_id/sections/:section_short_id/materials/:material_short_id", adminOrAbove, moduleCtrl.DeleteMaterial)
			}

			// Feedback Forms
			ffAuth := middleware.RequireRole(models.RoleSuperAdmin, models.RoleMentor, models.RoleTeamLead)
			ff := protected.Group("/feedback-forms")
			{
				ff.POST("", ffAuth, feedbackFormCtrl.Create)
				ff.GET("", feedbackFormCtrl.GetAll)
				ff.GET("/:short_id", feedbackFormCtrl.GetByShortID)
				ff.PATCH("/:short_id", ffAuth, feedbackFormCtrl.Update)
				ff.DELETE("/:short_id", ffAuth, feedbackFormCtrl.Delete)

				ff.POST("/:short_id/questions", ffAuth, feedbackFormCtrl.AddQuestion)
				ff.PATCH("/:short_id/questions/:q_short_id", ffAuth, feedbackFormCtrl.UpdateQuestion)
				ff.DELETE("/:short_id/questions/:q_short_id", ffAuth, feedbackFormCtrl.DeleteQuestion)

				ff.POST("/:short_id/responses", feedbackFormCtrl.SubmitResponse)
			}

			// Blogs — only super_admin / team_lead may create, edit, or delete
			blogs := protected.Group("/blogs")
			{
				blogs.POST("", adminOrAbove, blogCtrl.Create)
				blogs.GET("", blogCtrl.GetAll)
				blogs.PATCH("/:short_id", adminOrAbove, blogCtrl.Update)
				blogs.DELETE("/:short_id", adminOrAbove, blogCtrl.Delete)
			}

			// Banners — only super_admin / team_lead may create, edit, or delete
			banners := protected.Group("/banners")
			{
				banners.POST("", adminOrAbove, bannerCtrl.Create)
				banners.GET("", bannerCtrl.GetAll)
				banners.PATCH("/:short_id", adminOrAbove, bannerCtrl.Update)
				banners.DELETE("/:short_id", adminOrAbove, bannerCtrl.Delete)
			}

			// Assessments — mentors are the ones who actually run these for their
			// batches, so create/edit/delete is staffOrAbove (same as Assignments/
			// Projects/Resources), not admin-only.
			// Also doubles as the real exam engine: question links + timed attempts.
			assessments := protected.Group("/assessments")
			{
				assessments.POST("", collegeContentOrAbove, assessmentCtrl.Create)
				assessments.GET("", assessmentCtrl.GetAll)
				assessments.GET("/:short_id", assessmentCtrl.GetByShortID)
				assessments.PATCH("/:short_id", collegeContentOrAbove, assessmentCtrl.Update)
				assessments.DELETE("/:short_id", collegeContentOrAbove, assessmentCtrl.Delete)
				assessments.POST("/:short_id/cancel", collegeContentOrAbove, examAttemptCtrl.CancelAssessment)
				assessments.POST("/:short_id/publish-results", collegeContentOrAbove, assessmentCtrl.PublishResults)

				// Question links — attach/reorder/detach bank questions on an assessment
				assessments.POST("/:short_id/questions", staffOrAbove, assessmentCtrl.AttachQuestion)
				assessments.GET("/:short_id/questions", staffOrAbove, assessmentCtrl.GetQuestions)
				assessments.PATCH("/:short_id/questions/:question_short_id", staffOrAbove, assessmentCtrl.UpdateAttachedQuestion)
				assessments.DELETE("/:short_id/questions/:question_short_id", staffOrAbove, assessmentCtrl.DetachQuestion)

				// Attempts — students start/answer/submit their own; staff monitor and grade
				assessments.GET("/:short_id/access-status", examAttemptCtrl.GetAccessStatus)
				assessments.POST("/:short_id/attempts", examAttemptCtrl.StartAttempt)
				assessments.GET("/:short_id/attempts", collegeContentOrAbove, examAttemptCtrl.GetAllAttempts)
				assessments.GET("/:short_id/attempts/me", examAttemptCtrl.GetMyAttempts)
				assessments.GET("/:short_id/attempts/:attempt_short_id", examAttemptCtrl.GetAttempt)
				assessments.POST("/:short_id/attempts/:attempt_short_id/answers", examAttemptCtrl.SubmitAnswer)
				assessments.POST("/:short_id/attempts/:attempt_short_id/submit", examAttemptCtrl.SubmitAttempt)
				assessments.POST("/:short_id/attempts/:attempt_short_id/violations", examAttemptCtrl.RecordViolation)
				assessments.PATCH("/:short_id/attempts/:attempt_short_id/answers/:question_short_id/grade", collegeContentOrAbove, examAttemptCtrl.GradeAnswer)
				assessments.POST("/:short_id/reattempts", collegeContentOrAbove, examAttemptCtrl.GrantReattempt)
			}

			// Question bank — private/course/global visibility; staff manage, everyone reads what they can see
			questions := protected.Group("/questions", middleware.RequireActiveSubscription(collegeRepo), middleware.RequireFeature(collegeRepo, models.FeatureCodingQuestionBank))
			{
				questions.POST("", staffOrAbove, questionBankCtrl.Create)
				questions.GET("", questionBankCtrl.GetAll)
				questions.GET("/:short_id", questionBankCtrl.GetByShortID)
				questions.PATCH("/:short_id", staffOrAbove, questionBankCtrl.Update)
				questions.DELETE("/:short_id", staffOrAbove, questionBankCtrl.Delete)
			}

			// Question bank taxonomy — Subject -> Topic -> Subtopic reference tree + per-subject stats
			questionBank := protected.Group("/question-bank", middleware.RequireActiveSubscription(collegeRepo), middleware.RequireFeature(collegeRepo, models.FeatureCodingQuestionBank))
			{
				questionBank.GET("/taxonomy", questionBankCtrl.GetTaxonomy)
				questionBank.GET("/stats", questionBankCtrl.GetStats)
				questionBank.POST("/subjects", staffOrAbove, questionBankCtrl.CreateSubject)
				questionBank.DELETE("/subjects/:short_id", staffOrAbove, questionBankCtrl.DeleteSubject)
			}

			// Communities — scoped to a batch. super_admin / team_lead / mentor manage them;
			// any authenticated user can read.
			communities := protected.Group("/communities", middleware.RequireActiveSubscription(collegeRepo), middleware.RequireFeature(collegeRepo, models.FeatureCommunity))
			{
				communities.POST("", staffOrAbove, communityCtrl.Create)
				communities.GET("", communityCtrl.GetAll)
				communities.GET("/me", communityCtrl.GetMyCommunities)
				communities.GET("/:short_id", communityCtrl.GetByShortID)
				communities.PATCH("/:short_id", staffOrAbove, communityCtrl.Update)
				communities.DELETE("/:short_id", staffOrAbove, communityCtrl.Delete)

				// Members — nested under a community
				communities.GET("/:short_id/members", communityCtrl.GetMembers)
				communities.POST("/:short_id/members", staffOrAbove, communityCtrl.AddMembers)
				communities.DELETE("/:short_id/members/:user_id", staffOrAbove, communityCtrl.RemoveMember)

				// Posts — a community's feed. Any member can post/comment/like;
				// checkCommunityMembership inside the controller enforces that
				// per-request (staff bypass, since they moderate every community).
				communities.POST("/:short_id/posts", communityPostCtrl.CreatePost)
				communities.GET("/:short_id/posts", communityPostCtrl.GetPosts)
			}

			// Post-level actions, not nested under a specific community (the
			// post already carries its community).
			posts := protected.Group("/posts")
			{
				posts.DELETE("/:short_id", communityPostCtrl.DeletePost)
				posts.PATCH("/:short_id/pin", staffOrAbove, communityPostCtrl.SetPostPinned)
				posts.POST("/:short_id/like", communityPostCtrl.ToggleLike)
				posts.POST("/:short_id/comments", communityPostCtrl.AddComment)
				posts.GET("/:short_id/comments", communityPostCtrl.GetComments)
			}
			protected.DELETE("/comments/:short_id", communityPostCtrl.DeleteComment)

			// Sessions — scoped to a batch. Scheduling (create/reschedule/cancel) is
			// super_admin / team_lead only — mentors view sessions for batches they
			// manage but never create/edit/cancel them (see Mentor role lockdown).
			// Any authenticated user can read.
			sessions := protected.Group("/sessions", middleware.RequireActiveSubscription(collegeRepo), middleware.RequireFeature(collegeRepo, models.FeatureLiveSessions))
			{
				sessions.POST("", adminOrAbove, sessionCtrl.Create)
				sessions.GET("", sessionCtrl.GetAll)
				sessions.GET("/:short_id", sessionCtrl.GetByShortID)
				sessions.GET("/:short_id/feedback-status", sessionCtrl.GetFeedbackStatus)
				sessions.PATCH("/:short_id", adminOrAbove, sessionCtrl.Update)
				sessions.DELETE("/:short_id", adminOrAbove, sessionCtrl.Delete)

				// Attendance — taken per session by super_admin / team_lead / mentor.
				sessions.GET("/:short_id/attendance", staffOrAbove, attendanceCtrl.GetSessionAttendance)
				sessions.POST("/:short_id/attendance/bulk", staffOrAbove, attendanceCtrl.BulkMarkAttendance)
				sessions.PATCH("/:short_id/attendance/:student_id", staffOrAbove, attendanceCtrl.MarkStudentAttendance)
			}

			// Assignments — scoped to a batch. super_admin / team_lead / mentor manage
			// them and grade submissions; students submit their own work.
			assignments := protected.Group("/assignments")
			{
				assignments.POST("", collegeContentOrAbove, assignmentCtrl.Create)
				assignments.GET("", assignmentCtrl.GetAll)
				assignments.GET("/:short_id", assignmentCtrl.GetByShortID)
				assignments.PATCH("/:short_id", collegeContentOrAbove, assignmentCtrl.Update)
				assignments.DELETE("/:short_id", collegeContentOrAbove, assignmentCtrl.Delete)
				assignments.POST("/:short_id/publish-results", collegeContentOrAbove, assignmentCtrl.PublishResults)

				// Submissions — nested under an assignment
				assignments.POST("/:short_id/submissions", assignmentCtrl.CreateSubmission)
				assignments.GET("/:short_id/submissions/me", assignmentCtrl.GetMySubmission)
				assignments.GET("/:short_id/submissions", collegeContentOrAbove, assignmentCtrl.GetAllSubmissions)
				assignments.PATCH("/:short_id/submissions/:submission_short_id", collegeContentOrAbove, assignmentCtrl.GradeSubmission)
			}

			// Resources — learning materials. Leave batch unset on create for global
			// visibility; scoped resources are only visible to that batch's students.
			resources := protected.Group("/resources")
			{
				resources.POST("", staffOrAbove, resourceCtrl.Create)
				resources.GET("", resourceCtrl.GetAll)
				resources.GET("/:short_id", resourceCtrl.GetByShortID)
				resources.PATCH("/:short_id", staffOrAbove, resourceCtrl.Update)
				resources.DELETE("/:short_id", staffOrAbove, resourceCtrl.Delete)
			}

			// Projects — scoped to a batch, made up of milestones. Team-based projects
			// use nested teams; individual projects submit directly per student.
			projects := protected.Group("/projects", middleware.RequireActiveSubscription(collegeRepo), middleware.RequireFeature(collegeRepo, models.FeatureProjects))
			{
				projects.POST("", collegeContentOrAbove, projectCtrl.Create)
				projects.GET("", projectCtrl.GetAll)
				projects.GET("/:short_id", projectCtrl.GetByShortID)
				projects.PATCH("/:short_id", collegeContentOrAbove, projectCtrl.Update)
				projects.DELETE("/:short_id", collegeContentOrAbove, projectCtrl.Delete)

				// Milestones — nested under a project
				projects.POST("/:short_id/milestones", collegeContentOrAbove, projectCtrl.AddMilestone)
				projects.PATCH("/:short_id/milestones/:milestone_short_id", collegeContentOrAbove, projectCtrl.UpdateMilestone)
				projects.DELETE("/:short_id/milestones/:milestone_short_id", collegeContentOrAbove, projectCtrl.DeleteMilestone)

				// Teams — nested under a project, only relevant when is_team_project
				projects.POST("/:short_id/teams", collegeContentOrAbove, projectCtrl.CreateTeam)
				projects.DELETE("/:short_id/teams/:team_short_id", collegeContentOrAbove, projectCtrl.DeleteTeam)
				projects.POST("/:short_id/teams/:team_short_id/members", collegeContentOrAbove, projectCtrl.AddTeamMembers)
				projects.DELETE("/:short_id/teams/:team_short_id/members/:user_id", collegeContentOrAbove, projectCtrl.RemoveTeamMember)

				// Submissions — nested under a milestone
				projects.POST("/:short_id/milestones/:milestone_short_id/submissions", projectCtrl.CreateSubmission)
				projects.GET("/:short_id/milestones/:milestone_short_id/submissions/me", projectCtrl.GetMySubmission)
				projects.GET("/:short_id/milestones/:milestone_short_id/submissions", collegeContentOrAbove, projectCtrl.GetAllSubmissions)
				projects.PATCH("/:short_id/milestones/:milestone_short_id/submissions/:submission_short_id", collegeContentOrAbove, projectCtrl.GradeSubmission)
				projects.POST("/:short_id/milestones/:milestone_short_id/publish-results", collegeContentOrAbove, projectCtrl.PublishResults)

				// Direct submissions — no milestone required; submitted straight
				// against the project itself.
				projects.POST("/:short_id/submissions", projectCtrl.CreateProjectSubmission)
				projects.GET("/:short_id/submissions/me", projectCtrl.GetMyProjectSubmission)
				projects.GET("/:short_id/submissions", collegeContentOrAbove, projectCtrl.GetAllProjectSubmissions)
				projects.PATCH("/:short_id/submissions/:submission_short_id", collegeContentOrAbove, projectCtrl.GradeProjectSubmission)
				projects.POST("/:short_id/publish-results", collegeContentOrAbove, projectCtrl.PublishProjectResults)
			}

			// Notifications — GET/read are per-user (my inbox); manage-content is admin-only
			notifications := protected.Group("/notifications")
			{
				notifications.POST("", adminOrAbove, notificationCtrl.Create)
				notifications.GET("", notificationCtrl.GetInbox)
				notifications.PATCH("/:short_id", adminOrAbove, notificationCtrl.Update)
				notifications.PATCH("/:short_id/read", notificationCtrl.MarkRead)
				notifications.DELETE("/:short_id", adminOrAbove, notificationCtrl.Delete)
				// Mobile app device registration — any authenticated user may
				// register/unregister their own device's push token.
				notifications.POST("/push-token", devicePushTokenCtrl.RegisterToken)
				notifications.DELETE("/push-token", devicePushTokenCtrl.UnregisterToken)
			}

			// Help & Support — FAQs (admin-managed, published ones visible to
			// everyone) and student-submitted support tickets (college-scoped
			// for college_admin/college_staff on the list, same as everywhere
			// else — enforced inside GetAllTicketsAdmin).
			faqs := protected.Group("/faqs")
			{
				faqs.GET("", helpSupportCtrl.GetFAQs)
				faqs.GET("/admin", staffOrAbove, helpSupportCtrl.GetFAQsAdmin)
				faqs.POST("", staffOrAbove, helpSupportCtrl.CreateFAQ)
				faqs.PATCH("/:short_id", staffOrAbove, helpSupportCtrl.UpdateFAQ)
				faqs.DELETE("/:short_id", staffOrAbove, helpSupportCtrl.DeleteFAQ)
			}
			supportTickets := protected.Group("/support-tickets")
			{
				supportTickets.POST("", helpSupportCtrl.CreateTicket)
				supportTickets.GET("/me", helpSupportCtrl.GetMyTickets)
				supportTickets.GET("", collegeReadOrAbove, helpSupportCtrl.GetAllTicketsAdmin)
				supportTickets.PATCH("/:short_id/status", collegeReadOrAbove, helpSupportCtrl.UpdateTicketStatus)
			}

			// Upload
			protected.POST("/upload/image", uploadCtrl.UploadEventImage)
			protected.POST("/upload/blog-image", uploadCtrl.UploadBlogImage)
			protected.POST("/upload/banner-image", uploadCtrl.UploadBannerImage)
			protected.POST("/upload/material", uploadCtrl.UploadMaterial)
			protected.POST("/upload/assessment-thumbnail", uploadCtrl.UploadAssessmentThumbnail)
			protected.POST("/upload/course-thumbnail", uploadCtrl.UploadCourseThumbnail)
			protected.POST("/upload/assessment-file", uploadCtrl.UploadAssessmentFile)
			protected.POST("/upload/assignment-file", uploadCtrl.UploadAssignmentFile)
			protected.POST("/upload/resource-file", uploadCtrl.UploadResourceFile)
			protected.POST("/upload/project-file", uploadCtrl.UploadProjectFile)
			protected.POST("/upload/resume", uploadCtrl.UploadResumeFile)
			protected.POST("/upload/leave-certificate", leadOrAbove, uploadCtrl.UploadLeaveCertificate)
			protected.POST("/upload/attendance-selfie", leadOrAbove, uploadCtrl.UploadAttendanceSelfie)
		}
	}

	return r
}
