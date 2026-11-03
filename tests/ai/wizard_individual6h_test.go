package ai

import (
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The NPC ids of the eighth PR of the sixth individual behavior batch: the
// datapack ids each class binds.
const (
	i6hCurseID       = int32(20258) // WizardDDMagic2Curse
	i6hCurseAggrID   = int32(20068) // WizardDDMagic2CurseAggressive
	i6hHereticID     = int32(22139) // WizardHereticDDMagic2Curse
	i6hHereticAggrID = int32(22153) // WizardHereticDDMagic2CurseAggressive
	i6hStep3ID       = int32(21461) // WizardGrowthStep3Aggressive
	i6hStep4ID       = int32(21469) // WizardGrowthStep4Aggressive
	i6hFighterFormID = int32(29572) // no behavior: a grown beast
	i6hMysticFormID  = int32(29573) // no behavior: a grown beast
)

// The skills the PR's classes list by type beside the batch's long and
// short range DD magics and the debuff, and a skill whose effect marks the
// character.
const (
	i6hSelfBuff  = modelskill.ID(9460)
	i6hSelfRange = modelskill.ID(9461)
	i6hHold      = modelskill.ID(9462)
	i6hMark      = modelskill.ID(9463)
)

// individual6hScripts are the behavior chain down to
// WizardDDMagic2Aggressive, the classes of the PR in scripts.xml order,
// and the directors the tests call privates and send script events
// through.
func individual6hScripts() ([]script.Listing, script.Catalog) {
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{"script.ai.individual.Monster.WizardBase.Wizard.Wizard", wizard.Wizard},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2", wizard.DDMagic2},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Aggressive", wizard.DDMagic2Aggressive},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Curse", wizard.DDMagic2Curse},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2CurseAggressive", wizard.DDMagic2CurseAggressive},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardGrowth.WizardGrowthStep3Aggressive", wizard.GrowthStep3Aggressive},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardGrowth.WizardGrowthStep4Aggressive", wizard.GrowthStep4Aggressive},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHereticDDMagic2Curse", wizard.HereticDDMagic2Curse},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHereticDDMagic2CurseAggressive", wizard.HereticDDMagic2CurseAggressive},
		{i6dDirectorPath, func() script.Script { return script.Script{} }},
		{"test.Ind1Director", func() script.Script { return script.Script{} }},
	}
	var list []script.Listing
	catalog := script.Catalog{}
	for _, e := range entries {
		list = append(list, script.Listing{Path: e.path})
		catalog[e.path] = e.ctor
	}
	return list, catalog
}

// bootIndividual6h boots the PR's classes beside templates, every NPC
// casting i6hSkills, the scripts reading the mark, every draw the current
// value of the world's draw.
func bootIndividual6h(t *testing.T, extra raidBoot, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6hScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draw := new(atomic.Int32)
	extra.opts = append(extra.opts,
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(n int) int { return min(int(draw.Load()), n-1) }),
		gameservertest.WithNpcSpawnSkills(i6hSkills()),
		gameservertest.WithScriptSkills(i6hMarkSkill()),
	)
	return &individual6World{warriorWorld: bootRaid(t, extra, templates...), draw: draw}
}

// i6hTemplate is a fixture wizard of id listing every skill type the PR's
// classes cast.
func i6hTemplate(id int32, ai npc.AIParams) *npc.Template {
	return i6bTemplate(id, ai, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID,
		npc.SkillTypeDebuff: i6gDebuff, npc.SkillTypeSelfBuff: i6hSelfBuff,
		npc.SkillTypeSelfRangeDDMagic: i6hSelfRange, npc.SkillTypeHoldMagic: i6hHold,
	})
}

// i6hSkills are the fixture skills of the PR's NPCs: harmless one-target
// magics and a self buff, each costing nothing, or 100000 MP for the
// costly ones.
func i6hSkills(costly ...modelskill.ID) *modelskill.Table {
	defs := []modelskill.Definition{npcSkill(i6hSelfBuff, 1, false)}
	for _, id := range []modelskill.ID{wLongDDID, wShortDDID, i6gDebuff, i6hSelfRange, i6hHold} {
		defs = append(defs, harmlessSkill(id))
	}
	for i := range defs {
		for _, id := range costly {
			if defs[i].ID == id {
				defs[i].MPConsume = 100000
			}
		}
	}
	return modelskill.NewTable(defs)
}

// i6hMarkSkill is the character's skill whose effect, of its own stack,
// lasts a minute.
func i6hMarkSkill() modelskill.Definition {
	d := ind1Skill(i6hMark, 900, false)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "i6h_mark", StackOrder: 1}}
	return d
}

// i6hArmed spawns an NPC of id dx, dy from the character, its skills
// i6hSkills with costly ones.
func (w *individual6World) i6hArmed(t *testing.T, id int32, dx, dy int, costly ...modelskill.ID) *npc.Hostile {
	t.Helper()
	h := w.spawn(t, id, dx, dy)
	w.srv.InstallNPCCastSeam(t, h, i6hSkills(costly...))
	return h
}

// i6hMarkHit lands a hit of 10 with the mark skill on h from the
// character, on its queue.
func (w *individual6World) i6hMarkHit(t *testing.T, h *npc.Hostile) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		h.ReduceHP(10, w.player, i6hMarkSkill())
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// i6hMarkCharacter lands the mark's effect, cast by caster, on the
// character.
func (w *individual6World) i6hMarkCharacter(t *testing.T, caster *npc.Hostile) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), "I6dDirector", func(s *script.Script, p *script.Player) {
		s.ApplyEffects(modelskill.Ref{ID: i6hMark, Level: 1}, script.NPCOf(caster), p)
	}) {
		t.Fatal("landing the mark panicked")
	}
	w.srv.Settle(t)
}

// TestWizardDDMagic2CurseCursesItsAttacker runs WizardDDMagic2Curse's
// attacked hook for hits from 300 away. Its parent runs before and after
// its own part, drawing the long range DD magic twice (weight 2000000).
// Hated, on a draw under 33, a hit wants its debuff on the attacker,
// weight 1000000, when the attacker holds no effect of the skill it hit
// with: a plain hit always, the mark skill until the character holds the
// mark. On a draw of 33, no debuff. One that cannot pay for the debuff
// gives up its spells and wants to attack, weight 1000, to which its
// parent's second call adds the damage's 125.
func TestWizardDDMagic2CurseCursesItsAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual6h(t, raidBoot{}, i6hTemplate(i6hCurseID, nil))
	me := w.player.ObjectID()
	cursed := map[modelskill.ID]i6fCast{wLongDDID: {me, 2000000, false}, i6gDebuff: {me, 1000000, false}}
	h := w.i6hArmed(t, i6hCurseID, 300, 0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "plain hit", h, cursed)

	unmarked := w.i6hArmed(t, i6hCurseID, 0, 300)
	w.i6hMarkHit(t, unmarked)
	i6fWantCasts(t, "mark hit on an unmarked character", unmarked, cursed)

	w.i6hMarkCharacter(t, h)
	marked := w.i6hArmed(t, i6hCurseID, -300, 0)
	w.i6hMarkHit(t, marked)
	i6fWantCasts(t, "mark hit on a marked character", marked, map[modelskill.ID]i6fCast{wLongDDID: {me, 2000000, false}})
	plain := w.i6hArmed(t, i6hCurseID, 0, -300)
	w.hit(t, plain, 10)
	i6fWantCasts(t, "plain hit on a marked character", plain, cursed)

	w.draw.Store(33)
	calm := w.i6hArmed(t, i6hCurseID, 300, 300)
	w.hit(t, calm, 10)
	if d, ok := castDesireOf(calm, i6gDebuff); ok {
		t.Fatalf("debuff desire on a draw of 33 = %+v, want none", d)
	}

	w.draw.Store(0)
	broke := w.i6hArmed(t, i6hCurseID, -300, -300, i6gDebuff)
	w.hit(t, broke, 10)
	w.i6eGaveUp(t, broke, 1125)
}

// TestWizardDDMagic2CurseAnswersItsClan runs WizardDDMagic2Curse's
// clan-attacked hook for a hit on its clan member. Settled and hating the
// character, on a draw under 33, it wants its debuff on the attacker,
// weight 1000000; fresh, or on a draw of 33, nothing.
func TestWizardDDMagic2CurseAnswersItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6h(t, raidBoot{}, i6gFaction(i6hTemplate(i6hCurseID, nil)), i6gFaction(warriorTemplate(i6gMateID, nil)))
	me := w.player.ObjectID()
	h := w.i6hArmed(t, i6hCurseID, 300, 0)
	mate := w.spawn(t, i6gMateID, 150, 150)
	w.i6eSettled(t, h)
	fresh := w.i6hArmed(t, i6hCurseID, 0, 300)
	for _, n := range []*npc.Hostile{h, fresh} {
		w.onQueue(t, n, func() { n.AI().Desires().Clear(); n.AI().Hates().Add(w.player, 100) })
	}
	w.hit(t, mate, 10)
	i6fWantCasts(t, "settled", h, map[modelskill.ID]i6fCast{i6gDebuff: {me, 1000000, false}})
	i6fWantCasts(t, "fresh", fresh, nil)

	w.draw.Store(33)
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
	w.hit(t, mate, 10)
	i6fWantCasts(t, "a draw of 33", h, nil)
}

// TestWizardCurseAggressivesStrikeOnSight runs the see-creature hooks of
// WizardDDMagic2CurseAggressive and WizardHereticDDMagic2CurseAggressive.
// Settled in its territory with nobody on its hate list, each wants its
// short range DD magic on the character it sees 100 away, weight 1000000,
// and hates it; one that cannot pay gives up its spells and wants to
// attack instead. Hating another, or fresh, it does nothing.
func TestWizardCurseAggressivesStrikeOnSight(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{i6hCurseAggrID, i6hHereticAggrID} {
		t.Run(strconv.Itoa(int(id)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6h(t, raidBoot{}, i6hTemplate(id, nil), warriorTemplate(i6gFoeID, nil))
			me := w.player.ObjectID()
			settled := w.i6hArmed(t, id, 1500, 0)
			broke := w.i6hArmed(t, id, -1500, 0, wShortDDID)
			busy := w.i6hArmed(t, id, 0, 1500)
			foe := w.spawn(t, i6gFoeID, 0, 2500)
			w.i6eSettled(t, settled, broke, busy)
			w.onQueue(t, busy, func() { script.NPCOf(busy).HateList().Add(script.NPCOf(foe), 100) })
			w.ticks(t, 1)

			w.i6fLookAt(t, settled)
			i6fWantCasts(t, "settled", settled, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}})
			if got := w.hateListOf(settled); got <= 0 {
				t.Fatalf("hate list entry of the character it saw = %v, want some", got)
			}
			w.i6fLookAt(t, broke)
			w.i6eGaveUp(t, broke, 1000)
			w.i6fLookAt(t, busy)
			i6fWantCasts(t, "busy", busy, nil)
			if got := w.hateListOf(busy); got != 0 {
				t.Fatalf("busy hate list entry of the character it saw = %v, want none", got)
			}

			fresh := w.i6hArmed(t, id, 2500, 0)
			w.i6fLookAt(t, fresh)
			i6fWantCasts(t, "fresh", fresh, nil)
		})
	}
}

// TestWizardHereticDDMagic2CurseEngagesAndAnswersCalls runs
// WizardHereticDDMagic2Curse's created, attacked, clan-attacked and
// script-event hooks, every draw 0. It is created unengaged; a hit on it or
// on its clan member while it neither attacks nor casts engages it (int
// slot 4). Whoever sends them, a shouted target (10002) or a call for help
// (10033) naming the character 300 away drops its attack desires and
// wants its long range DD magic on the character, weight 1000000, hating
// it 200; the call for help also engages it. A call to walk home (10035)
// disengages it, drops its attack desires and wants its spawn point,
// weight 100. A call naming no creature does nothing.
func TestWizardHereticDDMagic2CurseEngagesAndAnswersCalls(t *testing.T) {
	t.Parallel()
	w := bootIndividual6h(t, raidBoot{}, i6gFaction(i6hTemplate(i6hHereticID, nil)), i6gFaction(warriorTemplate(i6gMateID, nil)))
	me := w.player.ObjectID()
	h := w.i6hArmed(t, i6hHereticID, 300, 0)
	if got := i6dSlot4(h); got != 0 {
		t.Fatalf("int slot 4 as created = %d, want 0", got)
	}
	w.hit(t, h, 10)
	if got := i6dSlot4(h); got != 1 {
		t.Fatalf("int slot 4 after a hit = %d, want 1", got)
	}
	called := w.i6hArmed(t, i6hHereticID, 1500, 1500)
	mate := w.spawn(t, i6gMateID, 1500, 1400)
	w.hit(t, mate, 10)
	if got := i6dSlot4(called); got != 1 {
		t.Fatalf("int slot 4 after a hit on its clan member = %d, want 1", got)
	}

	n := w.i6hArmed(t, i6hHereticID, 0, 300)
	struck := map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}}
	for _, ev := range []struct {
		id    int32
		slot4 int32
	}{{10002, 0}, {10033, 1}} {
		w.onQueue(t, n, func() {
			n.AI().Desires().Clear()
			n.AI().Hates().Clear()
			n.Scratch().SetInt(npc.IntAI4, 0)
			script.NPCOf(n).AddAttackDesire(script.NPCOf(mate), 500)
		})
		w.sendEvent6d(t, n, ev.id, me, 0)
		i6fWantCasts(t, "event "+strconv.Itoa(int(ev.id)), n, struck)
		if got := i6cAttackWeight(n, mate.ObjectID()); got != 0 {
			t.Fatalf("event %d: attack desire on the clan member = %v, want dropped", ev.id, got)
		}
		if got := w.hateListOf(n); got != 200 {
			t.Fatalf("event %d: hate list entry of the character = %v, want 200", ev.id, got)
		}
		if got := i6dSlot4(n); got != ev.slot4 {
			t.Fatalf("event %d: int slot 4 = %d, want %d", ev.id, got, ev.slot4)
		}
	}

	home := script.NPCOf(n).SpawnLocation()
	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.sendEvent6d(t, n, 10035, 0, 0)
	if got := i6dSlot4(n); got != 0 {
		t.Fatalf("int slot 4 after event 10035 = %d, want 0", got)
	}
	d, ok := desire(n, actorai.IntentionMoveTo)
	if !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire after event 10035 = %+v (%v), want its spawn point %d,%d with weight 100", d, ok, home.X, home.Y)
	}

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.sendEvent6d(t, n, 10033, 0, 0)
	if got := n.AI().Desires().Snapshot(); len(got) != 0 || i6dSlot4(n) != 0 {
		t.Fatalf("desires and int slot 4 after a call naming no one = %+v, %d, want none and 0", got, i6dSlot4(n))
	}
}

// i6hHereticDen is an npcmaker spawning one WizardHereticDDMagic2Curse in
// a 400 by 400 territory.
const i6hHereticDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="heretic_curse_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="heretic_curse_den" territory="heretic_curse_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22139" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// TestWizardHereticDDMagic2CurseWalksHomeFromOutside runs
// WizardHereticDDMagic2Curse's out-of-territory hook on one its npcmaker
// spawned: arriving outside its territory unengaged, it wants its spawn
// point, weight 100; engaged (int slot 4 set), it stays.
func TestWizardHereticDDMagic2CurseWalksHomeFromOutside(t *testing.T) {
	t.Parallel()
	for _, engaged := range []bool{false, true} {
		t.Run(strconv.FormatBool(engaged), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(i6hHereticDen), 0o644); err != nil {
				t.Fatalf("write spawnlist: %v", err)
			}
			table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				t.Fatalf("load spawnlist: %v", err)
			}
			w := bootIndividual6h(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}}, i6hTemplate(i6hHereticID, nil))
			var h *npc.Hostile
			for _, obj := range w.srv.State.Objects() {
				if o, ok := obj.(*npc.Hostile); ok && o.NpcID() == int(i6hHereticID) {
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

// i6hGrowthAI are the growth beasts' AI parameters: a 1 s eating
// animation, its fighter form and its mystic one.
var i6hGrowthAI = npc.AIParams{
	"TakeSocial": "30", "MoveAroundSocial": "72",
	"silhouette1": "29990", "silhouette2": "29990",
	"silhouette_ex": strconv.Itoa(int(i6hFighterFormID)), "silhouette_ex2": strconv.Itoa(int(i6hMysticFormID)),
}

// TestWizardGrowthStep3StrikesItsFeederAndGrowsByOccupation runs
// WizardGrowthStep3Aggressive's created, attacked and timer hooks, every
// draw 0. Called with the character 300 away as its feeder, it hates it
// 100 and wants its self buff, weight 1000000, and its long range DD magic
// on the character, weight 1000000. Fed by the character it eats and says
// its first line (2014) without its parent's answer to the hit and, for a
// draw within 20% of one feed, grows 1 s later: for a draw of 50 or less
// into the fighter form, the character's occupation.
func TestWizardGrowthStep3StrikesItsFeederAndGrowsByOccupation(t *testing.T) {
	t.Parallel()
	w := bootIndividual6h(t, raidBoot{}, i6hTemplate(i6hStep3ID, i6hGrowthAI), warriorTemplate(i6hFighterFormID, nil), warriorTemplate(i6gFoeID, nil))
	iw := &ind1World{warriorWorld: w.warriorWorld}
	me := w.player.ObjectID()
	iw.setClass(t, w.player, 0)
	master := w.spawn(t, i6gFoeID, 1500, 0)
	n := iw.ind4cCallPrivate(t, master, i6hStep3ID, 300, 0, script.Params{P1: me, P2: int32(ind1gFood1)})
	iw.ind1gWantFeeder(t, "created", n, ind1gFood1, 1)
	if got := w.hateListOf(n); got != 100 {
		t.Fatalf("hate list entry of its feeder = %v, want 100", got)
	}
	i6fWantCasts(t, "created", n, map[modelskill.ID]i6fCast{i6hSelfBuff: {n.ObjectID(), 1000000, false}, wLongDDID: {me, 1000000, false}})

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.srv.ReadQueued(t, w.srv.Client)
	iw.ind1gFeed(t, n, w.player, ind1gFood1)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), n.ObjectID()); len(got) != 1 || got[0] != "Don't keep trying to tame me.  I don't want to be tamed." {
		t.Fatalf("lines after the feed = %q, want its first", got)
	}
	ind1gWantEat(t, "after the feed", n, 72*1000/30)
	i6fWantCasts(t, "after the feed", n, nil)
	iw.ind1gWantFeeder(t, "after the feed", n, ind1gFood1, 3)
	w.srv.Advance(t, time.Second)
	iw.ind1gGrown(t, n, i6hFighterFormID, ind1gFood1)
}

// TestWizardGrowthStep3BlastsWhenItHatesAnother runs
// WizardGrowthStep3Aggressive's attacked hook for a plain hit from 300
// away, every draw 0. Its parent wants its long range DD magic on the
// character; hating most another, it also wants its self range DD magic on
// the character, weight 1000000, and short of MP for that gives up its
// spells and wants to attack instead. Hating most the character, no blast.
func TestWizardGrowthStep3BlastsWhenItHatesAnother(t *testing.T) {
	t.Parallel()
	w := bootIndividual6h(t, raidBoot{}, i6hTemplate(i6hStep3ID, i6hGrowthAI), warriorTemplate(i6gFoeID, nil))
	me := w.player.ObjectID()
	other := w.i6hArmed(t, i6hStep3ID, 300, 0)
	broke := w.i6hArmed(t, i6hStep3ID, 0, 300, i6hSelfRange)
	alone := w.i6hArmed(t, i6hStep3ID, -300, 0)
	foe := w.spawn(t, i6gFoeID, 0, 2500)
	w.i6eSettled(t, other, broke, alone)
	for _, h := range []*npc.Hostile{other, broke} {
		w.onQueue(t, h, func() { script.NPCOf(h).HateList().Add(script.NPCOf(foe), 1000000) })
	}
	w.hit(t, other, 10)
	i6fWantCasts(t, "hating another", other, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}, i6hSelfRange: {me, 1000000, false}})
	w.hit(t, broke, 10)
	w.i6eGaveUp(t, broke, 1000)
	w.hit(t, alone, 10)
	i6fWantCasts(t, "hating the character", alone, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
}

// TestWizardGrowthStep4MakesItsNeighboursGrow runs
// WizardGrowthStep4Aggressive's created hook and
// WizardGrowthStep3Aggressive's script-event hook, every draw 0. A fully
// grown beast appearing tells the beasts within 700 (event 10018): one
// grown twice with a feeder grows into its mystic form, one with no feeder
// stays as it is. The new beast keeps the character as its feeder, wants
// to attack it, weight 100, and wants its self buff and its long range DD
// magic on the character, weight 1000000 each.
func TestWizardGrowthStep4MakesItsNeighboursGrow(t *testing.T) {
	t.Parallel()
	w := bootIndividual6h(t, raidBoot{}, i6hTemplate(i6hStep3ID, i6hGrowthAI), i6hTemplate(i6hStep4ID, nil),
		warriorTemplate(i6hMysticFormID, nil), warriorTemplate(i6gFoeID, nil))
	iw := &ind1World{warriorWorld: w.warriorWorld}
	me := w.player.ObjectID()
	master := w.spawn(t, i6gFoeID, 1500, 0)
	fed := iw.ind4cCallPrivate(t, master, i6hStep3ID, 300, 0, script.Params{P1: me, P2: int32(ind1gFood1)})
	unfed := iw.ind4cCallPrivate(t, master, i6hStep3ID, 300, 300, script.Params{})
	grown := iw.ind4cCallPrivate(t, master, i6hStep4ID, 300, 150, script.Params{P1: me, P2: int32(ind1gFood1)})
	iw.ind1gGrown(t, fed, i6hMysticFormID, ind1gFood1)
	if !w.inWorld(unfed) {
		t.Fatal("the beast with no feeder grew")
	}
	iw.ind1gWantFeeder(t, "the fully grown beast", grown, ind1gFood1, 1)
	if got := i6cAttackWeight(grown, me); got != 100 {
		t.Fatalf("the fully grown beast's attack desire on the character = %v, want 100", got)
	}
	i6fWantCasts(t, "the fully grown beast", grown, map[modelskill.ID]i6fCast{i6hSelfBuff: {grown.ObjectID(), 1000000, false}, wLongDDID: {me, 1000000, false}})
}

// TestWizardGrowthStep4HoldsWhatItFights runs WizardGrowthStep4Aggressive's
// attacked and timer hooks, every draw 0. Its first hit starts its 5 s
// timer (i_ai1 1); fighting the character, the tick wants its hold magic
// on it, weight 1000000. A hit by its top desire target marks i_ai2, so
// the next tick wants no hold and clears the mark.
func TestWizardGrowthStep4HoldsWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual6h(t, raidBoot{}, i6hTemplate(i6hStep4ID, nil))
	iw := &ind1World{warriorWorld: w.warriorWorld}
	me := w.player.ObjectID()
	n := w.i6hArmed(t, i6hStep4ID, 300, 0)
	iw.setTop(t, n, script.PlayerOf(w.player))
	w.hit(t, n, 10)
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 1 || a2 != 0 {
		t.Fatalf("i_ai1, i_ai2 after the first hit = %d, %d, want 1, 0", a1, a2)
	}
	// It takes up its cast within the timer's first seconds.
	var elapsed time.Duration
	for i := n.AI().CurrentIntention(); i != actorai.IntentionAttack && i != actorai.IntentionCast; i = n.AI().CurrentIntention() {
		if elapsed == 4*time.Second {
			t.Fatalf("setup: intention after the hit = %v, want a fight", i)
		}
		w.ticks(t, 1)
		w.srv.Advance(t, time.Second)
		elapsed += time.Second
	}
	w.srv.Advance(t, 5*time.Second-elapsed)
	ind5bWantCast(t, "the tick", n, i6hHold, me)

	iw.setTop(t, n, script.PlayerOf(w.player))
	w.hit(t, n, 10)
	if got := ind1Slot(n, script.SlotIntAI2); got != 1 {
		t.Fatalf("i_ai2 after a hit by its top desire target = %d, want 1", got)
	}
	w.srv.Advance(t, 5*time.Second)
	ind5bNoCast(t, "the tick after a hit", n, i6hHold)
	if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after the tick = %d, want 0", got)
	}
}
