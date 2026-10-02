package network

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const tooLongPage = "<html><body>Html was too long.</body></html>"

// longLinkedPage is a page of more than 8192 characters whose last link
// lies past the limit.
func longLinkedPage() string {
	return `<html><body><a action="bypass -h npc_1_Chat 1">a</a>` + strings.Repeat("x", 8192) +
		`<a action="bypass -h npc_1_Chat 2">b</a></body></html>`
}

// TestSendFilledHTMLSendsAndRecordsWholePage pins NpcHtmlMessage.replace
// and runImpl: a set page its fillings grew past 8192 characters is sent
// whole, and every link on it becomes a valid bypass.
func TestSendFilledHTMLSendsAndRecordsWholePage(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, item.NewTable(nil), nil)
	page := longLinkedPage()

	sendFilledHTML(live, 5, page, 0)

	testsupport.AssertOpcodeSequence(t, capture.Frames(), serverpackets.OpcodeNpcHtmlMessage)
	assertNpcHtmlMessageFrame(t, capture.Frames()[0], 5, page, 0)
	for _, cmd := range []string{"npc_1_Chat 1", "npc_1_Chat 2"} {
		if !live.bypasses.allows(cmd) {
			t.Errorf("allows(%q) = false, want true", cmd)
		}
	}
}

// TestSendValidatedHTMLRefusesLongSetPage pins NpcHtmlMessage.setHtml: a
// page set longer than 8192 characters is replaced by the notice, and the
// notice, the page sent, leaves no valid bypass.
func TestSendValidatedHTMLRefusesLongSetPage(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, item.NewTable(nil), nil)
	sendValidatedHTML(live, 0, `<a action="bypass -h npc_1_Chat 1">a</a>`, 0)

	sendValidatedHTML(live, 5, longLinkedPage(), 0)

	frames := capture.Frames()
	testsupport.AssertOpcodeSequence(t, frames, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeNpcHtmlMessage)
	assertNpcHtmlMessageFrame(t, frames[1], 5, tooLongPage, 0)
	for _, cmd := range []string{"npc_1_Chat 1", "npc_1_Chat 2"} {
		if live.bypasses.allows(cmd) {
			t.Errorf("allows(%q) = true after the notice, want false", cmd)
		}
	}
}

// TestSetPageLimitsPageAsLoaded pins NpcHtmlMessage.setFile: the page as
// loaded is held to the limit, before any placeholder is filled; a missing
// page reads as the notice naming it.
func TestSetPageLimitsPageAsLoaded(t *testing.T) {
	long := strings.Repeat("x", 8193)
	short := "<html><body>%list%</body></html>"
	gcl := &GameClientLink{html: testHTMLCache(t, map[string]string{"long.htm": long, "short.htm": short})}

	// The page cache ends every page with a line break.
	if got := gcl.setPage("data/html/short.htm"); got != short+"\n" {
		t.Fatalf("setPage(short) = %q, want %q", got, short+"\n")
	}
	if got := gcl.setPage("data/html/long.htm"); got != tooLongPage {
		t.Fatalf("setPage(long) = %q, want the notice", got)
	}
	if got, want := gcl.setPage("data/html/none.htm"), "<html><body>My html is missing:<br>data/html/none.htm</body></html>"; got != want {
		t.Fatalf("setPage(missing) = %q, want %q", got, want)
	}

	pages := setPages{gcl.html}
	if got, ok := pages.Get("data/html/short.htm"); !ok || got != short+"\n" {
		t.Fatalf("setPages.Get(short) = %q, %v, want %q, true", got, ok, short+"\n")
	}
	if got, ok := pages.Get("data/html/long.htm"); !ok || got != tooLongPage {
		t.Fatalf("setPages.Get(long) = %q, %v, want the notice, true", got, ok)
	}
	if _, ok := pages.Get("data/html/none.htm"); ok {
		t.Fatal("setPages.Get(missing) found a page")
	}
}
