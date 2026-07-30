package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// DefaultWatermarkText is burned into every recording unless overridden.
// Session/viewer-specific watermarking isn't possible here — this runs once,
// server-side, before the file is ever stored, producing a single video
// shared by every viewer — so it deters casual re-distribution but can't
// trace a leak back to a specific person the way a per-viewer overlay
// rendered in the frontend player could.
const DefaultWatermarkText = "VedX • Unauthorized distribution prohibited"

// ffmpegBinary and watermarkFont are overridable via env vars because the
// production image places a static ffmpeg build and a bundled font at fixed
// paths (see Dockerfile) rather than relying on PATH/fontconfig, neither of
// which exist in the distroless runtime image.
var ffmpegBinary = envOrDefault("FFMPEG_PATH", "ffmpeg")
var watermarkFont = envOrDefault("WATERMARK_FONT_PATH", "")

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// WatermarkRecording re-encodes src with text burned into every frame and
// returns a handle to the result. Callers must Close() the returned
// ReadCloser — doing so also removes the backing temp files. ffmpeg needs
// random access to read the MP4 container (its index can sit at the end of
// the file) and to write one back out, so both the input and output are
// staged on local disk under os.TempDir() for the duration of the call —
// callers should expect roughly 2x the recording's size in free disk space
// while this runs.
func WatermarkRecording(ctx context.Context, src io.Reader, text string) (io.ReadCloser, error) {
	srcFile, err := os.CreateTemp("", "zoomrec-src-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("stage recording for watermarking: %w", err)
	}
	srcPath := srcFile.Name()
	defer os.Remove(srcPath)

	if _, err := io.Copy(srcFile, src); err != nil {
		srcFile.Close()
		return nil, fmt.Errorf("stage recording for watermarking: %w", err)
	}
	if err := srcFile.Close(); err != nil {
		return nil, fmt.Errorf("stage recording for watermarking: %w", err)
	}

	dstFile, err := os.CreateTemp("", "zoomrec-watermarked-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("create watermark output file: %w", err)
	}
	dstPath := dstFile.Name()
	dstFile.Close()

	if err := burnWatermark(ctx, srcPath, dstPath, text); err != nil {
		os.Remove(dstPath)
		return nil, err
	}

	out, err := os.Open(dstPath)
	if err != nil {
		os.Remove(dstPath)
		return nil, fmt.Errorf("open watermarked recording: %w", err)
	}
	return &tempFileReadCloser{File: out, path: dstPath}, nil
}

// burnWatermark shells out to ffmpeg to overlay text in the bottom-right
// corner of every frame. Only the video stream is re-encoded (libx264,
// veryfast preset — a full re-encode is unavoidable to alter pixels, but
// there's no reason to spend extra CPU tuning quality for a lecture
// recording); audio is stream-copied untouched.
func burnWatermark(ctx context.Context, srcPath, dstPath, text string) error {
	if watermarkFont == "" {
		return fmt.Errorf("watermark: WATERMARK_FONT_PATH not set — drawtext has no fontconfig to fall back on in this runtime")
	}

	drawtext := fmt.Sprintf(
		"drawtext=fontfile=%s:text='%s':fontcolor=white@0.6:fontsize=22:x=w-tw-24:y=h-th-24:box=1:boxcolor=black@0.35:boxborderw=6",
		escapeDrawtextValue(watermarkFont), escapeDrawtextValue(text),
	)

	cmd := exec.CommandContext(ctx, ffmpegBinary,
		"-y",
		"-i", srcPath,
		"-vf", drawtext,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-c:a", "copy",
		dstPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg watermark: %w: %s", err, string(out))
	}
	return nil
}

// escapeDrawtextValue escapes characters ffmpeg's drawtext filter treats as
// syntax (colons separate filter options, backslashes/quotes/percent are its
// own escape characters) so arbitrary text/paths can't break or inject into
// the filtergraph.
func escapeDrawtextValue(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`:`, `\:`,
		`'`, `\'`,
		`%`, `\%`,
	)
	return r.Replace(s)
}

// tempFileReadCloser deletes its backing file on Close, so callers that
// treat the return value as a plain io.ReadCloser automatically clean up the
// staged output without needing to know it's a temp file at all.
type tempFileReadCloser struct {
	*os.File
	path string
}

func (t *tempFileReadCloser) Close() error {
	err := t.File.Close()
	if rmErr := os.Remove(t.path); err == nil {
		err = rmErr
	}
	return err
}
