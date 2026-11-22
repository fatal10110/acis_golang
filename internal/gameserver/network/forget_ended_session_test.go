package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// A player whose session ended builds no DeleteObject for what it forgets:
// despawning it in a crowd would otherwise make one frame per object for a
// connection that drops them all.
func TestForgetAfterSessionEndedBuildsNoFrame(t *testing.T) {
	inst := &npc.Instance{ObjectID: 900001, Template: &npc.Template{ID: 19060004, TemplateID: 19060004}}
	deco, err := npc.NewDecoration(inst, "")
	if err != nil {
		t.Fatalf("NewDecoration: %v", err)
	}
	var opcodes []byte
	p := &livePlayer{visibilitySend: func(frame wire.Frame) bool {
		opcodes = append(opcodes, frame.Bytes()[wire.FrameHeaderSize])
		frame.Release()
		return true
	}}

	p.Forget(deco)
	if len(opcodes) != 1 || opcodes[0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("open session forget opcodes = %#x, want one DeleteObject %#x", opcodes, serverpackets.OpcodeDeleteObject)
	}

	opcodes = nil
	p.sessionEnded.Store(true)
	p.Forget(deco)
	if len(opcodes) != 0 {
		t.Fatalf("ended session forget opcodes = %#x, want none", opcodes)
	}
}
