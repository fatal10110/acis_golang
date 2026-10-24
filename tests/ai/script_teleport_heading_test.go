package ai

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// npcInfoHeading reads the heading of an NpcInfo frame.
func npcInfoHeading(f []byte) int32 { return int32(binary.LittleEndian.Uint32(f[25:29])) }

// onNPCQueue runs do with h's handle on h's queue, as its hook would, and
// returns the frames the character was sent.
func (w *warriorWorld) onNPCQueue(t *testing.T, h *npc.Hostile, do func(*script.NPC)) [][]byte {
	t.Helper()
	done := make(chan struct{})
	if !h.Queue().Post(func() {
		defer close(done)
		do(script.NPCOf(h))
	}) {
		t.Fatal("the monster's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// TestScriptTurnsAnNPCAroundItsTeleport (Baium.java onScriptEvent and
// Frintezza: setHeading, then teleportTo; FollowerOfFrintezza: teleportTo,
// then setHeading; Creature.teleportTo keeps the heading): a script turns a
// monster out of the character's sight, then teleports it beside the
// character, which sees it arrive facing the new heading. A later turn
// where it stands sends nothing; a teleport away and back keeps that turn
// and reads no heading off its point.
func TestScriptTurnsAnNPCAroundItsTeleport(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(warriorID, nil))
	h := w.spawn(t, warriorID, 7000, 0)
	near := script.Loc{X: w.at.X + 300, Y: w.at.Y, Z: w.at.Z}
	far := script.Loc{X: w.at.X + 7000, Y: w.at.Y, Z: w.at.Z}

	frames := framesOf(w.onNPCQueue(t, h, func(n *script.NPC) {
		n.SetHeading(-25348)
		n.TeleportTo(near)
	}), serverpackets.OpcodeNPCInfo, h.ObjectID())
	if len(frames) != 1 {
		t.Fatalf("NpcInfo frames on arrival = %d, want 1", len(frames))
	}
	if got := npcInfoHeading(frames[0]); got != -25348 {
		t.Fatalf("NpcInfo heading on arrival = %d, want -25348", got)
	}

	all := w.onNPCQueue(t, h, func(n *script.NPC) { n.SetHeading(16384) })
	if got := len(framesOf(all, serverpackets.OpcodeNPCInfo, h.ObjectID())); got != 0 {
		t.Fatalf("NpcInfo frames on a turn where it stands = %d, want none", got)
	}
	if got := h.Heading(); got != 16384 {
		t.Fatalf("heading after the turn = %d, want 16384", got)
	}

	w.onNPCQueue(t, h, func(n *script.NPC) { n.TeleportTo(far) })
	back := near
	back.Heading = 100
	frames = framesOf(w.onNPCQueue(t, h, func(n *script.NPC) { n.TeleportTo(back) }), serverpackets.OpcodeNPCInfo, h.ObjectID())
	if len(frames) != 1 {
		t.Fatalf("NpcInfo frames on the way back = %d, want 1", len(frames))
	}
	if got := npcInfoHeading(frames[0]); got != 16384 {
		t.Fatalf("NpcInfo heading on the way back = %d, want the kept 16384", got)
	}
}
