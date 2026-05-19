// Basic example. Run with:
//
//	VIDPICKR_API_KEY=vpk_live_... go run ./examples/basic [url]
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	vidpickr "github.com/vidpickr/sdk-go"
)

func main() {
	key := os.Getenv("VIDPICKR_API_KEY")
	if key == "" {
		log.Fatal("Set VIDPICKR_API_KEY first")
	}
	url := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	vp := vidpickr.New(key)
	err := vp.Download(context.Background(), url,
		vidpickr.Out("out.mp4"),
		vidpickr.Quality(1080),
		vidpickr.OnProgress(func(p vidpickr.Progress) {
			fmt.Printf("  %s\n", p.Phase)
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("wrote out.mp4")
}
