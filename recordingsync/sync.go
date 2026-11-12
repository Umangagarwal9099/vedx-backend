// Package recordingsync moves Zoom cloud recordings into the LMS reliably.
//
// Flow (every step idempotent, every outcome recorded on the job row):
//
//	webhook / reconciliation ─► zoom_recording_jobs (pending, keyed by instance UUID)
//	worker claims job (lease) ─► fetch fresh metadata from Zoom (OAuth, never the
//	   expiring webhook download_token) ─► match to LMS session ─► archive the
//	   untouched original to R2 ─► watermark from the archived copy ─► upload ─►
//	   attach to session (compare-and-set) or add as extra segment ─► completed
//	any failure ─► retry with backoff; exhausted ─► failed, re-queued daily by
//	   reconciliation while Zoom still holds the recording.
package recordingsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

// MinPartMinutes: extra recorded segments of a session shorter than this
// are host-rejoin noise (0–2 min clips seen in the audit) and are not
// published as separate videos. Longer ones are real class content — a
// class whose meeting was restarted mid-way (e.g. 16 + 35 min).
const MinPartMinutes = 3

// matchSlack is how far outside a session's scheduled slot a recording may
// start and still belong to it (besides being on the same calendar day).
const matchSlack = 3 * time.Hour

type Service struct {
	zoom      *service.ZoomService
	storage   *service.StorageService
	jobs      *repository.ZoomRecordingJobRepository
	batchRecs *repository.BatchRecordingRepository
	loc       *time.Location
}

// processLock allows one encode at a time per process (shared by the
// webhook nudge, the scheduler and the HTTP trigger) — each encode stages
// ~2x the recording in temp space, which on Cloud Run is RAM.
var processLock sync.Mutex

func New(zoom *service.ZoomService, storage *service.StorageService, jobs *repository.ZoomRecordingJobRepository, batchRecs *repository.BatchRecordingRepository, timezone string) *Service {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	return &Service{zoom: zoom, storage: storage, jobs: jobs, batchRecs: batchRecs, loc: loc}
}

// ---------------------------------------------------------------------------
// Enqueue

// EnqueueInstance records a Zoom instance as a job (no-op if already known).
func (s *Service) EnqueueInstance(ctx context.Context, inst *service.ZoomRecordingInstance, source string) (bool, error) {
	var start *time.Time
	if t := inst.StartTimeParsed(); !t.IsZero() {
		start = &t
	}
	type fileMeta struct {
		ID            string `json:"id"`
		FileType      string `json:"file_type"`
		Extension     string `json:"file_extension,omitempty"`
		RecordingType string `json:"recording_type"`
		Size          int64  `json:"file_size"`
		Status        string `json:"status,omitempty"`
		Start         string `json:"recording_start,omitempty"`
		End           string `json:"recording_end,omitempty"`
	}
	files := make([]fileMeta, 0, len(inst.RecordingFiles))
	var total int64
	for _, f := range inst.RecordingFiles {
		files = append(files, fileMeta{f.ID, f.FileType, f.FileExtension, f.RecordingType, f.FileSize, f.Status, f.RecordingStart, f.RecordingEnd})
		total += f.FileSize
	}
	if inst.TotalSize > total {
		total = inst.TotalSize
	}
	return s.jobs.Upsert(ctx, repository.RecordingJobUpsert{
		UUID: inst.UUID, MeetingID: inst.ID, Topic: inst.Topic, HostID: inst.HostID, HostEmail: inst.HostEmail,
		RecordingStart: start, DurationMin: inst.Duration, TotalSize: total, Files: files, Source: source,
	})
}

// Reconcile lists every recording Zoom holds in [from, to] and enqueues any
// instance the LMS has not seen — the safety net for lost/failed webhooks.
// Walks every account user when the app has user:read:list_users:admin,
// otherwise the app owner ("me"), where all LMS-created meetings live.
func (s *Service) Reconcile(ctx context.Context, from, to time.Time, source string) (models.RecordingSyncSummary, error) {
	sum := models.RecordingSyncSummary{Outcomes: map[string]int{}}
	insts, err := s.zoom.ListAccountRecordings(from, to)
	if err != nil {
		log.Printf("recording sync: account-wide listing unavailable (%v), falling back to app owner", err)
		insts, err = s.zoom.ListRecordings("me", from, to, false)
		if err != nil {
			return sum, fmt.Errorf("reconcile: list zoom recordings: %w", err)
		}
	}
	sum.Discovered = len(insts)
	for i := range insts {
		created, err := s.EnqueueInstance(ctx, &insts[i], source)
		if err != nil {
			sum.Errors = append(sum.Errors, err.Error())
			continue
		}
		if created {
			sum.Enqueued++
		}
	}
	if n, err := s.jobs.RequeueStale(ctx); err != nil {
		sum.Errors = append(sum.Errors, "requeue stale: "+err.Error())
	} else {
		sum.Requeued = n
	}
	log.Printf("recording sync: reconcile %s..%s discovered=%d enqueued=%d requeued=%d",
		from.Format("2006-01-02"), to.Format("2006-01-02"), sum.Discovered, sum.Enqueued, sum.Requeued)
	return sum, nil
}

// ---------------------------------------------------------------------------
// Process

// Options control one processing run.
type Options struct {
	MaxJobs     int           // 0 = no limit
	Budget      time.Duration // stop claiming new jobs after this long; 0 = no limit
	ArchiveOnly bool          // archive originals to R2, defer watermark/attach (no ffmpeg needed)
	Since       *time.Time    // only recordings that started on/after this (backfill prioritisation)
}

// ProcessDue works through due jobs one at a time until the queue is empty
// or the job/time budget is used up.
func (s *Service) ProcessDue(ctx context.Context, opt Options) models.RecordingSyncSummary {
	processLock.Lock()
	defer processLock.Unlock()

	sum := models.RecordingSyncSummary{Outcomes: map[string]int{}}
	started := time.Now()
	seen := map[string]bool{}
	for {
		if ctx.Err() != nil || (opt.MaxJobs > 0 && sum.Processed >= opt.MaxJobs) || (opt.Budget > 0 && time.Since(started) > opt.Budget) {
			return sum
		}
		job, err := s.jobs.ClaimNext(ctx, opt.Since)
		if err != nil {
			sum.Errors = append(sum.Errors, "claim: "+err.Error())
			return sum
		}
		if job == nil {
			return sum
		}
		if seen[job.ID] {
			// A job deferred earlier in this run came back round — push it
			// out so the rest of the queue gets its turn.
			_ = s.jobs.Defer(ctx, job.ID, job.LastError, 10*time.Minute)
			continue
		}
		seen[job.ID] = true
		outcome := s.processJob(ctx, job, opt)
		sum.Processed++
		sum.Outcomes[outcome]++
	}
}

// processJob runs one attempt and records the outcome on the job row.
func (s *Service) processJob(ctx context.Context, job *models.ZoomRecordingJob, opt Options) string {
	tag := fmt.Sprintf("job %s meeting %d uuid %s", job.ID, job.ZoomMeetingID, job.ZoomMeetingUUID)
	if job.Attempts > job.MaxAttempts {
		// Reclaimed after its lease expired on the final attempt — the worker
		// keeps dying on this one (e.g. out of memory on a huge file).
		_, _ = s.jobs.Retry(ctx, job.ID, fmt.Errorf("gave up after %d attempts: %s", job.MaxAttempts, job.LastError), 6*time.Hour)
		log.Printf("recording sync: %s FAILED permanently: %s", tag, job.LastError)
		return models.RecordingJobFailed
	}
	outcome, err := s.attempt(ctx, job, opt)
	if err == nil {
		log.Printf("recording sync: %s → %s", tag, outcome)
		return outcome
	}
	var d deferErr
	if errors.As(err, &d) {
		_ = s.jobs.Defer(ctx, job.ID, d.reason, d.delay)
		log.Printf("recording sync: %s deferred: %s", tag, d.reason)
		return "deferred"
	}
	delay := backoff(job.Attempts)
	status, rerr := s.jobs.Retry(ctx, job.ID, err, delay)
	if rerr != nil {
		log.Printf("recording sync: %s FAILED (%v) and could not record retry: %v", tag, err, rerr)
		return "error"
	}
	log.Printf("recording sync: %s attempt %d/%d FAILED: %v → %s (next try in %s)", tag, job.Attempts, job.MaxAttempts, err, status, delay)
	return "retry"
}

type deferErr struct {
	reason string
	delay  time.Duration
}

func (d deferErr) Error() string { return d.reason }

// backoff: 5m, 10m, 20m, 40m, 80m, 160m, then 6h.
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(5*math.Pow(2, float64(attempt-1))) * time.Minute
	if d > 6*time.Hour {
		d = 6 * time.Hour
	}
	return d
}

func (s *Service) attempt(ctx context.Context, job *models.ZoomRecordingJob, opt Options) (string, error) {
	// 1. Fresh metadata + download URLs straight from Zoom.
	inst, err := s.zoom.GetInstanceRecordings(job.ZoomMeetingUUID)
	if err != nil {
		if service.IsZoomNotFound(err) {
			if job.RawKey != "" { // Zoom deleted it, but we archived the original in time
				inst = nil
			} else {
				return models.RecordingJobGone, s.jobs.Finish(ctx, job.ID, models.RecordingJobGone, job.SessionID, "recording no longer exists in Zoom (deleted or retention policy)")
			}
		} else {
			return "", err
		}
	}
	var file *service.ZoomRecordingFile
	startUTC := time.Time{}
	if job.RecordingStart != nil {
		startUTC = *job.RecordingStart
	}
	if inst != nil {
		if inst.Duration > job.DurationMin {
			job.DurationMin = inst.Duration
		}
		if t := inst.StartTimeParsed(); !t.IsZero() {
			startUTC = t
		}
		if file = inst.PrimaryVideoFile(); file == nil {
			for _, f := range inst.RecordingFiles {
				if f.FileType == "MP4" && f.Status == "processing" {
					return "", fmt.Errorf("zoom is still processing the video file")
				}
			}
			return models.RecordingJobSkipped, s.jobs.Finish(ctx, job.ID, models.RecordingJobSkipped, "", "instance has no video file (audio/transcript/chat only)")
		}
		if job.PrimaryFileID == "" {
			job.PrimaryFileID = file.ID
			if err := s.jobs.SetPrimaryFile(ctx, job.ID, file.ID); err != nil {
				return "", err
			}
		}
	}
	if job.PrimaryFileID == "" {
		return "", fmt.Errorf("no primary file id")
	}

	// 2. Which session does this belong to?
	sess, reason, err := s.matchSession(ctx, job.ZoomMeetingID, startUTC)
	if err != nil {
		return "", err
	}
	if sess == nil {
		return models.RecordingJobUnmatched, s.jobs.Finish(ctx, job.ID, models.RecordingJobUnmatched, "", reason)
	}

	// 3. Primary recording, extra segment, or noise?
	role, demote, err := s.decideRole(ctx, job, sess)
	if err != nil {
		return "", err
	}
	if role == "already" {
		return "already_synced", s.jobs.Complete(ctx, job.ID, sess.ID, "primary", sess.RecordingURL, "")
	}
	if role == "" {
		return models.RecordingJobSkipped, s.jobs.Finish(ctx, job.ID, models.RecordingJobSkipped, sess.ID,
			fmt.Sprintf("extra %d-min segment shorter than %d min (host rejoin) — session already has its recording", job.DurationMin, MinPartMinutes))
	}

	// 4. Archive the untouched original (once).
	if job.RawKey == "" {
		if file == nil {
			return "", fmt.Errorf("cannot archive: zoom file unavailable")
		}
		rawKey := fmt.Sprintf("raw/%d/%s.%s", job.ZoomMeetingID, file.ID, fileExt(file))
		body, ct, err := s.zoom.DownloadRecordingWithOAuth(file.DownloadURL)
		if err != nil {
			return "", fmt.Errorf("download from zoom: %w", err)
		}
		_, err = s.storage.UploadRecording(ctx, body, rawKey, ct)
		body.Close()
		if err != nil {
			return "", fmt.Errorf("archive original to storage: %w", err)
		}
		if err := s.jobs.SetArchived(ctx, job.ID, file.ID, rawKey); err != nil {
			return "", err
		}
		job.RawKey = rawKey
	}

	if opt.ArchiveOnly {
		return "", deferErr{reason: "original archived; awaiting watermark worker", delay: 10 * time.Minute}
	}
	if err := service.WatermarkAvailable(); err != nil {
		return "", deferErr{reason: "original archived; watermark unavailable on this worker: " + err.Error(), delay: 30 * time.Minute}
	}

	// 5. Watermark from the archived copy and upload the published version.
	// Deterministic key: a retry overwrites the same object, never piles up copies.
	pubKey := fmt.Sprintf("%d/%s.mp4", job.ZoomMeetingID, job.PrimaryFileID)
	raw, err := s.storage.OpenRecording(ctx, job.RawKey)
	if err != nil {
		return "", err
	}
	wm, err := service.WatermarkRecording(ctx, raw, service.DefaultWatermarkText, startUTC.In(s.loc).Format("2 Jan 2006, 3:04 PM"))
	raw.Close()
	if err != nil {
		return "", fmt.Errorf("watermark: %w", err)
	}
	url, err := s.storage.UploadRecording(ctx, wm, pubKey, "video/mp4")
	wm.Close()
	if err != nil {
		return "", fmt.Errorf("upload watermarked recording: %w", err)
	}

	// 6. Attach.
	switch role {
	case "primary":
		ok, err := s.jobs.SwapSessionRecording(ctx, sess.ID, sess.RecordingURL, url)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("session %s recording changed while processing — will re-evaluate", sess.ShortID)
		}
		if demote != nil {
			brID := ""
			if demote.DurationMin >= MinPartMinutes {
				if brID, err = s.addPart(ctx, sess, demote, sess.RecordingURL); err != nil {
					log.Printf("recording sync: could not re-add replaced recording of session %s as a segment: %v", sess.ShortID, err)
				}
			}
			_ = s.jobs.DemoteToPart(ctx, demote.ID, brID)
		}
		return "attached_primary", s.jobs.Complete(ctx, job.ID, sess.ID, "primary", url, "")
	default: // part
		brID, err := s.addPart(ctx, sess, job, url)
		if err != nil {
			return "", err
		}
		return "attached_segment", s.jobs.Complete(ctx, job.ID, sess.ID, "part", url, brID)
	}
}

// matchSession picks the session a recording belongs to: a live session on
// the same Zoom meeting ID whose scheduled date matches the recording's
// local date, or whose slot is within matchSlack of it. Closest start wins.
// A recording on the right meeting ID but a different day (e.g. the host
// testing the room the evening before) is NOT attached — it is reported as
// unmatched for an admin to review instead of landing on the wrong class.
func (s *Service) matchSession(ctx context.Context, meetingID int64, startUTC time.Time) (*repository.RecordingSessionCandidate, string, error) {
	cands, err := s.jobs.SessionsForMeeting(ctx, meetingID)
	if err != nil {
		return nil, "", err
	}
	if len(cands) == 0 {
		return nil, fmt.Sprintf("no LMS session uses Zoom meeting %d", meetingID), nil
	}
	if best := pickSession(cands, startUTC, s.loc); best != nil {
		return best, "", nil
	}
	return nil, fmt.Sprintf("recorded %s, but no session on meeting %d is scheduled that day (sessions: %s) — likely a test/rejoin outside class time",
		startUTC.In(s.loc).Format("2006-01-02 15:04"), meetingID, describe(cands)), nil
}

// pickSession is the pure matching rule behind matchSession.
func pickSession(cands []repository.RecordingSessionCandidate, startUTC time.Time, loc *time.Location) *repository.RecordingSessionCandidate {
	if startUTC.IsZero() {
		if len(cands) == 1 {
			return &cands[0]
		}
		return nil
	}
	local := startUTC.In(loc)
	var best *repository.RecordingSessionCandidate
	bestDist := time.Duration(math.MaxInt64)
	for i := range cands {
		c := &cands[i]
		st, err1 := time.ParseInLocation("2006-01-02 15:04", c.SessionDate+" "+c.StartTime, loc)
		en, err2 := time.ParseInLocation("2006-01-02 15:04", c.SessionDate+" "+c.EndTime, loc)
		if err1 != nil || err2 != nil {
			continue
		}
		sameDay := c.SessionDate == local.Format("2006-01-02")
		inSlot := !local.Before(st.Add(-matchSlack)) && !local.After(en.Add(matchSlack))
		if !sameDay && !inSlot {
			continue
		}
		d := local.Sub(st)
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// decideRole returns "already" (this exact file is already the session's
// recording), "primary" (attach to the session — optionally demoting a
// shorter recording that currently holds that spot), "part" (publish as an
// extra segment), or "" (skip as noise).
func (s *Service) decideRole(ctx context.Context, job *models.ZoomRecordingJob, sess *repository.RecordingSessionCandidate) (string, *models.ZoomRecordingJob, error) {
	if sess.RecordingURL == "" {
		return "primary", nil, nil
	}
	if strings.Contains(sess.RecordingURL, job.PrimaryFileID) {
		return "already", nil, nil
	}
	siblings, err := s.jobs.FindByMeetingID(ctx, job.ZoomMeetingID)
	if err != nil {
		return "", nil, err
	}
	for i := range siblings {
		sib := &siblings[i]
		if sib.ID == job.ID || !holdsAttached(sib, sess.RecordingURL) {
			continue
		}
		// sib is the recording currently attached. Replace it only if this one is longer.
		if job.DurationMin > sib.DurationMin {
			return "primary", sib, nil
		}
		break
	}
	// Attached recording unknown to the ledger (pre-ledger upload) or longer: keep it.
	if job.DurationMin >= MinPartMinutes {
		return "part", nil, nil
	}
	return "", nil, nil
}

// addPart publishes an extra segment of a session as a batch recording,
// deduplicated by URL so a retry never creates a second row.
func (s *Service) addPart(ctx context.Context, sess *repository.RecordingSessionCandidate, job *models.ZoomRecordingJob, url string) (string, error) {
	if id, err := s.jobs.FindBatchRecordingByURL(ctx, url); err != nil || id != "" {
		return id, err
	}
	label := ""
	if job.RecordingStart != nil {
		label = job.RecordingStart.In(s.loc).Format("2 Jan, 3:04 PM") + ", "
	}
	title := fmt.Sprintf("%s — additional segment (%s%d min)", strings.TrimSpace(sess.Name), label, job.DurationMin)
	if r := []rune(title); len(r) > 250 {
		title = string(r[:249]) + "…"
	}
	rec, err := s.batchRecs.Create(ctx, sess.BatchShortID, title, url, job.TotalSize, "video/mp4", sess.MentorID)
	if err != nil {
		return "", fmt.Errorf("add extra segment to batch: %w", err)
	}
	return rec.ID, nil
}

// holdsAttached reports whether url (a session's current recording) was
// produced from one of sib's Zoom files. Checks the file list captured at
// enqueue time too, so it works before sib has been processed itself.
func holdsAttached(sib *models.ZoomRecordingJob, url string) bool {
	if sib.PrimaryFileID != "" && strings.Contains(url, sib.PrimaryFileID) {
		return true
	}
	var files []struct {
		ID       string `json:"id"`
		FileType string `json:"file_type"`
	}
	if err := json.Unmarshal(sib.Files, &files); err != nil {
		return false
	}
	for _, f := range files {
		if f.ID != "" && f.FileType == "MP4" && strings.Contains(url, f.ID) {
			return true
		}
	}
	return false
}

func describe(cands []repository.RecordingSessionCandidate) string {
	parts := make([]string, 0, len(cands))
	for _, c := range cands {
		parts = append(parts, c.ShortID+"@"+c.SessionDate+" "+c.StartTime)
	}
	return strings.Join(parts, ", ")
}

func fileExt(f *service.ZoomRecordingFile) string {
	if f.FileExtension != "" {
		return strings.ToLower(f.FileExtension)
	}
	if e := strings.TrimPrefix(path.Ext(f.DownloadURL), "."); e != "" && len(e) <= 4 {
		return strings.ToLower(e)
	}
	return "mp4"
}
