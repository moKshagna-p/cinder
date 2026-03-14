package audioinput

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

var preferredInputNames = []string{
	"BlackHole 2ch",
	"BlackHole",
	"Loopback Audio",
	"Loopback",
	"Background Music",
}

func ListInputDevices() ([]string, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"ffmpeg",
		"-hide_banner",
		"-f", "avfoundation",
		"-list_devices", "true",
		"-i", "",
	)

	var stderr bytes.Buffer
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &stderr
	_ = cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return nil, errors.New("timed out listing AVFoundation devices")
	}

	devices := parseAVFoundationAudioDevices(stderr.String())
	if len(devices) == 0 {
		return nil, errors.New("no AVFoundation audio devices found")
	}
	return devices, nil
}

func DetectPreferredInput() (string, bool, error) {
	devices, err := ListInputDevices()
	if err != nil {
		return "", false, err
	}

	lowered := make([]string, len(devices))
	for i, d := range devices {
		lowered[i] = strings.ToLower(d)
	}

	for _, preferred := range preferredInputNames {
		want := strings.ToLower(preferred)
		for i, have := range lowered {
			if have == want {
				return devices[i], true, nil
			}
		}
	}
	for _, preferred := range preferredInputNames {
		want := strings.ToLower(preferred)
		for i, have := range lowered {
			if strings.Contains(have, want) {
				return devices[i], true, nil
			}
		}
	}

	return "", false, nil
}

func parseAVFoundationAudioDevices(output string) []string {
	lines := strings.Split(output, "\n")
	var devices []string
	inAudio := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.Contains(line, "AVFoundation audio devices"):
			inAudio = true
			continue
		case strings.Contains(line, "AVFoundation video devices"):
			inAudio = false
			continue
		}
		if !inAudio {
			continue
		}
		start := strings.Index(line, "] ")
		if start == -1 {
			continue
		}
		name := strings.TrimSpace(line[start+2:])
		if strings.HasPrefix(name, "[") {
			if next := strings.Index(name, "] "); next != -1 {
				name = strings.TrimSpace(name[next+2:])
			}
		}
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "Error opening input") {
			continue
		}
		devices = append(devices, name)
	}
	return devices
}
