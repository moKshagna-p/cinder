package nowplaying

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Info struct {
	Source  string
	Track   string
	Artist  string
	State   string
	Playing bool
	Err     string
}

func (i Info) SongKey() string {
	return i.Source + "|" + i.Track + "|" + i.Artist
}

func sanitizeMetadata(s string) string {
	r := strings.NewReplacer("|", "/", "\n", " ", "\r", " ", "\t", " ")
	s = r.Replace(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func parsePlaybackState(rateRaw string, hasMetadata bool) string {
	rateRaw = strings.TrimSpace(rateRaw)
	if rateRaw == "" || strings.EqualFold(rateRaw, "null") {
		if hasMetadata {
			return "paused"
		}
		return "stopped"
	}

	r, err := strconv.ParseFloat(rateRaw, 64)
	if err != nil {
		if hasMetadata {
			return "paused"
		}
		return "stopped"
	}
	if r > 0 {
		return "playing"
	}
	if hasMetadata {
		return "paused"
	}
	return "stopped"
}

func pollNowPlayingCLI() (Info, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nowplaying-cli", "get", "artist", "title", "playbackRate", "isMusicApp")
	out, err := cmd.Output()
	if err != nil {
		return Info{}, false
	}

	rawLines := strings.Split(strings.TrimSpace(string(out)), "\n")
	lines := make([]string, 0, 4)
	for _, l := range rawLines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		lines = append(lines, l)
	}
	if len(lines) < 4 {
		return Info{}, false
	}
	lines = lines[len(lines)-4:]

	artist := sanitizeMetadata(lines[0])
	track := sanitizeMetadata(lines[1])
	playbackRate := strings.TrimSpace(lines[2])
	isMusicApp := strings.TrimSpace(lines[3])

	if strings.EqualFold(artist, "null") {
		artist = ""
	}
	if strings.EqualFold(track, "null") {
		track = ""
	}

	source := "System"
	if isMusicApp == "1" || strings.EqualFold(isMusicApp, "true") {
		source = "Music"
	}

	hasMetadata := artist != "" || track != ""
	state := parsePlaybackState(playbackRate, hasMetadata)
	info := Info{
		Source:  source,
		Track:   track,
		Artist:  artist,
		State:   state,
		Playing: state == "playing",
		Err:     "",
	}

	if !hasMetadata && state == "stopped" {
		return info, false
	}
	return info, true
}

func appSeemsInstalled(appName string) bool {
	if appName == "Music" {
		return true
	}
	paths := []string{filepath.Join("/Applications", appName+".app")}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, "Applications", appName+".app"))
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func pollAppViaAppleScript(appName string) (Info, bool, string) {
	script := fmt.Sprintf(`
set pstate to "stopped"
set tn to ""
set ar to ""

try
	if application %q is running then
		tell application %q
			try
				set pstate to (player state as string)
			end try
			try
				set tn to (name of current track as string)
			end try
			try
				set ar to (artist of current track as string)
			end try
		end tell
	end if
end try

return pstate & "|" & tn & "|" & ar
`, appName, appName)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		errText := strings.TrimSpace(string(out))
		if errText == "" {
			errText = err.Error()
		}
		return Info{}, false, errText
	}

	line := strings.TrimSpace(string(out))
	parts := strings.SplitN(line, "|", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}

	state := strings.ToLower(strings.TrimSpace(parts[0]))
	if state == "" {
		state = "stopped"
	}
	track := sanitizeMetadata(parts[1])
	artist := sanitizeMetadata(parts[2])
	if strings.EqualFold(track, "null") {
		track = ""
	}
	if strings.EqualFold(artist, "null") {
		artist = ""
	}

	info := Info{
		Source:  appName,
		State:   state,
		Track:   track,
		Artist:  artist,
		Playing: state == "playing",
		Err:     "",
	}
	return info, true, ""
}

func stateRank(state string) int {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "playing":
		return 3
	case "paused":
		return 2
	case "stopped":
		return 1
	default:
		return 0
	}
}

func betterInfo(a, b Info) Info {
	if stateRank(b.State) > stateRank(a.State) {
		return b
	}
	if stateRank(b.State) < stateRank(a.State) {
		return a
	}
	if a.Source == "Music" {
		return a
	}
	if b.Source == "Music" {
		return b
	}
	if (a.Track == "" && a.Artist == "") && (b.Track != "" || b.Artist != "") {
		return b
	}
	return a
}

func pollAppleScript() Info {
	best := Info{Source: "none", State: "stopped", Playing: false}
	errs := make([]string, 0, 2)

	if info, ok, errText := pollAppViaAppleScript("Music"); ok {
		best = betterInfo(best, info)
	} else if errText != "" {
		errs = append(errs, "Music: "+errText)
	}

	if appSeemsInstalled("Spotify") {
		if info, ok, errText := pollAppViaAppleScript("Spotify"); ok {
			best = betterInfo(best, info)
		} else if errText != "" {
			errs = append(errs, "Spotify: "+errText)
		}
	}

	if best.Source == "none" && len(errs) > 0 {
		best.Err = strings.Join(errs, " | ")
	}
	return best
}

func Poll() Info {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CINDER_NOWPLAYING_BACKEND"))) {
	case "applescript", "apple", "osascript":
		return pollAppleScript()
	case "cli", "nowplaying-cli", "nowplaying":
		if info, ok := pollNowPlayingCLI(); ok {
			return info
		}
		return Info{
			Source:  "none",
			State:   "stopped",
			Playing: false,
			Err:     "nowplaying-cli unavailable or unsupported; set CINDER_NOWPLAYING_BACKEND=applescript",
		}
	default:
		if info, ok := pollNowPlayingCLI(); ok {
			return info
		}
		return pollAppleScript()
	}
}
