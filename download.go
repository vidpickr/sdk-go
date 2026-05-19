package vidpickr

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/vidpickr/sdk-go/internal/mp4mux"
)

// VidPickr is the high-level SDK. Construct one, call Download per URL.
type VidPickr struct {
	c *Client
}

// New constructs a VidPickr backed by a low-level Client. Pass Option
// values (WithBaseURL, WithHTTPClient) to tune transport behaviour.
func New(apiKey string, opts ...Option) *VidPickr {
	return &VidPickr{c: NewClient(apiKey, opts...)}
}

// Client returns the underlying low-level client for custom pipelines.
func (vp *VidPickr) Client() *Client { return vp.c }

// Info resolves a URL without downloading. Useful when you want to
// inspect formats before deciding.
func (vp *VidPickr) Info(ctx context.Context, url string) (*VideoInfo, error) {
	return vp.c.Info(ctx, url)
}

// Download resolves, streams, muxes, and writes to disk in one call.
// Use the Out / Quality / Codec / Progress functional options to
// customise behaviour.
func (vp *VidPickr) Download(ctx context.Context, url string, opts ...DownloadOption) error {
	cfg := defaultDownloadConfig()
	for _, o := range opts {
		o(cfg)
	}
	if cfg.out == "" {
		return &NoFormatError{Reason: "Out option is required"}
	}

	tick(cfg.progress, "resolving", 0, 0)

	info, err := vp.c.Info(ctx, url)
	if err != nil {
		return err
	}

	resolution, err := pickResolution(info, cfg.quality)
	if err != nil {
		return err
	}
	audio, err := pickAudio(info)
	if err != nil {
		return err
	}

	split, err := vp.c.SplitToken(ctx, resolution.DownloadToken)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "vidpickr-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	vPath := filepath.Join(tmpDir, "video.mp4")
	aPath := filepath.Join(tmpDir, "audio.m4a")

	tick(cfg.progress, "fetching", 0, 0)

	var wg sync.WaitGroup
	var vErr, aErr error
	var vBytes, aBytes int64
	wg.Add(2)
	go func() {
		defer wg.Done()
		vBytes, vErr = vp.c.StreamToFile(ctx, split.VideoToken, vPath)
	}()
	go func() {
		defer wg.Done()
		aBytes, aErr = vp.c.StreamToFile(ctx, audio.DownloadToken, aPath)
	}()
	wg.Wait()
	if vErr != nil {
		return vErr
	}
	if aErr != nil {
		return aErr
	}

	tick(cfg.progress, "muxing", vBytes, aBytes)

	if err := mp4mux.MuxStreamCopy(vPath, aPath, cfg.out); err != nil {
		return &MuxError{Reason: err.Error()}
	}

	tick(cfg.progress, "done", vBytes, aBytes)
	return nil
}

// ─────────────────────────────────────────────────────────────────────
// Functional options

type downloadConfig struct {
	out      string
	quality  any
	codec    string
	progress func(Progress)
}

func defaultDownloadConfig() *downloadConfig {
	return &downloadConfig{quality: "best"}
}

// DownloadOption configures a single Download call.
type DownloadOption func(*downloadConfig)

// Out specifies the output file path. Required.
func Out(path string) DownloadOption { return func(c *downloadConfig) { c.out = path } }

// Quality selects the target video height. Accepts:
//
//	"best", "highest"  → top available resolution
//	"lowest"           → smallest available
//	int (e.g. 1080)    → exact match, fall back to next-lower
func Quality(q any) DownloadOption { return func(c *downloadConfig) { c.quality = q } }

// Codec picks a preferred video codec when multiple variants exist at
// the same height. One of: "av1", "vp9", "avc", "hevc".
func Codec(name string) DownloadOption { return func(c *downloadConfig) { c.codec = name } }

// OnProgress wires a callback invoked between phases.
func OnProgress(fn func(Progress)) DownloadOption {
	return func(c *downloadConfig) { c.progress = fn }
}

// ─────────────────────────────────────────────────────────────────────
// Internals

func pickResolution(info *VideoInfo, quality any) (*Resolution, error) {
	if len(info.Resolutions) == 0 {
		return nil, &NoFormatError{Reason: "no video formats in /info response"}
	}
	sorted := append([]Resolution(nil), info.Resolutions...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Height > sorted[j].Height })

	switch q := quality.(type) {
	case string:
		switch strings.ToLower(q) {
		case "lowest":
			return &sorted[len(sorted)-1], nil
		case "best", "highest", "":
			return &sorted[0], nil
		default:
			return nil, &NoFormatError{Reason: "unknown quality preset: " + q}
		}
	case int:
		for i := range sorted {
			if sorted[i].Height == q {
				return &sorted[i], nil
			}
		}
		for i := range sorted {
			if sorted[i].Height <= q {
				return &sorted[i], nil
			}
		}
		return &sorted[len(sorted)-1], nil
	default:
		return nil, &NoFormatError{Reason: "quality must be string or int"}
	}
}

func pickAudio(info *VideoInfo) (*AudioFormat, error) {
	if len(info.AudioOnly) == 0 {
		return nil, &NoFormatError{Reason: "no audio formats in /info response"}
	}
	// Highest bitrate; prefer m4a (AAC) over webm (Opus) when tied
	// because the MP4 stream-copy path likes AAC.
	best := &info.AudioOnly[0]
	for i := 1; i < len(info.AudioOnly); i++ {
		f := &info.AudioOnly[i]
		if f.Bitrate > best.Bitrate {
			best = f
		} else if f.Bitrate == best.Bitrate && f.Ext == "m4a" && best.Ext != "m4a" {
			best = f
		}
	}
	return best, nil
}

func tick(fn func(Progress), phase string, v, a int64) {
	if fn == nil {
		return
	}
	fn(Progress{Phase: phase, VideoBytes: v, AudioBytes: a})
}
