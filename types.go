// Package vidpickr is the official Go SDK for the VidPickr API.
//
// Quick start:
//
//	import "github.com/vidpickr/sdk-go"
//
//	vp := vidpickr.New(os.Getenv("VIDPICKR_API_KEY"))
//	err := vp.Download(ctx, "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
//	    vidpickr.Out("video.mp4"),
//	    vidpickr.Quality(1080),
//	)
//
// One call: resolve, split, stream both tracks in parallel, mux locally
// with a pure-Go MP4 muxer (no ffmpeg dependency), write to disk.
package vidpickr

// VideoFormat is one codec variant at a given resolution height.
type VideoFormat struct {
	Ext           string  `json:"ext"`
	VCodec        string  `json:"vcodec"`
	SizeMB        float64 `json:"size_mb"`
	DownloadToken string  `json:"download_token"`
	Bitrate       int     `json:"bitrate"`
}

// AudioFormat is a standalone audio track.
type AudioFormat struct {
	Ext           string  `json:"ext"`
	ACodec        string  `json:"acodec"`
	Bitrate       int     `json:"bitrate"`
	SizeMB        float64 `json:"size_mb"`
	DownloadToken string  `json:"download_token"`
	Endpoint      string  `json:"endpoint"`
}

// Resolution is one available video height with its codec options.
type Resolution struct {
	Height        int           `json:"height"`
	QualityLabel  string        `json:"quality_label"`
	SizeMB        float64       `json:"size_mb"`
	IsProgressive bool          `json:"is_progressive"`
	DownloadToken string        `json:"download_token"`
	Endpoint      string        `json:"endpoint"`
	Filename      string        `json:"filename"`
	VideoOnly     []VideoFormat `json:"video_only,omitempty"`
}

// SubtitleTrack is one caption track (manual or auto-generated).
type SubtitleTrack struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	IsAuto        bool   `json:"is_auto"`
	DownloadToken string `json:"download_token"`
	Filename      string `json:"filename"`
}

// VideoInfo is the full /v1/info response shape.
type VideoInfo struct {
	Title       string          `json:"title"`
	Thumbnail   string          `json:"thumbnail"`
	Platform    string          `json:"platform"`
	DurationSec int             `json:"duration_sec"`
	Resolutions []Resolution    `json:"resolutions"`
	AudioOnly   []AudioFormat   `json:"audio_only"`
	Subtitles   []SubtitleTrack `json:"subtitles"`
}

// SplitTokenResult is what /v1/split_token returns for a merge token.
type SplitTokenResult struct {
	VideoToken string `json:"video_token"`
	AudioToken string `json:"audio_token"`
}

// Progress reports state during a Download call.
type Progress struct {
	Phase       string // "resolving" | "fetching" | "muxing" | "finalizing" | "done"
	VideoBytes  int64
	AudioBytes  int64
	VideoTotal  int64
	AudioTotal  int64
}
