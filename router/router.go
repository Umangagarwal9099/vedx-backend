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
	r.Use(middleware.CORS())

	// Swagger UI — http://localhost:8080/swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Repositories
	userRepo             := repository.NewUserRepository(pool)
	courseRepo           := repository.NewCourseRepository(pool)
	batchRepo            := repository.NewBatchRepository(pool)
	eventRepo            := repository.NewEventRepository(pool)
	announcementRepo     := repository.NewAnnouncementRepository(pool)
	blogRepo             := repository.NewBlogRepository(pool)
	bannerRepo           := repository.NewBannerRepository(pool)
	codingQuestionRepo   := repository.NewCodingQuestionRepository(pool)
	submissionRepo       := repository.NewSubmissionRepository(pool)
	feedbackFormRepo     := repository.NewFeedbackFormRepository(pool)
	moduleRepo           := repository.NewModuleRepository(pool)
	assessmentRepo       := repository.NewAssessmentRepository(pool)
	communityRepo        := repository.NewCommunityRepository(pool)
	notificationRepo     := repository.NewNotificationRepository(pool)
	sessionRepo          := repository.NewSessionRepository(pool)
	assignmentRepo       := repository.NewAssignmentRepository(pool)
	resourceRepo         := repository.NewResourceRepository(pool)
	projectRepo          := repository.NewProjectRepository(pool)
	questionBankRepo     := repository.NewQuestionBankRepository(pool)
	examAttemptRepo      := repository.NewExamAttemptRepository(pool)

	// Services
	storageSvc := service.NewStorageService(cfg.Storage)
	zoomSvc := service.NewZoomService(cfg.Zoom)
	emailSvc := service.NewEmailService(cfg.SMTP)

	// Controllers
	authCtrl             := controller.NewAuthController(userRepo, cfg.JWT.Secret)
	userCtrl             := controller.NewUserController(userRepo)
	courseCtrl           := controller.NewCourseController(courseRepo, notificationRepo)
	batchCtrl            := controller.NewBatchController(batchRepo, notificationRepo, userRepo, emailSvc)
	eventCtrl            := controller.NewEventController(eventRepo, notificationRepo)
	announcementCtrl     := controller.NewAnnouncementController(announcementRepo, notificationRepo)
	uploadCtrl           := controller.NewUploadController(storageSvc)
	codingQuestionCtrl   := controller.NewCodingQuestionController(codingQuestionRepo, notificationRepo)
	submissionCtrl       := controller.NewSubmissionController(submissionRepo)
	feedbackFormCtrl     := controller.NewFeedbackFormController(feedbackFormRepo, notificationRepo)
	moduleCtrl           := controller.NewModuleController(moduleRepo, notificationRepo, userRepo, emailSvc)
	blogCtrl             := controller.NewBlogController(blogRepo, notificationRepo)
	bannerCtrl           := controller.NewBannerController(bannerRepo, notificationRepo)
	assessmentCtrl       := controller.NewAssessmentController(assessmentRepo, questionBankRepo, batchRepo, notificationRepo)
	communityCtrl        := controller.NewCommunityController(communityRepo, notificationRepo, userRepo, emailSvc)
	notificationCtrl     := controller.NewNotificationController(notificationRepo)
	sessionCtrl          := controller.NewSessionController(sessionRepo, batchRepo, notificationRepo, userRepo, zoomSvc, emailSvc, cfg.App.PublicURL, cfg.App.Timezone)
	zoomWebhookCtrl      := controller.NewZoomWebhookController(zoomSvc, sessionRepo)
	assignmentCtrl       := controller.NewAssignmentController(assignmentRepo, batchRepo, notificationRepo)
	resourceCtrl         := controller.NewResourceController(resourceRepo)
	projectCtrl          := controller.NewProjectController(projectRepo, batchRepo, notificationRepo)
	questionBankCtrl     := controller.NewQuestionBankController(questionBankRepo)
	examAttemptCtrl      := controller.NewExamAttemptController(examAttemptRepo, assessmentRepo, questionBankRepo, batchRepo, notificationRepo)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", controller.Health)

		// ── Public auth routes ────────────────────────────────────────────────
		auth := v1.Group("/auth")
		{
			auth.POST("/login", authCtrl.Login)
			auth.POST("/register", authCtrl.Register)
		}

		// Public session join-link resolution — the token itself is the credential
		// (like a magic link), so this stays outside the JWT-protected group.
		v1.GET("/sessions/by-token/:token", sessionCtrl.JoinByToken)

		// Zoom calls this directly (no JWT) — authenticity is instead verified via
		// the x-zm-signature header against the Event Subscriptions secret token.
		v1.POST("/zoom/webhook", zoomWebhookCtrl.HandleWebhook)

		// ── Protected routes — require a valid JWT ────────────────────────────
		protected := v1.Group("/")
		protected.Use(middleware.JWTAuth(cfg.JWT.Secret))
		{
			// Users — static paths registered before /:id so Gin matches them first
			protected.GET("/users", userCtrl.GetAll)
			protected.GET("/users/deleted", userCtrl.GetDeleted)
			protected.GET("/users/search", userCtrl.Search)
			protected.PATCH("/users/:id", userCtrl.Update)
			protected.PATCH("/users/:id/role", middleware.RequireRole(models.RoleSuperAdmin), userCtrl.ChangeRole)
			protected.DELETE("/users/:id", userCtrl.Delete)

			// Mentors list — for batch manager dropdown
			protected.GET("/mentors", userCtrl.GetMentors)

			// Role sets used across multiple route groups
			adminOrAbove := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead)
			staffOrAbove := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor)

			// Courses — only super_admin / team_lead may create, edit, or delete
			courses := protected.Group("/courses")
			{
				courses.POST("", adminOrAbove, courseCtrl.Create)
				courses.GET("", courseCtrl.GetAll)
				courses.GET("/search", courseCtrl.Search)
				courses.PATCH("/:short_id", adminOrAbove, courseCtrl.Update)
				courses.DELETE("/:short_id", adminOrAbove, courseCtrl.Delete)
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
				batches.GET("/:short_id/me/fees", batchCtrl.GetMyFeesStatus)

				// Sessions — scoped to this batch
				batches.GET("/:short_id/sessions", sessionCtrl.GetByBatch)
				batches.GET("/:short_id/recordings", sessionCtrl.GetBatchRecordings)
			}

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
				cq.GET("", codingQuestionCtrl.GetAll)
				cq.GET("/admin", codingQuestionCtrl.GetAllAdmin)
				cq.GET("/:short_id", codingQuestionCtrl.GetByShortID)
				cq.PATCH("/:short_id", staffOrAbove, codingQuestionCtrl.Update)
				cq.DELETE("/:short_id", staffOrAbove, codingQuestionCtrl.Delete)
			}

			// Submissions
			subs := protected.Group("/submissions")
			{
				subs.POST("", submissionCtrl.Create)
				subs.GET("/me", submissionCtrl.GetMySubmissions)
				subs.GET("/question/:short_id", submissionCtrl.GetByQuestion)
				// Admin/mentor — see all students' submissions
				subs.GET("/admin", submissionCtrl.GetAllAdmin)
				subs.GET("/user/:user_id", submissionCtrl.GetByUserAdmin)
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
				assessments.POST("",             staffOrAbove, assessmentCtrl.Create)
				assessments.GET("",              assessmentCtrl.GetAll)
				assessments.GET("/:short_id",    assessmentCtrl.GetByShortID)
				assessments.PATCH("/:short_id",  staffOrAbove, assessmentCtrl.Update)
				assessments.DELETE("/:short_id", staffOrAbove, assessmentCtrl.Delete)
				assessments.POST("/:short_id/cancel", staffOrAbove, examAttemptCtrl.CancelAssessment)

				// Question links — attach/reorder/detach bank questions on an assessment
				assessments.POST("/:short_id/questions",                            staffOrAbove, assessmentCtrl.AttachQuestion)
				assessments.GET("/:short_id/questions",                             assessmentCtrl.GetQuestions)
				assessments.PATCH("/:short_id/questions/:question_short_id",        staffOrAbove, assessmentCtrl.UpdateAttachedQuestion)
				assessments.DELETE("/:short_id/questions/:question_short_id",       staffOrAbove, assessmentCtrl.DetachQuestion)

				// Attempts — students start/answer/submit their own; staff monitor and grade
				assessments.POST("/:short_id/attempts",                             examAttemptCtrl.StartAttempt)
				assessments.GET("/:short_id/attempts",                staffOrAbove, examAttemptCtrl.GetAllAttempts)
				assessments.GET("/:short_id/attempts/me",                           examAttemptCtrl.GetMyAttempts)
				assessments.GET("/:short_id/attempts/:attempt_short_id",            examAttemptCtrl.GetAttempt)
				assessments.POST("/:short_id/attempts/:attempt_short_id/answers",   examAttemptCtrl.SubmitAnswer)
				assessments.POST("/:short_id/attempts/:attempt_short_id/submit",    examAttemptCtrl.SubmitAttempt)
				assessments.PATCH("/:short_id/attempts/:attempt_short_id/answers/:question_short_id/grade", staffOrAbove, examAttemptCtrl.GradeAnswer)
				assessments.POST("/:short_id/reattempts", staffOrAbove, examAttemptCtrl.GrantReattempt)
			}

			// Question bank — private/course/global visibility; staff manage, everyone reads what they can see
			questions := protected.Group("/questions")
			{
				questions.POST("",             staffOrAbove, questionBankCtrl.Create)
				questions.GET("",              questionBankCtrl.GetAll)
				questions.GET("/:short_id",    questionBankCtrl.GetByShortID)
				questions.PATCH("/:short_id",  staffOrAbove, questionBankCtrl.Update)
				questions.DELETE("/:short_id", staffOrAbove, questionBankCtrl.Delete)
			}

			// Communities — scoped to a batch. super_admin / team_lead / mentor manage them;
			// any authenticated user can read.
			communities := protected.Group("/communities")
			{
				communities.POST("", staffOrAbove, communityCtrl.Create)
				communities.GET("", communityCtrl.GetAll)
				communities.GET("/:short_id", communityCtrl.GetByShortID)
				communities.PATCH("/:short_id", staffOrAbove, communityCtrl.Update)
				communities.DELETE("/:short_id", staffOrAbove, communityCtrl.Delete)

				// Members — nested under a community
				communities.GET("/:short_id/members", communityCtrl.GetMembers)
				communities.POST("/:short_id/members", staffOrAbove, communityCtrl.AddMembers)
				communities.DELETE("/:short_id/members/:user_id", staffOrAbove, communityCtrl.RemoveMember)
			}

			// Sessions — scoped to a batch. super_admin / team_lead / mentor manage them;
			// any authenticated user can read.
			sessions := protected.Group("/sessions")
			{
				sessions.POST("", staffOrAbove, sessionCtrl.Create)
				sessions.GET("", sessionCtrl.GetAll)
				sessions.GET("/:short_id", sessionCtrl.GetByShortID)
				sessions.PATCH("/:short_id", staffOrAbove, sessionCtrl.Update)
				sessions.DELETE("/:short_id", staffOrAbove, sessionCtrl.Delete)
			}

			// Assignments — scoped to a batch. super_admin / team_lead / mentor manage
			// them and grade submissions; students submit their own work.
			assignments := protected.Group("/assignments")
			{
				assignments.POST("",             staffOrAbove, assignmentCtrl.Create)
				assignments.GET("",              assignmentCtrl.GetAll)
				assignments.GET("/:short_id",    assignmentCtrl.GetByShortID)
				assignments.PATCH("/:short_id",  staffOrAbove, assignmentCtrl.Update)
				assignments.DELETE("/:short_id", staffOrAbove, assignmentCtrl.Delete)

				// Submissions — nested under an assignment
				assignments.POST("/:short_id/submissions",                             assignmentCtrl.CreateSubmission)
				assignments.GET("/:short_id/submissions/me",                           assignmentCtrl.GetMySubmission)
				assignments.GET("/:short_id/submissions",                staffOrAbove, assignmentCtrl.GetAllSubmissions)
				assignments.PATCH("/:short_id/submissions/:submission_short_id", staffOrAbove, assignmentCtrl.GradeSubmission)
			}

			// Resources — learning materials. Leave batch unset on create for global
			// visibility; scoped resources are only visible to that batch's students.
			resources := protected.Group("/resources")
			{
				resources.POST("",             staffOrAbove, resourceCtrl.Create)
				resources.GET("",              resourceCtrl.GetAll)
				resources.GET("/:short_id",    resourceCtrl.GetByShortID)
				resources.PATCH("/:short_id",  staffOrAbove, resourceCtrl.Update)
				resources.DELETE("/:short_id", staffOrAbove, resourceCtrl.Delete)
			}

			// Projects — scoped to a batch, made up of milestones. Team-based projects
			// use nested teams; individual projects submit directly per student.
			projects := protected.Group("/projects")
			{
				projects.POST("",             staffOrAbove, projectCtrl.Create)
				projects.GET("",              projectCtrl.GetAll)
				projects.GET("/:short_id",    projectCtrl.GetByShortID)
				projects.PATCH("/:short_id",  staffOrAbove, projectCtrl.Update)
				projects.DELETE("/:short_id", staffOrAbove, projectCtrl.Delete)

				// Milestones — nested under a project
				projects.POST("/:short_id/milestones",                          staffOrAbove, projectCtrl.AddMilestone)
				projects.PATCH("/:short_id/milestones/:milestone_short_id",     staffOrAbove, projectCtrl.UpdateMilestone)
				projects.DELETE("/:short_id/milestones/:milestone_short_id",    staffOrAbove, projectCtrl.DeleteMilestone)

				// Teams — nested under a project, only relevant when is_team_project
				projects.POST("/:short_id/teams",                                    staffOrAbove, projectCtrl.CreateTeam)
				projects.DELETE("/:short_id/teams/:team_short_id",                   staffOrAbove, projectCtrl.DeleteTeam)
				projects.POST("/:short_id/teams/:team_short_id/members",             staffOrAbove, projectCtrl.AddTeamMembers)
				projects.DELETE("/:short_id/teams/:team_short_id/members/:user_id",  staffOrAbove, projectCtrl.RemoveTeamMember)

				// Submissions — nested under a milestone
				projects.POST("/:short_id/milestones/:milestone_short_id/submissions",                                projectCtrl.CreateSubmission)
				projects.GET("/:short_id/milestones/:milestone_short_id/submissions/me",                              projectCtrl.GetMySubmission)
				projects.GET("/:short_id/milestones/:milestone_short_id/submissions",             staffOrAbove,       projectCtrl.GetAllSubmissions)
				projects.PATCH("/:short_id/milestones/:milestone_short_id/submissions/:submission_short_id", staffOrAbove, projectCtrl.GradeSubmission)
			}

			// Notifications — GET/read are per-user (my inbox); manage-content is admin-only
			notifications := protected.Group("/notifications")
			{
				notifications.POST("", adminOrAbove, notificationCtrl.Create)
				notifications.GET("", notificationCtrl.GetInbox)
				notifications.PATCH("/:short_id", adminOrAbove, notificationCtrl.Update)
				notifications.PATCH("/:short_id/read", notificationCtrl.MarkRead)
				notifications.DELETE("/:short_id", adminOrAbove, notificationCtrl.Delete)
			}

			// Upload
			protected.POST("/upload/image", uploadCtrl.UploadEventImage)
			protected.POST("/upload/blog-image", uploadCtrl.UploadBlogImage)
			protected.POST("/upload/banner-image", uploadCtrl.UploadBannerImage)
			protected.POST("/upload/material", uploadCtrl.UploadMaterial)
			protected.POST("/upload/assessment-thumbnail", uploadCtrl.UploadAssessmentThumbnail)
			protected.POST("/upload/assessment-file",      uploadCtrl.UploadAssessmentFile)
			protected.POST("/upload/assignment-file",      uploadCtrl.UploadAssignmentFile)
			protected.POST("/upload/resource-file",        uploadCtrl.UploadResourceFile)
			protected.POST("/upload/project-file",         uploadCtrl.UploadProjectFile)
		}
	}

	return r
}
