package npcs

import (
	"os"
	"path/filepath"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The env-spawner fixture: a keeper whose script spawns a ghost beside it
// as the keeper is created, the way Q023 spawns its ghost from an event.
const (
	envKeeperID = 21301
	envGhostID  = 21302
)

const envSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="env_keeper" territory="field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21301" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

func envSpawnTable(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "env.xml"), []byte(envSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// ghostKeeper is a quest-like script holding its ghost in a shared cell:
// the keeper's creation spawns the ghost through the script's own spawner
// when the cell is empty, and the decay of the NPC held clears it.
func ghostKeeper(ghost *script.NPCCell) func() script.Script {
	return func() script.Script {
		return script.Script{
			Bind: script.Bindings{
				script.EventCreated: {envKeeperID},
				script.EventDecayed: {envKeeperID, envGhostID},
			},
			Hooks: script.Hooks{
				OnCreated: func(s *script.Script, e script.Created) {
					if !ghost.Empty() {
						return
					}
					ghost.SetIfEmpty(s.Spawner().AddSpawn(envGhostID, e.NPC, false, 0))
				},
				OnDecayed: func(_ *script.Script, e script.Decayed) {
					ghost.ClearIf(e.NPC)
				},
			},
		}
	}
}

// A script hook reaches the NPC spawner through its own script, from the
// first boot spawn on: the keeper's created hook spawns the ghost into the
// world and keeps it in a script-shared cell. The keeper's own decay
// leaves the cell alone; the ghost's clears it.
func TestScriptHookSpawnsThroughItsEnvSpawner(t *testing.T) {
	t.Parallel()
	executors(t, func(t *testing.T, opts ...gameservertest.Option) {
		ghost := &script.NPCCell{}
		kinds := map[int32]script.NPCKind{envKeeperID: script.KindHostile, envGhostID: script.KindHostile}
		srv := gameservertest.Boot(t, append([]gameservertest.Option{
			gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
				spawnMonster(envKeeperID, "Keeper"),
				spawnMonster(envGhostID, "Ghost"),
			})),
			gameservertest.WithNpcSpawns(envSpawnTable(t)),
			gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "quest.GhostKeeper"}},
				script.Catalog{"quest.GhostKeeper": ghostKeeper(ghost)}),
		}, opts...)...)

		keeper := liveOf(t, srv, envKeeperID).(attackable.Combatant)
		spawned := liveOf(t, srv, envGhostID).(attackable.Combatant)
		held := ghost.Get()
		if held == nil || held.ObjectID() != spawned.ObjectID() {
			t.Fatalf("cell holds %v, want the ghost %d spawned at boot", held, spawned.ObjectID())
		}
		kx, ky, _ := keeper.Position()
		if gx, gy, _ := spawned.Position(); gx != kx || gy != ky {
			t.Fatalf("ghost stands at (%d, %d), want the keeper's (%d, %d)", gx, gy, kx, ky)
		}

		onQueueOf(t, keeper, func() { script.NPCOf(keeper).DeleteMe() })
		if held := ghost.Get(); held == nil || held.ObjectID() != spawned.ObjectID() {
			t.Fatal("the keeper's decay cleared the ghost's cell")
		}

		onQueueOf(t, spawned, func() { script.NPCOf(spawned).DeleteMe() })
		if !ghost.Empty() {
			t.Fatalf("cell holds %v after the ghost decayed, want empty", ghost.Get())
		}
		if _, in := srv.State.Object(spawned.ObjectID()); in {
			t.Fatal("deleted ghost is still in the world")
		}
	})
}
