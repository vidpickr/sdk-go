// vidpickr CLI — wraps the Go SDK as a single-binary command. Install
// with:
//
//	go install github.com/vidpickr/sdk-go/cmd/vidpickr@latest
//
// Usage:
//
//	vidpickr --key vpk_live_... --quality 1080 -o out.mp4 \
//	    https://www.youtube.com/watch?v=dQw4w9WgXcQ
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	vidpickr "github.com/vidpickr/sdk-go"
)

func main() {
	key := flag.String("key", os.Getenv("VIDPICKR_API_KEY"), "API key (or set VIDPICKR_API_KEY)")
	out := flag.String("o", "out.mp4", "output file path")
	quality := flag.Int("quality", 1080, "target height in px (0 = best available)")
	codec := flag.String("codec", "", "preferred video codec: av1, vp9, avc, hevc")
	flag.Parse()

	if *key == "" {
		fmt.Fprintln(os.Stderr, "vidpickr: --key is required (or set VIDPICKR_API_KEY)")
		os.Exit(2)
	}
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "vidpickr: pass exactly one YouTube URL")
		os.Exit(2)
	}
	url := flag.Arg(0)

	q := any(*quality)
	if *quality == 0 {
		q = "best"
	}

	vp := vidpickr.New(*key)
	opts := []vidpickr.DownloadOption{
		vidpickr.Out(*out),
		vidpickr.Quality(q),
		vidpickr.OnProgress(func(p vidpickr.Progress) {
			fmt.Fprintf(os.Stderr, "  %s\n", p.Phase)
		}),
	}
	if *codec != "" {
		opts = append(opts, vidpickr.Codec(*codec))
	}

	if err := vp.Download(context.Background(), url, opts...); err != nil {
		fmt.Fprintln(os.Stderr, "vidpickr:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out)
}
