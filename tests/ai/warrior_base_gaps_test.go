package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// setPlayerHPRatio sets the character's HP to ratio of its maximum, on its
// queue.
func (w *warriorWorld) setPlayerHPRatio(t *testing.T, ratio float64) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		c := w.player.(player.CharacterHolder).PlayerCharacter()
		c.SetHP(ratio * c.MaxHPValue())
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// hitDownTo lands one hit that leaves h at ratio of its maximum HP.
func (w *warriorWorld) hitDownTo(t *testing.T, h *npc.Hostile, ratio float64) {
	t.Helper()
	damage := h.CurrentHP() - int(ratio*float64(h.MaxHP()))
	if damage <= 0 {
		t.Fatalf("setup: HP %d of %d is already at or below %v", h.CurrentHP(), h.MaxHP(), ratio)
	}
	w.hit(t, h, damage)
	w.srv.ReadQueued(t, w.srv.Client)
}

// hateOn makes h want to attack other with weight, so other is the creature
// it hates most until something outweighs it.
func (w *warriorWorld) hateOn(t *testing.T, h, other *npc.Hostile, weight float64) {
	t.Helper()
	w.onQueue(t, h, func() { script.NPCOf(h).AddAttackDesire(script.NPCOf(other), weight) })
}

// sayLines returns the text of every line h said among frames.
func sayLines(frames [][]byte, h *npc.Hostile) []string { return says(frames, h.ObjectID()) }

// TestWarriorBaseChargesSpiritShots ports the spirit shot recharge of
// WarriorBase's attacked hook: with a SpiritShotRate, a draw under it
// charges the NPC's spirit shot on a hit; a draw at or over it, or no rate,
// charges none.
func TestWarriorBaseChargesSpiritShots(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		params npc.AIParams
		draw   int
		want   bool
	}{
		{"rate 100, draw 0", npc.AIParams{"SpiritShot": "1", "SpiritShotRate": "100"}, 0, true},
		{"rate 50, draw 49", npc.AIParams{"SpiritShot": "1", "SpiritShotRate": "50"}, 49, true},
		{"rate 50, draw 50", npc.AIParams{"SpiritShot": "1", "SpiritShotRate": "50"}, 50, false},
		{"no rate", npc.AIParams{"SpiritShot": "1"}, 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2f(t, raidBoot{}, ind2fTemplate(warriorID, row.params, nil))
			h := w.spawn(t, warriorID, 150, 0)
			w.draws.set(0, row.draw)

			w.hit(t, h, 10)
			if got := h.SpiritshotCharged(); got != row.want {
				t.Fatalf("spirit shot charged after the hit = %v, want %v", got, row.want)
			}
		})
	}
}

// TestWarriorBaseCreviceOfDiminsionStopsHateOutsideItsTerritory ports the
// first check of WarriorBase's attacked hook: an NPC with CreviceOfDiminsion
// that a creature outside its spawn territory hits drops the hate for it
// and does nothing else; without the parameter it hates it.
func TestWarriorBaseCreviceOfDiminsionStopsHateOutsideItsTerritory(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		params npc.AIParams
		hated  bool
	}{
		{"without the parameter", nil, true},
		{"with the parameter", npc.AIParams{"CreviceOfDiminsion": "1"}, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2f(t, raidBoot{}, ind2fTemplate(warriorID, row.params, nil))
			// The character is far outside the NPC's spawn territory.
			h := w.spawn(t, warriorID, 2000, 0)

			w.hit(t, h, 10)
			if got := h.AggroHate(w.player); (got > 0) != row.hated {
				t.Fatalf("hate for the character = %v, want hated %v", got, row.hated)
			}
			if _, ok := attackOn(h, w.player); ok && !row.hated {
				t.Fatal("the NPC wants to attack the character")
			}
		})
	}
}

// TestWarriorBaseAttackLowHPRetargets ports the AttackLowHP branch of
// WarriorBase's attacked hook: hit by a creature under 30% of its HP while
// it hates another most, a draw under 10 drops every attack desire, wants
// the attacker with weight 100 and says one of three lines by a second
// draw. A creature at more HP, or a draw of 10, changes nothing.
func TestWarriorBaseAttackLowHPRetargets(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		hp       float64
		draws    []int
		retarget bool
	}{
		{"low HP, draws 0 and 2", 0.2, []int{0, 2}, true},
		{"low HP, draw 10", 0.2, []int{10}, false},
		{"half HP", 0.5, []int{0, 2}, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2f(t, raidBoot{}, ind2fTemplate(warriorID, npc.AIParams{"AttackLowHP": "1"}, nil), ind2fTemplate(warriorTwoID, nil, nil))
			h := w.spawn(t, warriorID, 150, 0)
			other := w.spawn(t, warriorTwoID, -150, 0)
			w.hateOn(t, h, other, 500)
			w.setPlayerHPRatio(t, row.hp)
			w.draws.set(0, row.draws...)

			w.hit(t, h, 10)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			_, onOther := attackOn(h, other)
			d, onChar := attackOn(h, w.player)
			if row.retarget {
				if onOther || !onChar || d.Weight != 100 {
					t.Fatalf("attack desires after the hit: on the other %v, on the character %+v (%v); want the other dropped and the character at 100", onOther, d, onChar)
				}
				if got, want := sayLines(frames, h), []string{script.NpcString(1000428)}; !slices.Equal(got, want) {
					t.Fatalf("lines = %q, want %q", got, want)
				}
				return
			}
			if !onOther {
				t.Fatal("the NPC dropped its attack desire on the other")
			}
			if got := sayLines(frames, h); len(got) != 0 {
				t.Fatalf("lines = %q, want none", got)
			}
		})
	}
}

// warriorBaseOnlyID is an NPC bound to WarriorBase itself, which the
// Warrior behaviors, answering clan calls on their own, never reach.
const warriorBaseOnlyID = int32(29981)

// bootWarriorBaseOnly boots templates bound to WarriorBase itself, every
// script draw 0 until d queues others.
func bootWarriorBaseOnly(t *testing.T, templates ...*npc.Template) (*warriorWorld, *ind1Draws) {
	t.Helper()
	list, catalog := warriorScripts()
	const path = "test.WarriorBaseOnly"
	catalog[path] = func() script.Script {
		s := warrior.Base()
		s.NPCs = []int32{warriorBaseOnlyID}
		return s
	}
	list = append(list, script.Listing{Path: path})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}}, templates...)
	return w, draws
}

// TestWarriorBaseCalledClanMatesRetargetOnLowHP ports the AttackLowHP
// branch of WarriorBase's clan-attacked hook: an NPC called by a clan mate
// hit by a creature under 30% of its HP, while it hates another most, drops
// every attack desire and wants the attacker with weight 100 on a draw
// under 3; a draw of 3 changes nothing.
func TestWarriorBaseCalledClanMatesRetargetOnLowHP(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		draw     int
		retarget bool
	}{
		{"draw 2", 2, true},
		{"draw 3", 3, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			clan := func(id int32, ai npc.AIParams) *npc.Template {
				tmpl := warriorTemplate(id, ai)
				tmpl.Clans, tmpl.ClanRange = []string{"base"}, 600
				return tmpl
			}
			w, draws := bootWarriorBaseOnly(t, clan(warriorBaseOnlyID, npc.AIParams{"AttackLowHP": "1"}), clan(warriorFarID, nil))
			caller := w.spawn(t, warriorBaseOnlyID, 150, 0)
			called := w.spawn(t, warriorBaseOnlyID, 150, 100)
			other := w.spawn(t, warriorFarID, -150, 0)
			w.hateOn(t, called, other, 500)
			w.setPlayerHPRatio(t, 0.2)
			draws.set(row.draw)

			w.hit(t, caller, 10)
			_, onOther := attackOn(called, other)
			d, onChar := attackOn(called, w.player)
			if row.retarget {
				if onOther || !onChar || d.Weight != 100 {
					t.Fatalf("the called NPC's desires: on the other %v, on the character %+v (%v); want the other dropped and the character at 100", onOther, d, onChar)
				}
				return
			}
			if !onOther || onChar {
				t.Fatalf("the called NPC's desires: on the other %v, on the character %v; want the other kept and the character not wanted", onOther, onChar)
			}
		})
	}
}

// TestWarriorBaseIsVsChallengesAPlayerThatHitsIt ports the IsVs branch of
// WarriorBase's attacked hook and its script event 10001. Hit by a player
// while it hates another most and holds itself as its challenger, the NPC
// says a line naming the player, takes the player as its challenger,
// restores itself 20 s later, and calls the NPCs within 600: an idle one
// wants the player with weight 1000000 and says a line, a listener hears
// the event with the player's object id. A second hit does not call again.
func TestWarriorBaseIsVsChallengesAPlayerThatHitsIt(t *testing.T) {
	t.Parallel()
	w := bootIndividual2f(t, raidBoot{},
		ind2fTemplate(warriorID, npc.AIParams{"IsVs": "1"}, nil), ind2fTemplate(warriorTwoID, nil, nil), ind2fTemplate(warriorFarID, nil, nil), ind2fTemplate(ind1ListenerID, nil, nil))
	needsDrivenClock(t, w.srv)
	h := w.spawn(t, warriorID, 150, 0)
	other := w.spawn(t, warriorFarID, -150, 0)
	neighbor := w.spawn(t, warriorTwoID, 0, 200)
	listener := w.spawn(t, ind1ListenerID, 0, -200)
	// Past the radius of 600 from the NPC, these two hear nothing.
	farNeighbor := w.spawn(t, warriorTwoID, 0, 700)
	farListener := w.spawn(t, ind1ListenerID, 0, -700)
	w.hateOn(t, h, other, 500)
	challenger := func() int32 {
		c := script.NPCOf(h).Slot(script.SlotCreature0)
		if c == nil {
			return 0
		}
		return c.ObjectID()
	}
	if got := challenger(); got != h.ObjectID() {
		t.Fatalf("the NPC's challenger on creation = %d, want itself", got)
	}

	w.hit(t, h, 10)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got, want := sayLines(frames, h), []string{script.NpcString(1000288, "Hunter")}; !slices.Equal(got, want) {
		t.Fatalf("the NPC's lines = %q, want %q", got, want)
	}
	if got := challenger(); got != w.player.ObjectID() {
		t.Fatalf("the NPC's challenger after the hit = %d, want the character %d", got, w.player.ObjectID())
	}
	if d, ok := attackOn(neighbor, w.player); !ok || d.Weight != 1000000 {
		t.Fatalf("the neighbor's attack desire on the character = %+v (%v), want weight 1000000", d, ok)
	}
	if got, want := sayLines(frames, neighbor), []string{script.NpcString(1000392)}; !slices.Equal(got, want) {
		t.Fatalf("the neighbor's lines = %q, want %q", got, want)
	}
	if got, want := w.heard2e.take(listener.ObjectID()), []string{fmt.Sprintf("event=10001 arg1=%d arg2=0", w.player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("the listener's events = %q, want %q", got, want)
	}
	if d, ok := attackOn(farNeighbor, w.player); ok {
		t.Fatalf("the far neighbor's attack desire on the character = %+v, want none", d)
	}
	if got := sayLines(frames, farNeighbor); len(got) != 0 {
		t.Fatalf("the far neighbor's lines = %q, want none", got)
	}
	if got := w.heard2e.take(farListener.ObjectID()); len(got) != 0 {
		t.Fatalf("the far listener's events = %q, want none", got)
	}

	w.hit(t, h, 10)
	if got := sayLines(w.srv.ReadQueued(t, w.srv.Client), h); len(got) != 0 {
		t.Fatalf("lines after a second hit = %q, want none while the character is its challenger", got)
	}
	if got := w.heard2e.take(listener.ObjectID()); len(got) != 0 {
		t.Fatalf("the listener's events after a second hit = %q, want none", got)
	}

	w.srv.Advance(t, 20*time.Second-time.Millisecond)
	if got := challenger(); got != w.player.ObjectID() {
		t.Fatalf("the NPC's challenger before 20 s = %d, want the character", got)
	}
	w.srv.Advance(t, time.Millisecond)
	if got := challenger(); got != h.ObjectID() {
		t.Fatalf("the NPC's challenger at 20 s = %d, want itself", got)
	}
}

// TestWarriorBaseIsVsChallengesALevelPeerOnSight ports the IsVs branch of
// WarriorBase's see-creature hook: an idle IsVs NPC that sees a player
// within 2 levels of its own says a line naming it and wants to attack it.
func TestWarriorBaseIsVsChallengesALevelPeerOnSight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level  int
		attack bool
	}{
		{18, true}, {22, true}, {17, false}, {23, false},
	} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2fTemplate(warriorID, npc.AIParams{"IsVs": "1"}, nil)
			tmpl.Level = row.level
			w := bootIndividual2f(t, raidBoot{}, tmpl)
			h := w.spawn(t, warriorID, 150, 0)

			frames := w.ticks(t, 2)
			_, ok := attackOn(h, w.player)
			if ok != row.attack {
				t.Fatalf("attack desire on the character at NPC level %d = %v, want %v", row.level, ok, row.attack)
			}
			got := sayLines(frames, h)
			if row.attack {
				if want := []string{script.NpcString(1000287, "Hunter")}; !slices.Equal(got, want) {
					t.Fatalf("lines = %q, want %q", got, want)
				}
			} else if len(got) != 0 {
				t.Fatalf("lines = %q, want none", got)
			}
		})
	}
}

// TestWarriorBaseHeroSilhouetteAnswersAPlayerAtLowHP ports the
// HelpHeroSilhouette branch of WarriorBase's attacked hook: hit by a player
// at 20% of its HP or less, a draw under 3 spawns the named NPC 80 east and
// 80 south of it as its private, whose third parameter is the master's
// object id. A player at more HP, or a draw of 3, spawns none.
func TestWarriorBaseHeroSilhouetteAnswersAPlayerAtLowHP(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		hp    float64
		draw  int
		spawn bool
	}{
		{"low HP, draw 2", 0.15, 2, true},
		{"low HP, draw 3", 0.15, 3, false},
		{"half HP, draw 0", 0.5, 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2f(t, raidBoot{},
				ind2fTemplate(warriorID, npc.AIParams{"HelpHeroSilhouette": "20003"}, nil), ind2fTemplate(warriorTwoID, nil, nil))
			h := w.spawn(t, warriorID, 150, 0)
			w.setPlayerHPRatio(t, row.hp)
			w.draws.set(row.draw)

			w.hit(t, h, 10)
			got := w.liveNPCs(warriorTwoID)
			if !row.spawn {
				if len(got) != 0 {
					t.Fatalf("%d silhouettes, want none", len(got))
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("%d silhouettes, want 1", len(got))
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

// holdSpecial lands the special skill's effect on h and waits for it.
func (w *ind2eWorld) holdSpecial(t *testing.T, h *npc.Hostile) {
	t.Helper()
	me := w.player.ObjectID()
	ref := modelskill.Ref{ID: ind2fSpecialSkill, Level: 1}
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

// TestWarriorBaseCastsItsSpecialSkillWhenHurt ports the special skill of
// WarriorBase's attacked hook: a hit that finds the NPC at 30% of its HP or
// less, on a draw under 10, says a line and wants its special skill on
// itself with weight 1000000, unless it already holds an abnormal of the
// skill.
func TestWarriorBaseCastsItsSpecialSkillWhenHurt(t *testing.T) {
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
			// The special skill holds an effect of its own stack for a minute.
			special := ind1Skill(ind2fSpecialSkill, 2000, false)
			special.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "warrior_base_special", StackOrder: 1}}
			w := bootIndividual2f(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(special)}},
				ind2fTemplate(warriorID, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeSpecialSkill: ind2fSpecialSkill}))
			h := w.spawn(t, warriorID, 150, 0)
			if row.held {
				w.holdSpecial(t, h)
			}
			w.hitDownTo(t, h, row.down)
			w.draws.set(row.draw)

			w.hit(t, h, 1)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			if row.cast {
				w.wantCasts(t, "after the hit", h, map[modelskill.ID]ind1Cast{ind2fSpecialSkill: {h.ObjectID(), 1000000}})
				if got, want := sayLines(frames, h), []string{script.NpcString(1000397)}; !slices.Equal(got, want) {
					t.Fatalf("lines = %q, want %q", got, want)
				}
				return
			}
			w.wantCasts(t, "after the hit", h, nil)
		})
	}
}

// TestWarriorBaseExplodesWhenItsSelfExplosionEnds ports the self explosion
// of WarriorBase's attacked hook and use-skill-finished hook: a hit that
// finds the NPC under half its HP, on a draw under 5 and a second draw
// under 10 minus its HP in tenths, wants its self explosion on itself; the
// NPC dies when it has cast it.
func TestWarriorBaseExplodesWhenItsSelfExplosionEnds(t *testing.T) {
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
			explosion := ind1Skill(ind2fDebuffSkill, 2000, true)
			w := bootIndividual2f(t, raidBoot{opts: []gameservertest.Option{
				gameservertest.WithNpcSpawnSkills(modelskill.NewTable([]modelskill.Definition{explosion})),
				gameservertest.WithScriptSkills(explosion),
			}}, ind2fTemplate(warriorID, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeSelfExplosion: ind2fDebuffSkill}))
			h := w.spawn(t, warriorID, 150, 0)
			w.hitDownTo(t, h, row.down)
			w.draws.set(row.draw)

			w.hit(t, h, 1)
			if d, ok := desire(h, actorai.IntentionCast); ok != row.explode || (ok && d.Weight != 1000000) {
				t.Fatalf("cast desire = %+v (%v), want a self explosion at 1000000: %v", d, ok, row.explode)
			}
			var frames [][]byte
			for range 5 {
				frames = append(frames, w.ticks(t, 1)...)
				w.srv.Advance(t, time.Second)
				frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
			}
			if got := slices.Contains(shotSkillsOf(frames, h.ObjectID()), int32(ind2fDebuffSkill)); got != row.explode {
				t.Fatalf("the NPC cast its self explosion = %v, want %v", got, row.explode)
			}
			if got := script.NPCOf(h).Dead(); got != row.explode {
				t.Fatalf("the NPC is dead = %v, want %v", got, row.explode)
			}
		})
	}
}

// TestWarriorBaseAnswersTheHelpAndTargetEvents ports script events 10001
// and 10020 of WarriorBase. Event 10001 makes an NPC that is not attacking
// and has another challenger say a line and want the creature named with
// weight 1000000; event 10020 makes it want a named player with weight 100,
// and a named non-player, or no one, nothing.
func TestWarriorBaseAnswersTheHelpAndTargetEvents(t *testing.T) {
	t.Parallel()
	w := bootIndividual2f(t, raidBoot{}, ind2fTemplate(warriorID, nil, nil), ind2fTemplate(warriorTwoID, nil, nil))
	h := w.spawn(t, warriorID, 150, 0)
	other := w.spawn(t, warriorTwoID, -150, 0)

	w.sendEvent(t, h, 10020, other.ObjectID(), 0)
	w.sendEvent(t, h, 10020, 999999999, 0)
	if d, ok := desire(h, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after event 10020 naming a non-player and no one = %+v, want none", d)
	}
	w.sendEvent(t, h, 10020, w.player.ObjectID(), 0)
	if d, ok := attackOn(h, w.player); !ok || d.Weight != 100 {
		t.Fatalf("attack desire after event 10020 naming the character = %+v (%v), want weight 100", d, ok)
	}

	idle := w.spawn(t, warriorID, 0, 300)
	w.sendEvent(t, idle, 10001, w.player.ObjectID(), 0)
	if d, ok := attackOn(idle, w.player); !ok || d.Weight != 1000000 {
		t.Fatalf("attack desire after event 10001 = %+v (%v), want weight 1000000", d, ok)
	}
	if got, want := sayLines(w.srv.ReadQueued(t, w.srv.Client), idle), []string{script.NpcString(1000392)}; !slices.Equal(got, want) {
		t.Fatalf("lines after event 10001 = %q, want %q", got, want)
	}
}
