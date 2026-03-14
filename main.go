package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"cinder/audioinput"
	"cinder/ui"
)

func main() {
	var (
		audioReactive    bool
		normalMode       bool
		audioDevice      string
		doctor           bool
		setupAudio       bool
		listAudioDevices bool
	)

	flag.BoolVar(&audioReactive, "audio-reactive", false, "enable live audio-reactive mode")
	flag.BoolVar(&normalMode, "normal", false, "force the non-audio-reactive visualization")
	flag.StringVar(&audioDevice, "audio-device", "", "AVFoundation input device to analyze")
	flag.BoolVar(&doctor, "doctor", false, "check audio-reactive prerequisites and detected devices")
	flag.BoolVar(&setupAudio, "setup-audio", false, "print one-time BlackHole setup guidance")
	flag.BoolVar(&listAudioDevices, "list-audio-devices", false, "list AVFoundation audio input devices")
	flag.Parse()

	switch {
	case doctor:
		runDoctor()
		return
	case setupAudio:
		printSetupAudio()
		return
	case listAudioDevices:
		printAudioDevices()
		return
	}

	if normalMode {
		_ = os.Unsetenv("CINDER_AUDIO_REACTIVE")
		_ = os.Unsetenv("CINDER_AUDIO_DEVICE")
	} else if audioReactive {
		_ = os.Setenv("CINDER_AUDIO_REACTIVE", "1")
	}
	if !normalMode && strings.TrimSpace(audioDevice) != "" {
		_ = os.Setenv("CINDER_AUDIO_DEVICE", strings.TrimSpace(audioDevice))
		_ = os.Setenv("CINDER_AUDIO_REACTIVE", "1")
	}

	m := ui.NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "visualizer error: %v\n", err)
		os.Exit(1)
	}
}

func runDoctor() {
	fmt.Println("Cinder Audio Doctor")
	fmt.Println()

	ffmpegPath, ffmpegErr := exec.LookPath("ffmpeg")
	if ffmpegErr != nil {
		fmt.Println("ffmpeg: missing")
		fmt.Println("  install with: brew install ffmpeg")
	} else {
		fmt.Printf("ffmpeg: ok (%s)\n", ffmpegPath)
	}

	if path, err := exec.LookPath("nowplaying-cli"); err == nil {
		fmt.Printf("nowplaying-cli: ok (%s)\n", path)
	} else {
		fmt.Println("nowplaying-cli: optional, not installed")
	}

	fmt.Println()
	devices, err := audioinput.ListInputDevices()
	if err != nil {
		fmt.Printf("audio inputs: unavailable (%v)\n", err)
	} else {
		fmt.Println("audio inputs:")
		for _, device := range devices {
			fmt.Printf("  - %s\n", device)
		}
	}

	if preferred, ok, err := audioinput.DetectPreferredInput(); err == nil && ok {
		fmt.Printf("\npreferred loopback input: %s\n", preferred)
		fmt.Printf("default launch: cinder\n")
		fmt.Printf("explicit launch: cinder --audio-device \"%s\"\n", preferred)
	} else if err == nil {
		fmt.Println("\npreferred loopback input: not found")
		fmt.Println("install BlackHole 2ch or use cinder --setup-audio")
	}
}

func printSetupAudio() {
	fmt.Println("Cinder Audio Setup")
	fmt.Println()
	fmt.Println("One-time setup for system-audio visualization on macOS:")
	fmt.Println("1. brew install --cask blackhole-2ch")
	fmt.Println("2. Open Audio MIDI Setup")
	fmt.Println("3. Create a Multi-Output Device")
	fmt.Println("4. In that Multi-Output Device, enable your headphones or speakers and BlackHole 2ch")
	fmt.Println("5. Set Primary Device to your headphones or speakers")
	fmt.Println("6. Enable Drift Correction for BlackHole 2ch")
	fmt.Println("7. In System Settings > Sound > Output, choose Multi-Output Device")
	fmt.Println()
	if preferred, ok, err := audioinput.DetectPreferredInput(); err == nil && ok {
		fmt.Printf("Detected loopback input: %s\n", preferred)
		fmt.Printf("After setup, just run: cinder\n")
		fmt.Printf("To force normal mode instead: cinder --normal\n")
	} else {
		fmt.Println("After setup, just run: cinder")
		fmt.Println("To force normal mode instead: cinder --normal")
	}
}

func printAudioDevices() {
	devices, err := audioinput.ListInputDevices()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to list audio devices: %v\n", err)
		os.Exit(1)
	}
	for _, device := range devices {
		fmt.Println(device)
	}
}
