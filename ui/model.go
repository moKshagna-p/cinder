package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cinder/config"
	"cinder/nowplaying"
	"cinder/visualizer"
)

const (
	particleCount = 420
	frameRate     = 30
)

type frameMsg time.Time
type pollMsg nowplaying.Info

type Model struct {
	width      int
	height     int
	vis        *visualizer.System
	now        nowplaying.Info
	lastSongID string
	lastFrame  time.Time
	pulse      float64
	flashUntil time.Time

	playingStyle lipgloss.Style
	pausedStyle  lipgloss.Style
	idleStyle    lipgloss.Style
	errStyle     lipgloss.Style
	footerStyle  lipgloss.Style
}

func NewModel() Model {
	return Model{
		vis: visualizer.NewSystem(particleCount),
		now: nowplaying.Info{Source: "none", State: "stopped", Playing: false},
		playingStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F5F5F5")).
			Bold(true),
		pausedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D0D0D0")),
		idleStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B0B0B0")),
		errStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF8F8F")).
			Bold(true),
		footerStyle: lipgloss.NewStyle().
			Width(0).
			Padding(0, 1).
			Background(lipgloss.Color("#121212")),
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
			return m, tea.Quit
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
		m.pulse += dt
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
			m.vis.SetSongSignature(songID)
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

	label, style := m.songLabel()
	if label == "" {
		label = ""
		style = m.idleStyle
	}

	maxLen := max(10, m.width-6)
	line := style.Render(truncate(label, maxLen))

	if time.Now().Before(m.flashUntil) {
		glow := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#2A2A2A")).
			Bold(true)
		line = glow.Render(truncate("* DETECTED *  "+label, maxLen))
	}
	footer := m.footerStyle.Width(m.width).Render(line)

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
	if m.now.Err != "" {
		errText := "osascript error: " + m.now.Err
		return errText, m.errStyle
	}

	if m.now.Track == "" && m.now.Artist == "" {
		if m.now.State == "paused" || m.now.State == "playing" {
			src := m.now.Source
			if src == "" || src == "none" {
				src = "player"
			}
			return src + " " + m.now.State + "  (no track metadata)", m.pausedStyle
		}
		return "no playback detected (Music/Spotify idle or permission blocked)", m.idleStyle
	}

	prefix := m.now.Source
	if prefix == "" || prefix == "none" {
		prefix = "Now Playing"
	}

	state := m.now.State
	if state == "" {
		state = "unknown"
	}

	if m.now.Artist == "" {
		label := fmt.Sprintf("%s [%s]  %s", prefix, state, m.now.Track)
		if m.now.Playing {
			return m.decoratePlaying(label), m.playingStyle
		}
		return label, m.pausedStyle
	}

	label := fmt.Sprintf("%s [%s]  %s - %s", prefix, state, m.now.Track, m.now.Artist)
	if m.now.Playing {
		return m.decoratePlaying(label), m.playingStyle
	}
	return label, m.pausedStyle
}

func (m Model) decoratePlaying(label string) string {
	dots := []string{".", "..", "...", "...."}
	idx := int(math.Mod(m.pulse*2.0, float64(len(dots))))
	if idx < 0 {
		idx = 0
	}
	return "LIVE" + dots[idx] + "  " + label
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
