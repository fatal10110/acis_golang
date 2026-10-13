package npcs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The maker operations of a behavior (#3748): the npcmaker that placed an
// NPC, read on its handle, and a maker script event sent by npcmaker name
// from a hook, as RaidBossForTeleportDungeon's created hook sends "2" to its
// own maker and AlarmDevice sends "1000" to another one.
const (
	makerOpsSenderID = 21401
	makerOpsTargetID = 21402
)

const makerOpsSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="ops_sender" territory="field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21401" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="ops_target" territory="field" maximumNpcs="1">
		<ai type="no_on_start_maker"/>
		<npc id="21402" total="1" pos="60;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// makerOpsSeen is what the sender's created hook saw: its maker's name and
// whether each maker event found its npcmaker.
type makerOpsSeen struct {
	maker          string
	found, missing bool
}

// makerOpsSender is a behavior-like script whose created hook reads the
// maker of the NPC created and sends the target maker its 1001 event.
func makerOpsSender(seen chan<- makerOpsSeen) func() script.Script {
	return func() script.Script {
		return script.Script{
			Bind: script.Bindings{script.EventCreated: {makerOpsSenderID}},
			Hooks: script.Hooks{
				OnCreated: func(s *script.Script, e script.Created) {
					got := makerOpsSeen{maker: e.NPC.MakerName()}
					got.found = s.Spawner().SendMakerEvent("OPS_TARGET", "1001", 0, 0)
					got.missing = s.Spawner().SendMakerEvent("no_such_maker", "1001", 0, 0)
					seen <- got
				},
			},
		}
	}
}

// A hook reads the npcmaker that placed its NPC, and a maker event it sends
// runs on the npcmaker of that name, found ignoring case, before the send
// returns: the target, a no-on-start maker, fills itself on 1001. A name no
// npcmaker has reports false and runs nothing. An NPC no npcmaker placed
// has no maker name: asking panics.
func TestScriptMakerNameAndMakerEvent(t *testing.T) {
	t.Parallel()
	executors(t, func(t *testing.T, opts ...gameservertest.Option) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "ops.xml"), []byte(makerOpsSpawnlist), 0o644); err != nil {
			t.Fatalf("write spawnlist: %v", err)
		}
		table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
		if err != nil {
			t.Fatalf("load spawnlist: %v", err)
		}
		seen := make(chan makerOpsSeen, 4)
		kinds := map[int32]script.NPCKind{makerOpsSenderID: script.KindHostile, makerOpsTargetID: script.KindHostile}
		srv := gameservertest.Boot(t, append([]gameservertest.Option{
			gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
				spawnMonster(makerOpsSenderID, "Sender"),
				spawnMonster(makerOpsTargetID, "Target"),
			})),
			gameservertest.WithNpcSpawns(table),
			gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "quest.MakerOpsSender"}},
				script.Catalog{"quest.MakerOpsSender": makerOpsSender(seen)}),
		}, opts...)...)

		select {
		case got := <-seen:
			if want := (makerOpsSeen{maker: "ops_sender", found: true}); got != want {
				t.Fatalf("created hook saw %+v, want %+v", got, want)
			}
		default:
			t.Fatal("the sender's created hook did not run at boot")
		}
		srv.Advance(t, time.Millisecond)
		target := liveOf(t, srv, makerOpsTargetID).(attackable.Combatant)
		if got := script.NPCOf(target).MakerName(); got != "ops_target" {
			t.Fatalf("target's maker = %q, want ops_target", got)
		}

		loose := srv.SpawnHostileNPCAt(t, location.Location{X: 200, Y: 200, Z: 30})
		var name string
		panicked := func() (p bool) {
			defer func() { p = recover() != nil }()
			name = script.NPCOf(loose).MakerName()
			return false
		}()
		if !panicked {
			t.Fatalf("MakerName of an npc no npcmaker placed = %q, want a panic", name)
		}
	})
}
