package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// serverBypassTime is the stock ServerBypassTime reuse delay.
const serverBypassTime = 100 * time.Millisecond

// TestBypassInsideServerBypassTimeIgnored pins the bypass reuse gate: a
// bypass inside ServerBypassTime of the last one that passed is dropped
// with no answer and no effect, the window it falls in is not pushed out,
// and an empty command neither runs nor spends the window.
func TestBypassInsideServerBypassTimeIgnored(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), gameservertest.WithReuseDelays(3*time.Second, serverBypassTime))
	if !w.srv.DrivesClock() {
		t.Skip("placing bypasses on either side of the reuse window needs the driven clock")
	}
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	w.talkTo(t, f)

	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 1")), chatWindowAnswer, f, wantChatPage(dialogPage1, f))
	// Inside the window: no frame, and page two (which offers no link
	// back) is not opened behind the client's back.
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 2")), nil, f, "")
	w.srv.Advance(t, serverBypassTime-time.Millisecond)
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 2")), nil, f, "")

	// The refusals did not push the window out; page one's links still
	// stand, so Chat 1 is admitted again.
	w.srv.Advance(t, time.Millisecond)
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 1")), chatWindowAnswer, f, wantChatPage(dialogPage1, f))

	// An empty command past the window is not answered and leaves the
	// window unspent: the bypass right after it passes.
	w.srv.Advance(t, serverBypassTime)
	assertAnswer(t, w.bypass(t, ""), nil, f, "")
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 2")), chatWindowAnswer, f, wantChatPage(dialogPage2, f))
}
