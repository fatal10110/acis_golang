package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestDecorationDiscoverThenForgetSendsNPCInfoThenDeleteObject(t *testing.T) {
	inst := &npc.Instance{ObjectID: 900001, Template: &npc.Template{ID: 19060004, TemplateID: 19060004}}
	deco, err := npc.NewDecoration(inst, "")
	if err != nil {
		t.Fatalf("NewDecoration: %v", err)
	}

	var frames [][]byte
	p := &livePlayer{visibilitySend: func(frame wire.Frame) bool {
		frames = append(frames, append([]byte(nil), frame.Bytes()...))
		frame.Release()
		return true
	}}

	p.Discover(deco)
	p.Forget(deco)

	if len(frames) != 2 {
		t.Fatalf("decoration visibility frames = %d, want 2", len(frames))
	}
	if got := frames[0][wire.FrameHeaderSize]; got != serverpackets.OpcodeNPCInfo {
		t.Fatalf("discover opcode = %#x, want NPCInfo %#x", got, serverpackets.OpcodeNPCInfo)
	}
	if got := frames[1][wire.FrameHeaderSize]; got != serverpackets.OpcodeDeleteObject {
		t.Fatalf("forget opcode = %#x, want DeleteObject %#x", got, serverpackets.OpcodeDeleteObject)
	}
}
