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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
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

const maxUploadSize = 10 << 20            // 10 MB
const maxMaterialSize = 500 << 20         // 500 MB
const maxResumeSize = 5 << 20             // 5 MB — resumes are short documents, not course material
const maxLeaveCertificateSize = 5 << 20   // 5 MB — a certificate is a short document/photo, not course material

// maxRecordingUploadSize is R2's (and S3's) hard limit for a single PUT —
// above this an object requires a multipart upload instead, which presigned
// batch recording uploads don't support yet.
const maxRecordingUploadSize = 5 << 30 // 5 GB

// presignedUploadExpiry is how long a presigned batch-recording upload URL
// stays valid — generous enough to cover a slow multi-GB upload over a weak
// connection without the link expiring mid-transfer.
const presignedUploadExpiry = 4 * time.Hour

// allowedRecordingMIME maps MIME type → file extension for videos uploaded
// directly to a batch (POST /batches/{short_id}/recordings/presign). Mirrors
// videoExtMIME in cmd/importbatchrecordings, which does the same match in
// reverse for objects that bypassed this endpoint entirely.
var allowedRecordingMIME = map[string]string{
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
	"video/x-msvideo": ".avi",
	"video/webm":      ".webm",
}

// allowedResumeMIME is deliberately narrower than allowedMaterialMIME — a
// resume is PDF or Word, never a video/audio/archive/image.
var allowedResumeMIME = map[string]string{
	"application/pdf":            ".pdf",
	"application/msword":         ".doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
}

// allowedLeaveCertificateMIME allows PDF alongside JPEG/PNG — a medical
// certificate is as often a phone photo of a paper document as a scan.
var allowedLeaveCertificateMIME = map[string]string{
	"application/pdf": ".pdf",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
}

// StorageService uploads files to Cloudflare R2. All files live in one
// bucket, under "events/" and "materials/" prefixes that mirror the two
// buckets this app used on Supabase Storage, so existing public URLs only
// need their host swapped, not their path.
type StorageService struct {
	cfg      config.StorageConfig
	client   *s3.Client
	uploader *manager.Uploader
	presign  *s3.PresignClient
}

func NewStorageService(cfg config.StorageConfig) *StorageService {
	client := s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
	})
	return &StorageService{cfg: cfg, client: client, uploader: manager.NewUploader(client), presign: s3.NewPresignClient(client)}
}

// PresignRecordingUpload returns a presigned PUT URL an admin's browser can
// upload a batch recording video directly to R2 with — the file's bytes
// never pass through this server, so there's no request-size or timeout
// limit from our own stack, only R2's own 5 GB single-PUT ceiling.
// filename is used only to fall back to extension-sniffing when contentType
// isn't one of the allowed video MIME types outright (some browsers report
// generic types for less common video containers).
func (s *StorageService) PresignRecordingUpload(ctx context.Context, batchShortID, filename, contentType string, fileSize int64) (uploadURL, key string, err error) {
	ext, ok := allowedRecordingMIME[contentType]
	if !ok {
		origExt := strings.ToLower(filepath.Ext(filename))
		for mime, e := range allowedRecordingMIME {
			if e == origExt {
				ext, contentType, ok = e, mime, true
				break
			}
		}
	}
	if !ok {
		return "", "", fmt.Errorf("unsupported file type — only MP4, MOV, AVI, and WebM recordings are accepted")
	}
	if fileSize <= 0 || fileSize > maxRecordingUploadSize {
		return "", "", fmt.Errorf("file size must be between 1 byte and 5 GB")
	}

	key = fmt.Sprintf("recordings/batch/%s/%s%s", batchShortID, randomHex(), ext)

	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(presignedUploadExpiry))
	if err != nil {
		return "", "", fmt.Errorf("presign recording upload: %w", err)
	}
	return req.URL, key, nil
}

// HeadRecordingObject returns the actual size and content type of an
// object already sitting in R2 — used to confirm a presigned upload really
// landed before trusting it enough to write a database row, rather than
// taking the browser's word for it.
func (s *StorageService) HeadRecordingObject(ctx context.Context, key string) (size int64, contentType string, err error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, "", fmt.Errorf("head recording object: %w", err)
	}
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return size, aws.ToString(out.ContentType), nil
}

// UploadRecording streams a session recording into "recordings/<key>".
// Unlike the multipart-form uploads above, this takes a plain io.Reader —
// recordings arrive as a direct byte stream from Zoom, not a browser upload
// — and goes through the multipart upload manager since a video can be many
// gigabytes with its total size not known upfront.
func (s *StorageService) UploadRecording(ctx context.Context, body io.Reader, key, contentType string) (string, error) {
	fullKey := "recordings/" + key
	if _, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.Bucket),
		Key:         aws.String(fullKey),
		Body:        body,
		ContentType: aws.String(contentType),
	}); err != nil {
		return "", fmt.Errorf("upload recording to storage: %w", err)
	}
	return strings.TrimRight(s.cfg.PublicURL, "/") + "/" + fullKey, nil
}

// PublicURLForKey returns the public R2 URL for an object key that's
// already been uploaded (e.g. via a presigned PUT).
func (s *StorageService) PublicURLForKey(key string) string {
	return strings.TrimRight(s.cfg.PublicURL, "/") + "/" + key
}

// RecordingObjectStream is one GetObject read of a recording, proxied
// through this server rather than handed to the browser as a direct or
// presigned R2 URL — R2 itself is never reachable from the browser on this
// path. ContentRange/Partial are only set when a Range header was forwarded
// and R2 honored it, so the controller can mirror a 206 back to the client
// the same way R2 would have — that's what lets browser seeking/scrubbing
// keep working.
type RecordingObjectStream struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
	ContentRange  string
	Partial       bool
}

// OpenRecordingStream opens a stored recording for proxied reading,
// forwarding rangeHeader (the client's Range request header, if any)
// straight through to R2. Callers must Close() the returned Body.
func (s *StorageService) OpenRecordingStream(ctx context.Context, storedURL, rangeHeader string) (*RecordingObjectStream, error) {
	key := s.recordingKeyFromURL(storedURL)
	if key == "" {
		return nil, fmt.Errorf("cannot determine object key from recording url %q", storedURL)
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(key),
	}
	if rangeHeader != "" {
		input.Range = aws.String(rangeHeader)
	}

	out, err := s.client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("get recording object: %w", err)
	}

	stream := &RecordingObjectStream{
		Body:        out.Body,
		ContentType: aws.ToString(out.ContentType),
	}
	if out.ContentLength != nil {
		stream.ContentLength = *out.ContentLength
	}
	if out.ContentRange != nil {
		stream.ContentRange = *out.ContentRange
		stream.Partial = true
	}
	return stream, nil
}

// recordingKeyFromURL strips the configured public URL prefix off a stored
// recording URL to recover the bare R2 object key. Passing an already-bare
// key through unchanged keeps this safe to call defensively.
func (s *StorageService) recordingKeyFromURL(storedURL string) string {
	prefix := strings.TrimRight(s.cfg.PublicURL, "/") + "/"
	if strings.HasPrefix(storedURL, prefix) {
		return strings.TrimPrefix(storedURL, prefix)
	}
	if !strings.Contains(storedURL, "://") {
		return storedURL
	}
	return ""
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

// UploadCourseThumbnail validates and uploads a course thumbnail. Returns its
// public URL — callers must store this URL in the course's thumbnail field,
// never the raw file/base64 data itself (a raw data: URI there previously
// bloated every course row to megabytes and made GET /courses catastrophically
// slow, since every listing call embeds every course's thumbnail inline).
func (s *StorageService) UploadCourseThumbnail(fh *multipart.FileHeader) (string, error) {
	return s.uploadImage(fh, "courses/thumbnails")
}

// UploadAttendanceSelfie validates and uploads a check-in/check-out selfie.
// Returns its public URL.
func (s *StorageService) UploadAttendanceSelfie(fh *multipart.FileHeader) (string, error) {
	return s.uploadImage(fh, "attendance/selfies")
}

// UploadResumeFile validates fh as a PDF/DOC/DOCX (max 5 MB) and uploads it
// under "resumes/". Returns its public URL.
func (s *StorageService) UploadResumeFile(fh *multipart.FileHeader) (string, error) {
	if fh.Size > maxResumeSize {
		return "", fmt.Errorf("file too large: maximum size is 5 MB")
	}

	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	buf := make([]byte, 512)
	if _, err := f.Read(buf); err != nil {
		f.Close()
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	f.Close()
	mimeType := strings.TrimSpace(strings.Split(http.DetectContentType(buf), ";")[0])

	ext, ok := allowedResumeMIME[mimeType]
	if !ok {
		origExt := strings.ToLower(filepath.Ext(fh.Filename))
		for mime, e := range allowedResumeMIME {
			if e == origExt {
				ext, mimeType, ok = e, mime, true
				break
			}
		}
	}
	if !ok {
		return "", fmt.Errorf("unsupported file type — only PDF, DOC, and DOCX resumes are accepted")
	}

	key := fmt.Sprintf("resumes/%s%s", randomHex(), ext)
	return s.put(fh, key, mimeType)
}

// UploadLeaveCertificate validates fh as a PDF/JPEG/PNG (max 5 MB) and
// uploads it under "leave-certificates/". Returns its public URL.
func (s *StorageService) UploadLeaveCertificate(fh *multipart.FileHeader) (string, error) {
	if fh.Size > maxLeaveCertificateSize {
		return "", fmt.Errorf("file too large: maximum size is 5 MB")
	}

	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	buf := make([]byte, 512)
	if _, err := f.Read(buf); err != nil {
		f.Close()
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	f.Close()
	mimeType := strings.TrimSpace(strings.Split(http.DetectContentType(buf), ";")[0])

	ext, ok := allowedLeaveCertificateMIME[mimeType]
	if !ok {
		origExt := strings.ToLower(filepath.Ext(fh.Filename))
		for mime, e := range allowedLeaveCertificateMIME {
			if e == origExt {
				ext, mimeType, ok = e, mime, true
				break
			}
		}
	}
	if !ok {
		return "", fmt.Errorf("unsupported file type — only PDF, JPEG, and PNG certificates are accepted")
	}

	key := fmt.Sprintf("leave-certificates/%s%s", randomHex(), ext)
	return s.put(fh, key, mimeType)
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
