package ai

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPCs the WizardBase tests bind: one bound to WizardBase itself, which
// the Wizard behaviors, answering clan calls on their own, never reach; one
// bound to Wizard; and two plain ones.
const (
	wizardBaseOnlyID = int32(29961)
	wizardTimerID    = int32(29962)
	wizardFoeID      = int32(29963)
	wizardMasterID   = int32(29964)
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
	list, cat := wizardScripts()
	const base, timer, director = "test.WizardBaseOnly", "test.WizardTimer", "test.Ind1Director"
	cat[base] = func() script.Script {
		s := wizard.Base()
		s.NPCs = []int32{wizardBaseOnlyID}
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
	skills := []modelskill.Definition{
		ind1Skill(wizardStunID, 900, false), ind1Skill(wizardExplodeID, 2000, true), ind1Skill(wizardSpecialID, 900, true),
	}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, cat),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(skills)),
		gameservertest.WithScriptSkills(skills...),
	}}, templates...)
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

// TestWizardBaseCreviceOfDiminsionStopsHateOutsideItsTerritory ports the
// first check of WizardBase's attacked hook: with CreviceOfDiminsion, a
// creature outside the spawn territory that hits the NPC is hated no more
// and the hook ends there, so its later recharge of the soul shot does not
// run; without the parameter it does.
func TestWizardBaseCreviceOfDiminsionStopsHateOutsideItsTerritory(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		params npc.AIParams
		hated  bool
	}{
		{"without the parameter", npc.AIParams{"SoulShot": "1", "SoulShotRate": "100"}, true},
		{"with the parameter", npc.AIParams{"SoulShot": "1", "SoulShotRate": "100", "CreviceOfDiminsion": "1"}, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardBase(t, wizardBaseTemplate(wizardBaseOnlyID, row.params, 1, nil))
			h := w.spawn(t, wizardBaseOnlyID, 2000, 0)

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
// WizardBase: an NPC that is hit while not fighting looks around 7 s later
// (timer 1), and one that sees a player within 300 more than 15 levels
// below it, while fighting, empties its hate list and lists that player
// with 700. Script event 10020 makes it look around at once, and without
// the parameter it does neither.
func TestWizardBaseAttackLowLevelLooksAroundForItsFoe(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		params npc.AIParams
		event  bool
		want   float64
	}{
		{"timer 1", npc.AIParams{"AttackLowLevel": "1"}, false, 700},
		{"script event 10020", npc.AIParams{"AttackLowLevel": "1"}, true, 700},
		{"without the parameter", nil, true, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardBase(t, wizardBaseTemplate(wizardBaseOnlyID, row.params, 80, nil), wizardBaseTemplate(wizardFoeID, nil, 1, nil))
			h := w.spawn(t, wizardBaseOnlyID, 150, 0)
			foe := w.spawn(t, wizardFoeID, -150, 0)
			w.hateOn(t, h, foe, 500)
			w.listHate(t, h, script.NPCOf(foe), 500)
			w.ticks(t, 2)
			if got := h.AI().CurrentIntention(); got != actorai.IntentionAttack {
				t.Fatalf("setup: intention = %v, want attack", got)
			}
			if row.event {
				w.wizardEvent(t, h, 10020, w.player.ObjectID(), 0)
			} else {
				w.hit(t, h, 1)
				w.srv.Advance(t, 7*time.Second)
			}
			if got := hateOf(h, w.player); got != row.want {
				t.Fatalf("hate list amount of the character = %v, want %v", got, row.want)
			}
			if row.want > 0 {
				if got := hateOf(h, foe); got != 0 {
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

// TestWizardBaseCastsItsSpecialSkillWhenHurt ports the special skill of
// WizardBase's attacked hook: hit at 30% of its HP or less, a draw under 10
// has it say a line and want the skill on itself with weight 1000000.
func TestWizardBaseCastsItsSpecialSkillWhenHurt(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		down float64
		draw int
		cast bool
	}{
		{"30% of its HP, draw 9", 0.3, 9, true},
		{"over 30% of its HP", 0.4, 0, false},
		{"30% of its HP, draw 10", 0.3, 10, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWizardBase(t, wizardBaseTemplate(wizardBaseOnlyID, nil, 1, map[npc.SkillType]modelskill.ID{npc.SkillTypeSpecialSkill: wizardSpecialID}))
			h := w.spawn(t, wizardBaseOnlyID, 150, 0)
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
// line and wants no social; a draw of 10 shouts nothing.
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

			if got, want := sayLines(frames, h), []string{script.NpcString(1000291)}; row.shout && !slices.Equal(got, want) {
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
