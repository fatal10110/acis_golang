package serverpackets

import (
	"bytes"
	"strings"
	"testing"
)

// showBoardHeader is the layout ShowBoard.writeImpl writes ahead of the
// page content: opcode 0x6e, the shown flag 1, then the eight tab links in
// this order.
func showBoardHeader() []byte {
	want := []byte{0x6e, 0x01}
	for _, tab := range []string{
		"bypass _bbshome",
		"bypass _bbsgetfav",
		"bypass _bbsloc",
		"bypass _bbsclan",
		"bypass _bbsmemo",
		"bypass _maillist_0_1_0_",
		"bypass _friendlist_0_",
		"bypass _bbsgetfav_add",
	} {
		want = append(want, encodeUTF16Z(tab)...)
	}
	return want
}

func TestFrameShowBoardPage(t *testing.T) {
	got := framePayload(t, FrameShowBoard("101", "<html>é</html>"))
	want := append(showBoardHeader(), encodeUTF16Z("101\b<html>é</html>")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShowBoard() = % x, want % x", got, want)
	}
}

// A page part left unused carries the text "null" after its id, as the
// reference's static 102 and 103 parts do.
func TestFrameShowBoardNoPage(t *testing.T) {
	got := framePayload(t, FrameShowBoard("103", ShowBoardNoPage))
	want := append(showBoardHeader(), encodeUTF16Z("103\bnull")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShowBoard(no page) = % x, want % x", got, want)
	}
}

// The edit-form fields follow "1002\b", each one followed by " \b".
func TestFrameShowBoardFields(t *testing.T) {
	got := framePayload(t, FrameShowBoardFields([]string{"0", "Alice", ""}))
	want := append(showBoardHeader(), encodeUTF16Z("1002\b0 \bAlice \b \b")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShowBoardFields() = % x, want % x", got, want)
	}
}

// pagePart reads the content string of a ShowBoard frame.
func pagePart(t *testing.T, payload []byte) string {
	t.Helper()
	header := showBoardHeader()
	if !bytes.HasPrefix(payload, header) {
		t.Fatalf("ShowBoard header = % x, want % x", payload[:min(len(payload), len(header))], header)
	}
	units := payload[len(header):]
	var runes []rune
	for i := 0; i+1 < len(units); i += 2 {
		u := rune(units[i]) | rune(units[i+1])<<8
		if u == 0 {
			break
		}
		runes = append(runes, u)
	}
	return string(runes)
}

// A page is cut at 4090 and 8180 UTF-16 code units into parts 101, 102 and
// 103; parts it does not reach carry "null"; a page of exactly 4090 units
// takes the two-part branch with an empty 102; 12270 units or more sends
// nothing.
func TestFramesShowBoardPageCuts(t *testing.T) {
	a, b, c := strings.Repeat("a", 4090), strings.Repeat("b", 4090), strings.Repeat("c", 10)
	tests := []struct {
		name string
		html string
		want []string
	}{
		{"short", "<html/>", []string{"101\b<html/>", "102\bnull", "103\bnull"}},
		{"exactly one part", a, []string{"101\b" + a, "102\b", "103\bnull"}},
		{"two parts", a + c, []string{"101\b" + a, "102\b" + c, "103\bnull"}},
		{"three parts", a + b + c, []string{"101\b" + a, "102\b" + b, "103\b" + c}},
		{"too long", a + b + strings.Repeat("c", 4090), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frames := FramesShowBoardPage(tt.html)
			if len(frames) != len(tt.want) {
				t.Fatalf("FramesShowBoardPage parts = %d, want %d", len(frames), len(tt.want))
			}
			for i, frame := range frames {
				if got := pagePart(t, framePayload(t, frame)); got != tt.want[i] {
					t.Fatalf("part %d = %.40q (len %d), want %.40q (len %d)", i, got, len(got), tt.want[i], len(tt.want[i]))
				}
			}
		})
	}
}

// The cut counts UTF-16 code units, not bytes: 4089 one-unit characters
// and one two-byte character fill a part exactly.
func TestFramesShowBoardPageCountsUTF16(t *testing.T) {
	html := strings.Repeat("a", 4089) + "é"
	frames := FramesShowBoardPage(html)
	if got := pagePart(t, framePayload(t, frames[0])); got != "101\b"+html {
		t.Fatalf("part 101 does not hold the whole page")
	}
	if got := pagePart(t, framePayload(t, frames[1])); got != "102\b" {
		t.Fatalf("part 102 = %q, want an empty part", got)
	}
}

// An edit page must be shorter than 8180 UTF-16 code units.
func TestFrameShowBoardEditLimit(t *testing.T) {
	if _, ok := FrameShowBoardEdit(strings.Repeat("a", 8180)); ok {
		t.Fatal("FrameShowBoardEdit(8180 units) built a frame, want none")
	}
	frame, ok := FrameShowBoardEdit(strings.Repeat("a", 8179))
	if !ok {
		t.Fatal("FrameShowBoardEdit(8179 units) built nothing")
	}
	if got := pagePart(t, framePayload(t, frame)); !strings.HasPrefix(got, "1001\b") {
		t.Fatalf("edit page part = %.10q, want 1001", got)
	}
}
