package pets

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/baium"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// Baium and its stone.
const (
	baiumID      = int32(29020)
	baiumStoneID = int32(29025)
)

// baiumGoneWaker is the object id of a waker that has left the world.
const baiumGoneWaker = math.MaxInt32

// baiumLairSpawnlist is the shipped lair territory and Baium's npcmaker,
// with Baium spawning outside its lair.
const baiumLairSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="23_18_baium_npc" minZ="10036" maxZ="10836">
		<node x="117660" y="15328"/>
		<node x="117508" y="16916"/>
		<node x="116688" y="18088"/>
		<node x="115180" y="19080"/>
		<node x="112364" y="17856"/>
		<node x="111680" y="15524"/>
		<node x="113820" y="13252"/>
		<node x="116484" y="13856"/>
	</territory>
	<npcmaker name="23_18_baium_npc" territory="23_18_baium_npc" maximumNpcs="10">
		<ai type="default_maker"/>
		<npc id="29020" pos="-105200;-253104;-15264;0" total="1" respawn="168hour" respawnRand="48hour" dbName="baium" dbSaving="DEATH_TIME;PARAMETERS;POS"/>
	</npcmaker>
</list>`

// TestBaiumCallsTheSummonItSawWhenItsWakerIsGone (Baium.java onTimer
// 2005: with its waker gone from the world it calls the creature it saw
// first, _c_ai1, through Creature.teleportTo, which a summon is): the
// owner's talk to the stone wakes Baium, and the test then names the
// owner's wolf as the creature Baium saw first and a waker no longer in
// the world as its waker. 18.8 s after the wake Baium says its
// line 22937, and the owner sees one ordinary TeleportToLocation of the
// wolf beside it, to 115910,17337,10105; the wolf stands there, the owner
// stays where it was, and Baium wants to cast 4136 at the wolf with
// 1100000.
func TestBaiumCallsTheSummonItSawWhenItsWakerIsGone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "baium.xml"), []byte(baiumLairSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	boss := gameservertest.MovingHostileTemplate("Baium")
	boss.ID, boss.TemplateID = int(baiumID), int(baiumID)
	boss.Type, boss.Level, boss.HPMax = "GrandBoss", 75, 1000000
	boss.AggroRange, boss.CanSeeThrough = 1000, true
	call := modelskill.Definition{
		ID: 4136, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "BUFF", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true,
	}
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{{X: 115960, Y: 17400, Z: 10107}}
	w := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			wolfTemplate(), treeTemplate(), boss, gameservertest.FolkTemplate("Folk", int(baiumStoneID)),
		})),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithSpawnData(),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable([]modelskill.Definition{call})),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{baiumID: script.KindHostile, baiumStoneID: script.KindFolk},
			[]script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: "script.ai.boss.baium.Baium"}, {Path: "script.ai.boss.baium.BaiumStone"}},
			script.Catalog{
				"script.ai.individual.DefaultNpc": defaultnpc.New,
				"script.ai.boss.baium.Baium":      baium.Baium,
				"script.ai.boss.baium.BaiumStone": baium.BaiumStone,
			}),
		gameservertest.WithHTMLPages(map[string]string{
			"default/29025.htm": `<html><body><a action="bypass -h npc_%objectId%_Quest BaiumStone">Wake Baium</a></body></html>`,
		}),
	})
	wolf, _ := w.spawnWolf(t)
	drainUntilQuiet(t, w.client)

	var baiumNPC *npc.Hostile
	var stoneNPC *npc.Folk
	for _, obj := range w.srv.State.Objects() {
		switch o := obj.(type) {
		case *npc.Hostile:
			if int32(o.Instance.Template.ID) == baiumID {
				baiumNPC = o
			}
		case *npc.Folk:
			if int32(o.Instance.Template.ID) == baiumStoneID {
				stoneNPC = o
			}
		}
	}
	if baiumNPC == nil || stoneNPC == nil {
		t.Fatalf("Baium %v and its stone %v, want both in the world", baiumNPC, stoneNPC)
	}
	sx, sy, sz := stoneNPC.Position()
	for range 2 {
		w.client.Send(encodeAction(stoneNPC.ObjectID(), int32(sx), int32(sy), int32(sz), false))
		w.srv.Settle(t)
	}
	w.client.Send(encodeBypass(fmt.Sprintf("npc_%d_Quest BaiumStone", stoneNPC.ObjectID())))
	w.srv.Settle(t)
	if !stoneNPC.Decayed() {
		t.Fatal("the stone is still in the world after the wake")
	}
	owner, ok := w.srv.State.Object(w.ownerID)
	if !ok {
		t.Fatal("the owner is not in the world")
	}
	var handle script.Creature
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.ownerID).Post(func() {
		defer close(done)
		handle = script.PlayerOf(owner.(attackable.Combatant)).ActiveSummon()
	}) {
		t.Fatal("the owner's queue is closed")
	}
	<-done
	if handle == nil || handle.ObjectID() != wolf.ObjectID() {
		t.Fatalf("the owner's summon handle = %v, want the wolf %d", handle, wolf.ObjectID())
	}
	// Baium saw the wolf first: its see-creature hook keeps the last
	// creature told within its first 10 s, and the order the owner and its
	// wolf are told in is not pinned, so the test names the wolf on its
	// queue, and a waker gone from the world.
	done = make(chan struct{})
	if !baiumNPC.Queue().Post(func() {
		defer close(done)
		script.NPCOf(baiumNPC).SetSlot(script.SlotCreature1, handle)
		script.NPCOf(baiumNPC).SetInt(script.SlotParam1, baiumGoneWaker)
	}) {
		t.Fatal("Baium's queue is closed")
	}
	<-done
	drainFrames(t, w.client)
	ox, oy, oz := w.srv.PlayerPosition(t, w.ownerID)

	// The call comes 11.5 s + 7.3 s after the wake, from two chained
	// timers; on the wall clock they can fire after the sleep, so the test
	// reads on until Baium's line and the wolf's teleport have come.
	w.srv.Advance(t, 18800*time.Millisecond)
	var frames [][]byte
	w.srv.AdvanceUntil(t, "Baium's call", func() bool {
		frames = append(frames, w.srv.ReadQueued(t, w.client)...)
		got, _ := teleports(frames, wolf.ObjectID())
		return len(got) > 0 && slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeNpcSay })
	})

	says := 0
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeNpcSay {
			says++
		}
	}
	if says != 1 {
		t.Fatalf("Baium's NpcSay frames = %d, want its one line", says)
	}
	call22937 := location.Location{X: 115910, Y: 17337, Z: 10105}
	got, fast := teleports(frames, wolf.ObjectID())
	if !slices.Equal(got, []location.Location{call22937}) || !slices.Equal(fast, []bool{false}) {
		t.Fatalf("wolf teleports = %v fast %v, want one ordinary to %v", got, fast, call22937)
	}
	if got := positionOf(wolf); got != call22937 {
		t.Fatalf("the wolf stands at %v, want %v", got, call22937)
	}
	if got, _ := teleports(frames, w.ownerID); len(got) != 0 {
		t.Fatalf("owner teleports = %v, want none", got)
	}
	if x, y, z := w.srv.PlayerPosition(t, w.ownerID); x != ox || y != oy || z != oz {
		t.Fatalf("owner at %d,%d,%d, want it kept at %d,%d,%d", x, y, z, ox, oy, oz)
	}
	var cast *actorai.Desire
	for _, d := range baiumNPC.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionCast {
			cast = &d
		}
	}
	if cast == nil || cast.Skill.ID != 4136 || cast.Weight != 1100000 || cast.FinalTarget == nil || cast.FinalTarget.ObjectID() != wolf.ObjectID() {
		t.Fatalf("Baium's cast desire = %s, want 4136 at the wolf with 1100000", fmt.Sprint(cast))
	}
}
