package audioinput

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var preferredInputNames = []string{
	"BlackHole 2ch",
	"BlackHole 16ch",
	"BlackHole",
	"Loopback Audio",
	"Loopback",
	"Background Music",
}

type inputDeviceEntry struct {
	Index int
	Name  string
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

func listInputDeviceEntries() ([]inputDeviceEntry, error) {
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

	devices := parseAVFoundationAudioDeviceEntries(stderr.String())
	if len(devices) == 0 {
		return nil, errors.New("no AVFoundation audio devices found")
	}
	return devices, nil
}

func lookupInputDeviceIndex(name string) (int, bool, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return 0, false, nil
	}
	entries, err := listInputDeviceEntries()
	if err != nil {
		return 0, false, err
	}
	for _, entry := range entries {
		if strings.ToLower(strings.TrimSpace(entry.Name)) == name {
			return entry.Index, true, nil
		}
	}
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(strings.TrimSpace(entry.Name)), name) {
			return entry.Index, true, nil
		}
	}
	return 0, false, nil
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
	entries := parseAVFoundationAudioDeviceEntries(output)
	devices := make([]string, 0, len(entries))
	for _, entry := range entries {
		devices = append(devices, entry.Name)
	}
	return devices
}

func parseAVFoundationAudioDeviceEntries(output string) []inputDeviceEntry {
	lines := strings.Split(output, "\n")
	var devices []inputDeviceEntry
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
		rest := strings.TrimSpace(line[start+2:])
		if !strings.HasPrefix(rest, "[") {
			continue
		}
		idxEnd := strings.Index(rest, "]")
		if idxEnd <= 1 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(rest[1:idxEnd]))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(rest[idxEnd+1:])
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "Error opening input") {
			continue
		}
		devices = append(devices, inputDeviceEntry{Index: idx, Name: name})
	}
	return devices
}
