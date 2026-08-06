package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/umangagarwal/vedx-backend/config"
)

func main() {
	_ = godotenv.Load(".env")
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	pool, err := pgxpool.New(context.Background(), cfg.Database.DSN())
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT short_id, name, length(thumbnail), left(thumbnail, 30)
		FROM courses WHERE deleted_at IS NULL ORDER BY length(thumbnail) DESC`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	totalLen := 0
	for rows.Next() {
		var shortID, name, prefix string
		var length int
		if err := rows.Scan(&shortID, &name, &length, &prefix); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-10s %-45s len=%d prefix=%q\n", shortID, name, length, prefix)
		totalLen += length
	}
	fmt.Println("\nTotal thumbnail bytes across all courses:", totalLen)
}
