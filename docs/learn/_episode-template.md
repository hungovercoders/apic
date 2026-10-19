# Episode template

The YouTube description, title and thumbnail conventions for the "From
zero to apic" series. Every lesson's episode is cut from this so the
series reads as one thing.

## Title

`From zero to apic #N: <lesson title>` for the numbered lessons, and
`From zero to apic: <topic>` for the occasional extra. Keep it under 60
characters so it survives the mobile crop.

## Thumbnail

The apic logo, the episode number, three to five words of the lesson
title, and one screenshot from the episode. The docs theme's purple on the
dark window background, so a row of thumbnails looks like one series.

## Description

Fill in every `<…>`. Keep the first line under 120 characters; it is the
line search results show.

```text
<One sentence: what you can do after this episode.>

This is lesson <N> of From zero to apic, a course on running API requests
from plain .http files in the terminal, in CI and from AI agents, with one
static binary. Every lesson runs against the fake API that ships inside
apic, so you can follow along with nothing but the binary.

📄 Lesson page, with every command to copy:
https://apic.sh/learn/<NN-slug>/

▶ Try it in 30 seconds:
    go install github.com/hungovercoders/apic/cmd/apic@latest
    apic ui --demo

Chapters
0:00 <Why>
<m:ss> <Step 1>
<m:ss> <Step 2>
<m:ss> Checkpoint
<m:ss> Exercise
<m:ss> Next lesson

Links
🏠 Docs: https://apic.sh/
📦 Source and issues: https://github.com/hungovercoders/apic
📚 The whole course: https://apic.sh/learn/
⬅ Previous: <link or "this is the first one">
➡ Next: <link or "coming soon">

apic is MIT licensed and built in the open. If something in this episode
does not work for you, the lesson page has a "Report a problem" link at
the bottom; the commands on it are checked in CI, so it is probably a
version difference, and the page says which version it was written for.
```

## Recording notes

- Every episode opens on its title card: the series label, the lesson's
  title and one command, then the hook. The row of episodes reads as one
  series that way, and a viewer who lands mid-course knows where they are
  before the first terminal appears. A lesson's cold open comes straight
  after the card. `task episodes:check` refuses a shot list that starts
  on anything else.
- One terminal, 100 columns by 30 rows, font size 16, dark background.
  The lesson's `docs/learn/tapes/NN.tape` (a [VHS](https://github.com/charmbracelet/vhs)
  script) reproduces the terminal parts on demand, so b-roll can be
  re-recorded without re-doing the voice.
- Say the command before typing it, then pause on the output for a beat
  longer than feels natural. Viewers read slower than you talk.
- Every episode ends on the checkpoint passing, then the one-line pitch
  for the next lesson.
- `task episodes` builds the whole episode from the shot list under
  `docs/learn/episodes/`, named by the lesson's number: the cards, the clips (one VHS
  tape each, against the demo API), the push-ins, and the captions from
  the lines the voice says, which set each shot's length. It writes the
  cut with and without captions, the SRT, the title, chapters and this
  description filled in, the narration script and the thumbnail.
- The voice, two ways. Voice first: record one clip per line, reading
  from the narration script, which numbers each line as the SRT does, and
  save them under a directory named by the episode's number with the
  three-digit cue number as the file name (001 and up, in any format
  ffmpeg reads; a text-to-speech tool that writes one file a line fits
  the same shape). Then `task episodes VOICE=<directory>` times each shot
  to its clips plus a short beat instead of the word count, so the cut
  fits the voice, puts the clips on the audio track at their cues and
  writes voiced copies of both cuts beside the silent ones. A missing or
  surplus clip stops the build naming the line, before anything renders.
  Picture first: build the episode silent, record the narration in one
  take to the script's timestamps, and `task episodes:voice EPISODE=NN
  VOICE=<file>` muxes the take onto both cuts, padded to the picture.
  Keep recordings out of the repository; a directory called voice at the
  root is ignored by git.
- The builder records against the demo API only: a lesson's calls to a
  real service are cards carrying the page's sample output, and a
  lesson that works on the repository's example projects copies them
  into the scratch directory in a setup step, which sees the repository
  root as `EPISODE_REPO`.
