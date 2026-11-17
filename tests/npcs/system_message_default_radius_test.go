package npcs

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestNPCSystemMessageInRadiusNegativeRadiusIsSixHundred: a system message
// an NPC sends in a negative radius (broadcastPacketInRadius turns any
// radius below 0 into 600) reaches the player within 600 of it and not the
// player 700 away, a monster and a folk NPC alike.
func TestNPCSystemMessageInRadiusNegativeRadiusIsSixHundred(t *testing.T) {
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
	crier := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Crier"), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	folk := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 31921), location.Location{X: w.at.X, Y: w.at.Y + 40, Z: w.at.Z})
	w.srv.ReadQueued(t, w.first)
	w.srv.ReadQueued(t, w.second)

	const msgID = 1500
	w.direct(t, func(_ *script.Player) {
		for _, radius := range []int{-1, -5000} {
			for _, n := range []*script.NPC{script.NPCOf(crier), script.NPCOf(folk)} {
				n.SystemMessageInRadius(script.SiegeMessage{ID: msgID}, radius)
			}
		}
	})

	// SystemMessage: id, then no parameters.
	want := binary.LittleEndian.AppendUint32([]byte{serverpackets.OpcodeSystemMessage}, msgID)
	want = binary.LittleEndian.AppendUint32(want, 0)

	got := pick(w.srv.ReadQueued(t, w.first), serverpackets.OpcodeSystemMessage)
	if len(got) != 4 {
		t.Fatalf("near player: %d SystemMessage, want 4", len(got))
	}
	for i, g := range got {
		if !bytes.Equal(g, want) {
			t.Errorf("near player: SystemMessage %d = % x, want % x", i, g, want)
		}
	}
	if got := pick(w.srv.ReadQueued(t, w.second), serverpackets.OpcodeSystemMessage); len(got) != 0 {
		t.Errorf("player 700 away got %d system messages of a negative radius, want none", len(got))
	}
}
