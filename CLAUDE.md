# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Cinder is a macOS-only fullscreen terminal music visualizer (Go, Bubble Tea + Lip Gloss). It polls now-playing metadata from Apple Music/Spotify and optionally analyzes live system audio via ffmpeg + a loopback device (BlackHole 2ch). The primary dev/test setup is: music routed through a Multi-Output Device into BlackHole 2ch, which cinder auto-detects and uses as its capture input.

## Commands

```bash
make run          # go run .
make build        # go build -o bin/cinder .
make fmt          # go fmt ./...
make tidy         # go mod tidy

go test ./...                              # all tests (only audioinput has tests)
go test ./audioinput -run TestName -v      # single test

go run . --normal                          # force non-audio-reactive mode
go run . --audio-device "BlackHole 2ch"    # explicit capture device
go run . --doctor                          # check ffmpeg/devices/nowplaying-cli
go run . --list-audio-devices
```

There is no linter configured. Visual changes can only be verified by running the app in a real terminal with music playing — there is no headless render test.

## Environment variables

- `CINDER_AUDIO_REACTIVE=1` / `CINDER_AUDIO_DEVICE=<name|index>` — set by main.go from flags; `ConfigFromEnv()` also auto-enables when a preferred loopback device (BlackHole etc., see `audioinput/devices.go` `preferredInputNames`) is detected.
- `CINDER_AUDIO_SAMPLE_RATE` (default 22050), `CINDER_AUDIO_FRAME_SIZE` (default 1024, rounded up to power of two).
- `CINDER_NOWPLAYING_BACKEND=applescript` — bypass nowplaying-cli.

## Architecture / data flow

Two independent input pipelines feed one simulation:

1. **Audio** (`audioinput/`): `Analyzer` spawns a goroutine that runs `ffmpeg -f avfoundation` as a subprocess, reading raw f32le mono PCM from stdout. `detectorState` (analyzer.go) does Hann window → FFT (fft.go, in-repo radix-2) → per-frame features: Level, Bass/Mid/Treble, spectral Flux, Onset (with adaptive noise floors), Centroid, autocorrelation BPM, a 256-sample rolling waveform, and 16 log-spaced spectrum bands. The UI polls `Snapshot()` (mutex-guarded copy) each frame — there is no push channel. Capture retries with fallback device-arg and sample-rate candidates; errors surface via `Features.Err`.

2. **Metadata** (`nowplaying/`): polled once per second; tries `nowplaying-cli` first, falls back to AppleScript (`osascript`) against Music/Spotify. `Info.SongKey()` (source|track|artist) is the song identity.

3. **UI** (`ui/model.go`): Bubble Tea model. Render tick at 30 fps; physics run at a fixed 120 Hz timestep via an accumulator (`simAccum`), so `System.Update(dt)` always gets dt = 1/120 — never assume variable dt there. The bottom terminal row is reserved for the footer HUD; the visualizer gets `height-1`. On song change: `SetSongSignature` + `PaletteFromSong` + `Explode()`.

4. **Simulation/render** (`visualizer/system.go`, ~1600 lines, the heart of the project): `System.Update` advances physics, `System.Render` dispatches to one of five modes (Nebula/Waveform/Spectrum/Vortex/Pulse, cycled with `m`). Each mode paints into a `[]pixel` float RGBA buffer, then `pixelBufToString` tone-maps and converts every cell to an ANSI truecolor escape + ASCII glyph (`glyphFor` picks `.:-*oO@` by density/luma).

## Key design conventions

- **Synthetic-first rhythm**: the visualizer must always animate, even with no audio. A synthetic beat clock (`songClock` × BPM derived from the song-title hash) drives `kick`/`snare`/`hat` envelopes; when `audio.Active`, real features are *blended over* the synthetic groove (see the `audioBlend` mixing in `Update`), never a hard switch. Every render mode has a synthetic fallback path (`synthSpec`, `synthWavePhase`).
- **Per-song determinism**: motion profile (`buildMotionProfile`) and palette (`config.PaletteFromSong`) are hashed from song title/artist (FNV / SHA-1), plus keyword heuristics ("remix" → faster, "ambient" → slower). Same song always looks the same.
- **Smoothing idiom**: time-based exponential blends `x += (target-x) * (1-exp(-dt*k))` for physics state; display bars/waveform use damped springs with asymmetric attack/release. Feature envelopes in `audioinput` use per-FFT-frame `smoothAttackRelease` coefficients (frame-rate dependent, not dt-based).
- **Terminal aspect ratio**: cells are ~2× taller than wide. Circular modes (Vortex, Pulse) correct with `aY = 2.0`; nebula spawn/void use ad-hoc y-squash factors (0.6, 1.25). Keep any new radial geometry aspect-corrected or circles render as tall ellipses.
- **All per-frame allocations matter**: `Render` runs 30×/sec over width×height cells; `Update` runs 120×/sec over 280 particles. Anything O(particles²) or allocating per cell is a performance hazard.
- `visualizer.AudioFeatures` mirrors `audioinput.Features` field-for-field (copied manually in ui/model.go) so the visualizer package doesn't depend on capture internals — keep them in sync when adding a feature.

## Gotchas

- `ui/model.go` has dead code kept intentionally (`songLabel`, `decoratePlaying`, `truncate`).
- A stale compiled binary `./cinder` sits untracked in the repo root; the Makefile builds to `bin/`.
- Release flow: `.github/workflows/release.yml` builds darwin binaries on tag push and updates the Homebrew tap (`moKshagna-p/cinder`, formula `cinder-tui`).
