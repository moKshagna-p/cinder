# Cinder

A fullscreen macOS terminal music visualizer in Go using Bubble Tea + Lip Gloss.

It polls Apple Music and Spotify via `osascript` every second, detects track + artist + playback state, and renders a live nebula-style particle field with song-derived colors.
If `nowplaying-cli` is installed, Cinder uses it first for more reliable now-playing detection and falls back to AppleScript automatically.

## Features

- Fullscreen terminal rendering with Bubble Tea AltScreen
- Cursor hidden for an immersive display
- Polling now-playing metadata from Apple Music and Spotify
- 260-particle physics system with:
  - velocity + damping
  - lifespan/decay
  - orbital drift and turbulence
  - dense glowing core
- Song-change explosion from center
- Rhythm-reactive motion layers (kick/snare/hat style pulses) with evolving sections per song
- Pause handling: particles gradually decelerate and freeze
- Resume handling: particles reignite and expand again
- Per-song color palettes hashed from title
- No chrome UI; only particles + dim now-playing text in bottom-left

## Project Structure

- `nowplaying/` — AppleScript polling and metadata parsing
- `visualizer/` — particle simulation and frame rendering
- `ui/` — Bubble Tea model, update loop, and terminal composition
- `config/` — palette generation and color utilities

## Requirements

- macOS
- Go 1.22+
- Apple Music and/or Spotify installed (optional but required for metadata)
- `nowplaying-cli` (optional, recommended for better media session detection)

## Run

```bash
make tidy
make run
```

If `nowplaying-cli` is installed but not compatible with your macOS version, force AppleScript backend:

```bash
CINDER_NOWPLAYING_BACKEND=applescript make run
```

Controls:

- `q`, `esc`, or `ctrl+c` to quit

## Build

```bash
make build
./bin/cinder
```
