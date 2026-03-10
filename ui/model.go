package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cinder/audioinput"
	"cinder/config"
	"cinder/nowplaying"
	"cinder/visualizer"
)

const (
	particleCount = 280
	frameRate     = 30
)

type frameMsg time.Time
type pollMsg nowplaying.Info

type Model struct {
	width      int
	height     int
	vis        *visualizer.System
	audio      *audioinput.Analyzer
	now        nowplaying.Info
	lastSongID string
	lastFrame  time.Time
	flashUntil time.Time

	playingStyle lipgloss.Style
	pausedStyle  lipgloss.Style
	idleStyle    lipgloss.Style
	errStyle     lipgloss.Style
	footerStyle  lipgloss.Style
}

func NewModel() Model {
	audio := audioinput.NewAnalyzer(audioinput.ConfigFromEnv())
	return Model{
		vis:   visualizer.NewSystem(particleCount),
		audio: audio,
		now:   nowplaying.Info{Source: "none", State: "stopped", Playing: false},
		playingStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E8E8E8")),
		pausedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")),
		idleStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#444444")),
		errStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")),
		footerStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("#000000")),
	}
}

func (m Model) Init() tea.Cmd {
	m.lastFrame = time.Now()
	return tea.Batch(tea.HideCursor, frameTick(), pollTick())
}

func frameTick() tea.Cmd {
	return tea.Tick(time.Second/time.Duration(frameRate), func(t time.Time) tea.Msg {
		return frameMsg(t)
	})
}

func pollTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return pollMsg(nowplaying.Poll())
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		s := msg.String()
		if s == "ctrl+c" || s == "q" || s == "esc" {
			if m.audio != nil {
				m.audio.Close()
			}
			return m, tea.Quit
		}
		if s == "m" || s == "M" {
			m.vis.NextMode()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		visH := m.height
		if visH > 1 {
			visH = m.height - 1
		}
		m.vis.Resize(m.width, visH)

	case frameMsg:
		now := time.Time(msg)
		dt := now.Sub(m.lastFrame).Seconds()
		if dt <= 0 || dt > 0.2 {
			dt = 1.0 / frameRate
		}
		m.lastFrame = now
		if m.audio != nil {
			audio := m.audio.Snapshot()
			m.vis.SetAudioFeatures(visualizer.AudioFeatures{
				Active:      audio.Active,
				Level:       audio.Level,
				Bass:        audio.Bass,
				Treble:      audio.Treble,
				MidRange:    audio.MidRange,
				Flux:        audio.Flux,
				BPM:         audio.BPM,
				WaveformBuf: audio.WaveformBuf,
				Spectrum:    audio.Spectrum,
			})
		}
		m.vis.Update(dt)
		return m, frameTick()

	case pollMsg:
		info := nowplaying.Info(msg)
		prev := m.now
		m.now = info
		m.vis.SetPlaying(info.Playing)

		songID := info.SongKey()
		if songID != m.lastSongID && info.Track != "" {
			m.lastSongID = songID
			m.vis.SetSongSignature(songID, info.Track, info.Artist)
			m.vis.SetPalette(config.PaletteFromSong(info.Track))
			m.vis.Explode()
			m.flashUntil = time.Now().Add(700 * time.Millisecond)
		}

		if !prev.Playing && info.Playing {
			m.vis.Explode()
			m.flashUntil = time.Now().Add(500 * time.Millisecond)
		}

		return m, pollTick()
	}

	return m, nil
}

func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	frame := m.vis.Render()

	// ── right-side HUD ───────────────────────────────────────────────────────
	// Compose three segments that stack right-aligned in one footer line:
	//   [track — artist]   ·   [mode]
	// Everything sits on pure black, separated by a dim mid-dot.

	modeStr := m.vis.Mode().String()

	// mode pill: dim, small
	modePill := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#3A3A3A")).
		Render(strings.ToLower(modeStr))

	// separator dot
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#2A2A2A")).
		Render("  ·  ")

	// song info
	track, artist := m.now.Track, m.now.Artist
	var songSegment string
	switch {
	case m.now.Err != "":
		songSegment = m.errStyle.Render("err")
	case track == "" && artist == "":
		songSegment = m.idleStyle.Render("—")
	case artist == "":
		songSegment = m.playingStyle.Render(track)
	default:
		trackPart := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C8C8C8")).
			Render(track)
		artistPart := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555")).
			Render(artist)
		dimDash := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#333333")).
			Render("  —  ")
		songSegment = trackPart + dimDash + artistPart
	}

	// paused indicator (subtle, no "LIVE")
	if !m.now.Playing && track != "" {
		pausedDot := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3A3A3A")).
			Render("⏸  ")
		songSegment = pausedDot + songSegment
	}

	// flash on song change: brief bright highlight on the track name only
	if time.Now().Before(m.flashUntil) && track != "" {
		songSegment = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Render(track) +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")).Render("  —  ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render(artist)
	}

	// build the right-aligned block
	// measure plain lengths for padding (strip ANSI would be ideal but lipgloss
	// Width() on a rendered string handles it)
	rightContent := songSegment + sep + modePill + "  "

	// right-pad with spaces so it hugs the right edge
	rightBlock := lipgloss.NewStyle().
		Background(lipgloss.Color("#000000")).
		Width(m.width).
		Align(lipgloss.Right).
		Render(rightContent)

	footer := m.footerStyle.Width(m.width).Render(rightBlock)

	if frame == "" {
		blank := strings.Repeat("\n", max(0, m.height-1))
		return blank + footer
	}

	if strings.HasSuffix(frame, "\x1b[0m") {
		frame = strings.TrimSuffix(frame, "\x1b[0m")
	}

	return frame + "\n" + footer + "\x1b[0m"
}

func (m Model) songLabel() (string, lipgloss.Style) {
	// kept for compatibility — not used by the new View()
	return m.now.Track, m.playingStyle
}

func (m Model) decoratePlaying(_ string) string {
	// no-op — "LIVE" removed
	return m.now.Track
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
