package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestPlayerHelpItemIDReadsAsTheReference pins a help bypass's
// "<page>#<itemId>" word to the reference's StringTokenizer, split("#")
// and Integer.parseInt: the item id reads any Basic Multilingual Plane
// decimal digit (fullwidth "７０６４" and Arabic-Indic "٧٠٦٤" are 7064, as
// a Java probe on OpenJDK 21 prints); the id is the piece between the
// first and second '#'; trailing empty pieces are dropped, so a bare
// trailing '#' sets no id and a word of only '#' opens nothing; and only
// ASCII space, tab, newline, carriage return and form feed end the word,
// so an ideographic space stays inside the id and fails it. An id that
// does not parse opens nothing.
func TestPlayerHelpItemIDReadsAsTheReference(t *testing.T) {
	t.Parallel()
	const page = "<html><body>diary</body></html>"
	html := testHTMLCache(t, map[string]string{"help/diary.htm": page})

	for _, tc := range []struct {
		name    string
		command string
		sent    bool
		itemID  int32
	}{
		{"ascii", "player_help diary.htm#7064", true, 7064},
		{"fullwidth", "player_help diary.htm#７０６４", true, 7064},
		{"arabic-indic", "player_help diary.htm#٧٠٦٤", true, 7064},
		{"mixed scripts", "player_help diary.htm#7٠６4", true, 7064},
		{"second hash", "player_help diary.htm#７０６４#9", true, 7064},
		{"trailing hash", "player_help diary.htm#", true, 0},
		{"trailing hashes", "player_help diary.htm##", true, 0},
		{"tab ends the word", "player_help diary.htm#７０６４\tx", true, 7064},
		{"empty id", "player_help diary.htm##7064", false, 0},
		{"only hashes", "player_help ##", false, 0},
		{"ideographic space", "player_help diary.htm#７０６４　x", false, 0},
		{"supplementary digit", "player_help diary.htm#\U0001D7CF", false, 0},
		{"fullwidth letter", "player_help diary.htm#７ａ", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &testsupport.FrameCapture{}
			live := newEquipTestLivePlayer(t, 1, capture, item.NewTable(nil), nil)
			gcl := &GameClientLink{html: html}

			gcl.requestBypassToServer(live, clientpackets.RequestBypassToServer{Command: tc.command})

			if !tc.sent {
				if len(capture.Frames()) != 0 {
					t.Fatalf("frames = %x, want none", capture.Frames())
				}
				return
			}
			testsupport.AssertOpcodeSequence(t, capture.Frames(), serverpackets.OpcodeNpcHtmlMessage)
			assertNpcHtmlMessageFrame(t, capture.Frames()[0], 0, page+"\n", tc.itemID)
		})
	}
}
