package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The NPC ids of the fourth PR of the sixth individual behavior batch: the
// datapack ids each class binds, and fixture ids for the leader the
// privates follow (bound to PartyLeaderWizard) and a plain clan member.
const (
	i6dCowardID  = int32(22158) // PartyPrivateWizardHereticCoward
	i6dCurseID   = int32(22165) // PartyPrivateWizardHereticDD2Curse
	i6dHealID    = int32(21543) // PartyPrivateWizardDD2Heal
	i6dHereticID = int32(22166) // PartyPrivateWizardHereticDD2
	i6dVampireID = int32(35634) // Agit02VampireArcherAgit
	i6dDoormanID = int32(22136) // DoormanOfDrain
	i6dDoorman2  = int32(18343) // DoormanOfDrain
	i6dLeaderID  = int32(29540) // PartyLeaderWizard (fixture binding)
	i6dLeader2ID = int32(29542) // PartyLeaderWizard (fixture binding)
	i6dMateID    = int32(29541) // no behavior
)

// The skills the PR's classes list by type, and a buff with aggro the
// character casts.
const (
	i6dHealSkillID  = modelskill.ID(9390)
	i6dFiendID      = modelskill.ID(9391)
	i6dDDMagicID    = modelskill.ID(9392)
	i6dAggroBuffID  = modelskill.ID(9393)
	i6dDirectorPath = "test.I6dDirector"
)

// individual6dScripts are the behavior chain down to WizardBase, the
// classes of the PR in scripts.xml order, and a director the tests send
// script events through.
func individual6dScripts() ([]script.Listing, script.Catalog) {
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizard", boundTo(wizard.PartyLeader, i6dLeaderID, i6dLeader2ID)},
		{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizard", wizard.PartyPrivate},
		{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2", wizard.PartyPrivateDD2},
		{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2Curse", wizard.PartyPrivateDD2Curse},
		{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2Heal", wizard.PartyPrivateDD2Heal},
		{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardHereticCoward", wizard.PartyPrivateHereticCoward},
		{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardHereticDD2", wizard.PartyPrivateHereticDD2},
		{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardHereticDD2Curse", wizard.PartyPrivateHereticDD2Curse},
		{"script.ai.individual.Monster.WizardBase.Wizard.Wizard", wizard.Wizard},
		{"script.ai.individual.Monster.WizardBase.Wizard.Agit02VampireArcherAgit", wizard.Agit02VampireArcherAgit},
		{"script.ai.individual.Monster.WizardBase.Wizard.DoormanOfDrain", wizard.DoormanOfDrain},
		{i6dDirectorPath, func() script.Script { return script.Script{} }},
	}
	var list []script.Listing
	catalog := script.Catalog{}
	for _, e := range entries {
		list = append(list, script.Listing{Path: e.path})
		catalog[e.path] = e.ctor
	}
	return list, catalog
}

func bootIndividual6d(t *testing.T, extra raidBoot, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6dScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draw := new(atomic.Int32)
	extra.opts = append(extra.opts,
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(n int) int { return min(int(draw.Load()), n-1) }),
	)
	return &individual6World{warriorWorld: bootRaid(t, extra, templates...), draw: draw}
}

// sendEvent6d sends h the script event eventID with arg1 and arg2 from the
// director script.
func (w *individual6World) sendEvent6d(t *testing.T, h *npc.Hostile, eventID, arg1, arg2 int32) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), "I6dDirector", func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(h), eventID, arg1, arg2)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// i6dSlot4 reads h's int slot 4, the heretic privates' engaged flag.
func i6dSlot4(h *npc.Hostile) int32 { return h.Scratch().Int(npc.IntAI4) }

// i6dHealSkill is a one-target heal of id that restores nothing.
func i6dHealSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: i6dHealSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Magic: true, SkillType: "HEAL", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true,
	}
}

// TestPartyPrivateWizardHereticEngagesOnHits runs the heretic privates'
// created, attacked and party-attacked hooks. Created, each clears int
// slot 4; a hit on itself, or on its leader, while it neither attacks nor
// casts sets it.
func TestPartyPrivateWizardHereticEngagesOnHits(t *testing.T) {
	t.Parallel()
	w := bootIndividual6d(t, raidBoot{},
		i6cLeader(i6dLeaderID, nil, i6dHereticID, 1), warriorTemplate(i6dHereticID, nil), warriorTemplate(i6dCurseID, nil))
	leader := w.spawn(t, i6dLeaderID, 300, 0)
	private := privatesOf(t, leader, 1)[0]
	alone := w.spawn(t, i6dCurseID, 0, 300)
	for _, h := range []*npc.Hostile{private, alone} {
		if got := i6dSlot4(h); got != 0 {
			t.Fatalf("heretic %d int slot 4 after its creation = %d, want 0", h.NpcID(), got)
		}
	}

	w.hit(t, leader, 10)
	if got := i6dSlot4(private); got != 1 {
		t.Fatalf("heretic private int slot 4 after a hit on its leader = %d, want 1", got)
	}
	w.hit(t, alone, 10)
	if got := i6dSlot4(alone); got != 1 {
		t.Fatalf("heretic int slot 4 after a hit on it = %d, want 1", got)
	}
}

// TestPartyPrivateWizardHereticAnswersItsMaster runs the heretic privates'
// script-event hook, which replaces their parent's. Its leader's call for
// help (10033) on the character drops its attack desires, sets int slot 4
// and enters the character in its hate list by 200. Its leader's call to
// walk home (10035) clears the slot and wants it home, weight 100. Its
// leader's shouted target (10002) adds 200 more to the hate list and says
// none of the parent's lines. A call naming another sender, or no
// creature, changes nothing.
func TestPartyPrivateWizardHereticAnswersItsMaster(t *testing.T) {
	t.Parallel()
	w := bootIndividual6d(t, raidBoot{}, i6cLeader(i6dLeaderID, nil, i6dCurseID, 1), warriorTemplate(i6dCurseID, nil))
	me := w.player.ObjectID()
	leader := w.spawn(t, i6dLeaderID, 300, 0)
	private := privatesOf(t, leader, 1)[0]
	w.onQueue(t, private, func() { script.NPCOf(private).AddAttackDesire(script.PlayerOf(w.player), 500) })

	w.sendEvent6d(t, private, 10033, me, leader.ObjectID())
	if got := i6dSlot4(private); got != 1 {
		t.Fatalf("int slot 4 after its leader's call for help = %d, want 1", got)
	}
	if d, ok := desire(private, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after its leader's call for help = %+v, want none", d)
	}
	if got := w.hateListOf(private); got != 200 {
		t.Fatalf("hate list entry after its leader's call for help = %v, want 200", got)
	}

	w.sendEvent6d(t, private, 10035, 0, leader.ObjectID())
	if got := i6dSlot4(private); got != 0 {
		t.Fatalf("int slot 4 after its leader's call home = %d, want 0", got)
	}
	home := script.NPCOf(private).SpawnLocation()
	if d, ok := desire(private, actorai.IntentionMoveTo); !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire after its leader's call home = %+v (%v), want its spawn point %d,%d with weight 100", d, ok, home.X, home.Y)
	}

	w.sendEvent6d(t, private, 10002, me, leader.ObjectID())
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := w.hateListOf(private); got != 400 {
		t.Fatalf("hate list entry after its leader's shouted target = %v, want 400", got)
	}
	if got := i6dSlot4(private); got != 0 {
		t.Fatalf("int slot 4 after its leader's shouted target = %d, want 0", got)
	}
	if got := says(frames, private.ObjectID()); len(got) != 0 {
		t.Fatalf("private lines on its leader's shouted target = %q, want none", got)
	}

	w.sendEvent6d(t, private, 10033, me, me)
	w.sendEvent6d(t, private, 10033, 0x7ffffff0, leader.ObjectID())
	if got := i6dSlot4(private); got != 0 {
		t.Fatalf("int slot 4 after calls from another sender or on no creature = %d, want 0", got)
	}
	if got := w.hateListOf(private); got != 400 {
		t.Fatalf("hate list entry after calls from another sender or on no creature = %v, want 400", got)
	}
}

// TestPartyPrivateWizardHereticCowardLeavesWithItsMaster runs the coward's
// party-died hook: its leader's death takes it out of the world at once,
// no corpse left, while a PartyPrivateWizardHereticDD2 whose leader dies
// stays.
func TestPartyPrivateWizardHereticCowardLeavesWithItsMaster(t *testing.T) {
	t.Parallel()
	w := bootIndividual6d(t, raidBoot{},
		i6cLeader(i6dLeaderID, nil, i6dCowardID, 1), warriorTemplate(i6dCowardID, nil),
		i6cLeader(i6dLeader2ID, nil, i6dHereticID, 1), warriorTemplate(i6dHereticID, nil))
	cowardLeader := w.spawn(t, i6dLeaderID, 300, 0)
	coward := privatesOf(t, cowardLeader, 1)[0]
	otherLeader := w.spawn(t, i6dLeader2ID, 0, 300)
	heretic := privatesOf(t, otherLeader, 1)[0]

	w.onQueue(t, cowardLeader, func() { script.NPCOf(cowardLeader).DoDie(nil) })
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if w.inWorld(coward) {
		t.Fatal("the coward is still in the world after its leader died")
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, coward.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames for the coward = %d, want 1", len(got))
	}
	if got := framesOf(frames, serverpackets.OpcodeDie, coward.ObjectID()); len(got) != 0 {
		t.Fatalf("Die frames for the coward = %d, want none: it leaves no corpse", len(got))
	}

	w.kill(t, otherLeader)
	if !w.inWorld(heretic) {
		t.Fatal("the heretic private left the world with its leader")
	}
}

// i6dHereticDen is an npcmaker spawning one heretic private on its own in
// a 400 by 400 territory.
const i6dHereticDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="heretic_private_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="heretic_private_den" territory="heretic_private_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22166" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// TestPartyPrivateWizardHereticWalksHomeFromOutside runs the heretic
// privates' out-of-territory hook on one its npcmaker spawned: arriving
// outside its territory unengaged, it wants its spawn point, weight 100;
// engaged (int slot 4 set), it stays.
func TestPartyPrivateWizardHereticWalksHomeFromOutside(t *testing.T) {
	t.Parallel()
	for _, engaged := range []bool{false, true} {
		name := "unengaged"
		if engaged {
			name = "engaged"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(i6dHereticDen), 0o644); err != nil {
				t.Fatalf("write spawnlist: %v", err)
			}
			table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				t.Fatalf("load spawnlist: %v", err)
			}
			w := bootIndividual6d(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}}, warriorTemplate(i6dHereticID, nil))
			var h *npc.Hostile
			for _, obj := range w.srv.State.Objects() {
				if o, ok := obj.(*npc.Hostile); ok && o.NpcID() == int(i6dHereticID) {
					h = o
				}
			}
			if h == nil {
				t.Fatal("the den spawned no heretic")
			}
			home := script.NPCOf(h).SpawnLocation()
			if engaged {
				w.onQueue(t, h, func() { h.Scratch().SetInt(npc.IntAI4, 1) })
			}

			to := location.Location{X: 40, Y: 700, Z: 30}
			if !h.AI().AddMoveToDesire(to, 1e6) {
				t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", to)
			}
			for i := 0; !h.IsMoving(); i++ {
				if i == 5 {
					t.Fatalf("no walk to %+v after five cycles", to)
				}
				cycle(t, w.srv)
			}
			for i := 0; h.IsMoving(); i++ {
				if i >= int(20*time.Second/move.PositionUpdateInterval) {
					t.Fatal("walk never arrived")
				}
				w.srv.TickPositions()
			}
			w.srv.Settle(t)
			if h.InTerritory() {
				t.Fatal("setup: the walk ended inside the territory")
			}
			d, ok := desire(h, actorai.IntentionMoveTo)
			if engaged {
				if ok {
					t.Fatalf("engaged heretic move desire after arriving outside = %+v, want none", d)
				}
				return
			}
			if !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
				t.Fatalf("move desire after arriving outside = %+v (%v), want its spawn point %d,%d with weight 100", d, ok, home.X, home.Y)
			}
		})
	}
}

// i6dHealTemplate is a fixture DD2 heal private of id listing its heal.
func i6dHealTemplate(id int32) *npc.Template {
	return i6bTemplate(id, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeMagicHeal: i6dHealSkillID})
}

// TestPartyPrivateWizardDD2HealHealsItsPartyAndItself runs
// PartyPrivateWizardDD2Heal's hooks, every draw 0. A hit while it holds
// 70% of its HP or more wants no heal; one landing under 70% wants its
// heal on itself. A hit landing on its leader under 70% wants its heal on
// the leader.
// A settled one with an empty hate list whose clan member is hit wants its
// heal on that member.
func TestPartyPrivateWizardDD2HealHealsItsPartyAndItself(t *testing.T) {
	t.Parallel()
	w := bootIndividual6d(t, raidBoot{},
		i6cLeader(i6dLeaderID, nil, i6dHealID, 1), inClan(i6dHealTemplate(i6dHealID)), inClan(warriorTemplate(i6dMateID, nil)))
	skills := modelskill.NewTable([]modelskill.Definition{i6dHealSkill()})
	alone := w.spawn(t, i6dHealID, 300, 0)
	w.srv.InstallNPCCastSeam(t, alone, skills)

	w.hit(t, alone, 10)
	if d, ok := castDesireOf(alone, i6dHealSkillID); ok {
		t.Fatalf("heal desire at 99%% of its HP = %+v, want none", d)
	}
	// The hook reads the HP the hit found: the hit that takes it under 70%
	// heals nothing, the next one does.
	w.hit(t, alone, 300)
	if d, ok := castDesireOf(alone, i6dHealSkillID); ok {
		t.Fatalf("heal desire on the hit that takes it under 70%% = %+v, want none", d)
	}
	w.hit(t, alone, 10)
	if !castOn(alone, i6dHealSkillID, alone.ObjectID()) {
		t.Fatal("heal private hit below 70% of its HP wants no heal on itself")
	}

	leader := w.spawn(t, i6dLeaderID, -300, 0)
	private := privatesOf(t, leader, 1)[0]
	w.srv.InstallNPCCastSeam(t, private, skills)
	w.hit(t, leader, 400)
	w.hit(t, leader, 10)
	if !castOn(private, i6dHealSkillID, leader.ObjectID()) {
		t.Fatal("heal private wants no heal on its leader under 70% of its HP")
	}

	clan := w.spawn(t, i6dHealID, 0, 300)
	mate := w.spawn(t, i6dMateID, 100, 300)
	w.srv.InstallNPCCastSeam(t, clan, skills)
	w.ticks(t, 8)
	w.onQueue(t, clan, func() { clan.AI().Desires().Clear(); clan.AI().Hates().Clear() })
	w.hit(t, mate, 10)
	if !castOn(clan, i6dHealSkillID, mate.ObjectID()) {
		t.Fatal("settled heal private wants no heal on its clan member that was hit")
	}
}

// i6dVampireTemplate is a fixture clan hall siege vampire listing its fiend
// arrow.
func i6dVampireTemplate() *npc.Template {
	return i6bTemplate(i6dVampireID, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeWFiendArcher: i6dFiendID})
}

// i6dVampireSkills are the vampire's skills, its fiend arrow costing mp MP.
func i6dVampireSkills(mp int) *modelskill.Table {
	fiend := harmlessSkill(i6dFiendID)
	fiend.MPConsume = mp
	return modelskill.NewTable([]modelskill.Definition{fiend})
}

// TestAgit02VampireArcherAgitShootsWhatItSees runs Agit02VampireArcherAgit's
// see-creature, attacked and no-desire hooks. The character it sees gets
// its fiend arrow, weight 1000000; one it cannot pay for sets int slot 0
// and wants the character attacked, weight 1000. A hit of 10 enters the
// character in its hate list, 10 / 8 * 100 + 300, and its parent adds
// 10 / 8 * 100 more. With no desire left it does nothing, not even wander.
func TestAgit02VampireArcherAgitShootsWhatItSees(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		mp   int
	}{{"shoots", 0}, {"broke", 100000}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6d(t, raidBoot{}, i6dVampireTemplate())
			me := w.player.ObjectID()
			h := w.spawn(t, i6dVampireID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, h, i6dVampireSkills(row.mp))
			w.moveNextTo(t, h, 300, 0)
			w.ticks(t, 1)
			if row.mp == 0 {
				if !castOn(h, i6dFiendID, me) {
					t.Fatal("vampire wants no fiend arrow on the character it sees")
				}
			} else {
				if got := h.Scratch().Int(npc.IntAI0); got != 1 {
					t.Fatalf("broke vampire int slot 0 = %d, want 1", got)
				}
				if got := i6cAttackWeight(h, me); got != 1000 {
					t.Fatalf("broke vampire attack desire on the character it sees = %v, want 1000", got)
				}
				return
			}

			w.onQueue(t, h, func() { h.AI().Desires().Clear() })
			w.hit(t, h, 10)
			if got := w.hateListOf(h); got != 550 {
				t.Fatalf("vampire hate list entry after a hit = %v, want 425 + 125", got)
			}
			if !castOn(h, i6dFiendID, me) {
				t.Fatal("vampire wants no fiend arrow on its attacker")
			}

			w.onQueue(t, h, func() { h.AI().Desires().Clear(); h.AI().Hates().Clear() })
			w.moveNextTo(t, h, 3000, 0)
			w.ticks(t, 3)
			if d, ok := desire(h, actorai.IntentionWander); ok {
				t.Fatalf("vampire with no desire left wants to wander: %+v", d)
			}
		})
	}
}

// i6dDoormanTemplate is a fixture gatekeeper of id listing its DD magic
// and heal.
func i6dDoormanTemplate(id int32) *npc.Template {
	return i6bTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeDDMagic: i6dDDMagicID, npc.SkillTypeMagicHeal: i6dHealSkillID,
	})
}

// i6dPasses are the item templates with the gatekeeper's three passes.
func i6dPasses() *item.Table {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{8064, 8065, 8067} {
		templates = append(templates, &item.Template{
			ID: id, Name: fmt.Sprint("Pass ", id), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Dropable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return item.NewTable(templates)
}

// i6dDoormanSkills are the gatekeeper's skills.
func i6dDoormanSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{harmlessSkill(i6dDDMagicID), i6dHealSkill()})
}

// TestDoormanOfDrainLetsPassHoldersBy runs DoormanOfDrain's see-creature
// hook on a settled gatekeeper. A character carrying a pass gets its heal,
// weight 1000000, and stays off its hate list; one without gets its DD
// magic, weight 1000000, and 100 on its hate list. A young gatekeeper
// answers neither.
func TestDoormanOfDrainLetsPassHoldersBy(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		pass  bool
		young bool
	}{{"pass", true, false}, {"stranger", false, false}, {"young", false, true}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			extra := raidBoot{opts: []gameservertest.Option{gameservertest.WithItemTemplates(i6dPasses())}}
			if row.pass {
				extra.ready = func(srv *gameservertest.Server, objID int32) { srv.GiveItem(t, objID, 8065, 1) }
			}
			w := bootIndividual6d(t, extra, i6dDoormanTemplate(i6dDoormanID))
			me := w.player.ObjectID()
			h := w.spawn(t, i6dDoormanID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, h, i6dDoormanSkills())
			if !row.young {
				w.ticks(t, 9)
			}
			w.moveNextTo(t, h, 300, 0)
			w.ticks(t, 1)
			switch {
			case row.young:
				for _, id := range []modelskill.ID{i6dDDMagicID, i6dHealSkillID} {
					if d, ok := castDesireOf(h, id); ok {
						t.Fatalf("young gatekeeper desire of %d = %+v, want none", id, d)
					}
				}
				if got := w.hateListOf(h); got != 0 {
					t.Fatalf("young gatekeeper hate list entry = %v, want none", got)
				}
			case row.pass:
				if !castOn(h, i6dHealSkillID, me) {
					t.Fatal("gatekeeper wants no heal on a pass holder")
				}
				if got := w.hateListOf(h); got != 0 {
					t.Fatalf("gatekeeper hate list entry of a pass holder = %v, want none", got)
				}
			default:
				if !castOn(h, i6dDDMagicID, me) {
					t.Fatal("gatekeeper wants no DD magic on a stranger")
				}
				if got := w.hateListOf(h); got != 100 {
					t.Fatalf("gatekeeper hate list entry of a stranger = %v, want 100", got)
				}
			}
		})
	}
}

// TestDoormanOfDrainAnswersHits runs DoormanOfDrain's attacked and
// clan-attacked hooks. A hit of 10 runs its parent twice (425, then 125
// more) and, with the character its top desire target, wants its DD magic
// on the character. A settled one whose clan member is hit
// enters the attacker through its parent (337.5), wants to attack it by
// 10 / 8 * 30 and casts its DD magic at it, and its second parent call adds
// 37.5 more.
func TestDoormanOfDrainAnswersHits(t *testing.T) {
	t.Parallel()
	// The gatekeeper that is hit is in no clan: a clan member's hit also
	// calls the clan hooks of the member itself.
	w := bootIndividual6d(t, raidBoot{}, i6dDoormanTemplate(i6dDoormanID), inClan(i6dDoormanTemplate(i6dDoorman2)), inClan(warriorTemplate(i6dMateID, nil)))
	me := w.player.ObjectID()
	h := w.spawn(t, i6dDoormanID, 300, 0)
	w.srv.InstallNPCCastSeam(t, h, i6dDoormanSkills())
	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		script.NPCOf(h).SetTopDesireTarget(script.PlayerOf(w.player))
	})

	w.hit(t, h, 10)
	if got := w.hateListOf(h); got != 550 {
		t.Fatalf("gatekeeper hate list entry after a hit = %v, want 425 + 125", got)
	}
	if !castOn(h, i6dDDMagicID, me) {
		t.Fatal("gatekeeper wants no DD magic on its top desire target")
	}

	clan := w.spawn(t, i6dDoorman2, 0, 300)
	mate := w.spawn(t, i6dMateID, 100, 300)
	w.srv.InstallNPCCastSeam(t, clan, i6dDoormanSkills())
	w.ticks(t, 8)
	w.onQueue(t, clan, func() { clan.AI().Desires().Clear(); clan.AI().Hates().Clear() })
	w.hit(t, mate, 10)
	if got := w.hateListOf(clan); got != 375 {
		t.Fatalf("gatekeeper hate list entry after a clan hit = %v, want 337.5 + 37.5", got)
	}
	if got := i6cAttackWeight(clan, me); got != 37.5 {
		t.Fatalf("gatekeeper attack desire after a clan hit = %v, want 37.5", got)
	}
	if !castOn(clan, i6dDDMagicID, me) {
		t.Fatal("gatekeeper wants no DD magic on its clan member's attacker")
	}
}

// TestDoormanOfDrainKeepsToItsPost runs DoormanOfDrain's move-finished
// hook: arriving away from its spawn point it wants to walk back, weight
// 30; arriving there it wants to do nothing, weight 30.
func TestDoormanOfDrainKeepsToItsPost(t *testing.T) {
	t.Parallel()
	w := bootIndividual6d(t, raidBoot{}, i6dDoormanTemplate(i6dDoormanID))
	h := w.spawn(t, i6dDoormanID, 1500, 0)
	home := script.NPCOf(h).SpawnLocation()
	x, y, z := h.Position()
	to := location.Location{X: x + 200, Y: y, Z: z}
	walk := func(what string) {
		t.Helper()
		for i := 0; !h.IsMoving(); i++ {
			if i == 5 {
				t.Fatalf("no walk %s after five cycles", what)
			}
			cycle(t, w.srv)
		}
		for i := 0; h.IsMoving(); i++ {
			if i >= int(20*time.Second/move.PositionUpdateInterval) {
				t.Fatalf("walk %s never arrived", what)
			}
			w.srv.TickPositions()
		}
		w.srv.Settle(t)
	}
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
	if !h.AI().AddMoveToDesire(to, 1e6) {
		t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", to)
	}
	walk("away")
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire after arriving away = %+v (%v), want its spawn point %d,%d with weight 30", d, ok, home.X, home.Y)
	}
	walk("home")
	if d, ok := desire(h, actorai.IntentionNothing); !ok || d.Weight != 30 {
		t.Fatalf("do-nothing desire after arriving home = %+v (%v), want weight 30", d, ok)
	}
}

// TestDoormanOfDrainDropsWhatLeavesItsReach runs DoormanOfDrain's
// skill-finished hook. A cast ending on the character 300 away, within its
// SkillRange of 500, with the character on its hate list, wants its DD
// magic on the character again; one ending 550 away drops every desire.
func TestDoormanOfDrainDropsWhatLeavesItsReach(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dist int
	}{{"within", 300}, {"beyond", 550}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6d(t, raidBoot{}, i6dDoormanTemplate(i6dDoormanID))
			me := w.player.ObjectID()
			h := w.spawn(t, i6dDoormanID, row.dist, 0)
			w.srv.InstallNPCCastSeam(t, h, i6dDoormanSkills())
			w.onQueue(t, h, func() {
				h.AI().Desires().Clear()
				h.AI().Hates().Add(w.player, 100)
				script.NPCOf(h).AddCastDesireUnchecked(script.PlayerOf(w.player), modelskill.Ref{ID: i6dHealSkillID, Level: 1}, 1e9)
				// A route with no node: the desire stays queued until dropped.
				script.NPCOf(h).AddMoveRouteDesire("i6d_no_route", 1)
			})
			for i := 0; ; i++ {
				if i == 20 {
					t.Fatalf("the gatekeeper %d away never answered the end of its cast", row.dist)
				}
				w.ticks(t, 1)
				w.srv.Advance(t, time.Second)
				if row.dist < 500 && castOn(h, i6dDDMagicID, me) {
					return
				}
				if _, ok := desire(h, actorai.IntentionMoveRoute); row.dist >= 500 && !ok {
					if _, ok := castDesireOf(h, i6dDDMagicID); ok {
						t.Fatal("the gatekeeper wants its DD magic on a character beyond its reach")
					}
					return
				}
			}
		})
	}
}

// TestDoormanOfDrainHatesSpellCasters runs DoormanOfDrain's see-spell hook:
// a buff with aggro the character casts beside a gatekeeper that hates
// humans by 20% makes it want to attack the character by that ratio times
// 150.
func TestDoormanOfDrainHatesSpellCasters(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	buff := modelskill.Definition{
		ID: i6dAggroBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: 100,
	}
	table := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, buff})
	tmpl := i6dDoormanTemplate(i6dDoormanID)
	tmpl.AIParams = npc.AIParams{"SetHateRace": "@race_human", "SetHateRaceRatio": "20"}
	w := bootIndividual6d(t, raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(buff),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(buff.ID), 1); err != nil {
				t.Fatalf("seed skill %d: %v", buff.ID, err)
			}
		},
	}, tmpl)
	me := w.player.ObjectID()
	h := w.spawn(t, i6dDoormanID, 150, 0)
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })

	castOwnSkill(w.warriorWorld, i6dAggroBuffID)
	w.srv.AdvanceUntil(t, "the buff's attack desire", func() bool { return i6cAttackWeight(h, me) != 0 })
	if got := i6cAttackWeight(h, me); got != 20*150 {
		t.Fatalf("gatekeeper attack desire after the buff = %v, want 3000", got)
	}
}

// TestAgit02VampireArcherAgitAnswersItsClanAndItsCasts runs
// Agit02VampireArcherAgit's clan-attacked and skill-finished hooks. A
// settled vampire with an empty hate list whose clan member is hit wants
// its fiend arrow on the attacker. A cast that ends with the character on
// its hate list wants its fiend arrow on the character.
func TestAgit02VampireArcherAgitAnswersItsClanAndItsCasts(t *testing.T) {
	t.Parallel()
	w := bootIndividual6d(t, raidBoot{}, inClan(i6dVampireTemplate()), inClan(warriorTemplate(i6dMateID, nil)))
	me := w.player.ObjectID()
	h := w.spawn(t, i6dVampireID, 1500, 0)
	w.srv.InstallNPCCastSeam(t, h, i6dVampireSkills(0))
	mate := w.spawn(t, i6dMateID, 1500, 100)
	w.ticks(t, 8)
	w.onQueue(t, h, func() { h.AI().Desires().Clear(); h.AI().Hates().Clear() })
	w.hit(t, mate, 10)
	if !castOn(h, i6dFiendID, me) {
		t.Fatal("settled vampire wants no fiend arrow on its clan member's attacker")
	}

	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		h.AI().Hates().Add(w.player, 100)
		script.NPCOf(h).AddCastDesireUnchecked(script.NPCOf(h), modelskill.Ref{ID: i6dFiendID, Level: 1}, 1e9)
	})
	for i := 0; !castOn(h, i6dFiendID, me); i++ {
		if i == 20 {
			t.Fatal("the vampire never answered the end of its cast")
		}
		w.ticks(t, 1)
		w.srv.Advance(t, time.Second)
	}
}
