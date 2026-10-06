package serverpackets

import (
	"bytes"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// appendUTF16 appends s as the wire writes a string: UTF-16LE, then a zero
// terminator.
func appendUTF16(b []byte, s string) []byte {
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return append(b, 0, 0)
}

// TestTutorialFrames pins the tutorial window packets and RadarControl:
// TutorialShowHtml 0xa0 with the page, TutorialShowQuestionMark 0xa1 and
// TutorialEnableClientEvent 0xa2 with an int32 id, TutorialCloseHtml 0xa3
// alone, and RadarControl 0xeb with five int32s.
func TestTutorialFrames(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame wire.Frame
		want  []byte
	}{
		{"TutorialShowHtml", FrameTutorialShowHTML("<html>é</html>"), appendUTF16([]byte{0xa0}, "<html>é</html>")},
		{"TutorialShowQuestionMark", FrameTutorialShowQuestionMark(26), appendD([]byte{0xa1}, 26)},
		{"TutorialEnableClientEvent", FrameTutorialEnableClientEvent(8388608), appendD([]byte{0xa2}, 8388608)},
		{"TutorialCloseHtml", FrameTutorialCloseHTML(), []byte{0xa3}},
		{
			"RadarControl", FrameRadarControl(Radar{Show: 2, Type: 1, X: -84318, Y: 244579, Z: -3730}),
			appendD(appendD(appendD(appendD(appendD([]byte{0xeb}, 2), 1), -84318), 244579), -3730),
		},
	} {
		if got := framePayload(t, tc.frame); !bytes.Equal(got, tc.want) {
			t.Errorf("%s = %x, want %x", tc.name, got, tc.want)
		}
	}
}
