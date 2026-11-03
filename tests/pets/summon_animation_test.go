package pets

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// summonAnimationBytes returns the summon-animation byte of every NpcInfo of
// a in frames: it follows the in-combat and alike-dead bytes.
func summonAnimationBytes(frames [][]byte, a *summon.Actor) []byte {
	var out []byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeNPCInfo && int32(binary.LittleEndian.Uint32(f[1:5])) == a.ObjectID() {
			out = append(out, f[1+120])
		}
	}
	return out
}

// requireSummonAnimation checks frames hold at least one NpcInfo of a and
// that each carries the summon-animation byte 2.
func requireSummonAnimation(t *testing.T, frames [][]byte, a *summon.Actor, who string) {
	t.Helper()
	got := summonAnimationBytes(frames, a)
	if len(got) == 0 {
		t.Fatalf("%s got no NpcInfo of the summon", who)
	}
	for i, b := range got {
		if b != 2 {
			t.Fatalf("%s NpcInfo %d summon-animation byte = %d, want 2", who, i, b)
		}
	}
}

// TestObserverSeesPetSummonAnimation pins #4001 on a pet: a player who is in
// sight when the pet is summoned, and one who comes into sight of it later,
// both get its NpcInfo with the summon-animation byte 2.
func TestObserverSeesPetSummonAnimation(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	watcher := h.joinSecondPlayer(t, "Watcher")
	pet, _ := h.spawnWolf(t)
	requireSummonAnimation(t, drainFrames(t, watcher.client), pet, "watcher at the summon")
	requireSummonAnimation(t, enterWorldSeeing(t, h.srv, "player3", "Late"), pet, "late viewer")
}

// TestObserverSeesServitorSummonAnimation pins #4001 on a servitor: a player
// coming into sight of it gets its NpcInfo with the summon-animation byte 2.
func TestObserverSeesServitorSummonAnimation(t *testing.T) {
	t.Parallel()
	o := bootServitorOwner(t)
	servitor := o.summonServitor(t)
	requireSummonAnimation(t, enterWorldSeeing(t, o.srv, "player2", "Late"), servitor, "late viewer")
}
