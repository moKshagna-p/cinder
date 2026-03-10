package nowplaying

import (
	"os/exec"
	"strings"
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

func Poll() Info {
	script := `
on cleanText(t)
	set s to (t as text)
	set s to my replaceText("|", "/", s)
	set s to my replaceText((ASCII character 10), " ", s)
	set s to my replaceText((ASCII character 13), " ", s)
	return s
end cleanText

on replaceText(find, repl, s)
	set AppleScript's text item delimiters to find
	set parts to every text item of s
	set AppleScript's text item delimiters to repl
	set out to parts as text
	set AppleScript's text item delimiters to ""
	return out
end replaceText

set outLine to "none|stopped||"

try
	if application "Spotify" is running then
		tell application "Spotify"
			set pstate to (player state as text)
			if (pstate is "playing") or (pstate is "paused") then
				set tn to ""
				set ar to ""
				try
					set tn to my cleanText(name of current track)
				end try
				try
					set ar to my cleanText(artist of current track)
				end try
				set outLine to "Spotify|" & pstate & "|" & tn & "|" & ar
			end if
		end tell
	end if
end try

try
	if outLine is "none|stopped||" then
		if application "Music" is running then
			tell application "Music"
				set pstate to (player state as text)
				if (pstate is "playing") or (pstate is "paused") then
					set tn to ""
					set ar to ""
					try
						set tn to my cleanText(name of current track)
					end try
					try
						set ar to my cleanText(artist of current track)
					end try
					set outLine to "Music|" & pstate & "|" & tn & "|" & ar
				end if
			end tell
		end if
	end if
end try

try
	if outLine is "none|stopped||" then
		if application "Music" is running then
			tell application "Music"
				set pstate to (player state as text)
				if pstate is "stopped" then
				set tn to ""
				set ar to ""
				try
					set tn to my cleanText(name of current track)
				end try
				try
					set ar to my cleanText(artist of current track)
				end try
				set outLine to "Music|" & pstate & "|" & tn & "|" & ar
				end if
			end if
		end tell
	end if
end try

return outLine
`

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		errText := strings.TrimSpace(string(out))
		if errText == "" {
			errText = err.Error()
		}
		return Info{Source: "none", State: "stopped", Playing: false, Err: errText}
	}

	line := strings.TrimSpace(string(out))
	parts := strings.SplitN(line, "|", 4)
	for len(parts) < 4 {
		parts = append(parts, "")
	}

	state := strings.ToLower(strings.TrimSpace(parts[1]))
	return Info{
		Source:  strings.TrimSpace(parts[0]),
		State:   state,
		Track:   strings.TrimSpace(parts[2]),
		Artist:  strings.TrimSpace(parts[3]),
		Playing: state == "playing",
		Err:     "",
	}
}
