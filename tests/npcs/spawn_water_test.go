package npcs

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// stanceTrace describes, in order, the NpcInfo frames among frames by their
// run flag and move type and the ChangeMoveType frames by their run and swim
// flags.
func stanceTrace(frames []npcFrame) []string {
	var out []string
	for _, fr := range frames {
		switch fr.opcode {
		case serverpackets.OpcodeNPCInfo:
			out = append(out, fmt.Sprintf("NpcInfo run=%t move=%d", npcInfoRunning(fr), npcInfoMoveType(fr.raw)))
		case serverpackets.OpcodeChangeMoveType:
			r := fr.body()
			out = append(out, fmt.Sprintf("ChangeMoveType run=%d swim=%d", r.ReadInt32(), r.ReadInt32()))
		}
	}
	return out
}

// TestScriptSpawnInWaterSwimsBeforeTheRunStance spawns a monster and a
// civilian in water and on dry land. Spawning enters the zones at the spawn
// point (Creature.setRegion -> revalidateZone, from WorldObject.spawnMe)
// before Npc.onSpawn takes the run stance, so in water the player who sees
// the NPC appear gets its NpcInfo walking on the ground, again walking and
// swimming for the water entry (WaterZone.onEnter), ChangeMoveType to
// running flagged swimming (isInWater), and its NpcInfo running and
// swimming. On dry land the stance change carries no swim flag. A stance
// change while the NPC swims is flagged swimming too.
func TestScriptSpawnInWaterSwimsBeforeTheRunStance(t *testing.T) {
	t.Parallel()
	ground, swim := int(move.MoveGround), int(move.MoveSwim)
	for _, c := range []struct {
		name string
		id   int32
	}{{"monster", spawnWolfID}, {"civilian", spawnGrocerID}} {
		t.Run(c.name+" in water", func(t *testing.T) {
			t.Parallel()
			zones := zone.NewIndex()
			zones.Add(zone.NewWater(1, zoneBox(t, -200_000, 200_000)))
			w := bootScriptSpawns(t, gameservertest.WithZones(zones))
			h := w.spawner.AddSpawn(c.id, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
			if h == nil {
				t.Fatal("the NPC did not spawn")
			}
			want := []string{
				fmt.Sprintf("NpcInfo run=false move=%d", ground),
				fmt.Sprintf("NpcInfo run=false move=%d", swim),
				"ChangeMoveType run=1 swim=1",
				fmt.Sprintf("NpcInfo run=true move=%d", swim),
			}
			if got := stanceTrace(aboutNPC(drainFrames(t, w.c), h.ObjectID())); !slices.Equal(got, want) {
				t.Fatalf("spawn frames = %q, want %q", got, want)
			}

			h.SetRunning(false)
			want = []string{"ChangeMoveType run=0 swim=1", fmt.Sprintf("NpcInfo run=false move=%d", swim)}
			if got := stanceTrace(aboutNPC(drainFrames(t, w.c), h.ObjectID())); !slices.Equal(got, want) {
				t.Fatalf("walk stance frames in the water = %q, want %q", got, want)
			}
		})
		t.Run(c.name+" on land", func(t *testing.T) {
			t.Parallel()
			zones := zone.NewIndex()
			zones.Add(zone.NewWater(1, zoneBox(t, 150_000, 200_000)))
			w := bootScriptSpawns(t, gameservertest.WithZones(zones))
			h := w.spawner.AddSpawn(c.id, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
			if h == nil {
				t.Fatal("the NPC did not spawn")
			}
			want := []string{
				fmt.Sprintf("NpcInfo run=false move=%d", ground),
				"ChangeMoveType run=1 swim=0",
				fmt.Sprintf("NpcInfo run=true move=%d", ground),
			}
			if got := stanceTrace(aboutNPC(drainFrames(t, w.c), h.ObjectID())); !slices.Equal(got, want) {
				t.Fatalf("spawn frames = %q, want %q", got, want)
			}
		})
	}
}
