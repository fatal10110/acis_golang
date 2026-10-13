package ai

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ootRoamerID is a walking NPC bound to a behavior that records its
// out-of-territory hook.
const ootRoamerID = int32(29096)

// ootDen is an npcmaker spawning one roamer, which respawns, in a 400 by
// 400 territory.
const ootDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="oot_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="oot_den" territory="oot_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="29096" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// ootDenKey is the slot key of the den's roamer.
const ootDenKey = "oot_den#0#0"

// ootRoamWorld is the character beside the den, with the roamer's hook log.
type ootRoamWorld struct {
	srv *gameservertest.Server
	log *folkLog
}

// bootOOTDen boots the den with a roamer of template type typ, Monster or
// Folk.
func bootOOTDen(t *testing.T, typ string) *ootRoamWorld {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(ootDen), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	log := &folkLog{}
	catalog := script.Catalog{"ai.OOTRoamer": func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ootRoamerID}, Hooks: script.Hooks{
			OnOutOfTerritory: func(_ *script.Script, _ script.OutOfTerritory) { log.add("OUT_OF_TERRITORY") },
		}}
	}}
	tmpl := &npc.Template{
		ID: int(ootRoamerID), TemplateID: int(ootRoamerID), Type: typ, Name: "Roamer", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CollisionRadius: 10, CollisionHeight: 20,
	}
	kind := script.KindHostile
	if typ == "Folk" {
		kind = script.KindFolk
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl})),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithAITask(),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{ootRoamerID: kind},
			[]script.Listing{{Path: "ai.OOTRoamer"}}, catalog),
	)
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	return &ootRoamWorld{srv: srv, log: log}
}

// roamer is one life of the den's roamer, of either kind.
type roamer interface {
	attackable.Combatant
	IsMoving() bool
	InTerritory() bool
	Queue() *sim.Queue
	Decay(*world.State, func()) bool
	Decayed() bool
}

// roamer returns the den's live roamer.
func (w *ootRoamWorld) roamer(t *testing.T) roamer {
	t.Helper()
	var live []roamer
	for _, obj := range w.srv.State.Objects() {
		if r, ok := obj.(roamer); ok && r.ObjectID() != w.srv.SoleObjectID(t) && !r.Decayed() {
			live = append(live, r)
		}
	}
	if len(live) != 1 {
		t.Fatalf("live roamers = %d, want 1", len(live))
	}
	return live[0]
}

// walkTo has r walk to to through its AI and waits for its arrival.
func (w *ootRoamWorld) walkTo(t *testing.T, r roamer, to location.Location) {
	t.Helper()
	var queued bool
	switch n := r.(type) {
	case *npc.Hostile:
		queued = n.AI().AddMoveToDesire(to, 1e6)
	case *npc.Folk:
		queued = n.AddMoveToDesire(to, 1e6)
	}
	if !queued {
		t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", to)
	}
	for i := 0; !r.IsMoving(); i++ {
		if i == 5 {
			t.Fatalf("no walk to %+v after five cycles", to)
		}
		cycle(t, w.srv)
	}
	for i := 0; r.IsMoving(); i++ {
		if i >= int(20*time.Second/move.PositionUpdateInterval) {
			t.Fatal("walk never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// hates reports whether r hates c.
func hates(r roamer, c attackable.Combatant) bool {
	switch n := r.(type) {
	case *npc.Hostile:
		return n.AggroHate(c) > 0
	case *npc.Folk:
		hated, ok := n.MostHated()
		return ok && hated.ObjectID() == c.ObjectID()
	}
	return false
}

// hooks asserts the out-of-territory hooks run since the last call.
func (w *ootRoamWorld) hooks(t *testing.T, when string, want ...string) {
	t.Helper()
	if got := w.log.take(); !slices.Equal(got, want) {
		t.Fatalf("hooks %s = %q, want %q", when, got, want)
	}
}

// TestRespawnKeepsOutOfTerritoryState pins the out-of-territory state of an
// NPC across a respawn (NpcAI.onEvtArrived keeps _isOOT and its sweep task
// on the Npc object ASpawn.doRespawn reuses, and nothing resets them): a
// roamer that died outside its territory raises no out-of-territory hook on
// its next life's first arrival outside. Only an arrival back inside clears
// the state, so the arrival outside after that raises the hook again.
func TestRespawnKeepsOutOfTerritoryState(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{"Monster", "Folk"} {
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			respawnKeepsOutOfTerritoryState(t, typ)
		})
	}
}

func respawnKeepsOutOfTerritoryState(t *testing.T, typ string) {
	w := bootOOTDen(t, typ)
	first := w.roamer(t)
	home := location.Location{X: 40, Y: 20, Z: 30}
	out := location.Location{X: 40, Y: 700, Z: 30}

	w.walkTo(t, first, out)
	if first.InTerritory() {
		t.Fatal("the first walk ended inside the territory, want outside")
	}
	w.hooks(t, "on the first life's arrival outside", "OUT_OF_TERRITORY")

	done := make(chan struct{})
	if !first.Queue().Post(func() {
		defer close(done)
		switch n := first.(type) {
		case *npc.Hostile:
			script.NPCOf(n).DoDie(nil)
		case *npc.Folk:
			n.Kill(nil)
		}
		first.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(first.ObjectID()))
	}) {
		t.Fatal("the roamer's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	w.srv.NpcSpawns.Respawn(ootDenKey)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	second := w.roamer(t)
	if second == first {
		t.Fatal("the slot did not respawn its roamer")
	}
	if !second.InTerritory() {
		t.Fatal("the respawned roamer stands outside its territory, want home")
	}

	w.walkTo(t, second, out)
	w.hooks(t, "on the respawned life's first arrival outside")
	if w.srv.DrivesClock() {
		// The first life's stale-hate sweep goes on, on its grid: hate
		// stamped now is stopped within 100 s, though this life's arrival
		// outside armed nothing.
		player := characterOf(t, w.srv)
		switch n := second.(type) {
		case *npc.Hostile:
			n.AddDamageHate(player, 0, 2000)
		case *npc.Folk:
			// The attack desire starts a chase, which the teleport (no
			// arrival) stops short.
			n.AddAttackDesire(player, 2000)
			n.TeleportTo(out)
		}
		w.srv.ReadQueued(t, w.srv.Client)
		if !hates(second, player) {
			t.Fatal("setup: the respawned roamer does not hate the character")
		}
		w.srv.Advance(t, 100*time.Second)
		if hates(second, player) {
			t.Fatal("the respawned roamer still hates the character 100 s on, outside: want the carried sweep to stop it")
		}
		if second.InTerritory() {
			t.Fatal("the respawned roamer went back inside during the sweep")
		}
	}

	w.walkTo(t, second, home)
	w.hooks(t, "on arriving back home")
	w.walkTo(t, second, out)
	w.hooks(t, "on leaving again after an arrival home", "OUT_OF_TERRITORY")
}
