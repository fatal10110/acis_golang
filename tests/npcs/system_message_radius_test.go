package npcs

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestNPCSystemMessageInRadiusReachesPlayersByDistance: a system message an
// NPC sends in a radius (broadcastPacketInRadius(SystemMessage, r)) reaches
// each player within that radius of it, however far, and no player beyond
// it, with the parameters in the order text, castle, number. A monster and
// a folk NPC send alike.
func TestNPCSystemMessageInRadiusReachesPlayersByDistance(t *testing.T) {
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

	const (
		msgID    = 1500
		castleID = 7
		number   = 12345
	)
	m := script.SiegeMessage{ID: msgID, Text: "Rainbow", CastleID: castleID, Number: number, HasNumber: true}
	w.direct(t, func(_ *script.Player) {
		for _, n := range []*script.NPC{script.NPCOf(crier), script.NPCOf(folk)} {
			n.SystemMessageInRadius(m, 600)
			n.SystemMessageInRadius(m, 8000)
		}
	})

	// SystemMessage: id, parameter count, then each parameter as its type
	// (0 text, 5 castle name, 1 number) and value.
	want := binary.LittleEndian.AppendUint32([]byte{serverpackets.OpcodeSystemMessage}, msgID)
	want = binary.LittleEndian.AppendUint32(want, 3)
	want = binary.LittleEndian.AppendUint32(want, 0)
	for _, r := range utf16.Encode([]rune("Rainbow")) {
		want = binary.LittleEndian.AppendUint16(want, r)
	}
	want = append(want, 0, 0)
	want = binary.LittleEndian.AppendUint32(want, 5)
	want = binary.LittleEndian.AppendUint32(want, castleID)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, number)

	for _, viewer := range []struct {
		name  string
		lines int
		get   func() [][]byte
	}{
		// Each NPC sent once within 600 and once within 8000.
		{"first, within 600", 4, func() [][]byte { return pick(w.srv.ReadQueued(t, w.first), serverpackets.OpcodeSystemMessage) }},
		// Only the 8000-radius sends reach the player 700 away.
		{"second, 700 away", 2, func() [][]byte { return pick(w.srv.ReadQueued(t, w.second), serverpackets.OpcodeSystemMessage) }},
	} {
		got := viewer.get()
		if len(got) != viewer.lines {
			t.Fatalf("%s: %d SystemMessage, want %d", viewer.name, len(got), viewer.lines)
		}
		for i, g := range got {
			if !bytes.Equal(g, want) {
				t.Errorf("%s: SystemMessage %d = % x, want % x", viewer.name, i, g, want)
			}
		}
	}
}
