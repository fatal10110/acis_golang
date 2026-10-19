package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/individual"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The eighth individual behavior batch's NPC ids (#193 PR 1), the datapack
// ids of its classes and of the NPCs they spawn, and a listener that hears
// script events.
const (
	ind8AltarID      = int32(32051) // AltarDoorController, Folk
	ind8SwitchID     = int32(32058) // AreadataSwitch, Folk
	ind8CorpseID     = int32(18119) // Corpse
	ind8DaimonID     = int32(31705) // DaimonTheWhiteEyed, Folk
	ind8KeeperID     = int32(27181) // ImperialGravekeeper
	ind8SlaveID      = int32(27180) // the Imperial Slave it calls
	ind8VictimID     = int32(18150) // RoyalRushMissionNpc
	ind8KeyBoxID     = int32(31455) // the victim's KeyBox
	ind8StrongID     = int32(18158) // the victim's StrongNPC
	ind8ListenerID   = int32(29801) // a script-event listener, no class of the batch
	ind8StageNPCID   = int32(29802) // the NPC of the altar's stage maker
	ind8DoorGateID   = 19160001     // the altar's DoorName door
	ind8DoorFlameID  = 19160002     // the altar's DoorName3rdFloor door
	ind8DoorGate     = "altar_inner_gate01"
	ind8DoorFlame    = "altar_flame_gate01"
	ind8DirectorName = "Ind8Director"
)

// ind8Batch are the batch's classes, in scripts.xml order around
// DefaultNpc.
var ind8Batch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.AltarDoorController", individual.AltarDoorController},
	{"script.ai.individual.AreadataSwitch", individual.AreadataSwitch},
	{"script.ai.individual.Corpse", individual.Corpse},
	{"script.ai.individual.DaimonTheWhiteEyed", individual.DaimonTheWhiteEyed},
	{"script.ai.individual.DefaultNpc", defaultnpc.New},
	{"script.ai.individual.ImperialGravekeeper", individual.ImperialGravekeeper},
	{"script.ai.individual.RoyalRushMissionNpc", individual.RoyalRushMissionNpc},
}

// ind8World is ind1World with the batch registered, a director script to
// send script events through and a listener behavior on ind8ListenerID.
type ind8World struct {
	*ind1World
}

// bootIndividual8 boots the batch beside templates, each NPC of the kind
// its template type gives, with extra, every draw 0 until a test queues
// others.
func bootIndividual8(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind8World {
	t.Helper()
	var list []script.Listing
	catalog := script.Catalog{}
	for _, c := range ind8Batch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	heard := &ind1Heard{}
	const listener, director = "test.Ind8Listener", "test." + ind8DirectorName
	catalog[listener] = func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ind8ListenerID}, Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				heard.mu.Lock()
				defer heard.mu.Unlock()
				heard.events = append(heard.events, fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
			},
		}}
	}
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: listener}, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
		if tmpl.Type == "Folk" {
			kinds[int32(tmpl.ID)] = script.KindFolk
		}
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(ind8Skills()),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind8World{&ind1World{warriorWorld: w, draws: draws, heard: heard}}
}

// ind8Skills are the skills the batch's NPCs cast, at the levels they cast
// them: Daimon's curses and blessings, the Gravekeeper's self skill and
// the victim's four.
func ind8Skills() *modelskill.Table {
	at := func(id modelskill.ID, level int) modelskill.Definition {
		d := ind1Skill(id, 600, false)
		d.Level = level
		return d
	}
	blessing := func(id modelskill.ID, level int) modelskill.Definition {
		d := at(id, level)
		d.Offensive, d.Magic, d.SkillType, d.Power = false, false, "BUFF", 0
		return d
	}
	self := ind1Skill(4080, 0, true)
	return modelskill.NewTable([]modelskill.Definition{
		at(1206, 19), at(1083, 17), blessing(1086, 2), blessing(1059, 3),
		self, at(4384, 1), at(4385, 1), at(4386, 1), at(4387, 1),
	})
}

// ind8Folk is a civilian fixture template of id with ai as its AI
// parameters.
func ind8Folk(id int32, ai npc.AIParams) *npc.Template {
	tmpl := gameservertest.FolkTemplate("Folk", int(id))
	tmpl.AIParams = ai
	return tmpl
}

// place spawns an NPC of id dx, dy from the character through the live
// population, so its created hooks run, and returns it with the frames its
// spawn sent the character.
func (w *ind8World) place(t *testing.T, id int32, dx, dy int) (attackable.Combatant, [][]byte) {
	t.Helper()
	n, err := w.srv.NpcSpawns.AddSpawn(id, w.at.X+dx, w.at.Y+dy, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", id, err)
	}
	w.srv.Settle(t)
	return n, w.srv.ReadQueued(t, w.srv.Client)
}

// ind8Slot reads slot of c's script memory.
func ind8Slot(c attackable.Combatant, slot script.IntSlot) int32 {
	return script.NPCOf(c).Int(slot)
}

// ind8Desires returns h's queued desires of kind.
func ind8Desires(h *npc.Hostile, kind actorai.Intention) []actorai.Desire {
	var out []actorai.Desire
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == kind {
			out = append(out, d)
		}
	}
	return out
}

// TestCorpseDiesAsItSpawns runs Corpse's created hook: the corpse kills
// itself the moment it appears, so the character sees it die.
func TestCorpseDiesAsItSpawns(t *testing.T) {
	t.Parallel()
	w := bootIndividual8(t, raidBoot{}, warriorTemplate(ind8CorpseID, nil))
	c, frames := w.place(t, ind8CorpseID, 200, 0)
	corpse := c.(*npc.Hostile)
	if !corpse.Dead() {
		t.Fatal("the corpse is alive after its spawn; want it dead")
	}
	if got := framesOf(frames, serverpackets.OpcodeDie, corpse.ObjectID()); len(got) != 1 {
		t.Fatalf("Die frames of the corpse = %d, want 1", len(got))
	}
}

// TestAreadataSwitchSpeaksAsItSpawns runs AreadataSwitch's created hook: a
// Triol's Revelation says its line as it appears; its dying hook only
// calls its parent, so a kill says nothing more.
func TestAreadataSwitchSpeaksAsItSpawns(t *testing.T) {
	t.Parallel()
	w := bootIndividual8(t, raidBoot{}, ind8Folk(ind8SwitchID, npc.AIParams{"AreaName": "19_16_altar_leftroom_01"}))
	sw, frames := w.place(t, ind8SwitchID, 200, 0)
	if got := says(frames, sw.ObjectID()); !slices.Equal(got, []string{script.NpcString(1010472)}) {
		t.Fatalf("switch lines on its spawn = %q, want the 1010472 line", got)
	}
}

// ind8MateID is an NPC of the Gravekeeper's clan with no behavior.
const ind8MateID = int32(29803)

// keeperWorld boots the Gravekeeper, its slave and a clan-mate, and spawns
// the Gravekeeper 100 from the character, which stands inside its spawn
// territory. With clan the Gravekeeper is of the mate's clan.
func keeperWorld(t *testing.T, clan bool) (*ind8World, *npc.Hostile) {
	t.Helper()
	keeper := warriorTemplate(ind8KeeperID, nil)
	if clan {
		keeper.Clans, keeper.ClanRange = []string{"imperial"}, 500
	}
	mate := warriorTemplate(ind8MateID, nil)
	mate.Clans, mate.ClanRange = []string{"imperial"}, 500
	w := bootIndividual8(t, raidBoot{}, keeper, warriorTemplate(ind8SlaveID, nil), mate)
	return w, w.spawn(t, ind8KeeperID, 100, 0)
}

// TestImperialGravekeeperStepsDownItsHPShares runs ImperialGravekeeper's
// created and attacked hooks, the hook seeing the HP a hit finds. Created,
// it starts at site 1, a teleport share of 50, a slave share of 80 and a
// weight point of 10. A hit at full HP asks it to cast 4080 on itself and
// steps the teleport share to 30, which its HP puts back to 50; a hit at
// 75 % calls four slaves and steps the slave share to 40; one at 45 % sends
// the character, inside its territory, to the first site, up to 200 away,
// and leaves the teleport share at 30. Every other hit adds damage / max HP /
// 0.05 * 100 to its hate of the character, and once the hit has left it
// leading slaves, the party call the hit then makes to itself adds damage /
// max HP / 0.05 * 50.
func TestImperialGravekeeperStepsDownItsHPShares(t *testing.T) {
	t.Parallel()
	w, keeper := keeperWorld(t, false)
	shares := func(when string, ai1, ai2 int32) {
		t.Helper()
		if got := []int32{ind8Slot(keeper, script.SlotIntAI0), ind8Slot(keeper, script.SlotIntAI1), ind8Slot(keeper, script.SlotIntAI2)}; !slices.Equal(got, []int32{1, ai1, ai2}) {
			t.Fatalf("%s: slots ai0..ai2 = %v, want [1 %d %d]", when, got, ai1, ai2)
		}
	}
	shares("created", 50, 80)
	if got := ind8Slot(keeper, script.SlotWeightPoint); got != 10 {
		t.Fatalf("weight point = %d, want 10", got)
	}
	maxHP := float64(script.NPCOf(keeper).MaxHP())
	hate := 0.0
	hit := func(damage int) {
		t.Helper()
		w.hit(t, keeper, damage)
		factor := 100.0
		if len(keeper.Minions()) > 0 {
			factor += 50
		}
		hate += float64(damage) / maxHP / 0.05 * factor
		if got := keeper.AI().Threats().Hate(w.player); got != hate {
			t.Fatalf("hate after a hit of %d = %v, want %v", damage, got, hate)
		}
	}

	hit(int(maxHP / 4))
	shares("hit at full HP", 50, 80)
	casts := ind8Desires(keeper, actorai.IntentionCast)
	if len(casts) != 1 || casts[0].Skill.ID != 4080 || casts[0].Weight != 1000000 || casts[0].FinalTarget.ObjectID() != keeper.ObjectID() {
		t.Fatalf("cast desires after a hit = %+v, want 4080 on itself with weight 1000000", casts)
	}
	if got := len(keeper.Minions()); got != 0 {
		t.Fatalf("slaves after a hit at full HP = %d, want 0", got)
	}

	hit(int(maxHP / 10))
	shares("hit at 75 %", 50, 40)
	slaves := privatesOf(t, keeper, 4)
	for _, s := range slaves {
		if s.NpcID() != int(ind8SlaveID) {
			t.Fatalf("private %d, want only Imperial Slaves", s.NpcID())
		}
	}

	hit(int(maxHP / 5))
	shares("hit at 65 %", 50, 40)
	privatesOf(t, keeper, 4)

	// The character leaves the Gravekeeper's sight, and so its hate list.
	w.hit(t, keeper, 1)
	shares("hit at 45 %", 30, 40)
	x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID())
	if x < 179520-200 || x > 179520+200 || y < 6464-200 || y > 6464+200 {
		t.Fatalf("character at %d,%d after the hit at 45 %%, want within 200 of 179520,6464", x, y)
	}
	privatesOf(t, keeper, 4)
}

// TestImperialGravekeeperHatesForItsPartyAndClan runs ImperialGravekeeper's
// party-attacked and clan-attacked hooks: a hit on one of its slaves, or on
// a clan-mate within its clan range, adds damage / its max HP / 0.05 * 50
// to its hate of the character.
func TestImperialGravekeeperHatesForItsPartyAndClan(t *testing.T) {
	t.Parallel()
	w, keeper := keeperWorld(t, true)
	w.hit(t, keeper, int(script.NPCOf(keeper).MaxHP()/4)) // at full HP: no slaves yet
	w.hit(t, keeper, 1)                                   // at 75 %: four slaves
	slave := privatesOf(t, keeper, 4)[0]
	maxHP := float64(script.NPCOf(keeper).MaxHP())
	before := keeper.AI().Threats().Hate(w.player)

	w.hit(t, slave, 100)
	want := before + 100/maxHP/0.05*50
	if got := keeper.AI().Threats().Hate(w.player); got != want {
		t.Fatalf("hate after a hit on a slave = %v, want %v", got, want)
	}

	mate := w.spawn(t, ind8MateID, 100, 100)
	w.hit(t, mate, 50)
	want += 50 / maxHP / 0.05 * 50
	if got := keeper.AI().Threats().Hate(w.player); got != want {
		t.Fatalf("hate after a hit on a clan-mate = %v, want %v", got, want)
	}
}

// TestImperialGravekeeperWandersAndDropsItsFightOutside runs
// ImperialGravekeeper's no-desire and out-of-territory hooks: with nothing
// to do it wants to wander with weight 5; chasing the character out of its
// territory, its arrival there zeroes its hate and drops every attack
// desire it has.
func TestImperialGravekeeperWandersAndDropsItsFightOutside(t *testing.T) {
	t.Parallel()
	w := bootIndividual8(t, raidBoot{}, warriorTemplate(ind8KeeperID, nil))
	keeper := w.spawn(t, ind8KeeperID, 100, 0)
	w.ticks(t, 2)
	wander := ind8Desires(keeper, actorai.IntentionWander)
	if len(wander) != 1 || wander[0].Weight != 5 {
		t.Fatalf("wander desires once idle = %+v, want one of weight 5", wander)
	}

	far := w.spawn(t, ind8KeeperID, 600, 0)
	w.onQueue(t, far, func() { script.NPCOf(far).AddAttackDesire(script.PlayerOf(w.player), 100) })
	// Positions only: no AI tick runs, so nothing but the hook touches the
	// hate.
	for i := 0; far.IsMoving(); i++ {
		if i == 300 {
			t.Fatal("the chase never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
	if far.InTerritory() {
		t.Fatal("the chase ended inside the Gravekeeper's territory; want outside")
	}
	if got := far.AI().Threats().Hate(w.player); got != 0 {
		t.Fatalf("hate of the character after the arrival outside = %v, want 0", got)
	}
	if got := ind8Desires(far, actorai.IntentionAttack); len(got) != 0 {
		t.Fatalf("attack desires after the arrival outside = %+v, want none", got)
	}
}

// victimTemplates are the Royal Rush victim, naming its key box and its
// strong NPC, those two and the script-event listener.
func victimTemplates() []*npc.Template {
	return []*npc.Template{
		warriorTemplate(ind8VictimID, npc.AIParams{"KeyBox": fmt.Sprint(ind8KeyBoxID), "StrongNPC": fmt.Sprint(ind8StrongID)}),
		warriorTemplate(ind8KeyBoxID, nil), warriorTemplate(ind8StrongID, nil), warriorTemplate(ind8ListenerID, nil),
	}
}

// TestRoyalRushMissionNpcCallsForHelpThenDropsItsKeyBox runs
// RoyalRushMissionNpc's created, see-creature and timer hooks. Created, it
// looks around at once: it tells the NPC it sees, on a draw below 80, that
// it is there (event 1234 with its object id), and follows the character
// with weight 100, naming it in its first line. Every 5 s it calls for
// help. After 5 minutes it drops its key box at its feet as its private,
// says its last line and calls for help no more.
func TestRoyalRushMissionNpcCallsForHelpThenDropsItsKeyBox(t *testing.T) {
	t.Parallel()
	w := bootIndividual8(t, raidBoot{}, victimTemplates()...)
	w.spawn(t, ind8ListenerID, 300, 100)
	c, frames := w.place(t, ind8VictimID, 200, 0)
	victim := c.(*npc.Hostile)
	if got, want := w.heard.take(), []string{fmt.Sprintf("event=1234 arg1=%d arg2=0", victim.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("events heard on the victim's spawn = %q, want %q", got, want)
	}
	if got := says(frames, victim.ObjectID()); !slices.Equal(got, []string{script.NpcString(1010483, "Hunter")}) {
		t.Fatalf("victim lines on its spawn = %q, want the 1010483 line naming Hunter", got)
	}
	follow := ind8Desires(victim, actorai.IntentionFollow)
	if len(follow) != 1 || follow[0].Weight != 100 || follow[0].FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("follow desires = %+v, want the character with weight 100", follow)
	}

	w.srv.Advance(t, 5*time.Second)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), victim.ObjectID()); !slices.Equal(got, []string{script.NpcString(1010484)}) {
		t.Fatalf("victim lines after 5 s = %q, want the 1010484 line", got)
	}
	w.srv.Advance(t, 290*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := len(w.liveNPCs(ind8KeyBoxID)); got != 0 {
		t.Fatalf("key boxes before 5 minutes = %d, want 0", got)
	}

	w.srv.Advance(t, 5*time.Second)
	lines := says(w.srv.ReadQueued(t, w.srv.Client), victim.ObjectID())
	if !slices.Contains(lines, script.NpcString(1000503)) {
		t.Fatalf("victim lines at 5 minutes = %q, want the 1000503 line", lines)
	}
	box := privatesOf(t, victim, 1)[0]
	if box.NpcID() != int(ind8KeyBoxID) || positionOf(box).Distance2D(positionOf(victim)) > 1 {
		t.Fatalf("private %d at %+v, want the key box at the victim's feet %+v", box.NpcID(), positionOf(box), positionOf(victim))
	}
	if got := ind8Slot(victim, script.SlotIntAI0); got != 1 {
		t.Fatalf("victim ai0 after its key box = %d, want 1", got)
	}
	w.srv.Advance(t, 5*time.Second)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), victim.ObjectID()); len(got) != 0 {
		t.Fatalf("victim lines after its key box = %q, want none", got)
	}
}

// TestRoyalRushMissionNpcFleesAndLeavesItsStrongNPC runs
// RoyalRushMissionNpc's attacked, no-desire and dying hooks: with no one
// in sight and nothing to do it wants to wander with weight 5; a hit makes it want to flee its
// attacker as far as its drift range with weight 100; killed before it
// drops its key box, it leaves its strong NPC where it fell, and killed
// after, nothing.
func TestRoyalRushMissionNpcFleesAndLeavesItsStrongNPC(t *testing.T) {
	t.Parallel()
	w := bootIndividual8(t, raidBoot{}, victimTemplates()...)
	// Out of its sight, the victim follows nobody.
	victim := w.spawn(t, ind8VictimID, 2000, 0)
	w.ticks(t, 2)
	if wander := ind8Desires(victim, actorai.IntentionWander); len(wander) != 1 || wander[0].Weight != 5 {
		t.Fatalf("wander desires once idle = %+v, want one of weight 5", wander)
	}
	w.hit(t, victim, 10)
	flee := ind8Desires(victim, actorai.IntentionFlee)
	if len(flee) != 1 || flee[0].Weight != 100 || flee[0].FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("flee desires after a hit = %+v, want the character with weight 100", flee)
	}

	fell := positionOf(victim)
	w.onQueue(t, victim, func() { script.NPCOf(victim).DoDie(nil) })
	w.srv.Advance(t, 3*time.Second)
	strong := privatesOf(t, victim, 1)[0]
	if strong.NpcID() != int(ind8StrongID) || positionOf(strong).Distance2D(fell) > 1 {
		t.Fatalf("private %d at %+v, want the strong NPC where the victim fell %+v", strong.NpcID(), positionOf(strong), fell)
	}

	late := w.spawn(t, ind8VictimID, -200, 0)
	w.srv.Advance(t, 300*time.Second)
	privatesOf(t, late, 1) // its key box
	w.onQueue(t, late, func() { script.NPCOf(late).DoDie(nil) })
	w.srv.Advance(t, 3*time.Second)
	if got := len(w.liveNPCs(ind8StrongID)); got != 1 {
		t.Fatalf("strong NPCs after the late victim's death = %d, want only the first", got)
	}
}

// send sends c the script event eventID with arg1 and arg2 from the
// director script, on the character's queue.
func (w *ind8World) send(t *testing.T, c attackable.Combatant, eventID, arg1, arg2 int32) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), ind8DirectorName, func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(c), eventID, arg1, arg2)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// opcodeCount counts the frames of opcode among frames.
func opcodeCount(frames [][]byte, opcode byte) int {
	n := 0
	for _, f := range frames {
		if len(f) > 0 && f[0] == opcode {
			n++
		}
	}
	return n
}

// selectNPC selects f, so the next action on it talks.
func (w *ind8World) selectNPC(t *testing.T, f *npc.Folk) {
	t.Helper()
	w.srv.Client.Send(encodeFolkAction(f.ObjectID(), w.at))
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// talk talks to f, already selected, and returns the frames the talk sent.
func (w *ind8World) talk(t *testing.T, f *npc.Folk) [][]byte {
	t.Helper()
	w.srv.Client.Send(encodeFolkAction(f.ObjectID(), w.at))
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// altarStage is an npcmaker of the altar's stage, no_on_start like the
// datapack's, holding one NPC.
const altarStage = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="altar_stage" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="rune14_andreas_m2" territory="altar_stage" maximumNpcs="1">
		<ai type="no_on_start_maker"/>
		<npc id="29802" total="1" pos="40;20;30;0" respawn="0sec"/>
	</npcmaker>
</list>`

// altarDoor is a fixture door of id named name, closed.
func altarDoor(id int, name string, x int) *door.Template {
	return &door.Template{
		ID: id, Name: name, Kind: door.KindDoor, Level: 1,
		Position:    location.Location{X: x, Y: 0, Z: 0},
		Coordinates: []location.Point{{X: x - 8, Y: -8}, {X: x + 8, Y: -8}, {X: x + 8, Y: 8}, {X: x - 8, Y: 8}},
		HP:          100, PDef: 10, MDef: 10, Height: 32,
	}
}

// bootAltar boots the altar door keeper, naming both its doors, beside the
// listener, the doors and the stage npcmaker.
func bootAltar(t *testing.T) *ind8World {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "altar.xml"), []byte(altarStage), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return bootIndividual8(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithDoors(altarDoor(ind8DoorGateID, ind8DoorGate, 40), altarDoor(ind8DoorFlameID, ind8DoorFlame, 80)),
	}},
		ind8Folk(ind8AltarID, npc.AIParams{"DoorName": ind8DoorGate, "DoorName3rdFloor": ind8DoorFlame}),
		warriorTemplate(ind8ListenerID, nil), warriorTemplate(ind8StageNPCID, nil))
}

// doorsOpened reports whether the altar's gate and flame doors are open.
func (w *ind8World) doorsOpened(t *testing.T) [2]bool {
	t.Helper()
	var out [2]bool
	for i, id := range []int{ind8DoorGateID, ind8DoorFlameID} {
		d, ok := w.srv.WorldObjects.Door(id)
		if !ok {
			t.Fatalf("door %d not spawned", id)
		}
		out[i] = d.Opened()
	}
	return out
}

// TestAltarDoorControllerRunsTheAltarStages runs AltarDoorController's
// created, first-talk, script-event and timer hooks through the altar's
// stages, each stage the event that set it (below 9). Talking to the
// keeper shows nothing. Stage 0 opens the gate and closes the flame door;
// stage 1 has the stage npcmaker spawn its NPC (event 1001) and closes the
// gate 3 minutes later; stage 2 shouts, opens the flame door and closes it
// 3 minutes later; stage 5 opens both, has the npcmaker delete its NPC
// (event 1000) and 3 minutes later tells the NPCs within 4000 of stage 6;
// stage 3 holds the flame door 2 minutes, then closes it a minute later;
// stage 7 opens the flame door and an hour later has the npcmaker spawn
// again, opens the gate and tells the NPCs within 4000 of stage 0; stage 6
// closes the flame door and 3 minutes later the keeper leaves the world.
func TestAltarDoorControllerRunsTheAltarStages(t *testing.T) {
	t.Parallel()
	w := bootAltar(t)
	w.spawn(t, ind8ListenerID, 300, 100)
	c, _ := w.place(t, ind8AltarID, 40, 0)
	altar := c.(*npc.Folk)
	stage := func(want int32) {
		t.Helper()
		if got := ind8Slot(altar, script.SlotIntAI0); got != want {
			t.Fatalf("stage = %d, want %d", got, want)
		}
	}
	doors := func(when string, gate, flame bool) {
		t.Helper()
		if got := w.doorsOpened(t); got != [2]bool{gate, flame} {
			t.Fatalf("%s: doors opened (gate, flame) = %v, want [%v %v]", when, got, gate, flame)
		}
	}
	stageNPCs := func(when string, want int) {
		t.Helper()
		if got := len(w.liveNPCs(ind8StageNPCID)); got != want {
			t.Fatalf("%s: stage NPCs = %d, want %d", when, got, want)
		}
	}

	w.selectNPC(t, altar)
	frames := w.talk(t, altar)
	if got := opcodeCount(frames, serverpackets.OpcodeNpcHtmlMessage); got != 0 {
		t.Fatalf("talking to the keeper showed %d pages, want none", got)
	}
	// The action's own ActionFailed, then the empty first talk's.
	if got := opcodeCount(frames, serverpackets.OpcodeActionFailed); got != 2 {
		t.Fatalf("talking to the keeper sent %d ActionFailed, want 2", got)
	}

	doors("created", false, false)
	w.send(t, altar, 0, 0, 0)
	stage(0)
	doors("stage 0", true, false)

	w.send(t, altar, 1, 0, 0)
	stage(1)
	stageNPCs("stage 1", 1)
	w.srv.Advance(t, 3*time.Minute)
	doors("3 minutes into stage 1", false, false)

	w.send(t, altar, 2, 0, 0)
	stage(2)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), altar.ObjectID()); !slices.Equal(got, []string{script.NpcString(10079)}) {
		t.Fatalf("keeper lines at stage 2 = %q, want the 10079 shout", got)
	}
	doors("stage 2", false, true)
	w.srv.Advance(t, 3*time.Minute)
	doors("3 minutes into stage 2", false, false)

	w.send(t, altar, 5, 0, 0)
	stage(5)
	doors("stage 5", true, true)
	stageNPCs("stage 5", 0)
	w.heard.take()
	w.srv.Advance(t, 3*time.Minute)
	if got, want := w.heard.take(), []string{fmt.Sprintf("event=6 arg1=%d arg2=0", altar.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("events heard 3 minutes into stage 5 = %q, want %q", got, want)
	}

	w.send(t, altar, 3, 0, 0)
	stage(3)
	w.srv.Advance(t, 2*time.Minute)
	doors("2 minutes into stage 3", true, true)
	w.srv.Advance(t, time.Minute)
	doors("3 minutes into stage 3", true, false)

	w.send(t, altar, 9, 0, 0)
	stage(3)

	w.send(t, altar, 7, 0, 0)
	stage(7)
	doors("stage 7", true, true)
	w.srv.Advance(t, time.Hour)
	stage(0)
	stageNPCs("an hour into stage 7", 1)
	if got, want := w.heard.take(), []string{fmt.Sprintf("event=0 arg1=%d arg2=0", altar.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("events heard an hour into stage 7 = %q, want %q", got, want)
	}

	w.send(t, altar, 6, 0, 0)
	stage(6)
	doors("stage 6", true, false)
	w.srv.Advance(t, 3*time.Minute)
	if _, ok := w.srv.State.Object(altar.ObjectID()); ok {
		t.Fatal("the keeper is in the world 3 minutes into stage 6; want it gone")
	}
}

// TestAltarDoorControllerCallsTheAltarOnWhatItSees runs
// AltarDoorController's see-creature hook: at stage 0 the keeper tells the
// NPCs within 4000 of stage 1 for the character it sees, at stage 2 of
// stage 3.
func TestAltarDoorControllerCallsTheAltarOnWhatItSees(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stage, event int32
	}{{0, 1}, {2, 3}} {
		t.Run(fmt.Sprintf("stage %d", tc.stage), func(t *testing.T) {
			t.Parallel()
			w := bootAltar(t)
			w.spawn(t, ind8ListenerID, 300, 100)
			c, _ := w.place(t, ind8AltarID, 40, 0)
			if tc.stage != 0 {
				w.send(t, c, tc.stage, 0, 0)
			}
			w.heard.take()
			w.ticks(t, 3)
			if got, want := w.heard.take(), []string{fmt.Sprintf("event=%d arg1=%d arg2=0", tc.event, w.player.ObjectID())}; !slices.Equal(got, want) {
				t.Fatalf("events heard once the keeper sees the character = %q, want %q", got, want)
			}
		})
	}
}

// daimonCasts returns f's queued cast desires.
func daimonCasts(f *npc.Folk) []actorai.Desire {
	var out []actorai.Desire
	for _, d := range f.Desires().Snapshot() {
		if d.Kind == actorai.IntentionCast {
			out = append(out, d)
		}
	}
	return out
}

// TestDaimonTheWhiteEyedCursesOrBlessesWhoTalks runs DaimonTheWhiteEyed's
// created, first-talk and timer hooks. Created, it wants to walk its route
// with weight 2000. A player who talks to it sees no page; on an even draw
// it curses the player, a fighter, with 1206 level 19, else blesses it
// with 1086 level 2, with weight 1000000. Its line, one of three per
// branch, comes once per 30 s. No AI tick runs, so a desire asked again
// adds its weight to the one still queued.
func TestDaimonTheWhiteEyedCursesOrBlessesWhoTalks(t *testing.T) {
	t.Parallel()
	w := bootIndividual8(t, raidBoot{}, ind8Folk(ind8DaimonID, nil))
	c, _ := w.place(t, ind8DaimonID, 40, 0)
	daimon := c.(*npc.Folk)
	if !daimon.Desires().Has(&actorai.Desire{Kind: actorai.IntentionMoveRoute, RouteName: "argos_daemon_roaming"}) {
		t.Fatalf("desires once created = %+v, want the argos_daemon_roaming route", daimon.Desires().Snapshot())
	}
	talk := func(when string, wantLines []string, cast modelskill.Ref, weight float64) {
		t.Helper()
		frames := w.talk(t, daimon)
		if got := says(frames, daimon.ObjectID()); !slices.Equal(got, wantLines) {
			t.Fatalf("%s: lines = %q, want %q", when, got, wantLines)
		}
		if got := opcodeCount(frames, serverpackets.OpcodeNpcHtmlMessage); got != 0 {
			t.Fatalf("%s: the talk showed %d pages, want none", when, got)
		}
		if got := opcodeCount(frames, serverpackets.OpcodeActionFailed); got != 2 {
			t.Fatalf("%s: the talk sent %d ActionFailed, want 2: the action's and the empty first talk's", when, got)
		}
		casts := daimonCasts(daimon)
		if !slices.ContainsFunc(casts, func(d actorai.Desire) bool {
			return d.Skill == cast && d.Weight == weight && d.FinalTarget.ObjectID() == w.player.ObjectID()
		}) {
			t.Fatalf("%s: cast desires = %+v, want %+v on the character with weight %v", when, casts, cast, weight)
		}
	}

	w.selectNPC(t, daimon)
	w.draws.set(0, 0, 1)
	talk("first talk, even draw", []string{script.NpcString(1000459)}, modelskill.Ref{ID: 1206, Level: 19}, 1000000)
	w.draws.set(1)
	talk("talk inside 30 s, odd draw", nil, modelskill.Ref{ID: 1086, Level: 2}, 1000000)
	w.srv.Advance(t, 30*time.Second)
	w.draws.set(0, 1, 2)
	talk("talk after 30 s, odd draw", []string{script.NpcString(1000463)}, modelskill.Ref{ID: 1086, Level: 2}, 2000000)
}
