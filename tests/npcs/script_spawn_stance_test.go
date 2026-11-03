package npcs

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// An NPC enters the world walking and takes its run stance: the player who
// sees a script spawn it gets NpcInfo (walking), ChangeMoveType (running)
// and NpcInfo (running), a monster and a civilian alike.
func TestScriptSpawnShowsTheNPCAppearThenRun(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		id   int32
	}{{"monster", spawnWolfID}, {"civilian", spawnGrocerID}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootScriptSpawns(t)
			h := w.spawner.AddSpawn(c.id, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
			if h == nil {
				t.Fatal("the NPC did not spawn")
			}
			frames := aboutNPC(drainFrames(t, w.c), h.ObjectID())
			want := []byte{serverpackets.OpcodeNPCInfo, serverpackets.OpcodeChangeMoveType, serverpackets.OpcodeNPCInfo}
			if got := npcOpcodes(frames); !bytes.Equal(got, want) {
				t.Fatalf("spawn frames = %#x, want NpcInfo, ChangeMoveType, NpcInfo", got)
			}
			if npcInfoRunning(frames[0]) || !moveTypeRunning(frames[1]) || !npcInfoRunning(frames[2]) {
				t.Fatalf("shown running %v, stance running %v, then running %v; want walk, run, run",
					npcInfoRunning(frames[0]), moveTypeRunning(frames[1]), npcInfoRunning(frames[2]))
			}
		})
	}
}
