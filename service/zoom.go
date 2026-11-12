package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"github.com/umangagarwal/vedx-backend/config"
)

const zoomOAuthURL = "https://zoom.us/oauth/token"
const zoomAPIBase = "https://api.zoom.us/v2"

// ZoomMeeting is the subset of Zoom's meeting response we care about.
// StartURL is a host token — anyone holding it can start/control the
// meeting as host, so callers must only expose it to mentors/admins.
type ZoomMeeting struct {
	ID       int64  `json:"id"`
	JoinURL  string `json:"join_url"`
	StartURL string `json:"start_url"`
}

// ZoomService wraps Zoom's REST API using a Server-to-Server OAuth app.
// Same pattern as StorageService: plain net/http, no SDK.
type ZoomService struct {
	cfg            config.ZoomConfig
	client         *http.Client
	downloadClient *http.Client // no timeout — cloud recordings can take a while to transfer

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

func NewZoomService(cfg config.ZoomConfig) *ZoomService {
	return &ZoomService{
		cfg:            cfg,
		client:         &http.Client{Timeout: 15 * time.Second},
		downloadClient: &http.Client{},
	}
}

// Configured reports whether Zoom credentials have been set.
func (z *ZoomService) Configured() bool {
	return z.cfg.Configured()
}

type zoomTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// getAccessToken returns a cached Server-to-Server OAuth token, refreshing
// it shortly before it expires.
func (z *ZoomService) getAccessToken() (string, error) {
	z.mu.Lock()
	defer z.mu.Unlock()

	if z.token != "" && time.Now().Before(z.tokenExpiry) {
		return z.token, nil
	}

	url := fmt.Sprintf("%s?grant_type=account_credentials&account_id=%s", zoomOAuthURL, z.cfg.AccountID)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("build zoom oauth request: %w", err)
	}
	basic := base64.StdEncoding.EncodeToString([]byte(z.cfg.ClientID + ":" + z.cfg.ClientSecret))
	req.Header.Set("Authorization", "Basic "+basic)

	resp, err := z.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("zoom oauth request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("zoom oauth failed (%d): %s", resp.StatusCode, string(body))
	}

	var tok zoomTokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("decode zoom oauth response: %w", err)
	}

	z.token = tok.AccessToken
	// Refresh a minute early to avoid racing expiry.
	z.tokenExpiry = time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second - time.Minute)
	return z.token, nil
}

func (z *ZoomService) do(method, url string, payload interface{}, out interface{}) error {
	token, err := z.getAccessToken()
	if err != nil {
		return err
	}

	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode zoom request: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("build zoom request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := z.client.Do(req)
	if err != nil {
		return fmt.Errorf("zoom request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ZoomAPIError{Status: resp.StatusCode, Body: string(respBody)}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode zoom response: %w", err)
		}
	}
	return nil
}

type zoomMeetingSettings struct {
	JoinBeforeHost bool   `json:"join_before_host"`
	WaitingRoom    bool   `json:"waiting_room"`
	AutoRecording  string `json:"auto_recording"` // "cloud" starts recording the moment the host starts the meeting, no manual click needed
	Watermark      bool   `json:"watermark"`      // overlays each viewer's name/email translucently over shared screen content, live — Zoom's own anti-leak deterrent, rendered by the Zoom client itself
}

// zoomTopic truncates a session name to fit Zoom's 200-character topic
// limit — Zoom rejects the entire create/update request with a 400 if this
// is exceeded, which is exactly what silently broke long-named sessions
// (e.g. a 242-character name) before this existed.
func zoomTopic(name string) string {
	const maxLen = 200
	r := []rune(name)
	if len(r) <= maxLen {
		return name
	}
	return string(r[:maxLen-1]) + "…"
}

type zoomMeetingRequest struct {
	Topic     string              `json:"topic"`
	Type      int                 `json:"type"`
	StartTime string              `json:"start_time,omitempty"`
	Duration  int                 `json:"duration,omitempty"`
	Timezone  string              `json:"timezone,omitempty"`
	Settings  zoomMeetingSettings `json:"settings"`
}

// CreateMeeting schedules a Zoom meeting starting at `start` (interpreted in
// `timezone`) lasting `durationMin` minutes. join_before_host is disabled so
// students can't enter until the mentor starts the meeting via the host start
// URL — Zoom shows them a "waiting for host" screen instead. WaitingRoom is
// disabled so that once the mentor does start the meeting, students who click
// join_url land in directly rather than needing to be manually admitted.
// AutoRecording is set to "cloud" so recording starts the moment the host
// starts the meeting and stops when it ends — this is what eventually
// triggers Zoom's recording.completed webhook and populates recording_url.
func (z *ZoomService) CreateMeeting(topic string, start time.Time, durationMin int, timezone string) (*ZoomMeeting, error) {
	req := zoomMeetingRequest{
		Topic:     zoomTopic(topic),
		Type:      2, // scheduled meeting
		StartTime: start.Format("2006-01-02T15:04:05"),
		Duration:  durationMin,
		Timezone:  timezone,
		Settings: zoomMeetingSettings{
			JoinBeforeHost: false,
			WaitingRoom:    false,
			AutoRecording:  "cloud",
			Watermark:      true,
		},
	}

	var meeting ZoomMeeting
	if err := z.do(http.MethodPost, zoomAPIBase+"/users/me/meetings", req, &meeting); err != nil {
		return nil, fmt.Errorf("create zoom meeting: %w", err)
	}
	return &meeting, nil
}

// UpdateMeeting reschedules an existing Zoom meeting.
func (z *ZoomService) UpdateMeeting(meetingID int64, topic string, start time.Time, durationMin int, timezone string) error {
	req := zoomMeetingRequest{
		Topic:     zoomTopic(topic),
		Type:      2,
		StartTime: start.Format("2006-01-02T15:04:05"),
		Duration:  durationMin,
		Timezone:  timezone,
		Settings: zoomMeetingSettings{
			JoinBeforeHost: false,
			WaitingRoom:    false,
			AutoRecording:  "cloud",
			Watermark:      true,
		},
	}
	if err := z.do(http.MethodPatch, fmt.Sprintf("%s/meetings/%d", zoomAPIBase, meetingID), req, nil); err != nil {
		return fmt.Errorf("update zoom meeting: %w", err)
	}
	return nil
}

// DeleteMeeting cancels a Zoom meeting.
func (z *ZoomService) DeleteMeeting(meetingID int64) error {
	if err := z.do(http.MethodDelete, fmt.Sprintf("%s/meetings/%d", zoomAPIBase, meetingID), nil, nil); err != nil {
		return fmt.Errorf("delete zoom meeting: %w", err)
	}
	return nil
}

// WebhookConfigured reports whether the Event Subscriptions secret token is set.
func (z *ZoomService) WebhookConfigured() bool {
	return z.cfg.WebhookConfigured()
}

// hmacHex returns the lowercase-hex HMAC-SHA256 of msg using the webhook
// secret token — the primitive Zoom's URL validation and event signatures
// both build on.
func (z *ZoomService) hmacHex(msg string) string {
	mac := hmac.New(sha256.New, []byte(z.cfg.WebhookSecretToken))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateWebhookURL answers Zoom's "endpoint.url_validation" challenge:
// given the plainToken Zoom sent, it returns the encryptedToken Zoom expects
// back to prove this endpoint holds the same secret token configured in the
// Zoom Marketplace app.
func (z *ZoomService) ValidateWebhookURL(plainToken string) string {
	return z.hmacHex(plainToken)
}

// VerifyWebhookSignature checks the x-zm-signature header on an incoming
// webhook event. Zoom computes it as HMAC-SHA256("v0:{timestamp}:{rawBody}")
// using the secret token, prefixed with "v0="; timestamp is the
// x-zm-request-timestamp header. Uses a constant-time comparison to avoid
// leaking the expected signature via timing.
func (z *ZoomService) VerifyWebhookSignature(signatureHeader, timestamp string, rawBody []byte) bool {
	expected := "v0=" + z.hmacHex("v0:"+timestamp+":"+string(rawBody))
	return subtle.ConstantTimeCompare([]byte(signatureHeader), []byte(expected)) == 1
}

// ZoomRecordingFile is one file within a recorded meeting instance — Zoom
// produces a separate file per view/format (combined screen+speaker MP4,
// audio-only M4A, VTT transcript, chat TXT, timeline JSON, ...).
type ZoomRecordingFile struct {
	ID             string `json:"id"`
	MeetingID      string `json:"meeting_id"` // the instance UUID, despite the name
	FileType       string `json:"file_type"`
	FileExtension  string `json:"file_extension"`
	FileSize       int64  `json:"file_size"`
	DownloadURL    string `json:"download_url"`
	PlayURL        string `json:"play_url"`
	Status         string `json:"status"` // "completed" once the file is ready to download
	RecordingType  string `json:"recording_type"`
	RecordingStart string `json:"recording_start"`
	RecordingEnd   string `json:"recording_end"`
}

// ZoomRecordingInstance is one recorded occurrence of a meeting — a host
// rejoining the same meeting ID after class ends produces a separate
// instance here, with its own uuid and start_time, not a merge into the
// original.
type ZoomRecordingInstance struct {
	UUID           string              `json:"uuid"`
	ID             int64               `json:"id"`
	AccountID      string              `json:"account_id"`
	HostID         string              `json:"host_id"`
	HostEmail      string              `json:"host_email"`
	Topic          string              `json:"topic"`
	StartTime      string              `json:"start_time"`
	Duration       int                 `json:"duration"` // minutes
	TotalSize      int64               `json:"total_size"`
	RecordingCount int                 `json:"recording_count"`
	RecordingFiles []ZoomRecordingFile `json:"recording_files"`
}

// PrimaryVideoFile returns the file the LMS plays back: the combined
// screen+speaker view Zoom records by default, else the largest completed
// MP4. Audio-only, transcript and chat files are never selected. nil if the
// instance has no video at all.
func (inst *ZoomRecordingInstance) PrimaryVideoFile() *ZoomRecordingFile {
	for i := range inst.RecordingFiles {
		f := &inst.RecordingFiles[i]
		if f.RecordingType == "shared_screen_with_speaker_view" && f.FileType == "MP4" && f.Status != "processing" {
			return f
		}
	}
	var best *ZoomRecordingFile
	for i := range inst.RecordingFiles {
		f := &inst.RecordingFiles[i]
		if f.FileType == "MP4" && f.Status != "processing" && (best == nil || f.FileSize > best.FileSize) {
			best = f
		}
	}
	return best
}

// StartTimeParsed parses Zoom's UTC start_time, zero time if missing.
func (inst *ZoomRecordingInstance) StartTimeParsed() time.Time {
	t, _ := time.Parse(time.RFC3339, inst.StartTime)
	return t
}

// ZoomUser is one user on the Zoom account. Meetings created through the
// Server-to-Server app land on "me" (the app owner), but a mentor hosting
// from their own Zoom login records under their own user — so a full audit
// has to walk every user, not just "me".
type ZoomUser struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Type   int    `json:"type"` // 1 basic, 2 licensed — basic users cannot cloud-record at all
	Status string `json:"status"`
}

// isRetryable reports whether a Zoom API failure is worth retrying: rate
// limiting (429) and Zoom-side 5xx are transient, 4xx are not.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// getWithRetry is do() for idempotent GETs, retried with backoff on network
// errors, 429 and 5xx. Writes (create/update/delete meeting) deliberately
// stay single-shot — retrying a POST that actually succeeded server-side
// would create a duplicate meeting.
func (z *ZoomService) getWithRetry(url string, out interface{}) error {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
		err := z.do(http.MethodGet, url, nil, out)
		if err == nil {
			return nil
		}
		lastErr = err
		var apiErr *ZoomAPIError
		if errors.As(err, &apiErr) && !isRetryableStatus(apiErr.Status) {
			return err
		}
	}
	return lastErr
}

// ZoomAPIError carries the HTTP status of a failed Zoom API call so callers
// can tell "not found / no permission" apart from transient failures.
type ZoomAPIError struct {
	Status int
	Body   string
}

func (e *ZoomAPIError) Error() string {
	return fmt.Sprintf("zoom API error (%d): %s", e.Status, e.Body)
}

// IsZoomNotFound reports whether err is a Zoom 404 — e.g. a meeting's
// recordings were deleted, or never existed.
func IsZoomNotFound(err error) bool {
	var apiErr *ZoomAPIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// ListUsers returns every user on the Zoom account (active, inactive and
// pending), following next_page_token pagination.
func (z *ZoomService) ListUsers() ([]ZoomUser, error) {
	var all []ZoomUser
	for _, status := range []string{"active", "inactive", "pending"} {
		token := ""
		for {
			u := fmt.Sprintf("%s/users?status=%s&page_size=300", zoomAPIBase, status)
			if token != "" {
				u += "&next_page_token=" + neturl.QueryEscape(token)
			}
			var resp struct {
				Users         []ZoomUser `json:"users"`
				NextPageToken string     `json:"next_page_token"`
			}
			if err := z.getWithRetry(u, &resp); err != nil {
				return nil, fmt.Errorf("list zoom users (%s): %w", status, err)
			}
			all = append(all, resp.Users...)
			if resp.NextPageToken == "" {
				break
			}
			token = resp.NextPageToken
		}
	}
	return all, nil
}

// ListRecordings lists every recorded meeting instance for userID ("me" or a
// Zoom user ID/email) within [from, to]. Zoom caps a single request at a
// one-month range and 300 results per page, so this walks the range in
// 30-day windows and follows next_page_token within each — the previous
// single-request version silently dropped everything beyond the first month
// and the first page. trash=true lists recordings in the Zoom trash instead
// (recoverable for 30 days after deletion).
func (z *ZoomService) ListRecordings(userID string, from, to time.Time, trash bool) ([]ZoomRecordingInstance, error) {
	var all []ZoomRecordingInstance
	seen := map[string]bool{}
	for winStart := from; !winStart.After(to); winStart = winStart.AddDate(0, 0, 30) {
		winEnd := winStart.AddDate(0, 0, 29)
		if winEnd.After(to) {
			winEnd = to
		}
		token := ""
		for {
			u := fmt.Sprintf("%s/users/%s/recordings?from=%s&to=%s&page_size=300&trash=%t",
				zoomAPIBase, neturl.PathEscape(userID), winStart.Format("2006-01-02"), winEnd.Format("2006-01-02"), trash)
			if token != "" {
				u += "&next_page_token=" + neturl.QueryEscape(token)
			}
			var resp struct {
				Meetings      []ZoomRecordingInstance `json:"meetings"`
				NextPageToken string                  `json:"next_page_token"`
			}
			if err := z.getWithRetry(u, &resp); err != nil {
				return nil, fmt.Errorf("list zoom recordings for %s %s..%s: %w", userID, winStart.Format("2006-01-02"), winEnd.Format("2006-01-02"), err)
			}
			for _, m := range resp.Meetings {
				// Windows are inclusive on both ends, so an instance on a
				// boundary day can come back twice.
				if !seen[m.UUID] {
					seen[m.UUID] = true
					all = append(all, m)
				}
			}
			if resp.NextPageToken == "" {
				break
			}
			token = resp.NextPageToken
		}
	}
	return all, nil
}

// ListAccountRecordings lists recordings for every user on the account, so
// nothing recorded under a mentor's own Zoom login is missed.
func (z *ZoomService) ListAccountRecordings(from, to time.Time) ([]ZoomRecordingInstance, error) {
	users, err := z.ListUsers()
	if err != nil {
		return nil, err
	}
	var all []ZoomRecordingInstance
	seen := map[string]bool{}
	for _, u := range users {
		recs, err := z.ListRecordings(u.ID, from, to, false)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			if !seen[r.UUID] {
				seen[r.UUID] = true
				all = append(all, r)
			}
		}
	}
	return all, nil
}

// ListUserRecordings is kept for the earlier one-off recovery tools in cmd/;
// it now paginates and handles ranges longer than a month.
func (z *ZoomService) ListUserRecordings(from, to time.Time) ([]ZoomRecordingInstance, error) {
	return z.ListRecordings("me", from, to, false)
}

// zoomUUIDPath encodes a meeting instance UUID for use in a URL path. Zoom
// requires UUIDs that start with "/" or contain "//" to be double-encoded.
func zoomUUIDPath(uuid string) string {
	// QueryEscape, not PathEscape: UUIDs are base64 and PathEscape leaves
	// "+" and "=" untouched, which Zoom then fails to match (404).
	enc := neturl.QueryEscape(uuid)
	if strings.HasPrefix(uuid, "/") || strings.Contains(uuid, "//") {
		enc = neturl.QueryEscape(enc)
	}
	return enc
}

// GetInstanceRecordings fetches one recorded instance by UUID, including
// fresh download URLs. Always use this right before downloading — download
// URLs and the per-event download_token in a webhook expire (24h for the
// token), so a retried job must never rely on the URL it was first given.
func (z *ZoomService) GetInstanceRecordings(uuid string) (*ZoomRecordingInstance, error) {
	var inst ZoomRecordingInstance
	if err := z.getWithRetry(fmt.Sprintf("%s/meetings/%s/recordings", zoomAPIBase, zoomUUIDPath(uuid)), &inst); err != nil {
		return nil, fmt.Errorf("get zoom recording %s: %w", uuid, err)
	}
	return &inst, nil
}

// ZoomMeetingDetails is the audit-relevant subset of GET /meetings/{id}.
type ZoomMeetingDetails struct {
	ID        int64  `json:"id"`
	HostID    string `json:"host_id"`
	HostEmail string `json:"host_email"`
	Topic     string `json:"topic"`
	Status    string `json:"status"`
	StartTime string `json:"start_time"`
	Settings  struct {
		AutoRecording string `json:"auto_recording"`
	} `json:"settings"`
}

// GetMeeting returns a scheduled meeting's details (host, recording setting).
func (z *ZoomService) GetMeeting(meetingID int64) (*ZoomMeetingDetails, error) {
	var m ZoomMeetingDetails
	if err := z.getWithRetry(fmt.Sprintf("%s/meetings/%d", zoomAPIBase, meetingID), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ZoomPastInstance is one occurrence of a meeting that actually took place.
type ZoomPastInstance struct {
	UUID      string `json:"uuid"`
	StartTime string `json:"start_time"`
}

// ListPastInstances returns the occurrences of meetingID that were actually
// held — distinguishes "class happened but recording is missing" from "class
// never started on this meeting ID at all".
func (z *ZoomService) ListPastInstances(meetingID int64) ([]ZoomPastInstance, error) {
	var resp struct {
		Meetings []ZoomPastInstance `json:"meetings"`
	}
	if err := z.getWithRetry(fmt.Sprintf("%s/past_meetings/%d/instances", zoomAPIBase, meetingID), &resp); err != nil {
		return nil, err
	}
	return resp.Meetings, nil
}

// AccessToken returns a valid Server-to-Server OAuth bearer token, for
// callers that need to authenticate a recording download outside the
// webhook flow.
func (z *ZoomService) AccessToken() (string, error) {
	return z.getAccessToken()
}

// DownloadRecording streams a cloud recording file straight from Zoom.
// token is either a webhook's per-event download_token or a Server-to-Server
// OAuth access token — Zoom accepts both as a Bearer credential on
// download_url. Sent as a header rather than ?access_token= so the token
// never ends up in logged URLs or error messages. Caller must close the
// returned body.
func (z *ZoomService) DownloadRecording(downloadURL, token string) (body io.ReadCloser, contentType string, err error) {
	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build recording download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := z.downloadClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download recording: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, "", &ZoomAPIError{Status: resp.StatusCode, Body: "download recording: " + string(b)}
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" || ct == "application/octet-stream" || strings.HasPrefix(ct, "binary/") {
		ct = "video/mp4"
	}
	return resp.Body, ct, nil
}

// DownloadRecordingWithOAuth downloads using a fresh Server-to-Server OAuth
// token — the path every retry/reconciliation download takes, since it
// never expires the way a webhook's download_token does.
func (z *ZoomService) DownloadRecordingWithOAuth(downloadURL string) (io.ReadCloser, string, error) {
	token, err := z.getAccessToken()
	if err != nil {
		return nil, "", err
	}
	return z.DownloadRecording(downloadURL, token)
}
