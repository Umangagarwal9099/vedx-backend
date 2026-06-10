package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/umangagarwal/vedx-backend/config"
)

var allowedMIME = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

const maxUploadSize = 10 << 20 // 10 MB

type StorageService struct {
	cfg    config.StorageConfig
	client *http.Client
}

func NewStorageService(cfg config.StorageConfig) *StorageService {
	return &StorageService{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// UploadEventImage validates and uploads a multipart file to Supabase Storage.
// Returns the public URL of the uploaded file.
func (s *StorageService) UploadEventImage(fh *multipart.FileHeader) (string, error) {
	if fh.Size > maxUploadSize {
		return "", fmt.Errorf("file too large: maximum size is 10 MB")
	}

	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	// Detect MIME from first 512 bytes, then seek back
	buf := make([]byte, 512)
	if _, err := f.Read(buf); err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	if _, err := f.(io.Seeker).Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("cannot seek file: %w", err)
	}

	mimeType := http.DetectContentType(buf)
	// DetectContentType may return "image/jpeg; charset=..." — normalise it
	mimeType = strings.Split(mimeType, ";")[0]
	mimeType = strings.TrimSpace(mimeType)

	ext, ok := allowedMIME[mimeType]
	if !ok {
		// Fall back to original file extension for types DetectContentType misses (e.g. webp)
		orig := strings.ToLower(filepath.Ext(fh.Filename))
		for _, v := range allowedMIME {
			if v == orig {
				ext = orig
				ok = true
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("unsupported file type: only JPEG, PNG, WebP and GIF are allowed")
		}
		// Use original MIME for the upload
		for k, v := range allowedMIME {
			if v == ext {
				mimeType = k
				break
			}
		}
	}

	filename := fmt.Sprintf("events/%s%s", randomHex(), ext)
	uploadURL := fmt.Sprintf("%s/storage/v1/object/%s/%s",
		strings.TrimRight(s.cfg.ProjectURL, "/"),
		s.cfg.Bucket,
		filename,
	)

	req, err := http.NewRequest(http.MethodPost, uploadURL, f)
	if err != nil {
		return "", fmt.Errorf("build upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.ServiceRoleKey)
	req.Header.Set("Content-Type", mimeType)
	req.Header.Set("x-upsert", "true")
	req.ContentLength = fh.Size

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload to storage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("storage upload failed (%d): %s", resp.StatusCode, string(body))
	}

	publicURL := fmt.Sprintf("%s/storage/v1/object/public/%s/%s",
		strings.TrimRight(s.cfg.ProjectURL, "/"),
		s.cfg.Bucket,
		filename,
	)
	return publicURL, nil
}

func randomHex() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("storage: crypto/rand unavailable")
	}
	return hex.EncodeToString(b)
}
