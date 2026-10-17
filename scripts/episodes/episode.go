package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hungovercoders/apic/internal/lessons"
	"github.com/hungovercoders/apic/scripts/internal/record"
)

// Where the episodes, the course and the description template live,
// relative to the repository root.
const (
	EpisodesDir = "docs/learn/episodes"
	Template    = "docs/learn/_episode-template.md"
)

// Episode is one lesson's episode: the shots in order, what the voice
// says over each, and the cards between the terminal clips. It is read
// from docs/learn/episodes/NN.yaml; the lesson page supplies the title
// and the slug.
type Episode struct {
	Lesson    int       `yaml:"lesson"`
	Summary   string    `yaml:"summary"`
	Project   bool      `yaml:"project"`
	Thumbnail Thumbnail `yaml:"thumbnail"`
	Shots     []Shot    `yaml:"shots"`

	Name  string `yaml:"-"` // "01"
	Slug  string `yaml:"-"` // "01-first-request"
	Title string `yaml:"-"` // the page title without its "Lesson N: " prefix
}

// Thumbnail is what the YouTube thumbnail shows beside the logo and the
// episode number: a few words of the title, and a frame from a clip.
type Thumbnail struct {
	Words string `yaml:"words"`
	Clip  string `yaml:"clip"`
}

// Shot is one piece of the episode: a card, a terminal clip or a zoom on
// the last frame of the previous clip. Its lines are what the voice says
// over it, one caption each; they set how long the shot runs.
type Shot struct {
	Chapter string   `yaml:"chapter"`
	Say     []string `yaml:"say"`
	Card    *Card    `yaml:"card"`
	Clip    string   `yaml:"clip"`
	Tape    string   `yaml:"tape"`
	Wide    bool     `yaml:"wide"`
	Setup   []Setup  `yaml:"setup"`
	Zoom    *Zoom    `yaml:"zoom"`
}

// Card is a full-frame slide rendered from HTML: a series label and
// gradient title (the title and next-lesson cards), or a heading with a
// command grid, a table or a block of text, and a footer line. Backticks
// in text, table cells and the footer mark highlighted spans.
type Card struct {
	Series   string     `yaml:"series"`
	Title    string     `yaml:"title"`
	Heading  string     `yaml:"heading"`
	Commands []Command  `yaml:"commands"`
	Table    [][]string `yaml:"table"`
	Text     string     `yaml:"text"`
	Foot     string     `yaml:"foot"`
}

// Command is one cell of a card's command grid.
type Command struct {
	Label   string `yaml:"label"`
	Command string `yaml:"command"`
}

// Setup is state a clip needs before it is recorded: a shell snippet, or
// runnable blocks of the lesson page (1-based, in page order), optionally
// with a text substitution applied to them first.
type Setup struct {
	Sh      string   `yaml:"sh"`
	Blocks  []int    `yaml:"blocks"`
	Replace []string `yaml:"replace"`
}

// Zoom names the terminal cell (1-based column and row) the push-in on
// the previous clip's last frame must keep in view.
type Zoom struct {
	Col int `yaml:"col"`
	Row int `yaml:"row"`
}

// Kind is "card", "clip" or "zoom", or "" when the shot is none or more
// than one of them.
func (s Shot) Kind() string {
	var kinds []string
	if s.Card != nil {
		kinds = append(kinds, "card")
	}
	if s.Clip != "" {
		kinds = append(kinds, "clip")
	}
	if s.Zoom != nil {
		kinds = append(kinds, "zoom")
	}
	if len(kinds) != 1 {
		return ""
	}
	return kinds[0]
}

// Load reads one episode and fills in what the lesson page supplies.
func Load(root, path string) (*Episode, error) {
	data, err := os.ReadFile(path) //nolint:gosec // an episode of this repository, by glob
	if err != nil {
		return nil, err
	}
	var ep Episode
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&ep); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	ep.Name = strings.TrimSuffix(filepath.Base(path), ".yaml")
	page, err := lessons.Page(root, ep.Name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	ep.Slug = strings.TrimSuffix(filepath.Base(page), ".md")
	src, err := os.ReadFile(page) //nolint:gosec // a course page, by glob
	if err != nil {
		return nil, err
	}
	m := reTitle.FindSubmatch(src)
	if m == nil {
		return nil, fmt.Errorf("%s: no \"Lesson N: title\" in the front matter", page)
	}
	ep.Title = string(m[1])
	if err := ep.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// A setup step naming a block the lesson does not have, or text that
	// is not in it, would otherwise fail minutes into a recording.
	blocks, err := lessons.Blocks(root, ep.Name)
	if err != nil {
		return nil, err
	}
	for _, s := range ep.Shots {
		for _, st := range s.Setup {
			if _, err := st.step(ep.Name).Code(blocks); err != nil {
				return nil, fmt.Errorf("%s: clip %s: %w", path, s.Clip, err)
			}
		}
	}
	return &ep, nil
}

var reTitle = regexp.MustCompile(`(?m)^title: "Lesson \d+: (.+)"$`)

// LoadAll reads every episode, in lesson order.
func LoadAll(root string) ([]*Episode, error) {
	paths, err := filepath.Glob(filepath.Join(root, EpisodesDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var eps []*Episode
	for _, p := range paths {
		ep, err := Load(root, p)
		if err != nil {
			return nil, err
		}
		eps = append(eps, ep)
	}
	return eps, nil
}

// Validate checks the shot list for what would otherwise fail halfway
// through a recording: a shot that is not exactly one kind, a zoom with no
// clip before it or on a cell the push-in cannot keep in frame, a line the
// caption renderer would misread.
func (ep *Episode) Validate() error {
	var errs []error
	if ep.Lesson < 1 {
		errs = append(errs, errors.New("lesson must be a positive number"))
	}
	if fmt.Sprintf("%02d", ep.Lesson) != ep.Name {
		errs = append(errs, fmt.Errorf("lesson %d does not match the file name %s", ep.Lesson, ep.Name))
	}
	if strings.TrimSpace(ep.Summary) == "" {
		errs = append(errs, errors.New("summary (the first line of the description) is empty"))
	}
	if len(ep.Shots) == 0 {
		errs = append(errs, errors.New("no shots"))
	}
	clips := map[string]bool{}
	wide := map[string]bool{}
	lastWide := false
	for i, s := range ep.Shots {
		at := fmt.Sprintf("shot %d", i+1)
		if s.Chapter == "" {
			errs = append(errs, fmt.Errorf("%s: no chapter", at))
		}
		if len(s.Say) == 0 {
			errs = append(errs, fmt.Errorf("%s: nothing to say", at))
		}
		for _, line := range s.Say {
			if strings.ContainsAny(line, "{}\\\n") {
				errs = append(errs, fmt.Errorf("%s: a line holds a brace, backslash or newline, which the caption renderer would misread: %q", at, line))
			}
		}
		switch s.Kind() {
		case "card":
			errs = append(errs, s.validateCard(at)...)
		case "clip":
			errs = append(errs, s.validateClip(at, clips)...)
			clips[s.Clip], wide[s.Clip], lastWide = true, s.Wide, s.Wide
		case "zoom":
			errs = append(errs, s.validateZoom(at, len(clips) > 0, lastWide)...)
		default:
			errs = append(errs, fmt.Errorf("%s: a shot is exactly one of card, clip or zoom", at))
		}
	}
	if ep.Thumbnail.Words == "" || !clips[ep.Thumbnail.Clip] {
		errs = append(errs, errors.New("thumbnail needs words and the name of a clip"))
	} else if wide[ep.Thumbnail.Clip] {
		errs = append(errs, fmt.Errorf("thumbnail: clip %q is wide; the thumbnail cuts the standard terminal out of its frame", ep.Thumbnail.Clip))
	}
	return errors.Join(errs...)
}

// clipName is what a clip may be called: it is spliced into file names.
var clipName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (s Shot) validateCard(at string) []error {
	var errs []error
	if s.Card.Title == "" && s.Card.Heading == "" {
		errs = append(errs, fmt.Errorf("%s: a card needs a title or a heading", at))
	}
	if s.Tape != "" || s.Wide || len(s.Setup) > 0 {
		errs = append(errs, fmt.Errorf("%s: tape, wide and setup belong to a clip", at))
	}
	return errs
}

func (s Shot) validateClip(at string, clips map[string]bool) []error {
	var errs []error
	if !clipName.MatchString(s.Clip) {
		errs = append(errs, fmt.Errorf("%s: clip name %q must be lower-case letters, digits and hyphens", at, s.Clip))
	}
	if clips[s.Clip] {
		errs = append(errs, fmt.Errorf("%s: another clip is already named %q", at, s.Clip))
	}
	if strings.TrimSpace(s.Tape) == "" {
		errs = append(errs, fmt.Errorf("%s: clip %q has no tape", at, s.Clip))
	}
	for _, st := range s.Setup {
		if (st.Sh == "") == (len(st.Blocks) == 0) {
			errs = append(errs, fmt.Errorf("%s: a setup step is a shell snippet or lesson blocks, not both or neither", at))
		}
		if len(st.Replace) != 0 && len(st.Replace) != 2 {
			errs = append(errs, fmt.Errorf("%s: replace is [old, new]", at))
		}
	}
	return errs
}

func (s Shot) validateZoom(at string, afterClip, afterWide bool) []error {
	var errs []error
	if !afterClip {
		errs = append(errs, fmt.Errorf("%s: a zoom needs a clip before it", at))
	}
	if afterWide {
		errs = append(errs, fmt.Errorf("%s: the clip before a zoom cannot be wide; the push-in assumes the standard terminal", at))
	}
	if s.Tape != "" || s.Wide || len(s.Setup) > 0 {
		errs = append(errs, fmt.Errorf("%s: tape, wide and setup belong to a clip", at))
	}
	if cols, rows := zoomWindow(); s.Zoom.Col > cols || s.Zoom.Row > rows || s.Zoom.Col < 1 || s.Zoom.Row < 1 {
		errs = append(errs, fmt.Errorf("%s: zoom target (%d,%d) is outside the %dx%d window the push-in shows", at, s.Zoom.Col, s.Zoom.Row, cols, rows))
	}
	return errs
}

// step is the setup step as the shared recorder runs it.
func (s Setup) step(lesson string) record.Step {
	st := record.Step{Sh: s.Sh, Lesson: lesson, Blocks: s.Blocks}
	if len(s.Replace) == 2 {
		st.Replace = [2]string{s.Replace[0], s.Replace[1]}
	}
	return st
}

// sayTime is how long a line takes to say at about 150 words a minute,
// with a beat after it: the template says to pause for longer than feels
// natural, because viewers read slower than you talk.
func sayTime(line string) float64 {
	words := float64(len(strings.Fields(line)))
	return max(2.4, words/2.5+0.9)
}

// Cue is one caption: when it starts and ends, and the line.
type Cue struct {
	Start, End float64
	Text       string
}

// spread lays a shot's lines over its length: each in proportion to how
// long it takes to say, stretched when the clip runs longer than the
// narration.
func spread(start, secs float64, lines []string) []Cue {
	var want float64
	for _, l := range lines {
		want += sayTime(l)
	}
	cues := make([]Cue, 0, len(lines))
	t := start
	for _, l := range lines {
		d := sayTime(l) * secs / want
		cues = append(cues, Cue{Start: t, End: t + d, Text: l})
		t += d
	}
	return cues
}

// Chapter is a YouTube chapter: where it starts and its name.
type Chapter struct {
	Start float64
	Title string
}

// Description fills the template's description in for this episode.
func (ep *Episode) Description(root string, chapters []Chapter, prev *Episode) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, Template)) //nolint:gosec // the template, a file of this repository
	if err != nil {
		return "", err
	}
	_, rest, ok := strings.Cut(string(data), "```text\n")
	body, _, ok2 := strings.Cut(rest, "\n```")
	if !ok || !ok2 {
		return "", fmt.Errorf("%s has no ```text block to fill", Template)
	}
	head, tail, ok := strings.Cut(body, "Chapters\n")
	_, after, ok2 := strings.Cut(tail, "\n\nLinks")
	if !ok || !ok2 {
		return "", fmt.Errorf("%s: the description's Chapters block is not where it was", Template)
	}
	var ch strings.Builder
	for _, c := range chapters {
		fmt.Fprintf(&ch, "%s %s\n", ytTime(c.Start), c.Title)
	}
	body = head + "Chapters\n" + strings.TrimSuffix(ch.String(), "\n") + "\n\nLinks" + after
	previous := `this is the first one`
	if prev != nil {
		previous = "https://apic.sh/learn/" + prev.Slug + "/"
	}
	r := strings.NewReplacer(
		"<One sentence: what you can do after this episode.>", strings.TrimSpace(ep.Summary),
		"<N>", fmt.Sprint(ep.Lesson),
		"<NN-slug>", ep.Slug,
		`<link or "this is the first one">`, previous,
		`<link or "coming soon">`, "coming soon",
	)
	out := r.Replace(body)
	if strings.Contains(out, "<") {
		return "", fmt.Errorf("the template has a placeholder this generator does not fill: %s", out[strings.Index(out, "<"):])
	}
	return out + "\n", nil
}

// VideoTitle is the YouTube title, per the template.
func (ep *Episode) VideoTitle() string {
	return fmt.Sprintf("From zero to apic #%d: %s", ep.Lesson, ep.Title)
}

// Narration is the script to read: every line with the time it starts,
// under its chapter.
func (ep *Episode) Narration(cues []Cue, chapters []Chapter, total float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", ep.VideoTitle())
	fmt.Fprintf(&b, "Narration script. Runs %s. Each line is one caption in `%s.srt`; the timestamp is where it starts in `%s-clean.mp4`, which has a silent track the voice replaces. Say each line as it appears and the cut stays in sync.\n", ytTime(total), ep.Name, ep.Name)
	ci := 0
	for _, c := range cues {
		for ci < len(chapters) && chapters[ci].Start <= c.Start+0.01 {
			fmt.Fprintf(&b, "\n## %s %s\n\n", ytTime(chapters[ci].Start), chapters[ci].Title)
			ci++
		}
		fmt.Fprintf(&b, "- **%s** %s\n", ytTime(c.Start), c.Text)
	}
	return b.String()
}
