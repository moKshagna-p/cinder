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
brew install cinder
```

Then just run:

```bash
cinder
```

Homebrew installs a prebuilt macOS binary from the tap, so end users do not need Go installed.

If you prefer a single command without tapping first:

```bash
brew install moKshagna-p/cinder/cinder
```

Optional enhancements after install:

```bash
# Real-time audio reactivity
brew install ffmpeg
CINDER_AUDIO_REACTIVE=1 cinder

# Richer now-playing metadata
brew install nowplaying-cli
cinder
```

---

## Requirements (manual build)

- macOS
- Go 1.22+
- Apple Music and/or Spotify installed (optional but required for metadata)
- `nowplaying-cli` (optional, recommended for better media session detection)
- `ffmpeg` (optional, required for live audio-reactive input)
- A loopback input such as BlackHole (optional, recommended if you want system-output reactivity instead of microphone reactivity)

## Run

```bash
make tidy
make run
```

Enable live audio reactivity from the default input device:

```bash
CINDER_AUDIO_REACTIVE=1 make run
```

Use a specific AVFoundation audio input device, such as a loopback device:

```bash
CINDER_AUDIO_DEVICE="BlackHole 2ch" make run
```

If `nowplaying-cli` is installed but not compatible with your macOS version, force AppleScript backend:

```bash
CINDER_NOWPLAYING_BACKEND=applescript make run
```

If you want the visuals to follow the actual music output, route your player audio into a loopback input and point `CINDER_AUDIO_DEVICE` at that device. If you only enable `CINDER_AUDIO_REACTIVE=1`, Cinder will use the current default input device.

Controls:

- `q`, `esc`, or `ctrl+c` to quit
- `m` to cycle animation modes (Nebula → Waveform → Spectrum → Vortex → Pulse)

## Build

```bash
make build
./bin/cinder
```
