package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/umangagarwal/vedx-backend/config"
	"github.com/umangagarwal/vedx-backend/db"
	"github.com/umangagarwal/vedx-backend/docs"
	"github.com/umangagarwal/vedx-backend/router"
)

//	@title			Vedex API
//	@version		1.0
//	@description	Vedex backend API. Authenticate via POST /api/v1/auth/login to get a JWT, then pass it as `Authorization: Bearer <token>` on protected endpoints.

//	@contact.name	Umang Agarwal
//	@contact.email	umangagarwal9099@gmail.com

//	@host		localhost:8080
//	@BasePath	/api/v1

//	@schemes	http https

//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Format: `Bearer <token>`
func main() {
	// Load .env for local development — silently ignored in production
	// where env vars are injected by the platform (Render, Railway, etc.)
	_ = godotenv.Load()

	if host := os.Getenv("PUBLIC_HOST"); host != "" {
		docs.SwaggerInfo.Host = host
		docs.SwaggerInfo.Schemes = []string{"https"}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	pool, err := db.NewPool(cfg.Database)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer pool.Close()
	log.Println("Connected to Supabase PostgreSQL")

	r := router.New(pool, cfg)

	srv := &http.Server{
		Addr:    ":" + cfg.App.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Server running  →  http://localhost:%s", cfg.App.Port)
		log.Printf("Swagger UI      →  http://localhost:%s/swagger/index.html", cfg.App.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Forced shutdown: %v", err)
	}
	log.Println("Server exited")
}
