# Cinder

A fullscreen macOS terminal music visualizer in Go using Bubble Tea + Lip Gloss.

It polls Apple Music and Spotify via `osascript` every second, detects track + artist + playback state, and renders a live nebula-style particle field with song-derived colors.
If `nowplaying-cli` is installed, Cinder uses it first for more reliable now-playing detection and falls back to AppleScript automatically.
For true audio-reactive motion, Cinder can also analyze a live AVFoundation audio input via `ffmpeg`.

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
- Optional live audio-reactive input with onset, bass/treble energy, and rolling BPM estimation
- Per-song motion profiles that change pace, trail length, orbit count, and pulse sharpness
- Tempo-shaped synthetic rhythm layers: fast tracks feel tighter and faster, slower tracks drift and breathe more
- Cleaner trippy ribbon fields with stronger section shifts and less muddy overlap
- Pause handling: particles gradually decelerate and freeze
- Resume handling: particles reignite and expand again
- Per-song color palettes hashed from title
- No chrome UI; only particles + dim now-playing text in bottom-left

## Project Structure

- `nowplaying/` — AppleScript polling and metadata parsing
- `visualizer/` — particle simulation and frame rendering
- `ui/` — Bubble Tea model, update loop, and terminal composition
- `config/` — palette generation and color utilities

## Install

```bash
brew tap moKshagna-p/cinder
brew install cinder-tui
```

Then just run:

```bash
cinder
```

If BlackHole is installed and available, `cinder` will automatically use it for the audio-reactive version. If you want the original non-audio-reactive view, run:

```bash
cinder --normal
```

Homebrew installs a prebuilt macOS binary from the tap, so end users do not need Go installed.

If you prefer a single command without tapping first:

```bash
brew install moKshagna-p/cinder/cinder-tui
```

Optional enhancements after install:

```bash
# One-time audio-reactive setup
cinder --setup-audio
cinder --doctor
cinder

# Richer now-playing metadata
brew install nowplaying-cli
cinder
```

`ffmpeg` is installed automatically with `cinder-tui`. For full system-audio reactivity, users only need to install and configure `BlackHole 2ch` once. After that, plain `cinder` will automatically launch the BlackHole-driven version when it is available.

---

## Requirements (manual build)

- macOS
- Go 1.22+
- Apple Music and/or Spotify installed (optional but required for metadata)
- `nowplaying-cli` (optional, recommended for better media session detection)
- `ffmpeg` (required for live audio-reactive input, installed automatically by Homebrew)
- A loopback input such as BlackHole (optional, recommended if you want system-output reactivity instead of microphone reactivity)

## Run

```bash
make tidy
make run
```

After BlackHole is configured once, plain `cinder` will launch the audio-reactive version automatically:

```bash
go run .
```

Force the original non-audio-reactive view:

```bash
go run . --normal
```

Use a specific AVFoundation audio input device explicitly:

```bash
go run . --audio-device "BlackHole 2ch"
```

List available audio devices:

```bash
go run . --list-audio-devices
```

Check whether `ffmpeg`, loopback inputs, and metadata helpers are available:

```bash
go run . --doctor
```

If `nowplaying-cli` is installed but not compatible with your macOS version, force AppleScript backend:

```bash
CINDER_NOWPLAYING_BACKEND=applescript make run
```

If you want the visuals to follow the actual music output, route your player audio into a loopback input. When Cinder sees a preferred loopback device such as `BlackHole 2ch`, it will automatically choose the audio-reactive version on startup.

Controls:

- `q`, `esc`, or `ctrl+c` to quit
- `m` to cycle animation modes (Nebula → Waveform → Spectrum → Vortex → Pulse)

## Build

```bash
make build
./bin/cinder
```
