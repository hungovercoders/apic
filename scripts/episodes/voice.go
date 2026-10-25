package main

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A recorded voice. Without one, a shot runs as long as its lines take to
// say at a reading pace, and the narration is recorded afterwards to the
// script's timestamps. With one, the recording comes first and the cut is
// timed to it: a directory holds one clip per line, under the episode's
// name and numbered like the SRT's cues (three digits, so the first line
// of lesson 4 is 001 under 04, in any format ffmpeg reads), which suits
// recording line by line from the narration script and a text-to-speech
// tool that writes one file a line. Each shot then runs as long as its
// clips plus a beat after each, or as long as its terminal clip, whichever
// is longer, and the clips land on the audio track at their cues' starts.

// voicePad is the beat left after each recorded line.
const voicePad = 0.7

// Voice is an episode's narration as one clip per line, in cue order.
type Voice struct {
	Clips []string  // each line's clip
	Secs  []float64 // and how long it runs
}

// probe is a clip's length in seconds; tests stand in for ffprobe.
var probe = duration

// LoadVoice reads the clips for one episode from dir/NN: every file named
// by a cue number, in whatever format, since ffprobe decides what ffmpeg
// can read and fails on what it cannot. No such directory means no voice,
// and the episode builds silent as it would without one; a directory with
// a clip missing, two for one cue, or one past the last line fails naming
// the cue and its line, so a half-recorded lesson is obvious before
// anything is rendered.
func LoadVoice(dir string, ep *Episode) (*Voice, error) {
	if dir == "" {
		return nil, nil
	}
	epDir := filepath.Join(dir, ep.Name)
	entries, err := os.ReadDir(epDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := ep.Lines()
	byCue := map[int]string{}
	var errs []error
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		n, err := strconv.Atoi(strings.TrimSuffix(name, filepath.Ext(name)))
		if err != nil || n < 1 {
			continue
		}
		switch prev, dup := byCue[n]; {
		case dup:
			errs = append(errs, fmt.Errorf("%s: two clips for cue %03d, %s and %s", epDir, n, filepath.Base(prev), name))
		case n > len(lines):
			errs = append(errs, fmt.Errorf("%s: %s is cue %03d, past the last line; the episode has %d", epDir, name, n, len(lines)))
		default:
			byCue[n] = filepath.Join(epDir, name)
		}
	}
	v := &Voice{Clips: make([]string, len(lines)), Secs: make([]float64, len(lines))}
	for i, line := range lines {
		p, ok := byCue[i+1]
		if !ok {
			errs = append(errs, fmt.Errorf("%s: no clip %03d for the line %q", epDir, i+1, line))
			continue
		}
		secs, err := probe(p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		v.Clips[i], v.Secs[i] = p, secs
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return v, nil
}

// Total is how long the recorded lines run with their beats.
func (v *Voice) Total() float64 {
	var t float64
	for _, s := range v.Secs {
		t += s + voicePad
	}
	return t
}

// Lines is every line of the episode in cue order, so cue n is Lines()[n-1].
func (ep *Episode) Lines() []string {
	var lines []string
	for _, s := range ep.Shots {
		lines = append(lines, s.Say...)
	}
	return lines
}

// withVoice puts the recorded lines on the audio track of a cut, each at
// its cue's start, and writes the result. The clips are brought to one
// format and mixed without scaling, so each plays at the level it was
// recorded; the track is padded to the picture.
func withVoice(src string, cues []Cue, dst string) error {
	args := []string{"-i", src}
	var filter, mix strings.Builder
	n := 0
	for _, c := range cues {
		if c.Audio == "" {
			continue
		}
		n++
		args = append(args, "-i", c.Audio)
		fmt.Fprintf(&filter, "[%d:a]aformat=sample_rates=48000:channel_layouts=stereo,adelay=%d:all=1[a%d];", n, int(math.Round(c.Start*1000)), n)
		fmt.Fprintf(&mix, "[a%d]", n)
	}
	if n == 0 {
		return errors.New("no recorded lines to mix")
	}
	fmt.Fprintf(&filter, "%samix=inputs=%d:normalize=0:dropout_transition=0,apad[voice]", mix.String(), n)
	return ffmpeg(append(args, "-filter_complex", filter.String(), "-map", "0:v", "-map", "[voice]", "-shortest",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", dst)...)
}

// mux replaces a cut's audio with a narration recorded in one take to the
// script's timestamps, padded to the picture so a take that ends early
// does not cut the video short.
func mux(video, audio, dst string) error {
	return ffmpeg("-i", video, "-i", audio, "-map", "0:v", "-map", "1:a",
		"-af", "aformat=sample_rates=48000:channel_layouts=stereo,apad", "-shortest",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", dst)
}

// Mux writes the voiced cuts of an already built episode from one
// narration file: NN-clean-voiced.mp4 and NN-voiced.mp4 beside the silent
// NN-clean.mp4 and NN.mp4 under out/NN.
func Mux(ep *Episode, out, audio string) error {
	dir := filepath.Join(out, ep.Name)
	for _, cut := range []string{"-clean", ""} {
		src := filepath.Join(dir, ep.Name+cut+".mp4")
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("%s is not built; run the build first: %w", src, err)
		}
		if err := mux(src, audio, filepath.Join(dir, ep.Name+cut+"-voiced.mp4")); err != nil {
			return err
		}
	}
	fmt.Printf("%s: wrote %s-clean-voiced.mp4 and %s-voiced.mp4 in %s\n", ep.Name, ep.Name, ep.Name, dir)
	return nil
}
