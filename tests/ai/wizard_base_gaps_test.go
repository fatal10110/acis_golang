package ai

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
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The NPCs the WizardBase tests bind: the ones bound to WizardBase itself,
// the first of which the Wizard behaviors, answering clan calls on their
// own, never reach, the others being its transformation steps; one bound to
// Wizard; and two plain ones.
const (
	wizardBaseOnlyID = int32(29961)
	wizardTimerID    = int32(29962)
	wizardFoeID      = int32(29963)
	wizardMasterID   = int32(29964)
	wizardStep1ID    = int32(29965)
	wizardStep2ID    = int32(29966)
	wizardStep3ID    = int32(29967)
	wizardStunID     = modelskill.ID(4663)
	wizardExplodeID  = modelskill.ID(4664)
	wizardSpecialID  = modelskill.ID(4665)
)

// wizardBaseWorld is a world of WizardBase NPCs, every script draw 0 until
// draws queues others.
type wizardBaseWorld struct {
	*warriorWorld
	draws *ind1Draws
}

// bootWizardBase boots templates bound by their ids to WizardBase and
// Wizard, with the skills the tests give them cast through the spawn
// seam.
func bootWizardBase(t *testing.T, templates ...*npc.Template) *wizardBaseWorld {
	t.Helper()
	return bootWizardBaseWith(t, nil, templates...)
}

// bootWizardBaseWith is bootWizardBase with extra options, which come last.
func bootWizardBaseWith(t *testing.T, extra []gameservertest.Option, templates ...*npc.Template) *wizardBaseWorld {
	t.Helper()
	list, cat := wizardScripts()
	const base, timer, director = "test.WizardBaseOnly", "test.WizardTimer", "test.Ind1Director"
	cat[base] = func() script.Script {
		s := wizard.Base()
		s.NPCs = []int32{wizardBaseOnlyID, wizardStep1ID, wizardStep2ID, wizardStep3ID}
		return s
	}
	cat[timer] = func() script.Script {
		s := wizard.Wizard()
		s.NPCs = []int32{wizardTimerID}
		return s
	}
	cat[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: base}, script.Listing{Path: timer}, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	// The special skill holds an effect of its own stack for a minute, so
	// the NPC under it holds an abnormal of the skill.
	special := ind1Skill(wizardSpecialID, 900, true)
	special.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "wizard_base_special", StackOrder: 1}}
	skills := []modelskill.Definition{ind1Skill(wizardStunID, 900, false), ind1Skill(wizardExplodeID, 2000, true), special}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{opts: append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, cat),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(skills)),
		gameservertest.WithScriptSkills(skills...),
	}, extra...)}, templates...)
	return &wizardBaseWorld{warriorWorld: w, draws: draws}
}

// wizardTemplate is a fixture wizard of id with ai as its AI parameters
// and the skills by type, in a clan of its own.
func wizardBaseTemplate(id int32, ai npc.AIParams, level int, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.Level = level
	tmpl.Clans, tmpl.ClanRange = []string{"wizbase"}, 600
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for k, v := range skills {
		tmpl.SkillsByType[k] = modelskill.Ref{ID: v, Level: 1}
	}
	return tmpl
}

// hateOf returns c's amount in h's hate list, 0 when it is not listed.
func hateOf(h *npc.Hostile, c attackable.Combatant) float64 {
	return h.AI().HatesIfBuilt().Hate(c)
}

// listHate adds amount of h's hate list hate for c, on h's queue.
func (w *wizardBaseWorld) listHate(t *testing.T, h *npc.Hostile, c script.Creature, amount float64) {
	t.Helper()
	w.onQueue(t, h, func() { script.NPCOf(h).HateList().Add(c, amount) })
}

// wizardEvent sends h the script event eventID with arg1 and arg2.
func (w *wizardBaseWorld) wizardEvent(t *testing.T, h *npc.Hostile, eventID, arg1, arg2 int32) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(h), eventID, arg1, arg2)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// TestWizardBaseChargesItsShots ports the shot recharges of WizardBase's
// attacked hook: a draw under SoulShotRate charges its soul shot, one under
// SpiritShotRate its spirit shot; a draw at or over a rate, or no rate,
// charges none.
func TestWizardBaseChargesItsShots(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name         string
		params       npc.AIParams
		draw         int
		soul, spirit bool
	}{
		{"soul rate 50, draw 49", npc.AIParams{"SoulShot": "1", "SoulShotRate": "50"}, 49, true, false},
		{"soul rate 50, draw 50", npc.AIParams{"SoulShot": "1", "SoulShotRate": "50"}, 50, false, false},
		{"spirit rate 50, draw 49", npc.AIParams{"SpiritShot": "1", "SpiritShotRate": "50"}, 49, false, true},
		{"spirit rate 50, draw 50", npc.AIParams{"SpiritShot": "1", "SpiritShotRate": "50"}, 50, false, false},
		{"no rate", npc.AIParams{"SoulShot": "1", "SpiritShot": "1"}, 0, false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardBase(t, wizardBaseTemplate(wizardBaseOnlyID, row.params, 1, nil))
			h := w.spawn(t, wizardBaseOnlyID, 150, 0)
			w.draws.set(row.draw)

			w.hit(t, h, 10)
			if got := h.SoulshotCharged(); got != row.soul {
				t.Errorf("soul shot charged = %v, want %v", got, row.soul)
			}
			if got := h.SpiritshotCharged(); got != row.spirit {
				t.Errorf("spirit shot charged = %v, want %v", got, row.spirit)
			}
		})
	}
}

// wizardDen is an npcmaker spawning one WizardBase NPC in a 400 by 400
// territory, its spawn entry setting ai.
func wizardDen(ai string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="wizard_den" minZ="-1000" maxZ="1000"><node x="2000" y="2000"/><node x="2400" y="2000"/><node x="2400" y="2400"/><node x="2000" y="2400"/></territory>
	<npcmaker name="wizard_den" territory="wizard_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="%d" total="1" pos="2040;2020;30;0" respawn="60sec">%s</npc>
	</npcmaker>
</list>`, wizardBaseOnlyID, ai)
}

// TestWizardBaseCreviceOfDiminsionStopsHateOutsideItsTerritory ports the
// first check of WizardBase's attacked hook: with CreviceOfDiminsion set by
// the NPC's spawn entry, a creature outside the spawn territory that hits
// the NPC is hated no more and the hook ends there, so its later recharge
// of the soul shot does not run; without the parameter it does.
func TestWizardBaseCreviceOfDiminsionStopsHateOutsideItsTerritory(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		ai    string
		hated bool
	}{
		{"without the parameter", "", true},
		{"with the parameter", `<ai><set name="CreviceOfDiminsion" val="1"/></ai>`, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(wizardDen(row.ai)), 0o644); err != nil {
				t.Fatalf("write spawnlist: %v", err)
			}
			table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				t.Fatalf("load spawnlist: %v", err)
			}
			w := bootWizardBaseWith(t, []gameservertest.Option{gameservertest.WithNpcSpawns(table)},
				wizardBaseTemplate(wizardBaseOnlyID, npc.AIParams{"SoulShot": "1", "SoulShotRate": "100"}, 1, nil))
			if got := w.liveNPCs(wizardBaseOnlyID); len(got) != 1 {
				t.Fatalf("the den spawned %d wizards, want 1", len(got))
			}
			h := w.liveNPCs(wizardBaseOnlyID)[0]
			if script.NPCOf(h).InSpawnTerritory(script.PlayerOf(w.player)) {
				x, y, z := w.player.Position()
				t.Fatalf("setup: the character at %d,%d,%d stands in the den's territory", x, y, z)
			}

			w.hit(t, h, 10)
			// The hook returns before its later steps, a soul shot recharge
			// among them, so a charge shows it ran on.
			if got := h.SoulshotCharged(); got != row.hated {
				t.Fatalf("soul shot charged = %v, want %v", got, row.hated)
			}
		})
	}
}

// TestWizardBaseAttackLowHPRetargetsItsSpells ports the AttackLowHP
// branches of WizardBase: hit, or its clan mate hit, by a creature at 30%
// of its HP or less while its hate list names another, a draw under 10 (3
// for the clan call) empties the hate list and lists the attacker with the
// damage's hate times 100 plus 300; otherwise nothing changes.
func TestWizardBaseAttackLowHPRetargetsItsSpells(t *testing.T) {
	t.Parallel()
	params := npc.AIParams{"AttackLowHP": "1"}
	// A hit of 10 on a level 1 NPC is 10 / 8 of hate, times 100, plus 300.
	const listed = 10.0/8*100 + 300
	for _, row := range []struct {
		name     string
		clan     bool
		hp       float64
		draw     int
		retarget bool
	}{
		{"hit, low HP, draw 9", false, 0.3, 9, true},
		{"hit, low HP, draw 10", false, 0.3, 10, false},
		{"hit, half HP", false, 0.5, 0, false},
		{"clan call, low HP, draw 2", true, 0.25, 2, true},
		{"clan call, low HP, draw 3", true, 0.25, 3, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardBase(t, wizardBaseTemplate(wizardBaseOnlyID, params, 1, nil), wizardBaseTemplate(wizardFoeID, nil, 1, nil))
			victim := w.spawn(t, wizardBaseOnlyID, 150, 0)
			subject := victim
			if row.clan {
				subject = w.spawn(t, wizardBaseOnlyID, 150, 100)
			}
			foe := w.spawn(t, wizardFoeID, -150, 0)
			w.listHate(t, subject, script.NPCOf(foe), 500)
			w.setPlayerHPRatio(t, row.hp)
			w.draws.set(row.draw)

			w.hit(t, victim, 10)
			if row.retarget {
				if got := hateOf(subject, foe); got != 0 {
					t.Errorf("hate list amount of the foe = %v, want it emptied", got)
				}
				if got := hateOf(subject, w.player); math.Abs(got-listed) > 0.001 {
					t.Errorf("hate list amount of the character = %v, want %v", got, listed)
				}
				return
			}
			if got := hateOf(subject, foe); got != 500 {
				t.Errorf("hate list amount of the foe = %v, want 500 kept", got)
			}
			if got := hateOf(subject, w.player); got != 0 {
				t.Errorf("hate list amount of the character = %v, want none", got)
			}
		})
	}
}

// TestWizardBaseAttackLowLevelLooksAroundForItsFoe ports AttackLowLevel of
// WizardBase: an NPC that is hit, or whose clan mate is hit, while not
// fighting looks around 7 s later (timer 1), and one that sees a player
// within 300 more than 15 levels below it, while fighting, empties its hate
// list and lists that player with 700. Script event 10020 makes a fighting
// one look around at once, and without the parameter none does either.
func TestWizardBaseAttackLowLevelLooksAroundForItsFoe(t *testing.T) {
	t.Parallel()
	const (
		hitIdle  = "hit while idle"
		clanIdle = "clan mate hit while idle"
		event    = "script event"
	)
	for _, row := range []struct {
		name   string
		params npc.AIParams
		how    string
		want   float64
	}{
		{"timer 1 after a hit", npc.AIParams{"AttackLowLevel": "1"}, hitIdle, 700},
		{"timer 1 after a hit, without the parameter", nil, hitIdle, 0},
		{"timer 1 after a clan call", npc.AIParams{"AttackLowLevel": "1"}, clanIdle, 700},
		{"timer 1 after a clan call, without the parameter", nil, clanIdle, 0},
		{"script event 10020", npc.AIParams{"AttackLowLevel": "1"}, event, 700},
		{"script event 10020, without the parameter", nil, event, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			// The NPC stands still, 500 from the character: out of the 300 a
			// look reaches, so its cycles around the fight list the
			// character no more.
			subjectTmpl := wizardBaseTemplate(wizardBaseOnlyID, row.params, 80, nil)
			subjectTmpl.CanMove = false
			if row.how == hitIdle {
				// A clan call to itself starts the same timer, so the case
				// keeps it out of its own clan.
				subjectTmpl.Clans, subjectTmpl.ClanRange = nil, 0
			}
			w := bootWizardBase(t, subjectTmpl, wizardBaseTemplate(wizardFoeID, nil, 1, nil))
			h := w.spawn(t, wizardBaseOnlyID, 500, 0)
			subject := h
			foe := w.spawn(t, wizardFoeID, 700, 0)
			// fight sets the subject on the foe, as a wizard that fights on
			// its hate list is.
			fight := func() {
				w.hateOn(t, subject, foe, 500)
				w.listHate(t, subject, script.NPCOf(foe), 500)
				w.ticks(t, 2)
				if got := subject.AI().CurrentIntention(); got != actorai.IntentionAttack {
					t.Fatalf("setup: intention = %v, want attack", got)
				}
			}
			// comeClose brings the character to 150 from the subject, so a
			// look lists it: the look the case asks for is the first.
			comeClose := func() {
				obj, ok := w.srv.State.Player(w.player.ObjectID())
				if !ok {
					t.Fatal("the character is not in the world")
				}
				x, y, z := subject.Position()
				if err := w.srv.State.Move(obj, x-150, y, z); err != nil {
					t.Fatalf("move the character: %v", err)
				}
				w.srv.Settle(t)
				if got := hateOf(subject, w.player); got != 0 {
					t.Fatalf("setup: hate list amount of the character = %v, want none", got)
				}
			}
			switch row.how {
			case hitIdle, clanIdle:
				// The hit finds the subject idle and starts its timer 1; it is
				// fighting by the time the timer fires.
				if row.how == clanIdle {
					subject = w.spawn(t, wizardBaseOnlyID, 500, 100)
				}
				w.hit(t, h, 1)
				fight()
				comeClose()
				w.srv.Advance(t, 7*time.Second)

			case event:
				fight()
				comeClose()
				w.wizardEvent(t, h, 10020, w.player.ObjectID(), 0)
			}
			if got := hateOf(subject, w.player); got != row.want {
				t.Fatalf("hate list amount of the character = %v, want %v", got, row.want)
			}
			if row.want > 0 {
				if got := hateOf(subject, foe); got != 0 {
					t.Errorf("hate list amount of the foe = %v, want it emptied", got)
				}
			}
		})
	}
}

// TestWizardBaseHeroSilhouetteAnswersAPlayerAtLowHP ports the
// HelpHeroSilhouette branch of WizardBase's attacked hook: a player at 20%
// of its HP or less that hits it, on a draw under 3, has the named NPC
// spawn 80 east and 80 south of it as its private.
func TestWizardBaseHeroSilhouetteAnswersAPlayerAtLowHP(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		hp    float64
		draw  int
		spawn bool
	}{
		{"low HP, draw 2", 0.2, 2, true},
		{"low HP, draw 3", 0.2, 3, false},
		{"over 20% of its HP", 0.5, 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardBase(t, wizardBaseTemplate(wizardBaseOnlyID, npc.AIParams{"HelpHeroSilhouette": "29963"}, 1, nil), wizardBaseTemplate(wizardFoeID, nil, 1, nil))
			h := w.spawn(t, wizardBaseOnlyID, 150, 0)
			w.setPlayerHPRatio(t, row.hp)
			w.draws.set(row.draw)

			w.hit(t, h, 10)
			got := w.liveNPCs(wizardFoeID)
			if len(got) != btoi(row.spawn) {
				t.Fatalf("%d silhouettes, want %d", len(got), btoi(row.spawn))
			}
			if !row.spawn {
				return
			}
			x, y, z := h.Position()
			if sx, sy, sz := got[0].Position(); sx != x+80 || sy != y+80 || sz != z {
				t.Fatalf("silhouette at %d,%d,%d, want %d,%d,%d", sx, sy, sz, x+80, y+80, z)
			}
			if p3 := got[0].Scratch().Int(npc.IntParam3); p3 != h.ObjectID() {
				t.Fatalf("silhouette's third parameter = %d, want the master %d", p3, h.ObjectID())
			}
		})
	}
}

// btoi is 1 for true and 0 for false.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// holdSpecial lands the special skill's effect on h and waits for it.
func (w *wizardBaseWorld) holdSpecial(t *testing.T, h *npc.Hostile) {
	t.Helper()
	me := w.player.ObjectID()
	ref := modelskill.Ref{ID: wizardSpecialID, Level: 1}
	if !w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, _ *script.Player) {
		s.ApplyEffects(ref, script.NPCOf(h), script.NPCOf(h))
	}) {
		t.Fatal("the special skill's landing panicked")
	}
	w.srv.AdvanceUntil(t, "the special skill landed", func() bool {
		var level int32
		w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, _ *script.Player) {
			level = s.AbnormalLevel(script.NPCOf(h), ref)
		})
		return level > 0
	})
	w.srv.ReadQueued(t, w.srv.Client)
}

// TestWizardBaseCastsItsSpecialSkillWhenHurt ports the special skill of
// WizardBase's attacked hook: hit at 30% of its HP or less, a draw under 10
// has it say a line and want the skill on itself with weight 1000000, unless
// it already holds an abnormal of the skill.
func TestWizardBaseCastsItsSpecialSkillWhenHurt(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		down float64
		draw int
		held bool // the NPC holds an abnormal of the skill
		cast bool
	}{
		{"30% of its HP, draw 9", 0.3, 9, false, true},
		{"over 30% of its HP", 0.4, 0, false, false},
		{"30% of its HP, draw 10", 0.3, 10, false, false},
		{"30% of its HP, draw 0, under the skill's abnormal", 0.3, 0, true, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardBase(t, wizardBaseTemplate(wizardBaseOnlyID, nil, 1, map[npc.SkillType]modelskill.ID{npc.SkillTypeSpecialSkill: wizardSpecialID}))
			h := w.spawn(t, wizardBaseOnlyID, 150, 0)
			if row.held {
				w.holdSpecial(t, h)
			}
			w.hitDownTo(t, h, row.down)
			w.draws.set(row.draw)

			w.hit(t, h, 1)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			d, ok := desire(h, actorai.IntentionCast)
			if ok != row.cast || (ok && (d.Skill.ID != wizardSpecialID || d.Weight != 1000000 || d.FinalTarget.ObjectID() != h.ObjectID())) {
				t.Fatalf("cast desire = %+v (%v), want the special skill on itself at 1000000: %v", d, ok, row.cast)
			}
			if got, want := len(sayLines(frames, h)), btoi(row.cast); got != want {
				t.Fatalf("%d lines said, want %d", got, want)
			}
		})
	}
}

// TestWizardBaseExplodesWhenItsSelfExplosionEnds ports the self explosion
// of WizardBase: a hit that finds it under half its HP, on a draw under 5
// and a second draw under 10 minus its HP in tenths, wants the skill on
// itself with weight 3000000; it dies when it has cast it.
func TestWizardBaseExplodesWhenItsSelfExplosionEnds(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name    string
		down    float64
		draw    int
		explode bool
	}{
		{"40% of its HP, draw 0", 0.4, 0, true},
		{"40% of its HP, draw 5", 0.4, 5, false},
		{"half its HP, draw 0", 0.5, 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardBase(t, wizardBaseTemplate(wizardBaseOnlyID, nil, 1, map[npc.SkillType]modelskill.ID{npc.SkillTypeSelfExplosion: wizardExplodeID}))
			h := w.spawn(t, wizardBaseOnlyID, 150, 0)
			w.hitDownTo(t, h, row.down)
			w.draws.set(row.draw)

			w.hit(t, h, 1)
			if d, ok := desire(h, actorai.IntentionCast); ok != row.explode || (ok && d.Weight != 3000000) {
				t.Fatalf("cast desire = %+v (%v), want a self explosion at 3000000: %v", d, ok, row.explode)
			}
			var frames [][]byte
			for range 5 {
				frames = append(frames, w.ticks(t, 1)...)
				w.srv.Advance(t, time.Second)
				frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
			}
			if got := slices.Contains(shotSkillsOf(frames, h.ObjectID()), int32(wizardExplodeID)); got != row.explode {
				t.Fatalf("the NPC cast its self explosion = %v, want %v", got, row.explode)
			}
			if got := script.NPCOf(h).Dead(); got != row.explode {
				t.Fatalf("the NPC is dead = %v, want %v", got, row.explode)
			}
		})
	}
}

// TestWizardBaseStunsTheFoeItIsMadeFor ports WizardBase's created hook for
// a private made with 1000 as its first parameter: it wants skill 4663 on
// the creature named by its second parameter with weight 1000000, and
// lists it with 500 in its hate list.
func TestWizardBaseStunsTheFoeItIsMadeFor(t *testing.T) {
	t.Parallel()
	w := bootWizardBase(t, wizardBaseTemplate(wizardBaseOnlyID, nil, 1, nil), wizardBaseTemplate(wizardMasterID, nil, 1, nil))
	master := w.spawn(t, wizardMasterID, -150, 0)
	if !w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(s *script.Script, _ *script.Player) {
		s.Spawner().CreateOnePrivateEx(script.NPCOf(master), wizardBaseOnlyID, script.LocOf(script.NPCOf(master)), 0,
			script.Params{P1: 1000, P2: w.player.ObjectID()})
	}) {
		t.Fatal("the spawn panicked")
	}
	w.srv.Settle(t)
	got := w.liveNPCs(wizardBaseOnlyID)
	if len(got) != 1 {
		t.Fatalf("%d privates, want 1", len(got))
	}
	d, ok := desire(got[0], actorai.IntentionCast)
	if !ok || d.Skill.ID != wizardStunID || d.Weight != 1000000 || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("cast desire = %+v (%v), want 4663 on the character at 1000000", d, ok)
	}
	if got := hateOf(got[0], w.player); got != 500 {
		t.Fatalf("hate list amount of the character = %v, want 500", got)
	}
}

// wizardSocials are the social AI parameters of the wizard whose timer
// "1001" the tests fire: social 3 for 3 s, 2 for 2 s, 1 for 1 s, and the
// line it calls out in a fight.
func wizardSocials(social, social1, social2 int) npc.AIParams {
	return npc.AIParams{
		"MoveAroundSocial": fmt.Sprint(social), "MoveAroundSocial1": fmt.Sprint(social1), "MoveAroundSocial2": fmt.Sprint(social2),
		"ShoutMsg3": "1000291",
	}
}

// TestWizardActsOutAtPeaceByItsTimer ports the idle branch of Wizard's
// timer "1001", fired every 10 s: on a draw under 20 it wants the social
// of its first named parameter of MoveAroundSocial2 (3), MoveAroundSocial1
// (2) and MoveAroundSocial (1), for the seconds named in thirtieths of a
// second, with weight 50; on a draw of 20 or more, or naming none, it wants
// none.
func TestWizardActsOutAtPeaceByItsTimer(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		params npc.AIParams
		draw   int
		social int32
		timer  int
	}{
		{"all three named", wizardSocials(1, 2, 3), 19, 3, 100},
		{"the first and second", wizardSocials(1, 2, 0), 19, 2, 66},
		{"the first only", wizardSocials(6, 0, 0), 19, 1, 200},
		{"draw 20", wizardSocials(1, 2, 3), 20, 0, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardBase(t, wizardBaseTemplate(wizardTimerID, row.params, 1, nil))
			h := w.spawn(t, wizardTimerID, 150, 0)
			w.draws.set(row.draw)
			w.srv.Advance(t, 10*time.Second)
			w.srv.ReadQueued(t, w.srv.Client)

			d, ok := desire(h, actorai.IntentionSocial)
			if row.social == 0 {
				if ok {
					t.Fatalf("social desire = %+v, want none", d)
				}
				return
			}
			if !ok || d.ItemObjectID != row.social || d.Timer != row.timer || d.Weight != 50 {
				t.Fatalf("social desire = %+v (%v), want social %d for %d with weight 50", d, ok, row.social, row.timer)
			}
		})
	}
}

// TestWizardCallsOutInAFightByItsTimer ports the attack branch of Wizard's
// timer "1001": while it attacks, a draw under 10 shouts its ShoutMsg3
// line (a shout, not a say) and wants no social; a draw of 10 shouts
// nothing.
func TestWizardCallsOutInAFightByItsTimer(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		draw  int
		shout bool
	}{{"draw 9", 9, true}, {"draw 10", 10, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardBase(t, wizardBaseTemplate(wizardTimerID, wizardSocials(1, 2, 3), 1, nil))
			h := w.spawn(t, wizardTimerID, 150, 0)
			w.onQueue(t, h, func() { script.NPCOf(h).AddAttackDesire(script.PlayerOf(w.player), 500) })
			w.ticks(t, 2)
			if got := h.AI().CurrentIntention(); got != actorai.IntentionAttack {
				t.Fatalf("setup: intention = %v, want attack", got)
			}
			w.srv.ReadQueued(t, w.srv.Client)
			w.draws.set(row.draw)
			w.srv.Advance(t, 10*time.Second)
			frames := w.srv.ReadQueued(t, w.srv.Client)

			if got, want := shouts(t, frames, h.ObjectID()), []string{script.NpcString(1000291)}; row.shout && !slices.Equal(got, want) {
				t.Fatalf("lines = %q, want %q", got, want)
			} else if !row.shout && len(got) != 0 {
				t.Fatalf("lines = %q, want none", got)
			}
			if d, ok := desire(h, actorai.IntentionSocial); ok {
				t.Fatalf("social desire = %+v, want none in a fight", d)
			}
		})
	}
}

// wizardTransformParams are the AI parameters of the NPCs of a
// transformation chain, which hold up to steps steps.
func wizardTransformParams(steps int) npc.AIParams {
	return npc.AIParams{
		"IsTransform": fmt.Sprint(steps),
		"step1":       fmt.Sprint(wizardStep1ID), "step2": fmt.Sprint(wizardStep2ID), "step3": fmt.Sprint(wizardStep3ID),
	}
}

// bootWizardChain boots the NPCs of a transformation chain of up to steps
// steps, each holding the chain's parameters.
func bootWizardChain(t *testing.T, steps int) *wizardBaseWorld {
	t.Helper()
	var templates []*npc.Template
	for _, id := range []int32{wizardBaseOnlyID, wizardStep1ID, wizardStep2ID, wizardStep3ID} {
		templates = append(templates, wizardBaseTemplate(id, wizardTransformParams(steps), 1, nil))
	}
	return bootWizardBase(t, templates...)
}

// TestWizardBaseTransformsWhenHurtAtItsStep ports the IsTransform steps of
// WizardBase's attacked hook: a hit that finds an NPC at its step 0 between
// 70% and 50% of its HP, on a draw under 30, replaces it with its step1 NPC
// (step 1: between 50% and 30%, draw under 20, step2; step 2: between 30%
// and 5%, draw under 10, step3). The NPC says one of three lines, and the
// successor, made with 1000, the attacker and its step, wants skill 4663 on
// the attacker and lists it with 500; a hit outside the HP range or on a
// draw at or over the chance transforms nothing.
func TestWizardBaseTransformsWhenHurtAtItsStep(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		step  int // the NPC's own step: 0 is the first of the chain
		found float64
		draw  int
		line  int32 // the line it says on the draw 0 picks the first of
		next  bool
	}{
		{"step 0, 60%", 0, 0.6, 0, 1000406, true},
		{"step 0, 70%", 0, 0.7, 0, 0, false},
		{"step 0, 50%", 0, 0.5, 0, 0, false},
		{"step 0, draw 29", 0, 0.6, 29, 1000408, true},
		{"step 0, draw 30", 0, 0.6, 30, 0, false},
		{"step 1, 40%", 1, 0.4, 0, 1000409, true},
		{"step 1, 50%", 1, 0.5, 0, 0, false},
		{"step 1, 30%", 1, 0.3, 0, 0, false},
		{"step 1, draw 20", 1, 0.4, 20, 0, false},
		{"step 2, 20%", 2, 0.2, 0, 1000412, true},
		{"step 2, 30%", 2, 0.3, 0, 0, false},
		{"step 2, draw 10", 2, 0.2, 10, 0, false},
		{"step 3 never transforms", 3, 0.2, 0, 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardChain(t, 3)
			ids := []int32{wizardBaseOnlyID, wizardStep1ID, wizardStep2ID, wizardStep3ID}
			h := w.spawn(t, ids[row.step], 150, 0)
			// The step is the parameter a private is made with.
			w.onQueue(t, h, func() { h.Scratch().SetInt(npc.IntParam3, int32(row.step)) })
			w.hitDownTo(t, h, row.found)
			w.draws.set(row.draw)

			w.hit(t, h, 1)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			if got := h.Decayed(); got != row.next {
				t.Fatalf("the NPC left the world = %v, want %v", got, row.next)
			}
			if !row.next {
				if got := sayLines(frames, h); len(got) != 0 {
					t.Fatalf("lines = %q, want none", got)
				}
				return
			}
			if got, want := sayLines(frames, h), []string{script.NpcString(row.line)}; !slices.Equal(got, want) {
				t.Fatalf("lines = %q, want %q", got, want)
			}
			successors := w.liveNPCs(ids[row.step+1])
			if len(successors) != 1 {
				t.Fatalf("%d successors, want 1", len(successors))
			}
			next := successors[0]
			if got := next.Scratch().Int(npc.IntParam3); got != int32(row.step+1) {
				t.Fatalf("successor's step = %d, want %d", got, row.step+1)
			}
			d, ok := desire(next, actorai.IntentionCast)
			if !ok || d.Skill.ID != wizardStunID || d.Weight != 1000000 || d.FinalTarget.ObjectID() != w.player.ObjectID() {
				t.Fatalf("successor's cast desire = %+v (%v), want 4663 on the character at 1000000", d, ok)
			}
			if got := hateOf(next, w.player); got != 500 {
				t.Fatalf("successor's hate list amount of the character = %v, want 500", got)
			}
		})
	}
}

// TestWizardBaseTransformsNoFurtherThanItsLimit ports the step limit of
// WizardBase's transformation: an NPC at step 1 whose IsTransform is 1 does
// not take its second step.
func TestWizardBaseTransformsNoFurtherThanItsLimit(t *testing.T) {
	t.Parallel()
	w := bootWizardChain(t, 1)
	h := w.spawn(t, wizardStep1ID, 150, 0)
	w.onQueue(t, h, func() { h.Scratch().SetInt(npc.IntParam3, 1) })
	w.hitDownTo(t, h, 0.4)

	w.hit(t, h, 1)
	if h.Decayed() {
		t.Fatal("the NPC took a step past its IsTransform limit")
	}
}
