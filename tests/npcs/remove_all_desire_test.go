package npcs

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Npc.removeAllDesire and Npc.broadcastOnScreen exist on every NPC kind
// (#3748): Doctor Chaos's and the Frintezza organ dummy's behaviors clear a
// civilian NPC's desires, and the clan hall watering game's managers, also
// civilian, show screen messages.

// TestScriptRemoveAllDesireStopsAMovingNPC pins Npc.removeAllDesire on a
// walking hostile NPC and a walking civilian one: every desire goes, every
// hate on the aggro list (and a hostile's hate list) drops to zero with its
// creature still listed, and the walk stops, its observers shown StopMove.
func TestScriptRemoveAllDesireStopsAMovingNPC(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	target := w.srv.SpawnHostileNPCAt(t, location.Location{X: w.at.X - 60, Y: w.at.Y, Z: w.at.Z})

	t.Run("hostile", func(t *testing.T) {
		h := w.srv.SpawnMovingHostileNPCAt(t, "Monster", location.Location{X: w.at.X + 600, Y: w.at.Y, Z: w.at.Z},
			location.Location{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z})
		drainUntilQuiet(t, w.c)
		if !h.ReturnHome() {
			t.Fatal("NPC did not start walking home")
		}
		drainFrames(t, w.c)
		h.AI().Threats().AddDamage(target, 0, 100)
		h.AI().Hates().Add(target, 50)
		h.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionCast, FinalTarget: target, Weight: 10})
		if !h.IsMoving() {
			t.Fatal("NPC is not walking home")
		}

		script.NPCOf(h).RemoveAllDesire()
		frames := drainFrames(t, w.c)
		if frameAt(frames, serverpackets.OpcodeStopMove, h.ObjectID()) < 0 || h.IsMoving() {
			t.Fatalf("frames %x, moving %v; want StopMove and the NPC standing", opcodes(frames), h.IsMoving())
		}
		threats := h.AI().Threats()
		if threats.Hate(target) != 0 || len(threats.Snapshot()) != 1 || h.AI().Hates().Hate(target) != 0 || h.AI().Hates().Len() != 1 {
			t.Fatalf("attack hate %v (%d listed), list hate %v (%d listed); want 0 (1), 0 (1)",
				threats.Hate(target), len(threats.Snapshot()), h.AI().Hates().Hate(target), h.AI().Hates().Len())
		}
		if n := h.AI().Desires().Len(); n != 0 {
			t.Fatalf("%d desires left, want 0", n)
		}
	})

	t.Run("civilian", func(t *testing.T) {
		at := location.Location{X: w.at.X + 150, Y: w.at.Y + 40, Z: w.at.Z}
		f := w.spawnWalker(t, at, location.Location{X: at.X - 50, Y: at.Y, Z: at.Z}, location.Location{X: at.X + 400, Y: at.Y, Z: at.Z})
		drainFrames(t, w.c)
		if !f.IsMoving() {
			t.Fatal("walker is not walking its route")
		}
		f.Aggro().AddDamage(target, 0, 100)
		f.Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionFollow, FinalTarget: target, Weight: 10})

		script.NPCOf(f).RemoveAllDesire()
		frames := drainFrames(t, w.c)
		if frameAt(frames, serverpackets.OpcodeStopMove, f.ObjectID()) < 0 || f.IsMoving() {
			t.Fatalf("frames %x, moving %v; want StopMove and the walker standing", opcodes(frames), f.IsMoving())
		}
		if hate, listed := f.Aggro().Hate(target), len(f.Aggro().Snapshot()); hate != 0 || listed != 1 {
			t.Fatalf("aggro hate %v (%d listed), want 0 (1 listed)", hate, listed)
		}
		if n := f.Desires().Len(); n != 0 {
			t.Fatalf("%d desires left, want 0", n)
		}
	})
}

// shownOnScreen returns the text of each ExShowScreenMessage among frames
// and the duration it is shown for.
func shownOnScreen(frames [][]byte) (texts []string, durations []int32) {
	for _, f := range frames {
		if len(f) < 3 || f[0] != serverpackets.OpcodeExtended || binary.LittleEndian.Uint16(f[1:3]) != serverpackets.OpcodeExShowScreenMessage {
			continue
		}
		r := wire.NewReader(f[3:])
		for range 8 {
			r.ReadInt32()
		}
		durations = append(durations, r.ReadInt32())
		r.ReadInt32()
		texts = append(texts, r.ReadString())
	}
	return texts, durations
}

// TestScriptCivilianBroadcastOnScreen pins Npc.broadcastOnScreen on a
// civilian NPC: the player around it sees the text for the duration given.
func TestScriptCivilianBroadcastOnScreen(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	f := w.srv.SpawnFolkNPCAt(t, folkTemplate("Folk", 35596), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	drainFrames(t, w.c)

	script.NPCOf(f).BroadcastOnScreen(3000, "the watering game begins")
	texts, durations := shownOnScreen(drainFrames(t, w.c))
	if len(texts) != 1 || texts[0] != "the watering game begins" || durations[0] != 3000 {
		t.Fatalf("screen messages %q for %v ms, want one of 3000 ms", texts, durations)
	}
}
