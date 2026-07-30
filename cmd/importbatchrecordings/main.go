// Command importbatchrecordings backfills batch_recordings rows for videos
// that were uploaded straight to Cloudflare R2 (e.g. via rclone/dashboard)
// instead of through the admin panel's upload flow. It scans an R2 prefix,
// skips objects that already have a matching row (by URL), and inserts one
// batch_recordings row per remaining video — mirroring what
// BatchRecordingController.Complete does after a presigned upload, minus the
// actual PUT.
//
// Usage:
//
//	go run ./cmd/importbatchrecordings -batch A3F72C1D -email admin@example.com
//	go run ./cmd/importbatchrecordings -batch A3F72C1D -email admin@example.com -dry-run
//	go run ./cmd/importbatchrecordings -batch A3F72C1D -email admin@example.com -prefix recordings/batch/A3F72C1D/week1/
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/joho/godotenv"

	"github.com/umangagarwal/vedx-backend/config"
	"github.com/umangagarwal/vedx-backend/db"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

// videoExtMIME mirrors service.allowedVideoMIME (unexported), used as a
// fallback when R2's HeadObject content-type is generic/missing.
var videoExtMIME = map[string]string{
	".mp4":  "video/mp4",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
	".webm": "video/webm",
}

func main() {
	batchShortID := flag.String("batch", "", "batch short_id these recordings belong to (required)")
	email := flag.String("email", "", "email of the user recordings should be attributed to (required)")
	prefix := flag.String("prefix", "", "R2 key prefix to scan (default: recordings/batch/<batch>/)")
	dryRun := flag.Bool("dry-run", false, "list what would be imported without writing to the database")
	flag.Parse()

	if *batchShortID == "" || *email == "" {
		log.Fatal("usage: importbatchrecordings -batch <BATCH_SHORT_ID> -email <uploader@email> [-prefix recordings/batch/XXXX/] [-dry-run]")
	}
	if *prefix == "" {
		*prefix = fmt.Sprintf("recordings/batch/%s/", *batchShortID)
	}

	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	pool, err := db.NewPool(cfg.Database)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	// Fail fast with a clear message instead of a cryptic NOT NULL/constraint
	// error partway through 144 inserts.
	var batchID string
	if err := pool.QueryRow(ctx, `SELECT id FROM batches WHERE short_id = $1 AND deleted_at IS NULL`, *batchShortID).Scan(&batchID); err != nil {
		log.Fatalf("batch %q not found (create it in the admin panel first): %v", *batchShortID, err)
	}

	var uploaderID string
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1 AND deleted_at IS NULL`, *email).Scan(&uploaderID); err != nil {
		log.Fatalf("user with email %q not found: %v", *email, err)
	}

	storage := service.NewStorageService(cfg.Storage)
	recordingRepo := repository.NewBatchRecordingRepository(pool)

	existing, err := recordingRepo.FindByBatchShortID(ctx, *batchShortID)
	if err != nil {
		log.Fatalf("could not load existing recordings: %v", err)
	}
	existingURLs := make(map[string]bool, len(existing))
	for _, r := range existing {
		existingURLs[r.URL] = true
	}

	s3Client := s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.Storage.AccountID)),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.Storage.AccessKeyID, cfg.Storage.SecretAccessKey, ""),
	})

	var keys []string
	paginator := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(cfg.Storage.Bucket),
		Prefix: aws.String(*prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Fatalf("list R2 objects under %q: %v", *prefix, err)
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if strings.HasSuffix(key, "/") {
				continue // folder placeholder, not a file
			}
			if _, ok := videoExtMIME[strings.ToLower(filepath.Ext(key))]; !ok {
				log.Printf("skip (not a recognized video extension): %s", key)
				continue
			}
			keys = append(keys, key)
		}
	}

	if len(keys) == 0 {
		log.Fatalf("no video objects found under %q in bucket %q", *prefix, cfg.Storage.Bucket)
	}
	log.Printf("found %d video object(s) under %q", len(keys), *prefix)

	imported, skipped := 0, 0
	for _, key := range keys {
		publicURL := storage.PublicURLForKey(key)
		if existingURLs[publicURL] {
			log.Printf("skip (already imported): %s", key)
			skipped++
			continue
		}

		head, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(cfg.Storage.Bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			log.Printf("skip (HeadObject failed for %s): %v", key, err)
			continue
		}

		contentType := aws.ToString(head.ContentType)
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = videoExtMIME[strings.ToLower(filepath.Ext(key))]
		}
		var size int64
		if head.ContentLength != nil {
			size = *head.ContentLength
		}

		base := filepath.Base(key)
		title := strings.TrimSuffix(base, filepath.Ext(base))

		if *dryRun {
			log.Printf("[dry-run] would import: title=%q url=%s size=%d type=%s", title, publicURL, size, contentType)
			imported++
			continue
		}

		if _, err := recordingRepo.Create(ctx, *batchShortID, title, publicURL, size, contentType, uploaderID); err != nil {
			log.Printf("FAILED to import %s: %v", key, err)
			continue
		}
		log.Printf("imported: title=%q url=%s", title, publicURL)
		imported++
	}

	log.Printf("done: %d imported, %d skipped (already present)", imported, skipped)
}
