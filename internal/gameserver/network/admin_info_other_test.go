package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminInfoOtherSelectionClearsBypasses pins //info on a selection
// with no info page (a ground item): one HTML window with no page, which
// still drops the links of the page sent before it.
func TestAdminInfoOtherSelectionClearsBypasses(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	gm := newTestLivePlayer(t, 1, capture)
	sendFilledHTML(gm, 0, `<html><body><a action="bypass -h npc_1_Chat 1">a</a></body></html>`, 0)
	if !gm.bypasses.allows("npc_1_Chat 1") {
		t.Fatal("the first page's link is not a valid bypass")
	}
	tmpl := &item.Template{ID: item.AdenaID, Name: "Adena", Kind: item.KindEtcItem}
	ground, err := grounditem.New(item.Instance{ObjectID: 2, TemplateID: item.AdenaID, Count: 1}, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	gm.StoreTarget(ground)
	sent := len(capture.Frames())

	(&GameClientLink{}).adminInfo(gm, "admin_info")

	frames := capture.Frames()[sent:]
	testsupport.AssertOpcodeSequence(t, frames, serverpackets.OpcodeNpcHtmlMessage)
	assertNpcHtmlMessageFrame(t, frames[0], 0, "", 0)
	if gm.bypasses.allows("npc_1_Chat 1") {
		t.Fatal("the first page's link is still a valid bypass after the empty window")
	}
}
