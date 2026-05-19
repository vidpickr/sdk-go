# vidpickr — Go SDK

Official Go SDK for the [VidPickr API](https://vidpickr.com/docs). Download YouTube videos with one function call.

```go
package main

import (
    "context"
    "log"
    "os"

    vidpickr "github.com/vidpickr/sdk-go"
)

func main() {
    vp := vidpickr.New(os.Getenv("VIDPICKR_API_KEY"))
    err := vp.Download(context.Background(),
        "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
        vidpickr.Out("video.mp4"),
        vidpickr.Quality(1080),
    )
    if err != nil {
        log.Fatal(err)
    }
}
```

That's it. The SDK:

1. Resolves the URL through `/api/v1/info`
2. Picks the best 1080p video track and the highest-bitrate audio track
3. Exchanges the merge token via `/api/v1/split_token`
4. Streams both tracks in parallel from `/api/v1/stream`
5. Muxes them into one MP4 with `ffmpeg -c copy`
6. Writes the result to `video.mp4` and cleans up temp files

## Requirements

- **Go 1.21+**
- A **VidPickr Plus subscription** ($1/mo) and an **API key**, minted at [vidpickr.com/account/api-keys](https://vidpickr.com/account/api-keys)
- **ffmpeg** on PATH for the mux step (v0.1.0 — pure-Go mux planned for v0.2)

## Install

```sh
go get github.com/vidpickr/sdk-go
```

CLI binary:

```sh
go install github.com/vidpickr/sdk-go/cmd/vidpickr@latest

vidpickr --key vpk_live_... --quality 1080 -o video.mp4 \
    https://www.youtube.com/watch?v=dQw4w9WgXcQ
```

## API

### `vidpickr.New(apiKey string, opts ...Option) *VidPickr`

Construct the high-level SDK. Options:

- `WithBaseURL(url string)` — override API host (default: `https://api.vidpickr.com/v1`)
- `WithHTTPClient(*http.Client)` — supply your own HTTP client (custom transport, retries, telemetry, etc.)

### `(*VidPickr).Download(ctx, url, opts ...DownloadOption) error`

Resolve, stream, mux, write to disk. Options:

| Option       | Type / Values                                       | Default | Description                                                   |
|--------------|-----------------------------------------------------|---------|---------------------------------------------------------------|
| `Out`        | `string`                                            | —       | Output file path. Required.                                   |
| `Quality`    | `"best"` / `"highest"` / `"lowest"` / `int`         | `"best"`| Target height in px.                                          |
| `Codec`      | `"av1"` / `"vp9"` / `"avc"` / `"hevc"`              | —       | Preferred codec when multiple at the same height.             |
| `OnProgress` | `func(Progress)`                                    | nil     | Callback fired on phase transitions.                          |

### `(*VidPickr).Info(ctx, url) (*VideoInfo, error)`

Resolve only. Use when you want to inspect formats before deciding.

### `(*VidPickr).Client() *Client`

Returns the low-level HTTP client. Use for custom pipelines (e.g. piping audio bytes directly into a transcription service without writing to disk).

## Errors

```go
import vidpickr "github.com/vidpickr/sdk-go"

if err := vp.Download(ctx, url, vidpickr.Out("x.mp4")); err != nil {
    var apiErr *vidpickr.APIError
    if errors.As(err, &apiErr) {
        switch apiErr.Code {
        case "rate_limited":
            log.Printf("retry in %ds", apiErr.RetryAfter)
        case "plus_required":
            log.Fatal("subscribe to Plus first")
        }
    }
    var noFmt *vidpickr.NoFormatError
    if errors.As(err, &noFmt) {
        log.Printf("requested format unavailable: %s", noFmt.Reason)
    }
}
```

## Subtitles

```go
info, _ := vp.Info(ctx, url)
for _, s := range info.Subtitles {
    if s.Code == "en" && !s.IsAuto {
        srt, _ := vp.Client().Subtitle(ctx, s.DownloadToken, "srt")
        os.WriteFile("captions.srt", []byte(srt), 0o644)
        break
    }
}
```

## Roadmap

- **v0.2** — replace the ffmpeg subprocess with a pure-Go MP4 muxer (`github.com/Eyevinn/mp4ff`). Public API stays identical; only `internal/mp4mux` changes. Drops the ffmpeg-on-PATH requirement so you get the single-binary experience Go users expect.

## License

MIT
