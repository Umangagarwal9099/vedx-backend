package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		return fmt.Errorf("zoom API error (%d): %s", resp.StatusCode, string(respBody))
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
		Topic:     topic,
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
		Topic:     topic,
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

// ZoomRecordingFile is one file within a recorded meeting instance.
type ZoomRecordingFile struct {
	ID             string `json:"id"`
	FileType       string `json:"file_type"`
	FileSize       int64  `json:"file_size"`
	DownloadURL    string `json:"download_url"`
	RecordingType  string `json:"recording_type"`
	RecordingStart string `json:"recording_start"`
}

// ZoomRecordingInstance is one recorded occurrence of a meeting — a host
// rejoining the same meeting ID after class ends produces a separate
// instance here, with its own uuid and start_time, not a merge into the
// original.
type ZoomRecordingInstance struct {
	UUID           string              `json:"uuid"`
	ID             int64               `json:"id"`
	Topic          string              `json:"topic"`
	StartTime      string              `json:"start_time"`
	RecordingFiles []ZoomRecordingFile `json:"recording_files"`
}

type zoomUserRecordingsResponse struct {
	Meetings []ZoomRecordingInstance `json:"meetings"`
}

// ListUserRecordings lists every recorded meeting instance for the account's
// Zoom user within [from, to] (inclusive, YYYY-MM-DD). Used for manual
// recovery when a meeting ID has multiple recorded instances (e.g. a host
// rejoin) and the webhook-driven pipeline attached the wrong one — this
// endpoint returns every instance with its own start_time, unlike
// GET /meetings/{id}/recordings which only reflects the latest instance.
func (z *ZoomService) ListUserRecordings(from, to time.Time) ([]ZoomRecordingInstance, error) {
	url := fmt.Sprintf("%s/users/me/recordings?from=%s&to=%s&page_size=300",
		zoomAPIBase, from.Format("2006-01-02"), to.Format("2006-01-02"))
	var resp zoomUserRecordingsResponse
	if err := z.do(http.MethodGet, url, nil, &resp); err != nil {
		return nil, fmt.Errorf("list zoom recordings: %w", err)
	}
	return resp.Meetings, nil
}

// AccessToken returns a valid Server-to-Server OAuth bearer token, for
// callers (e.g. a manual recovery script) that need to authenticate a
// recording download outside the normal webhook flow, where Zoom instead
// supplies a short-lived per-event download_token in the payload.
func (z *ZoomService) AccessToken() (string, error) {
	return z.getAccessToken()
}

// DownloadRecording streams a completed cloud recording's file straight from
// Zoom. downloadToken comes from the recording.completed webhook body and
// authorizes access to that meeting's recording files without a separate
// OAuth call. Caller must close the returned body.
func (z *ZoomService) DownloadRecording(downloadURL, downloadToken string) (body io.ReadCloser, contentType string, err error) {
	req, err := http.NewRequest(http.MethodGet, downloadURL+"?access_token="+downloadToken, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build recording download request: %w", err)
	}

	resp, err := z.downloadClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download recording: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, "", fmt.Errorf("download recording failed (%d): %s", resp.StatusCode, string(b))
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}
