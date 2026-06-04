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
)

func New(pool *pgxpool.Pool, cfg *config.Config) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.CORS())

	// Swagger UI — http://localhost:8080/swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Repositories
	userRepo   := repository.NewUserRepository(pool)
	courseRepo := repository.NewCourseRepository(pool)
	batchRepo  := repository.NewBatchRepository(pool)

	// Controllers
	authCtrl   := controller.NewAuthController(userRepo, cfg.JWT.Secret)
	userCtrl   := controller.NewUserController(userRepo)
	courseCtrl := controller.NewCourseController(courseRepo)
	batchCtrl  := controller.NewBatchController(batchRepo)

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

			// Courses — static paths registered before /:short_id
			courses := protected.Group("/courses")
			{
				courses.POST("",                middleware.RequireRole(models.RoleSuperAdmin), courseCtrl.Create)
				courses.GET("",                 courseCtrl.GetAll)
				courses.GET("/search",          courseCtrl.Search)
				courses.PATCH("/:short_id",     courseCtrl.Update)
				courses.DELETE("/:short_id",    courseCtrl.Delete)
			}

			// Batches — static paths registered before /:short_id
			batches := protected.Group("/batches")
			{
				batches.POST("",                middleware.RequireRole(models.RoleSuperAdmin), batchCtrl.Create)
				batches.GET("",                 batchCtrl.GetAll)
				batches.GET("/filter",          batchCtrl.Filter)
				batches.PATCH("/:short_id",     batchCtrl.Update)
				batches.DELETE("/:short_id",    batchCtrl.Delete)
			}
		}
	}

	return r
}
