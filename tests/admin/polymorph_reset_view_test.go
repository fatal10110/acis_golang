package admin

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// npcInfoRunningAt is the offset of an NpcInfo frame's running flag: after
// the opcode, the object, template and attackable ids, the position and
// heading, a zero, the two attack speeds, the eight move speeds, the four
// multiplier and collision doubles, the three paperdoll ids and the
// name-above-head byte.
const npcInfoRunningAt = 1 + 4*3 + 4*4 + 4 + 4*2 + 4*8 + 8*4 + 4*3 + 1

// npcInfoMoveTypeOf reads an NpcInfo frame's move type byte: the field
// after the ally crest, followed by the team byte, the collision radius and
// height (two doubles), the enchant effect and the flying flag.
func npcInfoMoveTypeOf(frame []byte) byte { return frame[len(frame)-1-(1+8+8+4+4)] }

// npcViewTrace names, in order, the NpcInfo, DeleteObject and
// ChangeMoveType frames about objID: an NpcInfo with its template id, run
// flag and move type.
func npcViewTrace(frames [][]byte, objID int32) []string {
	var out []string
	for _, f := range frames {
		if len(f) < 5 || wire.NewReader(f[1:]).ReadInt32() != objID {
			continue
		}
		switch f[0] {
		case serverpackets.OpcodeNPCInfo:
			tmpl := wire.NewReader(f[5:]).ReadInt32() - 1_000_000
			out = append(out, fmt.Sprintf("NpcInfo(%d run=%d move=%d)", tmpl, f[npcInfoRunningAt], npcInfoMoveTypeOf(f)))
		case serverpackets.OpcodeDeleteObject:
			out = append(out, "DeleteObject")
		case serverpackets.OpcodeChangeMoveType:
			out = append(out, "ChangeMoveType")
		}
	}
	return out
}

// TestAdminPolymorphReshowsThroughZones pins what the players around an
// NPC see of a //polymorph re-show (Creature.polymorph: decayMe, spawnMe;
// Creature.setRegion, Creature.java:1773-1791; WaterZone.onExit and
// onEnter; Npc.onSpawn's forceRunStance): the NPC leaves its water zone
// while they still know it (NpcInfo on the ground), they forget it, see it
// again in its form, and see it swim once it enters the water again; it
// holds the water zone afterwards. A walking NPC then shows its run stance
// once more; a running one shows nothing more.
func TestAdminPolymorphReshowsThroughZones(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		walking bool
	}{{"Running", false}, {"Walking", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			box, err := zone.NewCuboid(spawnX+30, spawnX+1_000, spawnY-1_000, spawnY+1_000, -1_000, 1_000)
			if err != nil {
				t.Fatalf("water box: %v", err)
			}
			zones := zone.NewIndex()
			zones.Add(zone.NewWater(1, box))
			own, templates := resetTemplates("Monster")
			srv, _ := bootAdmin(t, adminLevel, gameservertest.WithNPCs(templates), gameservertest.WithZones(zones))
			gm := srv.Client
			enterWorld(t, gm)
			monster := srv.SpawnHostileNPCTemplateAt(t, own, location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
			if !monster.InsideZone(zone.FlagWater) {
				t.Fatal("setup: the NPC spawned in the water is not in it")
			}
			if tc.walking && !monster.SetRunning(false) {
				t.Fatal("setup: the NPC did not take its walk stance")
			}
			exchange(t, gm, encodeAction(monster.ObjectID()))
			srv.Settle(t)
			drain(t, gm)

			frames := exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
			srv.Settle(t)
			frames = append(frames, gm.ReadQueued()...)

			form := polymorphNPCID + 1
			run := 1
			if tc.walking {
				run = 0
			}
			view := func(run, move int) string { return fmt.Sprintf("NpcInfo(%d run=%d move=%d)", form, run, move) }
			want := []string{view(run, 0), "DeleteObject", view(run, 0), view(run, 1)}
			if tc.walking {
				want = append(want, "ChangeMoveType", view(1, 1))
			}
			if got := npcViewTrace(frames, monster.ObjectID()); !slices.Equal(got, want) {
				t.Fatalf("NPC frames of the polymorph =\n%q\nwant\n%q", got, want)
			}
			if !monster.InsideZone(zone.FlagWater) {
				t.Fatal("the polymorphed NPC left the water zone it stands in")
			}
			if !monster.Running() {
				t.Fatal("the polymorphed NPC is not running")
			}
		})
	}
}
