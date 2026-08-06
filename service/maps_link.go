package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/umangagarwal/vedx-backend/models"
)

// allowedMapsHosts is the set of hosts this endpoint will ever fetch —
// deliberately narrow since the URL comes from an authenticated caller but
// is otherwise untrusted input (SSRF surface).
var allowedMapsHosts = map[string]bool{
	"maps.app.goo.gl": true,
	"goo.gl":          true,
	"maps.google.com": true,
	"www.google.com":  true,
	"google.com":      true,
}

var (
	// Place links: .../data=!4m7!3m6!...!8m2!3d<lat>!4d<lng>!...
	rePlaceLatLng = regexp.MustCompile(`!3d(-?\d+\.\d+)!4d(-?\d+\.\d+)`)
	// Map-view links: .../@<lat>,<lng>,<zoom>z
	reAtLatLng = regexp.MustCompile(`@(-?\d+\.\d+),(-?\d+\.\d+)`)
	// Pin-drop / directions links: ...?q=<lat>,<lng>...
	reQueryLatLng = regexp.MustCompile(`[?&]q=(-?\d+\.\d+),(-?\d+\.\d+)`)
	// Place name segment: .../maps/place/<name>/...
	rePlaceName = regexp.MustCompile(`/maps/place/([^/]+)/`)
)

var ErrNotGoogleMapsLink = fmt.Errorf("that doesn't look like a Google Maps link")
var ErrNoCoordinatesFound = fmt.Errorf("couldn't find coordinates in that link")

var mapsLinkClient = &http.Client{
	Timeout: 8 * time.Second,
}

// ResolveGoogleMapsLink follows a Google Maps URL's redirects (short share
// links like maps.app.goo.gl expand to a full URL) and extracts the lat/lng
// it encodes. Only the URL is ever fetched — the response body is never
// read, so there's no HTML-parsing surface, just the final resolved URL
// string.
func ResolveGoogleMapsLink(ctx context.Context, rawURL string) (*models.ResolvedMapsLocation, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || !allowedMapsHosts[strings.ToLower(parsed.Host)] {
		return nil, ErrNotGoogleMapsLink
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := mapsLinkClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch maps link: %w", err)
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL
	if !allowedMapsHosts[strings.ToLower(finalURL.Host)] {
		return nil, ErrNotGoogleMapsLink
	}
	finalURLStr := finalURL.String()

	lat, lng, ok := extractLatLng(finalURLStr)
	if !ok {
		return nil, ErrNoCoordinatesFound
	}

	loc := &models.ResolvedMapsLocation{Latitude: lat, Longitude: lng}
	if m := rePlaceName.FindStringSubmatch(finalURLStr); len(m) == 2 {
		if name, err := url.PathUnescape(strings.ReplaceAll(m[1], "+", " ")); err == nil {
			loc.Name = name
		}
	}
	return loc, nil
}

func extractLatLng(s string) (lat, lng float64, ok bool) {
	for _, re := range []*regexp.Regexp{rePlaceLatLng, reAtLatLng, reQueryLatLng} {
		if m := re.FindStringSubmatch(s); len(m) == 3 {
			lat, errLat := strconv.ParseFloat(m[1], 64)
			lng, errLng := strconv.ParseFloat(m[2], 64)
			if errLat == nil && errLng == nil {
				return lat, lng, true
			}
		}
	}
	return 0, 0, false
}
