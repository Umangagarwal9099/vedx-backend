package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/umangagarwal/vedx-backend/config"
)

var allowedEventMIME = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// allowedMaterialMIME maps MIME type → file extension for all material uploads.
// The folder key is used to organise files inside the materials/ prefix.
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
	"video/mp4":       {".mp4", "videos"},
	"video/quicktime": {".mov", "videos"},
	"video/x-msvideo": {".avi", "videos"},
	"video/webm":      {".webm", "videos"},
	// audio
	"audio/mpeg": {".mp3", "audios"},
	"audio/wav":  {".wav", "audios"},
	"audio/ogg":  {".ogg", "audios"},
	"audio/mp4":  {".m4a", "audios"},
	// documents
	"application/pdf":            {".pdf", "pdfs"},
	"application/msword":         {".doc", "docs"},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {".docx", "docs"},
	"application/vnd.ms-excel":                                          {".xls", "sheets"},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       {".xlsx", "sheets"},
	"application/vnd.ms-powerpoint":                                     {".ppt", "slides"},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {".pptx", "slides"},
	// archives / generic
	"application/zip":              {".zip", "files"},
	"application/x-zip-compressed": {".zip", "files"},
	"application/octet-stream":     {".bin", "files"},
}

const maxUploadSize = 10 << 20    // 10 MB
const maxMaterialSize = 500 << 20 // 500 MB

// StorageService uploads files to Cloudflare R2. All files live in one
// bucket, under "events/" and "materials/" prefixes that mirror the two
// buckets this app used on Supabase Storage, so existing public URLs only
// need their host swapped, not their path.
type StorageService struct {
	cfg    config.StorageConfig
	client *s3.Client
}

func NewStorageService(cfg config.StorageConfig) *StorageService {
	client := s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
	})
	return &StorageService{cfg: cfg, client: client}
}

// detectImageType sniffs an image's MIME type from its first 512 bytes,
// falling back to the original filename extension when DetectContentType
// can't identify it (e.g. webp). Leaves f seeked back to the start.
func detectImageType(f multipart.File, filename string) (ext, mimeType string, err error) {
	buf := make([]byte, 512)
	if _, err := f.Read(buf); err != nil {
		return "", "", fmt.Errorf("cannot read file: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", "", fmt.Errorf("cannot seek file: %w", err)
	}
	mimeType = strings.TrimSpace(strings.Split(http.DetectContentType(buf), ";")[0])

	if e, ok := allowedEventMIME[mimeType]; ok {
		return e, mimeType, nil
	}
	orig := strings.ToLower(filepath.Ext(filename))
	for mime, e := range allowedEventMIME {
		if e == orig {
			return e, mime, nil
		}
	}
	return "", "", fmt.Errorf("unsupported file type: only JPEG, PNG, WebP and GIF are allowed")
}

// detectMaterialType sniffs a material file's MIME type, folder and
// extension the same way detectImageType does for images.
func detectMaterialType(f multipart.File, filename string) (ext, folder, mimeType string, err error) {
	buf := make([]byte, 512)
	if _, err := f.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("cannot read file: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", "", "", fmt.Errorf("cannot seek file: %w", err)
	}
	mimeType = strings.TrimSpace(strings.Split(http.DetectContentType(buf), ";")[0])

	if m, ok := allowedMaterialMIME[mimeType]; ok {
		return m.ext, m.folder, mimeType, nil
	}
	origExt := strings.ToLower(filepath.Ext(filename))
	for mime, m := range allowedMaterialMIME {
		if m.ext == origExt {
			return m.ext, m.folder, mime, nil
		}
	}
	return "", "", "", fmt.Errorf("unsupported file type: %s", filepath.Ext(filename))
}

// put uploads fh to the given key under mimeType and returns its public URL.
func (s *StorageService) put(fh *multipart.FileHeader, key, mimeType string) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	_, err = s.client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.Bucket),
		Key:         aws.String(key),
		Body:        f,
		ContentType: aws.String(mimeType),
	})
	if err != nil {
		return "", fmt.Errorf("upload to storage: %w", err)
	}
	return strings.TrimRight(s.cfg.PublicURL, "/") + "/" + key, nil
}

// uploadImage validates fh as an image (max 10 MB) and uploads it under
// "events/<folder>/". Shared by every image-upload entry point below.
func (s *StorageService) uploadImage(fh *multipart.FileHeader, folder string) (string, error) {
	if fh.Size > maxUploadSize {
		return "", fmt.Errorf("file too large: maximum size is 10 MB")
	}

	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	ext, mimeType, err := detectImageType(f, fh.Filename)
	f.Close()
	if err != nil {
		return "", err
	}

	key := fmt.Sprintf("events/%s/%s%s", folder, randomHex(), ext)
	return s.put(fh, key, mimeType)
}

// uploadMaterialFile validates fh against allowedMaterialMIME (max 500 MB)
// and uploads it under "materials/<folder>/". Shared by every material-file
// upload entry point below.
func (s *StorageService) uploadMaterialFile(fh *multipart.FileHeader, folder string) (string, error) {
	if fh.Size > maxMaterialSize {
		return "", fmt.Errorf("file too large: maximum size is 500 MB")
	}

	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	ext, detectedFolder, mimeType, err := detectMaterialType(f, fh.Filename)
	f.Close()
	if err != nil {
		return "", err
	}
	if folder == "" {
		folder = detectedFolder
	}

	key := fmt.Sprintf("materials/%s/%s%s", folder, randomHex(), ext)
	return s.put(fh, key, mimeType)
}

// UploadEventImage validates and uploads an event image. Returns its public URL.
func (s *StorageService) UploadEventImage(fh *multipart.FileHeader) (string, error) {
	return s.uploadImage(fh, "events")
}

// UploadBlogImage validates and uploads a blog featured image. Returns its public URL.
func (s *StorageService) UploadBlogImage(fh *multipart.FileHeader) (string, error) {
	return s.uploadImage(fh, "blogs")
}

// UploadBannerImage validates and uploads a banner thumbnail. Returns its public URL.
func (s *StorageService) UploadBannerImage(fh *multipart.FileHeader) (string, error) {
	return s.uploadImage(fh, "banners")
}

// UploadAssessmentThumbnail validates and uploads an assessment thumbnail. Returns its public URL.
func (s *StorageService) UploadAssessmentThumbnail(fh *multipart.FileHeader) (string, error) {
	return s.uploadImage(fh, "assessments/thumbnails")
}

// UploadMaterial validates and uploads a generic material file, filed under
// its MIME-derived folder (images/videos/audios/pdfs/docs/sheets/slides/files).
// Returns its public URL.
func (s *StorageService) UploadMaterial(fh *multipart.FileHeader) (string, error) {
	return s.uploadMaterialFile(fh, "")
}

// UploadAssessmentFile validates and uploads an assessment file (PDF, doc, etc.). Returns its public URL.
func (s *StorageService) UploadAssessmentFile(fh *multipart.FileHeader) (string, error) {
	return s.uploadMaterialFile(fh, "assessments/files")
}

// UploadAssignmentFile validates and uploads a student's assignment submission file. Returns its public URL.
func (s *StorageService) UploadAssignmentFile(fh *multipart.FileHeader) (string, error) {
	return s.uploadMaterialFile(fh, "assignments/submissions")
}

// UploadResourceFile validates and uploads a learning resource file. Returns its public URL.
func (s *StorageService) UploadResourceFile(fh *multipart.FileHeader) (string, error) {
	return s.uploadMaterialFile(fh, "resources/files")
}

// UploadProjectFile validates and uploads a project submission file. Returns its public URL.
func (s *StorageService) UploadProjectFile(fh *multipart.FileHeader) (string, error) {
	return s.uploadMaterialFile(fh, "projects/submissions")
}

func randomHex() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("storage: crypto/rand unavailable")
	}
	return hex.EncodeToString(b)
}
