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

var allowedEventMIME = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// allowedMaterialMIME maps MIME type → file extension for all material uploads.
// The folder key is used to organise files inside the materials bucket.
var allowedMaterialMIME = map[string]struct {
	ext    string
	folder string
}{
	// images
	"image/jpeg": {".jpg", "images"},
	"image/png":  {".png", "images"},
	"image/webp": {".webp", "images"},
	"image/gif":  {".gif", "images"},
	// video
	"video/mp4":        {".mp4", "videos"},
	"video/quicktime":  {".mov", "videos"},
	"video/x-msvideo": {".avi", "videos"},
	"video/webm":       {".webm", "videos"},
	// audio
	"audio/mpeg":  {".mp3", "audios"},
	"audio/wav":   {".wav", "audios"},
	"audio/ogg":   {".ogg", "audios"},
	"audio/mp4":   {".m4a", "audios"},
	// documents
	"application/pdf": {".pdf", "pdfs"},
	"application/msword":                                                        {".doc", "docs"},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   {".docx", "docs"},
	"application/vnd.ms-excel":                                                  {".xls", "sheets"},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         {".xlsx", "sheets"},
	"application/vnd.ms-powerpoint":                                             {".ppt", "slides"},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {".pptx", "slides"},
	// archives / generic
	"application/zip":              {".zip", "files"},
	"application/x-zip-compressed": {".zip", "files"},
	"application/octet-stream":     {".bin", "files"},
}

const maxUploadSize = 10 << 20  // 10 MB
const maxMaterialSize = 500 << 20 // 500 MB

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

	ext, ok := allowedEventMIME[mimeType]
	if !ok {
		// Fall back to original file extension for types DetectContentType misses (e.g. webp)
		orig := strings.ToLower(filepath.Ext(fh.Filename))
		for _, v := range allowedEventMIME {
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
		for k, v := range allowedEventMIME {
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

// UploadMaterial validates and uploads a material file to the materials Supabase bucket.
// Returns the public URL of the uploaded file.
func (s *StorageService) UploadMaterial(fh *multipart.FileHeader) (string, error) {
	if fh.Size > maxMaterialSize {
		return "", fmt.Errorf("file too large: maximum size is 500 MB")
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

	mimeType := strings.TrimSpace(strings.Split(http.DetectContentType(buf), ";")[0])

	meta, ok := allowedMaterialMIME[mimeType]
	if !ok {
		// Fall back to file extension when DetectContentType returns a generic type
		origExt := strings.ToLower(filepath.Ext(fh.Filename))
		for mime, m := range allowedMaterialMIME {
			if m.ext == origExt {
				meta = m
				mimeType = mime
				ok = true
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("unsupported file type: %s", filepath.Ext(fh.Filename))
		}
	}

	filename := fmt.Sprintf("%s/%s%s", meta.folder, randomHex(), meta.ext)
	uploadURL := fmt.Sprintf("%s/storage/v1/object/%s/%s",
		strings.TrimRight(s.cfg.ProjectURL, "/"),
		s.cfg.MaterialBucket,
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
		s.cfg.MaterialBucket,
		filename,
	)
	return publicURL, nil
}

// UploadBlogImage validates and uploads a blog featured image to Supabase Storage.
// Files are stored under the blogs/ prefix. Returns the public URL.
func (s *StorageService) UploadBlogImage(fh *multipart.FileHeader) (string, error) {
	if fh.Size > maxUploadSize {
		return "", fmt.Errorf("file too large: maximum size is 10 MB")
	}

	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 512)
	if _, err := f.Read(buf); err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	if _, err := f.(io.Seeker).Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("cannot seek file: %w", err)
	}

	mimeType := strings.TrimSpace(strings.Split(http.DetectContentType(buf), ";")[0])

	ext, ok := allowedEventMIME[mimeType]
	if !ok {
		orig := strings.ToLower(filepath.Ext(fh.Filename))
		for _, v := range allowedEventMIME {
			if v == orig {
				ext = orig
				ok = true
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("unsupported file type: only JPEG, PNG, WebP and GIF are allowed")
		}
		for k, v := range allowedEventMIME {
			if v == ext {
				mimeType = k
				break
			}
		}
	}

	filename := fmt.Sprintf("blogs/%s%s", randomHex(), ext)
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

// UploadBannerImage validates and uploads a banner thumbnail to Supabase Storage.
// Files are stored under the banners/ prefix. Returns the public URL.
func (s *StorageService) UploadBannerImage(fh *multipart.FileHeader) (string, error) {
	if fh.Size > maxUploadSize {
		return "", fmt.Errorf("file too large: maximum size is 10 MB")
	}

	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 512)
	if _, err := f.Read(buf); err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	if _, err := f.(io.Seeker).Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("cannot seek file: %w", err)
	}

	mimeType := strings.TrimSpace(strings.Split(http.DetectContentType(buf), ";")[0])

	ext, ok := allowedEventMIME[mimeType]
	if !ok {
		orig := strings.ToLower(filepath.Ext(fh.Filename))
		for _, v := range allowedEventMIME {
			if v == orig {
				ext = orig
				ok = true
				break
			}
		}
		if !ok {
			return "", fmt.Errorf("unsupported file type: only JPEG, PNG, WebP and GIF are allowed")
		}
		for k, v := range allowedEventMIME {
			if v == ext {
				mimeType = k
				break
			}
		}
	}

	filename := fmt.Sprintf("banners/%s%s", randomHex(), ext)
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
