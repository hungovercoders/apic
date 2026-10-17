package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// The picture. Terminal clips are recorded at the template's shape, 100
// columns by 31 lines at font size 16, which VHS draws at 1150x720 with a
// 60px pad; most of the pad is cropped and the rest scaled up onto a 1080p
// canvas, leaving a band at the bottom for the captions. VHS at that font
// size reports 105 columns for 1200px, which gives the cell size.
const (
	fps       = 30
	canvasW   = 1920
	canvasH   = 1080
	termW     = 1150
	termH     = 720
	wideW     = 2000 // apic list is 179 columns wide; this holds it unwrapped
	pad       = 60
	crop      = 40
	termScale = 1.5
	termY     = 16
	zoomMax   = 1.7
	bg        = "#1e1e2e"
)

var (
	cellW = float64(1200-2*pad) / 105
	cellH = 600.0 / 31
)

// termBox is where a clip lands on the canvas: its scaled size and offset.
func termBox() (w, h, x, y int) {
	w = int((termW-2*crop)*termScale) / 2 * 2
	h = int((termH-2*crop)*termScale) / 2 * 2
	return w, h, (canvasW - w) / 2, termY
}

// zoomWindow is how many terminal cells the push-in shows at full zoom.
func zoomWindow() (cols, rows int) {
	return int((canvasW/zoomMax - 30) / (cellW * termScale)), int((canvasH/zoomMax - 30) / (cellH * termScale))
}

// tapeHead is every clip's preamble; the body follows it.
const tapeHead = `Output %s
Set Shell "bash"
Set FontSize 16
Set Width %d
Set Height %d
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Framerate 30
Set Padding 60
`

func ffmpeg(args ...string) error {
	cmd := exec.Command("ffmpeg", append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)...) //nolint:gosec // ffmpeg on files this process wrote
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// duration is a media file's length in seconds.
func duration(path string) (float64, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output() //nolint:gosec // ffprobe on a file this process wrote
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

func encodeArgs() []string {
	return []string{"-r", fmt.Sprint(fps), "-c:v", "libx264", "-preset", "medium", "-crf", "18", "-pix_fmt", "yuv420p", "-an"}
}

// composeClip puts a terminal recording on the canvas and holds its last
// frame for `hold` seconds, so the shot lasts as long as its narration.
func composeClip(src, dst string, hold float64, wide bool) error {
	var cw, ch, sw, sh, x, y int
	if wide {
		cw, ch = wideW-2*crop, termH-2*crop
		sw = canvasW - 80
		sh = int(float64(ch)*float64(sw)/float64(cw)) / 2 * 2
		x, y = 40, 170
	} else {
		cw, ch = termW-2*crop, termH-2*crop
		sw, sh, x, y = termBox()
	}
	vf := fmt.Sprintf("fps=%d,crop=%d:%d:%d:%d,scale=%d:%d:flags=lanczos,pad=%d:%d:%d:%d:color=%s,tpad=stop_mode=clone:stop_duration=%.3f",
		fps, cw, ch, crop, crop, sw, sh, canvasW, canvasH, x, y, bg, hold)
	return ffmpeg(append([]string{"-i", src, "-vf", vf}, append(encodeArgs(), dst)...)...)
}

// lastFrame writes a clip's final frame as a PNG.
func lastFrame(src, png string) error {
	return ffmpeg("-sseof", "-0.1", "-i", src, "-frames:v", "1", "-update", "1", png)
}

// composeZoom pushes in on a canvas-sized frame over two seconds and
// holds. The window is anchored at the top-left of the terminal's content,
// so the command line stays in frame and the zoom reads as a push-in, not
// a search; Validate checks the target cell fits the window.
func composeZoom(frame, dst string, secs float64) error {
	_, _, tx, ty := termBox()
	px := float64(tx) + (pad-crop)*termScale - 30
	py := float64(ty) + (pad-crop)*termScale - 30
	n := int(secs * fps)
	ramp := fps * 2
	z := fmt.Sprintf("min(1+(%g-1)*on/%d,%g)", zoomMax, ramp, zoomMax)
	vf := fmt.Sprintf("zoompan=z='%s':x='%.1f*(1-1/zoom)/(1-1/%g)':y='%.1f*(1-1/zoom)/(1-1/%g)':d=%d:s=%dx%d:fps=%d",
		z, px, zoomMax, py, zoomMax, n, canvasW, canvasH, fps)
	return ffmpeg(append([]string{"-loop", "1", "-i", frame, "-vf", vf, "-frames:v", fmt.Sprint(n)}, append(encodeArgs(), dst)...)...)
}

// composeCard shows a rendered card for `secs`.
func composeCard(png, dst string, secs float64) error {
	return ffmpeg(append([]string{"-loop", "1", "-framerate", fmt.Sprint(fps), "-i", png, "-t", fmt.Sprintf("%.3f", secs),
		"-vf", fmt.Sprintf("fps=%d,format=yuv420p", fps)}, append(encodeArgs(), dst)...)...)
}

// concat joins same-encoded segments without re-encoding.
func concat(list []string, listFile, dst string) error {
	var b strings.Builder
	for _, p := range list {
		fmt.Fprintf(&b, "file '%s'\n", p)
	}
	if err := os.WriteFile(listFile, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return ffmpeg("-f", "concat", "-safe", "0", "-i", listFile, "-c", "copy", dst)
}

// withSilence adds a silent stereo track, so a voice recorded to the
// captions drops straight in and players treat the file as having audio.
func withSilence(src, dst string) error {
	return ffmpeg("-i", src, "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-shortest",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart", dst)
}

// burn renders the captions into the picture.
func burn(src, ass, dst string) error {
	return ffmpeg("-i", src, "-vf", "ass="+ass, "-c:v", "libx264", "-preset", "medium", "-crf", "18",
		"-pix_fmt", "yuv420p", "-c:a", "copy", "-movflags", "+faststart", dst)
}

// cropTerm cuts the terminal out of a composed segment's last frame, for
// the thumbnail.
func cropTerm(src, png string) error {
	w, h, x, y := termBox()
	return ffmpeg("-sseof", "-0.1", "-i", src, "-frames:v", "1", "-update", "1",
		"-vf", fmt.Sprintf("crop=%d:%d:%d:%d", w, h, x, y), png)
}

func srtTime(t float64) string {
	ms := int(math.Round(t * 1000))
	h, ms := ms/3600000, ms%3600000
	m, ms := ms/60000, ms%60000
	s, ms := ms/1000, ms%1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func assTime(t float64) string {
	cs := int(math.Round(t * 100))
	h, cs := cs/360000, cs%360000
	m, cs := cs/6000, cs%6000
	s, cs := cs/100, cs%100
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}

// ytTime is the m:ss YouTube's chapter list reads.
func ytTime(t float64) string {
	s := int(t)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// SRT is the captions as a sidecar file.
func SRT(cues []Cue) string {
	var b strings.Builder
	for i, c := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(c.Start), srtTime(c.End), c.Text)
	}
	return b.String()
}

// ASS is the captions styled for burning in: Inter at the bottom of the
// canvas, in the docs theme's text colour on a translucent box of the
// background colour, so a caption stays readable over a push-in whose
// output reaches the bottom of the frame.
func ASS(cues []Cue) string {
	var b strings.Builder
	b.WriteString("[Script Info]\nScriptType: v4.00+\nPlayResX: 1920\nPlayResY: 1080\nWrapStyle: 0\n\n")
	b.WriteString("[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	b.WriteString("Style: Cap,Inter,40,&H00F4D6CD,&H00FFFFFF,&H502E1E1E,&H502E1E1E,0,0,0,0,100,100,0,0,3,14,0,2,140,140,34,1\n\n")
	b.WriteString("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, c := range cues {
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Cap,,0,0,0,,%s\n", assTime(c.Start), assTime(c.End), c.Text)
	}
	return b.String()
}
