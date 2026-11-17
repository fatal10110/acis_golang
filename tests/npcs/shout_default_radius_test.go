package npcs

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestNPCShoutInRadiusNegativeRadiusIsSixHundred: a line an NPC shouts in a
// negative radius (broadcastPacketInRadius turns any radius below 0 into
// 600) reaches the player within 600 of it and not the player about 700
// away, a monster and a folk NPC alike.
func TestNPCShoutInRadiusNegativeRadiusIsSixHundred(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t)
	viewer, ok := w.srv.State.PlayerByName("Viewer")
	if !ok {
		t.Fatal("the viewer is not in the world")
	}
	secondID := viewer.ObjectID()
	away := location.Location{X: w.at.X + 700, Y: w.at.Y, Z: w.at.Z}
	if !w.srv.RunScript(t, secondID, "Director", func(_ *script.Script, p *script.Player) {
		p.InstantTeleportTo(script.Loc{X: away.X, Y: away.Y, Z: away.Z})
	}) {
		t.Fatal("teleport invocation panicked")
	}
	w.srv.AdvanceUntil(t, "the viewer stands 700 away", func() bool {
		x, _, _ := w.srv.PlayerPosition(t, secondID)
		return x == away.X
	})
	const folkID = 31921
	crierAt := location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z}
	folkAt := location.Location{X: w.at.X, Y: w.at.Y + 40, Z: w.at.Z}
	crier := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Crier"), crierAt)
	folk := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", folkID), folkAt)

	// The test's premise: the first player stands within 600 of both NPCs
	// and the second beyond 600 of both, in the plane and in 3D.
	ax, ay, az := w.srv.PlayerPosition(t, secondID)
	for _, at := range []location.Location{crierAt, folkAt} {
		if d := math.Hypot(math.Hypot(float64(ax-at.X), float64(ay-at.Y)), float64(az-at.Z)); d <= 600 {
			t.Fatalf("the second player stands %.0f from an NPC at %v, want more than 600", d, at)
		}
	}
	w.srv.ReadQueued(t, w.first)
	w.srv.ReadQueued(t, w.second)

	w.direct(t, func(_ *script.Player) {
		for _, radius := range []int{-1, -5000} {
			for _, n := range []*script.NPC{script.NPCOf(crier), script.NPCOf(folk)} {
				n.ShoutInRadius("Default reach", radius)
			}
		}
	})

	say := func(objectID int32, npcID int, text string) []byte {
		out := []byte{serverpackets.OpcodeNpcSay}
		for _, v := range []int32{objectID, 1, int32(npcID) + 1_000_000} {
			out = binary.LittleEndian.AppendUint32(out, uint32(v))
		}
		for _, r := range utf16.Encode([]rune(text)) {
			out = binary.LittleEndian.AppendUint16(out, r)
		}
		return append(out, 0, 0)
	}
	crierSay := say(crier.ObjectID(), cutsceneMonsterID, "Default reach")
	folkSay := say(folk.ObjectID(), folkID, "Default reach")

	got := pick(w.srv.ReadQueued(t, w.first), serverpackets.OpcodeNpcSay)
	want := [][]byte{crierSay, folkSay, crierSay, folkSay}
	if len(got) != len(want) {
		t.Fatalf("near player: %d NpcSay, want %d", len(got), len(want))
	}
	for i := range got {
		if !bytes.Equal(got[i], want[i]) {
			t.Errorf("near player: NpcSay %d = % x, want % x", i, got[i], want[i])
		}
	}
	if got := pick(w.srv.ReadQueued(t, w.second), serverpackets.OpcodeNpcSay); len(got) != 0 {
		t.Errorf("player 700 away heard %d lines of a negative-radius shout, want none", len(got))
	}
}
