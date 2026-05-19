// Package mp4mux performs the local "join video.mp4 + audio.m4a into
// out.mp4" step using a pure-Go MP4 box rewrite. No ffmpeg dependency,
// no subprocess, no temp binaries — the SDK ships as a single-binary
// experience Go users expect.
//
// YouTube's /v1/stream returns fragmented MP4 (CMAF-style: ftyp + moov
// + a series of moof+mdat pairs). The naive "two single-mdat files
// stitched" approach doesn't apply. Instead we follow the pattern from
// mp4ff's examples/combine-segs:
//
//   1. Parse both files in slice-reader mode so the mdat data stays
//      addressable cheaply (no streaming reader gymnastics for the
//      lazy mode's seek-back semantics).
//   2. Renumber tracks: video → trackID 1, audio → trackID 2.
//   3. Build a combined init segment by merging the two moovs (and
//      their mvex/trex entries).
//   4. Walk every input fragment, pull the FullSamples for its single
//      track, and append them to a single output multi-track Fragment.
//   5. Write the combined init followed by the combined fragment.
//
// The output is fragmented MP4 with one big fragment containing every
// sample from both inputs. VLC, QuickTime, Safari, Chrome, the
// browser-side <video> element, ffmpeg — all of them play it natively.
package mp4mux

import (
	"errors"
	"fmt"
	"os"

	"github.com/Eyevinn/mp4ff/bits"
	"github.com/Eyevinn/mp4ff/mp4"
)

// ErrInputShape fires when an input doesn't look like a YouTube-style
// single-track fragmented MP4 with a parseable init+segment layout.
var ErrInputShape = errors.New("vidpickr mp4mux: unexpected input shape")

const (
	videoTrackID uint32 = 1
	audioTrackID uint32 = 2
)

// MuxStreamCopy joins videoPath + audioPath into outPath. Both inputs
// are expected to be single-track fragmented MP4 (the format YouTube's
// /v1/stream returns). The output is a fragmented MP4 with two tracks.
func MuxStreamCopy(videoPath, audioPath, outPath string) error {
	videoInit, videoFrags, videoTrex, err := readSingleTrackFragmented(videoPath)
	if err != nil {
		return fmt.Errorf("read video: %w", err)
	}
	audioInit, audioFrags, audioTrex, err := readSingleTrackFragmented(audioPath)
	if err != nil {
		return fmt.Errorf("read audio: %w", err)
	}

	combinedInit, err := combineInits(videoInit, audioInit)
	if err != nil {
		return fmt.Errorf("combine init: %w", err)
	}

	outFrag, err := mp4.CreateMultiTrackFragment(1, []uint32{videoTrackID, audioTrackID})
	if err != nil {
		return fmt.Errorf("create output fragment: %w", err)
	}

	if err := appendSamples(outFrag, videoFrags, videoTrex, videoTrackID); err != nil {
		return fmt.Errorf("append video samples: %w", err)
	}
	if err := appendSamples(outFrag, audioFrags, audioTrex, audioTrackID); err != nil {
		return fmt.Errorf("append audio samples: %w", err)
	}

	outFH, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer outFH.Close()

	if err := combinedInit.Encode(outFH); err != nil {
		return fmt.Errorf("write init: %w", err)
	}
	if err := outFrag.Encode(outFH); err != nil {
		return fmt.Errorf("write fragment: %w", err)
	}
	return nil
}

// readSingleTrackFragmented loads a fragmented MP4 file fully into a
// SliceReader-backed mp4.File. Returns the init segment, the list of
// fragments (across all media segments), and the trex box for that
// single track (needed by GetFullSamples to fill in default values).
func readSingleTrackFragmented(path string) (*mp4.InitSegment, []*mp4.Fragment, *mp4.TrexBox, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, err
	}
	sr := bits.NewFixedSliceReader(data)
	f, err := mp4.DecodeFileSR(sr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("decode mp4: %w", err)
	}
	if f.Init == nil || f.Init.Moov == nil {
		return nil, nil, nil, fmt.Errorf("%w: missing init segment", ErrInputShape)
	}
	if len(f.Init.Moov.Traks) != 1 {
		return nil, nil, nil, fmt.Errorf("%w: expected 1 track, got %d", ErrInputShape, len(f.Init.Moov.Traks))
	}
	var trex *mp4.TrexBox
	if f.Init.Moov.Mvex != nil {
		trex = f.Init.Moov.Mvex.Trex
	}

	// Gather fragments across every media segment. YouTube usually
	// delivers a single segment with many fragments; some videos come
	// as multiple segments. Either way we just want a flat list.
	var frags []*mp4.Fragment
	for _, seg := range f.Segments {
		for _, fr := range seg.Fragments {
			frags = append(frags, fr)
		}
	}
	if len(frags) == 0 {
		return nil, nil, nil, fmt.Errorf("%w: no fragments found", ErrInputShape)
	}
	return f.Init, frags, trex, nil
}

// combineInits builds a multi-track init segment by stacking video and
// audio init segments. Track IDs are renumbered to 1 (video) and 2
// (audio); trex track IDs follow.
func combineInits(videoInit, audioInit *mp4.InitSegment) (*mp4.InitSegment, error) {
	if videoInit.Moov.Trak == nil || audioInit.Moov.Trak == nil {
		return nil, fmt.Errorf("%w: init missing trak", ErrInputShape)
	}

	// Renumber.
	videoInit.Moov.Trak.Tkhd.TrackID = videoTrackID
	audioInit.Moov.Trak.Tkhd.TrackID = audioTrackID

	if videoInit.Moov.Mvex != nil && videoInit.Moov.Mvex.Trex != nil {
		videoInit.Moov.Mvex.Trex.TrackID = videoTrackID
	}
	if audioInit.Moov.Mvex != nil && audioInit.Moov.Mvex.Trex != nil {
		audioInit.Moov.Mvex.Trex.TrackID = audioTrackID
	}

	// Update the global next_track_id to sit past both.
	videoInit.Moov.Mvhd.NextTrackID = audioTrackID + 1

	// Stack: keep video's init as the base, splice in audio's trak +
	// audio's mvex children (trex/mehd) under the existing mvex box.
	videoInit.Moov.AddChild(audioInit.Moov.Trak)
	if audioInit.Moov.Mvex != nil {
		if videoInit.Moov.Mvex == nil {
			// Defensive: input lacked mvex but we'll need one to
			// describe both fragmented tracks. Build a minimal one.
			videoInit.Moov.Mvex = &mp4.MvexBox{}
			videoInit.Moov.AddChild(videoInit.Moov.Mvex)
		}
		if audioInit.Moov.Mvex.Trex != nil {
			videoInit.Moov.Mvex.AddChild(audioInit.Moov.Mvex.Trex)
		}
		if audioInit.Moov.Mvex.Mehd != nil {
			videoInit.Moov.Mvex.AddChild(audioInit.Moov.Mvex.Mehd)
		}
	}
	return videoInit, nil
}

// appendSamples pulls every FullSample from the input fragments and
// adds them to outFrag under newTrackID. trex carries the input
// track's default flags / sample duration; we pass it to
// GetFullSamples so per-trun fields with zero defaults resolve
// correctly.
func appendSamples(outFrag *mp4.Fragment, frags []*mp4.Fragment, trex *mp4.TrexBox, newTrackID uint32) error {
	for _, fr := range frags {
		samples, err := fr.GetFullSamples(trex)
		if err != nil {
			return err
		}
		for _, s := range samples {
			if err := outFrag.AddFullSampleToTrack(s, newTrackID); err != nil {
				return err
			}
		}
	}
	return nil
}
