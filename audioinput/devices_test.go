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
