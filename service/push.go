package service

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// expoPushURL is Expo's public push-sending API — no API key required for
// the default (unauthenticated) tier, which is all this platform needs.
const expoPushURL = "https://exp.host/--/api/v2/push/send"

// expoBatchSize is Expo's documented per-request cap on push messages.
const expoBatchSize = 100

// pushSendTimeout bounds a single batch request to Expo.
const pushSendTimeout = 10 * time.Second

var pushHTTPClient = &http.Client{Timeout: pushSendTimeout}

type expoPushMessage struct {
	To    string                 `json:"to"`
	Title string                 `json:"title"`
	Body  string                 `json:"body"`
	Sound string                 `json:"sound,omitempty"`
	Data  map[string]interface{} `json:"data,omitempty"`
}

// SendExpoPushAsync fans a notification out to Expo push in the background —
// best-effort, matching EmailService.SendAsync's convention elsewhere in this
// codebase: a failed/slow push must never block or fail the request that
// triggered it (e.g. creating an in-app notification). Errors are logged,
// not surfaced. tokens with no value are skipped silently (a user with no
// registered device is the common case, not an error).
func SendExpoPushAsync(tokens []string, title, body string, data map[string]interface{}) {
	if len(tokens) == 0 {
		return
	}
	go func() {
		for start := 0; start < len(tokens); start += expoBatchSize {
			end := start + expoBatchSize
			if end > len(tokens) {
				end = len(tokens)
			}
			sendExpoPushBatch(tokens[start:end], title, body, data)
		}
	}()
}

func sendExpoPushBatch(tokens []string, title, body string, data map[string]interface{}) {
	messages := make([]expoPushMessage, len(tokens))
	for i, t := range tokens {
		messages[i] = expoPushMessage{To: t, Title: title, Body: body, Sound: "default", Data: data}
	}

	payload, err := json.Marshal(messages)
	if err != nil {
		log.Printf("expo push: marshal failed: %v", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, expoPushURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("expo push: build request failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := pushHTTPClient.Do(req)
	if err != nil {
		log.Printf("expo push: send failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("expo push: unexpected status %d", resp.StatusCode)
	}
}
