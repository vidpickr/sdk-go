// Package mp4mux performs the local "join video.mp4 + audio.m4a into
// out.mp4" step. The Download flow streams two tracks from the API
// and then needs them packaged into a single container.
//
// v0.1.0 implementation: shells out to ffmpeg (-c copy, no re-encode).
// This is the same pragmatic call the Python SDK makes for its first
// release — ships a working SDK fast.
//
// v0.2 roadmap: replace the subprocess with a pure-Go MP4 muxer using
// github.com/Eyevinn/mp4ff. Public API in download.go stays identical;
// only this file changes. That removes the ffmpeg-on-PATH requirement
// so users get the "zero runtime deps" promise Go-land usually expects.
package mp4mux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// ErrFFmpegMissing means the binary couldn't be located on PATH and
// no VIDPICKR_FFMPEG override was supplied.
var ErrFFmpegMissing = errors.New(
	"ffmpeg required for muxing — install it system-wide (brew/apt/winget install ffmpeg) " +
		"or set VIDPICKR_FFMPEG to an absolute path. Pure-Go mux planned for v0.2.",
)

// MuxStreamCopy joins videoPath + audioPath into outPath using stream
// copy (no re-encoding, fast).
func MuxStreamCopy(videoPath, audioPath, outPath string) error {
	bin, err := findFFmpeg()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin,
		"-y",
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		outPath,
	)
	// Discard stdout, capture stderr so the failure message is useful.
	cmd.Stdout = nil
	stderr, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg mux failed: %v — %s", err, truncate(string(stderr), 400))
	}
	return nil
}

func findFFmpeg() (string, error) {
	if env := os.Getenv("VIDPICKR_FFMPEG"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
	}
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path, nil
	}
	return "", ErrFFmpegMissing
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
