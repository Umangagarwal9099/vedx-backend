package router

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/umangagarwal/vedx-backend/docs"
	"github.com/umangagarwal/vedx-backend/config"
	"github.com/umangagarwal/vedx-backend/controller"
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
	codingQuestionRepo   := repository.NewCodingQuestionRepository(pool)
	submissionRepo       := repository.NewSubmissionRepository(pool)
	feedbackFormRepo     := repository.NewFeedbackFormRepository(pool)

	// Services
	storageSvc := service.NewStorageService(cfg.Storage)

	// Controllers
	authCtrl             := controller.NewAuthController(userRepo, cfg.JWT.Secret)
	userCtrl             := controller.NewUserController(userRepo)
	courseCtrl           := controller.NewCourseController(courseRepo)
	batchCtrl            := controller.NewBatchController(batchRepo)
	eventCtrl            := controller.NewEventController(eventRepo)
	announcementCtrl     := controller.NewAnnouncementController(announcementRepo)
	uploadCtrl           := controller.NewUploadController(storageSvc)
	codingQuestionCtrl   := controller.NewCodingQuestionController(codingQuestionRepo)
	submissionCtrl       := controller.NewSubmissionController(submissionRepo)
	feedbackFormCtrl     := controller.NewFeedbackFormController(feedbackFormRepo)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", controller.Health)

		// ── Public auth routes ────────────────────────────────────────────────
		auth := v1.Group("/auth")
		{
			auth.POST("/login", authCtrl.Login)
			auth.POST("/register", authCtrl.Register)
		}

		// ── Protected routes — require a valid JWT ────────────────────────────
		protected := v1.Group("/")
		protected.Use(middleware.JWTAuth(cfg.JWT.Secret))
		{
			// Users — static paths registered before /:id so Gin matches them first
			protected.GET("/users",                                                        userCtrl.GetAll)
			protected.GET("/users/deleted",                                                userCtrl.GetDeleted)
			protected.GET("/users/search",                                                 userCtrl.Search)
			protected.PATCH("/users/:id",                                                  userCtrl.Update)
			protected.PATCH("/users/:id/role", middleware.RequireRole(models.RoleSuperAdmin), userCtrl.ChangeRole)
			protected.DELETE("/users/:id",                                                 userCtrl.Delete)

			// Mentors list — for batch manager dropdown
			protected.GET("/mentors", userCtrl.GetMentors)

			// Role sets used across multiple route groups
			adminOrAbove   := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead)
			staffOrAbove   := middleware.RequireRole(models.RoleSuperAdmin, models.RoleTeamLead, models.RoleMentor)

			// Courses — only super_admin / team_lead may create, edit, or delete
			courses := protected.Group("/courses")
			{
				courses.POST("",             adminOrAbove, courseCtrl.Create)
				courses.GET("",              courseCtrl.GetAll)
				courses.GET("/search",       courseCtrl.Search)
				courses.PATCH("/:short_id",  adminOrAbove, courseCtrl.Update)
				courses.DELETE("/:short_id", adminOrAbove, courseCtrl.Delete)
			}

			// Batches — only super_admin / team_lead may create, edit, or delete
			batches := protected.Group("/batches")
			{
				batches.POST("",             adminOrAbove, batchCtrl.Create)
				batches.GET("",              batchCtrl.GetAll)
				batches.GET("/filter",       batchCtrl.Filter)
				batches.PATCH("/:short_id",  adminOrAbove, batchCtrl.Update)
				batches.DELETE("/:short_id", adminOrAbove, batchCtrl.Delete)
			}

			// Events — super_admin / team_lead / mentor may create, edit, or delete
			events := protected.Group("/events")
			{
				events.POST("",              staffOrAbove, eventCtrl.Create)
				events.GET("",               eventCtrl.GetAll)
				events.PATCH("/:short_id",   staffOrAbove, eventCtrl.Update)
				events.DELETE("/:short_id",  staffOrAbove, eventCtrl.Delete)
			}

			// Announcements — super_admin / team_lead / mentor may create, edit, or delete
			announcements := protected.Group("/announcements")
			{
				announcements.POST("",             staffOrAbove, announcementCtrl.Create)
				announcements.GET("",              announcementCtrl.GetAll)
				announcements.PATCH("/:short_id",  staffOrAbove, announcementCtrl.Update)
				announcements.DELETE("/:short_id", staffOrAbove, announcementCtrl.Delete)
			}

			// Coding Questions — super_admin / team_lead / mentor may create, edit, or delete
			cq := protected.Group("/coding-questions")
			{
				cq.POST("",              staffOrAbove, codingQuestionCtrl.Create)
				cq.GET("",               codingQuestionCtrl.GetAll)
				cq.GET("/admin",         codingQuestionCtrl.GetAllAdmin)
				cq.GET("/:short_id",     codingQuestionCtrl.GetByShortID)
				cq.PATCH("/:short_id",   staffOrAbove, codingQuestionCtrl.Update)
				cq.DELETE("/:short_id",  staffOrAbove, codingQuestionCtrl.Delete)
			}

			// Submissions
			subs := protected.Group("/submissions")
			{
				subs.POST("",                    submissionCtrl.Create)
				subs.GET("/me",                  submissionCtrl.GetMySubmissions)
				subs.GET("/question/:short_id",  submissionCtrl.GetByQuestion)
				// Admin/mentor — see all students' submissions
				subs.GET("/admin",               submissionCtrl.GetAllAdmin)
				subs.GET("/user/:user_id",        submissionCtrl.GetByUserAdmin)
			}

			// Feedback Forms
			ffAuth := middleware.RequireRole(models.RoleSuperAdmin, models.RoleMentor, models.RoleTeamLead)
			ff := protected.Group("/feedback-forms")
			{
				ff.POST("",              ffAuth, feedbackFormCtrl.Create)
				ff.GET("",              feedbackFormCtrl.GetAll)
				ff.GET("/:short_id",   feedbackFormCtrl.GetByShortID)
				ff.PATCH("/:short_id", ffAuth, feedbackFormCtrl.Update)
				ff.DELETE("/:short_id",ffAuth, feedbackFormCtrl.Delete)

				ff.POST("/:short_id/questions",                ffAuth, feedbackFormCtrl.AddQuestion)
				ff.PATCH("/:short_id/questions/:q_short_id",  ffAuth, feedbackFormCtrl.UpdateQuestion)
				ff.DELETE("/:short_id/questions/:q_short_id", ffAuth, feedbackFormCtrl.DeleteQuestion)

				ff.POST("/:short_id/responses", feedbackFormCtrl.SubmitResponse)
			}

			// Upload
			protected.POST("/upload/image", uploadCtrl.UploadEventImage)
		}
	}

	return r
}
