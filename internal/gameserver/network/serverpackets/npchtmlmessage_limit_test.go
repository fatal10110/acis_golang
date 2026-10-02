package serverpackets

import (
	"bytes"
	"strings"
	"testing"
)

const tooLongNotice = "<html><body>Html was too long.</body></html>"

// TestNpcHtmlBodyCountsUTF16Units pins NpcHtmlMessage.setHtml: a page is
// refused when it holds more than 8192 UTF-16 code units (Java
// String.length), whatever its UTF-8 size.
func TestNpcHtmlBodyCountsUTF16Units(t *testing.T) {
	tests := []struct {
		name string
		html string
		kept bool
	}{
		{"ascii at the limit", strings.Repeat("a", 8192), true},
		{"ascii past the limit", strings.Repeat("a", 8193), false},
		// 8192 two-byte characters: 16384 bytes, 8192 units.
		{"latin at the limit", strings.Repeat("é", 8192), true},
		{"latin past the limit", strings.Repeat("é", 8193), false},
		// 6000 three-byte characters: 18000 bytes, 6000 units.
		{"cjk under the limit", strings.Repeat("漢", 6000), true},
		// A character past the Basic Multilingual Plane is a surrogate
		// pair: 4096 of them are 8192 units, 4097 are 8194.
		{"surrogates at the limit", strings.Repeat("😀", 4096), true},
		{"surrogates past the limit", strings.Repeat("😀", 4097), false},
		{"surrogates one unit past", "a" + strings.Repeat("😀", 4096), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tooLongNotice
			if tt.kept {
				want = tt.html
			}
			if got := NpcHtmlBody(tt.html); got != want {
				t.Fatalf("NpcHtmlBody(%d bytes) = %d bytes, want %d bytes", len(tt.html), len(got), len(want))
			}
		})
	}
}

// TestFrameNpcHtmlMessageCarriesPageWhole pins NpcHtmlMessage.writeImpl:
// the packet writes the page it holds, which placeholder fillings may have
// grown past the limit set pages are held to.
func TestFrameNpcHtmlMessageCarriesPageWhole(t *testing.T) {
	page := "<html><body>" + strings.Repeat("x", 9000) + "</body></html>"
	got := framePayload(t, FrameNpcHtmlMessage(7, page, 0))
	want := []byte{OpcodeNpcHtmlMessage}
	want = appendD(want, 7)
	want = append(want, encodeUTF16Z(page)...)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameNpcHtmlMessage(%d-unit page) = %d bytes, want %d", len(page), len(got), len(want))
	}
}
