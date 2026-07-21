package util

import (
	"regexp"
	"strings"
)

// ParsedUserAgent is a coarse, best-effort breakdown of a User-Agent header —
// enough to render "Laptop · Android 10" / "Chrome 149.0.0.0" in a UI, not a
// full device-fingerprinting library.
type ParsedUserAgent struct {
	DeviceType     string // mobile | tablet | laptop | unknown
	OSName         string
	BrowserName    string
	BrowserVersion string
}

var (
	osPatterns = []struct {
		match *regexp.Regexp
		name  string
	}{
		{regexp.MustCompile(`Android (\d+(\.\d+)?)`), "Android"},
		{regexp.MustCompile(`iPhone OS (\d+[_.]\d+)`), "iOS"},
		{regexp.MustCompile(`iPad.*OS (\d+[_.]\d+)`), "iPadOS"},
		{regexp.MustCompile(`Windows NT 10\.0`), "Windows 10/11"},
		{regexp.MustCompile(`Windows NT (\d+\.\d+)`), "Windows"},
		{regexp.MustCompile(`Mac OS X (\d+[_.]\d+)`), "macOS"},
		{regexp.MustCompile(`Linux`), "Linux"},
	}

	browserPatterns = []struct {
		match *regexp.Regexp
		name  string
	}{
		{regexp.MustCompile(`Edg/([\d.]+)`), "Edge"},
		{regexp.MustCompile(`OPR/([\d.]+)`), "Opera"},
		{regexp.MustCompile(`Chrome/([\d.]+)`), "Chrome"},
		{regexp.MustCompile(`CriOS/([\d.]+)`), "Chrome"},
		{regexp.MustCompile(`FxiOS/([\d.]+)`), "Firefox"},
		{regexp.MustCompile(`Firefox/([\d.]+)`), "Firefox"},
		{regexp.MustCompile(`Version/([\d.]+).*Safari`), "Safari"},
	}
)

// ParseUserAgent extracts a rough device type, OS name, and browser
// name+version from a raw User-Agent header string.
func ParseUserAgent(ua string) ParsedUserAgent {
	p := ParsedUserAgent{DeviceType: "unknown"}

	switch {
	case strings.Contains(ua, "Mobile") || strings.Contains(ua, "iPhone") || strings.Contains(ua, "Android"):
		p.DeviceType = "mobile"
	case strings.Contains(ua, "iPad") || strings.Contains(ua, "Tablet"):
		p.DeviceType = "tablet"
	case strings.Contains(ua, "Windows") || strings.Contains(ua, "Macintosh") || strings.Contains(ua, "Linux"):
		p.DeviceType = "laptop"
	}

	for _, o := range osPatterns {
		if m := o.match.FindStringSubmatch(ua); m != nil {
			p.OSName = o.name
			if len(m) > 1 && m[1] != "" {
				p.OSName = o.name + " " + strings.ReplaceAll(m[1], "_", ".")
			}
			break
		}
	}

	for _, b := range browserPatterns {
		if m := b.match.FindStringSubmatch(ua); m != nil {
			p.BrowserName = b.name
			if len(m) > 1 {
				p.BrowserVersion = m[1]
			}
			break
		}
	}

	return p
}
