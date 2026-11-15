package npcs

import (
	stdhtml "html"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// linkPage sends RequestLinkHtml for link and returns the one window it
// opens, with no NPC.
func (w *folkWorld) linkPage(t *testing.T, link string) string {
	t.Helper()
	w.c.Send(encodeLinkHTML(link))
	frames := drainFrames(t, w.c)
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeNpcHtmlMessage}) {
		t.Fatalf("link %q = %x, want NpcHtmlMessage", link, got)
	}
	objectID, page, itemID := htmlMessage(t, frames[0])
	if objectID != 0 || itemID != 0 {
		t.Fatalf("link %q window = object %d item %d, want 0/0", link, objectID, itemID)
	}
	return page
}

// TestRequestLinkHTMLOffersNoBypass: a page the player opens by name is
// shown, but no link on it becomes a command the player may send back, so
// a page holding a link for any npc_ command opens none. The links of the
// last dialog stay as they were.
func TestRequestLinkHTMLOffersNoBypass(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)

	if got := w.linkPage(t, "test/any.htm"); got != anyNpcPage+"\n" {
		t.Fatalf("link page = %q, want %q", got, anyNpcPage)
	}
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 1")), nil, f, "")

	w.talkTo(t, f)
	w.linkPage(t, "test/any.htm")
	// The merchant's first page offers Chat 1, not Chat 2.
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 2")), nil, f, "")
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 1")), chatWindowAnswer, f, wantChatPage(dialogPage1, f))
}

// TestRequestLinkHTMLMissingNoticeEscapesTheName: the notice for a page
// the cache does not hold shows the name the player sent as text, so
// markup in it opens no link, and nothing it names becomes a command the
// player may send back.
func TestRequestLinkHTMLMissingNoticeEscapesTheName(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)

	link := `x.htm<a action="bypass -h npc_` + strconv.Itoa(int(f.ObjectID())) + `_Chat 1">Go</a>`
	want := "<html><body>My html is missing:<br>" + stdhtml.EscapeString(link) + "</body></html>"
	if got := w.linkPage(t, link); got != want {
		t.Fatalf("missing-page notice = %q, want %q", got, want)
	}
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 1")), nil, f, "")
}
