package audioinput

import (
	"reflect"
	"testing"
)

func TestParseAVFoundationAudioDevices(t *testing.T) {
	raw := `
[AVFoundation indev @ 0x123] AVFoundation video devices:
[AVFoundation indev @ 0x123] [0] FaceTime HD Camera
[AVFoundation indev @ 0x123] AVFoundation audio devices:
[AVFoundation indev @ 0x123] [0] MacBook Air Microphone
[AVFoundation indev @ 0x123] [1] BlackHole 2ch
[AVFoundation indev @ 0x123] [2] External Headphones
`
	got := parseAVFoundationAudioDevices(raw)
	want := []string{"MacBook Air Microphone", "BlackHole 2ch", "External Headphones"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected parse result: %#v", got)
	}
}

func TestParseAVFoundationAudioDeviceEntries(t *testing.T) {
	raw := `
[AVFoundation indev @ 0x123] AVFoundation audio devices:
[AVFoundation indev @ 0x123] [0] MacBook Pro Microphone
[AVFoundation indev @ 0x123] [1] BlackHole 2ch
`

	got := parseAVFoundationAudioDeviceEntries(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Index != 0 || got[0].Name != "MacBook Pro Microphone" {
		t.Fatalf("unexpected first entry: %#v", got[0])
	}
	if got[1].Index != 1 || got[1].Name != "BlackHole 2ch" {
		t.Fatalf("unexpected second entry: %#v", got[1])
	}
}
