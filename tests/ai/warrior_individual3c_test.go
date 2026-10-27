package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the scenarios of the third individual behavior batch's
// trailing PR spawn (#188 PR 2b), each a fixture monster of level 1.
const (
	ind3cPetID       = int32(22104) // FreyaPet
	ind3cSecretaryID = int32(22102) // FreyaSecretaryLeader
	ind3cCalledID    = int32(18327) // the private the secretary calls
	ind3cSelfBuff    = modelskill.ID(9711)
	ind3cClericClass = 15 // a class of the @cleric_group
	ind3cGateID      = 19210031
)

// ind3cBatch are the PR's classes, in scripts.xml order.
var ind3cBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaPet", warrior.FreyaPet},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaSecretaryLeader", warrior.FreyaSecretaryLeader},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarriorAggressive", warrior.PartyLeaderWarriorAggressive},
}

// bootIndividual3c boots the PR's classes over bootIndividual3b's world:
// its scripts, draws, director and listener on ind3bListenerID.
func bootIndividual3c(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, cat := partyScripts()
	for _, c := range ind3cBatch {
		list = append(list, script.Listing{Path: c.path})
		cat[c.path] = c.ctor
	}
	heard := &ind1Heard{}
	const listener, director = "test.Ind3bListener", "test.Ind1Director"
	cat[listener] = func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ind3bListenerID}, Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				heard.mu.Lock()
				defer heard.mu.Unlock()
				heard.events = append(heard.events, fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
			},
		}}
	}
	cat[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: listener}, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, cat),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: heard}
}

// petTemplate is the fixture FreyaPet with its self buff and privates of
// the fixture party private, each respawning after 240 s.
func petTemplate(privates int) *npc.Template {
	tmpl := leaderTemplate(ind3cPetID, nil, privates)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypeSelfBuff: {ID: ind3cSelfBuff, Level: 1}}
	return tmpl
}

// secretaryTemplate is the fixture FreyaSecretaryLeader, leading one
// listener respawning after 240 s when led.
func secretaryTemplate(led bool) *npc.Template {
	tmpl := warriorTemplate(ind3cSecretaryID, nil)
	if led {
		tmpl.Privates = []npc.PrivateEntry{{NpcID: int(ind3bListenerID), Weight: 1, RespawnDelay: 240 * time.Second}}
	}
	return tmpl
}

// setClass makes c, a character in the world, play class.
func (w *ind1World) setClass(t *testing.T, c attackable.Combatant, class int) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, c.ObjectID()).Post(func() {
		defer close(done)
		c.(player.CharacterHolder).PlayerCharacter().SetClassID(class)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// moveNear puts c, a character in the world, dx from h.
func (w *ind1World) moveNear(t *testing.T, c attackable.Combatant, h *npc.Hostile, dx int) {
	t.Helper()
	obj, ok := w.srv.State.Player(c.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	x, y, z := h.Position()
	if err := w.srv.State.Move(obj, x+dx, y, z); err != nil {
		t.Fatalf("move the character: %v", err)
	}
}

// slotID is the object id of the creature h's script memory holds in
// slot, 0 for none.
func slotID(h *npc.Hostile, slot script.CreatureSlot) int32 {
	if c := script.NPCOf(h).Slot(slot); c != nil {
		return c.ObjectID()
	}
	return 0
}

// noPrivateRespawn fails when a script private's slot has a respawn
// pending.
func (w *ind1World) noPrivateRespawn(t *testing.T, when string) {
	t.Helper()
	for i := 1; i <= 64; i++ {
		if key := fmt.Sprintf("private#%d", i); w.srv.NpcRespawns.Tracked(key) {
			t.Fatalf("%s: private slot %s armed a respawn, want none", when, key)
		}
	}
}

// TestFreyaPetRemembersTheClericsItSees ports FreyaPet's created and
// see-creature hooks: it starts with c_ai0 to c_ai2 empty, and fills them,
// in turn, with the first three clerics it sees; a fourth finds them
// full. A character of another class is never remembered, and every
// character it sees still gets the parent's hate.
func TestFreyaPetRemembersTheClericsItSees(t *testing.T) {
	t.Parallel()
	w := bootIndividual3c(t, raidBoot{}, petTemplate(0))
	n := w.spawn(t, ind3cPetID, 1500, 0)
	mate := w.joinCharacter(t, "mate", "Mate")
	third := w.joinCharacter(t, "third", "Third")
	fourth := w.joinCharacter(t, "fourth", "Fourth")
	fifth := w.joinCharacter(t, "fifth", "Fifth")
	slots := func() []int32 {
		return []int32{slotID(n, script.SlotCreature0), slotID(n, script.SlotCreature1), slotID(n, script.SlotCreature2)}
	}
	w.ticks(t, 3)
	if got := slots(); !slices.Equal(got, []int32{0, 0, 0}) {
		t.Fatalf("created c_ai0..2 = %v, want none", got)
	}

	w.moveNear(t, w.player, n, -150)
	w.ticks(t, 1)
	if got := slots(); !slices.Equal(got, []int32{0, 0, 0}) {
		t.Fatalf("c_ai0..2 after seeing a fighter = %v, want none", got)
	}
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the fighter it sees = %v, want 200", got)
	}

	want := []int32{0, 0, 0}
	for i, c := range []attackable.Combatant{mate, third, fourth, fifth} {
		w.setClass(t, c, ind3cClericClass)
		w.moveNear(t, c, n, 150)
		w.ticks(t, 1)
		if i < 3 {
			want[i] = c.ObjectID()
		}
		if got := slots(); !slices.Equal(got, want) {
			t.Fatalf("c_ai0..2 after seeing cleric %d = %v, want %v", i+1, got, want)
		}
		if got := n.AggroHate(c); got <= 0 {
			t.Fatalf("hate of cleric %d = %v, want the parent's", i+1, got)
		}
	}
}

// TestFreyaPetTurnsOnItsAttacker ports FreyaPet's attacked hook. A first
// draw under 5 wants its self buff, weight 1000000. A second draw under 3
// drops all its hate for 1000000 on the attacker, stores the attacker in
// its flag and calls those within 2000 with event 10002 and its id; it
// shouts the 1000399 line naming the attacker only when the attacker was
// already its top desire target. Under 80% of its HP every hit calls those
// within 2000 with event 10034. Draws of 5 and 3 do none of it.
func TestFreyaPetTurnsOnItsAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual3c(t, raidBoot{}, petTemplate(0), warriorTemplate(ind3bListenerID, nil))
	n := w.spawn(t, ind3cPetID, 1500, 0)
	w.srv.InstallNPCCastSeam(t, n, modelskill.NewTable([]modelskill.Definition{ind1Skill(ind3cSelfBuff, 0, true)}))
	x, y, z := n.Position()
	if _, err := w.srv.NpcSpawns.AddSpawn(ind3bListenerID, x+1500, y, z, 0, false, 0); err != nil {
		t.Fatalf("spawn the listener: %v", err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	me := w.player.ObjectID()
	name := w.charOf(t).Name()

	w.draws.set(99, 5, 3)
	w.hit(t, n, 1)
	w.wantCasts(t, "draws of 5 and 3", n, nil)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("draws of 5 and 3: the listener heard %q, want nothing", got)
	}
	if got := ind1Slot(n, script.SlotFlag); got != 0 {
		t.Fatalf("draws of 5 and 3: flag = %d, want 0", got)
	}

	w.draws.set(99, 4, 2)
	w.hit(t, n, 1)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	w.wantCasts(t, "draws of 4 and 2", n, map[modelskill.ID]ind1Cast{ind3cSelfBuff: {n.ObjectID(), 1000000}})
	if got := w.heard.take(); !slices.Equal(got, []string{fmt.Sprintf("event=10002 arg1=%d arg2=0", n.ObjectID())}) {
		t.Fatalf("draws of 4 and 2: the listener heard %q, want event 10002 with the pet's id", got)
	}
	if got := ind1Slot(n, script.SlotFlag); got != me {
		t.Fatalf("draws of 4 and 2: flag = %d, want the character %d", got, me)
	}
	if got := n.AggroHate(w.player); got < 1000000 {
		t.Fatalf("draws of 4 and 2: hate of the attacker = %v, want 1000000 or more", got)
	}
	if got := says(frames, n.ObjectID()); len(got) != 0 {
		t.Fatalf("draws of 4 and 2 before it targets the attacker: lines %q, want none", got)
	}

	w.setTop(t, n, w.charOf(t))
	w.draws.set(99, 99, 2)
	w.hit(t, n, 1)
	frames = w.srv.ReadQueued(t, w.srv.Client)
	if got := says(frames, n.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000399, name)}) {
		t.Fatalf("a draw of 2 on its target: lines %q, want the 1000399 line naming %s", got, name)
	}
	w.wantCasts(t, "a first draw of 99", n, nil)
	w.heard.take()

	w.setHPShare(t, n, 0.5)
	w.draws.set(99)
	w.hit(t, n, 1)
	if got := w.heard.take(); !slices.Equal(got, []string{"event=10034 arg1=0 arg2=0"}) {
		t.Fatalf("a hit under 80%% of its HP: the listener heard %q, want event 10034", got)
	}
}

// TestFreyaPetTurnsOnTheClericsAsAPrivateFalls ports FreyaPet's party-died
// hook, which calls no parent. A fallen private makes it look around, which
// stores the cleric it already holds in c_ai0 again in c_ai1, then turns
// it on each creature of c_ai0 to c_ai2 still alive, in turn: it shouts
// the 1000399 line naming it, drops all its hate for 1000000 on it, stores
// it in its flag and calls those within 500 with event 10002 and its id.
// The fallen private, held in c_ai2, is skipped, and its respawn is never
// armed.
func TestFreyaPetTurnsOnTheClericsAsAPrivateFalls(t *testing.T) {
	t.Parallel()
	w := bootIndividual3c(t, raidBoot{}, petTemplate(1), warriorTemplate(partyPrivateID, nil), warriorTemplate(ind3bListenerID, nil))
	me := w.player.ObjectID()
	name := w.charOf(t).Name()
	w.setClass(t, w.player, ind3cClericClass)
	n := w.spawn(t, ind3cPetID, 300, 0)
	w.ticks(t, 2)
	if got := slotID(n, script.SlotCreature0); got != me {
		t.Fatalf("setup: c_ai0 = %d, want the cleric %d", got, me)
	}
	x, y, z := n.Position()
	if _, err := w.srv.NpcSpawns.AddSpawn(ind3bListenerID, x+400, y, z, 0, false, 0); err != nil {
		t.Fatalf("spawn the listener: %v", err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	w.heard.take()

	private := privatesOf(t, n, 1)[0]
	w.onQueue(t, n, func() { script.NPCOf(n).SetSlot(script.SlotCreature2, script.NPCOf(private)) })
	w.onQueue(t, private, func() { script.NPCOf(private).DoDie(nil) })
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := slotID(n, script.SlotCreature1); got != me {
		t.Fatalf("c_ai1 after the look = %d, want the cleric %d", got, me)
	}
	line := script.NpcString(1000399, name)
	if got := says(frames, n.ObjectID()); !slices.Equal(got, []string{line, line}) {
		t.Fatalf("lines as the private fell = %q, want the 1000399 line naming %s twice", got, name)
	}
	call := fmt.Sprintf("event=10002 arg1=%d arg2=0", n.ObjectID())
	if got := w.heard.take(); !slices.Equal(got, []string{call, call}) {
		t.Fatalf("the listener heard %q, want event 10002 with the pet's id twice", got)
	}
	if got := ind1Slot(n, script.SlotFlag); got != me {
		t.Fatalf("flag = %d, want the cleric %d", got, me)
	}
	if got := n.AggroHate(w.player); got != 1000000 {
		t.Fatalf("hate of the cleric = %v, want 1000000", got)
	}
	w.noPrivateRespawn(t, "the pet's private fell")
}

// TestFreyaSecretaryLeaderCallsHelpForAWoundedPrivate ports
// FreyaSecretaryLeader's created and party-attacked hooks. Its private
// hit at a third of its HP or more calls nothing. Under a third, each hit
// calls two privates of 18327 onto the private's spot, with spawn
// parameters 1000 and the attacker, and the client sees them appear; the
// private hears event 10023, and i_ai1 counts the call. At 15 calls it
// calls no more.
func TestFreyaSecretaryLeaderCallsHelpForAWoundedPrivate(t *testing.T) {
	t.Parallel()
	w := bootIndividual3c(t, raidBoot{}, secretaryTemplate(true), warriorTemplate(ind3bListenerID, nil), warriorTemplate(ind3cCalledID, nil))
	n := w.spawn(t, ind3cSecretaryID, 1500, 0)
	me := w.player.ObjectID()
	if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
		t.Fatalf("created i_ai1 = %d, want 0", got)
	}
	private := privatesOf(t, n, 1)[0]
	w.heard.take()

	w.hit(t, private, 1)
	privatesOf(t, n, 1)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("a hit at full HP: the private heard %q, want nothing", got)
	}

	w.setHPShare(t, private, 0.3)
	px, py, _ := private.Position()
	w.hit(t, private, 1)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	called := privatesOf(t, n, 3)
	if got := w.heard.take(); !slices.Equal(got, []string{"event=10023 arg1=0 arg2=0"}) {
		t.Fatalf("a hit under a third of its HP: the private heard %q, want event 10023", got)
	}
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after the call = %d, want 1", got)
	}
	for _, c := range called {
		if c.ObjectID() == private.ObjectID() {
			continue
		}
		if c.NpcID() != int(ind3cCalledID) {
			t.Fatalf("called private npc id = %d, want %d", c.NpcID(), ind3cCalledID)
		}
		if cx, cy, _ := c.Position(); cx != px || cy != py {
			t.Fatalf("called private at %d,%d, want the private's spot %d,%d", cx, cy, px, py)
		}
		if p1, p2, p3 := ind1Slot(c, script.SlotParam1), ind1Slot(c, script.SlotParam2), ind1Slot(c, script.SlotParam3); p1 != 1000 || p2 != me || p3 != 0 {
			t.Fatalf("called private spawn parameters = %d, %d, %d, want 1000, %d, 0", p1, p2, p3, me)
		}
		if got := framesOf(frames, serverpackets.OpcodeNPCInfo, c.ObjectID()); len(got) != 2 {
			t.Fatalf("NpcInfo frames of the called private = %d, want 2 (appears walking, then running)", len(got))
		}
	}

	w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI1, 15) })
	w.hit(t, private, 1)
	privatesOf(t, n, 3)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("a hit at 15 calls: the private heard %q, want nothing", got)
	}
}

// TestFreyaSecretaryLeaderAttacksOnSightAndLetsPrivatesLie ports
// FreyaSecretaryLeader's see-creature and party-died hooks. Seeing the
// character inside its territory it adds hate 200 before the parent's
// 200. Its party-died hook does nothing: a fallen private's respawn is
// never armed.
func TestFreyaSecretaryLeaderAttacksOnSightAndLetsPrivatesLie(t *testing.T) {
	t.Parallel()
	w := bootIndividual3c(t, raidBoot{}, secretaryTemplate(true), warriorTemplate(ind3bListenerID, nil))
	n := w.spawn(t, ind3cSecretaryID, 1500, 0)
	w.ticks(t, 3)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate before it saw the character = %v, want 0", got)
	}
	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got != 400 {
		t.Fatalf("hate of the character it sees = %v, want 400", got)
	}

	private := privatesOf(t, n, 1)[0]
	w.kill(t, private)
	w.noPrivateRespawn(t, "the secretary's private fell")
}

// sirrSpawnlist places Ice Fairy Sirra's npcmaker under its datapack name,
// following the gate, with the listener as its NPC.
const sirrSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="sirr_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="schuttgart13_mb2314_05m1" territory="sirr_field" maximumNpcs="1">
		<ai type="ice_fairy_sirr_maker"><set name="DoorName" val="sirr_gate"/></ai>
		<npc id="29982" total="1" pos="50;20;30;0" respawn="no"/>
	</npcmaker>
</list>`

// TestFreyaPetAndSecretaryTellSirraAsTheyDie ports the dying hooks of
// FreyaPet and FreyaSecretaryLeader, neither calling a parent: each runs
// maker script event 10005 on the npcmaker schuttgart13_mb2314_05m1, whose
// Ice Fairy Sirra maker counts it and sends its NPC event 10001 with the
// count, then calls those around it with event 11036: arg 2 within 7000
// for the pet, arg 1 within 8000 for the secretary.
func TestFreyaPetAndSecretaryTellSirraAsTheyDie(t *testing.T) {
	t.Parallel()
	sirr := catalog.Makers()["makers_3"]["ice_fairy_sirr_maker"]
	if sirr == nil {
		t.Fatal("no maker is bound to ice_fairy_sirr_maker")
	}
	gate := &door.Template{
		ID: ind3cGateID, Name: "sirr_gate", Kind: door.KindDoor, Level: 1,
		Position:    location.Location{X: 600, Y: 0, Z: 0},
		Coordinates: []location.Point{{X: 592, Y: -8}, {X: 608, Y: -8}, {X: 608, Y: 8}, {X: 592, Y: 8}},
		HP:          100, PDef: 10, MDef: 10, Height: 32,
	}
	w := bootIndividual3c(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithNpcSpawns(ind3bSpawnTable(t, sirrSpawnlist)),
		gameservertest.WithMakers(script.MakerCatalog{"ice_fairy_sirr_maker": sirr}),
		gameservertest.WithDoors(gate),
	}},
		petTemplate(0), secretaryTemplate(false), warriorTemplate(ind3bListenerID, nil))
	for _, open := range []bool{true, false} {
		w.srv.WorldObjects.SetDoorOpen(ind3cGateID, open)
		w.srv.Settle(t)
	}
	var sirra *npc.Hostile
	w.srv.State.FindObject(func(o world.Tracked) bool {
		h, ok := o.(*npc.Hostile)
		if ok && h.NpcID() == int(ind3bListenerID) {
			sirra = h
		}
		return ok && sirra != nil
	})
	if sirra == nil {
		t.Fatal("setup: the gate closing spawned no Sirra")
	}
	x, y, z := sirra.Position()
	place := func(id int32, dx int) *npc.Hostile {
		t.Helper()
		n, err := w.srv.NpcSpawns.AddSpawn(id, x+dx, y, z, 0, false, 0)
		if err != nil {
			t.Fatalf("spawn %d: %v", id, err)
		}
		w.srv.Settle(t)
		return n.(*npc.Hostile)
	}
	pet := place(ind3cPetID, 6900)
	secretary := place(ind3cSecretaryID, -7900)
	w.heard.take()

	w.die(t, pet)
	if got := w.heard.take(); !slices.Equal(got, []string{"event=10001 arg1=1 arg2=0", "event=11036 arg1=2 arg2=0"}) {
		t.Fatalf("Sirra heard %q as the pet died, want 10001 with count 1, then 11036 with 2", got)
	}
	w.die(t, secretary)
	if got := w.heard.take(); !slices.Equal(got, []string{"event=10001 arg1=2 arg2=0", "event=11036 arg1=1 arg2=0"}) {
		t.Fatalf("Sirra heard %q as the secretary died, want 10001 with count 2, then 11036 with 1", got)
	}
}
