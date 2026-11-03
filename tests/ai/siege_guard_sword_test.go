package ai

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// More siege guard ids: Aden's spear and sword guards, a Sir Cronenberg,
// and the sword guard and wizard mercenaries (GludioSwordGuardUseSkill and
// GludioWizardUseSkill).
const (
	adenSpearGuardID      = int32(35239) // GludioSpearGuard
	adenSwordGuardID      = int32(35240) // GludioSwordGuard
	sirCronenbergID       = int32(35065) // GludioSirCronenberg
	swordGuardUseSkillID  = int32(35060)
	wizardUseSkillID      = int32(35061)
	siegeSpecialID        = modelskill.ID(9403)
	siegeDispelID         = modelskill.ID(9404)
	siegeHoldMagicID      = modelskill.ID(9405)
	siegeShoutLordsOfDawn = "Death to those who challenge the Lords of Dawn!" // NpcString 1800012
	siegeShoutLord        = "Death to those who challenge the Lord!"          // NpcString 1800013
)

// siegeSkillfulTemplate is siegeGuardTemplate listing the skills of the
// guards that use them: a special attack, a dispel, DD magic and hold
// magic.
func siegeSkillfulTemplate(id int32) *npc.Template {
	tmpl := siegeGuardTemplate(id)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypePhysicalSpecial: {ID: siegeSpecialID, Level: 1},
		npc.SkillTypeDispell:         {ID: siegeDispelID, Level: 1},
		npc.SkillTypeDDMagic:         {ID: siegeDDMagicID, Level: 1},
		npc.SkillTypeHoldMagic:       {ID: siegeHoldMagicID, Level: 1},
	}
	return tmpl
}

// siegeSkillfulNPCSkills are the skills siegeSkillfulTemplate lists.
func siegeSkillfulNPCSkills() *modelskill.Table {
	one := func(id modelskill.ID, magic bool, skillType string, castRange int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			Offensive: true, Magic: magic, SkillType: skillType, CastRange: castRange, HitTime: 1000,
			StaticHitTime: true, StaticReuse: true, Power: 1,
		}
	}
	return modelskill.NewTable([]modelskill.Definition{
		one(siegeSpecialID, false, "PDAM", 40),
		{
			ID: siegeDispelID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "CANCEL_DEBUFF", HitTime: 1000, StaticHitTime: true, StaticReuse: true,
		},
		one(siegeDDMagicID, true, "MDAM", 600),
		one(siegeHoldMagicID, true, "ROOT", 600),
	})
}

// castDesires returns h's queued cast desires by skill id.
func castDesires(h *npc.Hostile) map[modelskill.ID]actorai.Desire {
	out := map[modelskill.ID]actorai.Desire{}
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionCast {
			out[d.Skill.ID] = d
		}
	}
	return out
}

// wantCasts fails unless h's queued cast desires are exactly want, skill id
// to final target, each of weight 1000000 and walking to its target.
func wantCasts(t *testing.T, when, name string, h *npc.Hostile, want map[modelskill.ID]int32) {
	t.Helper()
	got := castDesires(h)
	if len(got) != len(want) {
		t.Fatalf("%s: %s cast desires = %+v, want skills %v", when, name, got, want)
	}
	for id, target := range want {
		d, ok := got[id]
		if !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != target || d.Weight != 1000000 || !d.MoveToTarget {
			t.Fatalf("%s: %s cast desire of %d = %+v, %v, want one on %d, weight 1000000, walking", when, name, id, d, ok, target)
		}
	}
}

// shouts returns the lines objectID shouts in frames.
func shouts(t *testing.T, frames [][]byte, objectID int32) []string {
	t.Helper()
	lines := says(frames, objectID)
	for _, f := range framesOf(frames, serverpackets.OpcodeNpcSay, objectID) {
		if kind := binary.LittleEndian.Uint32(f[5:9]); kind != 1 {
			t.Fatalf("NpcSay of %d has say type %d, want a shout (1)", objectID, kind)
		}
	}
	return lines
}

// TestGludioHoldGuardsAttackTheSealOfRulerToo ports the spelled hooks of
// GludioSirCronenberg, GludioSpearGuard and GludioSwordGuard: Seal of Ruler
// landing on any of them from the character, who neither defends nor owns
// their castle, makes it want to attack the character with weight 50000;
// another good skill does nothing.
func TestGludioHoldGuardsAttackTheSealOfRulerToo(t *testing.T) {
	t.Parallel()
	ids := []int32{sirCronenbergID, adenSpearGuardID, adenSwordGuardID}
	var templates []*npc.Template
	for _, id := range ids {
		templates = append(templates, siegeGuardTemplate(id))
	}
	w := bootSiegeGuards(t, siegeSkillBoot(t), templates...)
	for i, id := range ids {
		// Out of one another's clan range: no hit here calls a clan mate.
		h := w.spawn(t, id, 100, -400+400*i)
		w.selectTarget(t, h)
		w.cast(t, siegeBlessID)
		if got := h.AggroHate(w.player); got != 0 {
			t.Fatalf("guard %d hate after another good skill = %v, want none", id, got)
		}
		w.cast(t, sealOfRulerID)
		if got := h.AggroHate(w.player); got != 50000 {
			t.Fatalf("guard %d hate after Seal of Ruler = %v, want 50000", id, got)
		}
	}
}

// TestGludioSwordGuardUseSkillAnswersAHit ports
// GludioSwordGuardUseSkill's attacked hook on clanless sword guards 100 and
// 200 from the character. A guard's first hit finds it wanting no one, so
// it only adds GludioHold's 100. The second, from the one it now wants
// most, on a winning draw, makes it want its special attack on the
// character and, the character standing within 150, its dispel on itself,
// both at weight 1000000, before GludioHold's 100 more. On losing draws it
// wants neither.
func TestGludioSwordGuardUseSkillAnswersAHit(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		draw     int
		dx       int
		special  bool
		dispelOn bool
	}{{0, 100, true, true}, {0, 200, true, false}, {1, 100, false, false}} {
		t.Run(fmt.Sprintf("draw %d at %d", c.draw, c.dx), func(t *testing.T) {
			t.Parallel()
			tmpl := siegeSkillfulTemplate(swordGuardUseSkillID)
			tmpl.Clans = nil
			w := bootSiegeGuards(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptRand(func(int) int { return c.draw })}}, tmpl)
			guard := w.spawn(t, swordGuardUseSkillID, c.dx, 0)
			w.srv.InstallNPCCastSeam(t, guard, siegeSkillfulNPCSkills())

			w.hit(t, guard, siegeTwentieth)
			wantCasts(t, "after the first hit", "sword guard", guard, nil)
			if got := guard.AggroHate(w.player); got != 100 {
				t.Fatalf("sword guard hate after the first hit = %v, want 100", got)
			}

			w.hit(t, guard, siegeTwentieth)
			want := map[modelskill.ID]int32{}
			if c.special {
				want[siegeSpecialID] = w.player.ObjectID()
			}
			if c.dispelOn {
				want[siegeDispelID] = guard.ObjectID()
			}
			wantCasts(t, "after the second hit", "sword guard", guard, want)
			if got := guard.AggroHate(w.player); got != 200 {
				t.Fatalf("sword guard hate after the second hit = %v, want 200", got)
			}
		})
	}
}

// TestGludioSwordGuardUseSkillAnswersAClanCall ports
// GludioSwordGuardUseSkill's clan-attacked hook: two sword guards, one 200
// from the character and one within 150, are called by a knight the
// character hits. The first call finds them wanting no one: each only adds
// GludioHold's 50. On the second, the character is the one each wants
// most: both want their special attack on it, and only the near one its
// dispel, before 50 more.
func TestGludioSwordGuardUseSkillAnswersAClanCall(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{}, siegeGuardTemplate(adenKnightID), siegeSkillfulTemplate(swordGuardUseSkillID))
	knight := w.spawn(t, adenKnightID, 100, 0)
	far := w.spawn(t, swordGuardUseSkillID, 0, 200)
	near := w.spawn(t, swordGuardUseSkillID, -100, 50)
	for _, h := range []*npc.Hostile{far, near} {
		w.srv.InstallNPCCastSeam(t, h, siegeSkillfulNPCSkills())
	}
	names := map[*npc.Hostile]string{far: "far sword guard", near: "near sword guard"}

	w.hit(t, knight, siegeTwentieth)
	for h, name := range names {
		wantCasts(t, "after the first call", name, h, nil)
	}
	w.hates(t, "after the first call", map[*npc.Hostile]float64{far: 50, near: 50}, names)

	w.hit(t, knight, siegeTwentieth)
	wantCasts(t, "after the second call", "far sword guard", far, map[modelskill.ID]int32{siegeSpecialID: w.player.ObjectID()})
	wantCasts(t, "after the second call", "near sword guard", near, map[modelskill.ID]int32{
		siegeSpecialID: w.player.ObjectID(),
		siegeDispelID:  near.ObjectID(),
	})
	w.hates(t, "after the second call", map[*npc.Hostile]float64{far: 100, near: 100}, names)
}

// TestGludioSwordGuardUseSkillAimsItsSpecialAtWhatItFights pins the
// attacker test of GludioSwordGuardUseSkill's special attack roll: a sword
// guard 100 from the character that wants a monster most (weight 10000),
// hit by the character or called by a knight the character hits, on a
// winning draw, wants only its dispel on itself, the character within 150:
// no special attack, the character not being the one it fights.
func TestGludioSwordGuardUseSkillAimsItsSpecialAtWhatItFights(t *testing.T) {
	t.Parallel()
	for _, called := range []bool{false, true} {
		name := "hit"
		if called {
			name = "clan call"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tmpl := siegeSkillfulTemplate(swordGuardUseSkillID)
			if !called {
				tmpl.Clans = nil
			}
			w := bootSiegeGuards(t, raidBoot{}, tmpl, siegeGuardTemplate(adenKnightID), warriorTemplate(siegeDummyID, nil))
			guard := w.spawn(t, swordGuardUseSkillID, 100, 0)
			monster := w.spawn(t, siegeDummyID, 120, 0)
			w.srv.InstallNPCCastSeam(t, guard, siegeSkillfulNPCSkills())
			w.onQueue(t, guard, func() { script.NPCOf(guard).AddAttackDesire(script.NPCOf(monster), 10000) })
			if top := guard.AI().TopDesireTarget(); top == nil || top.ObjectID() != monster.ObjectID() {
				t.Fatalf("setup: sword guard top desire target = %v, want the monster", top)
			}

			if called {
				w.hit(t, w.spawn(t, adenKnightID, 100, 100), siegeTwentieth)
			} else {
				w.hit(t, guard, siegeTwentieth)
			}
			wantCasts(t, "after the "+name, "sword guard", guard, map[modelskill.ID]int32{siegeDispelID: guard.ObjectID()})
		})
	}
}

// TestGludioSwordGuardUseSkillShoutsAtWhatItSees ports
// GludioSwordGuardUseSkill's see-creature hook, which overrides
// GludioHold's: from its first AI cycle on, the sword guard that sees the
// character wants to attack it with weight 200; a draw of 0 makes it shout
// NpcString 1800012 first, a draw of 1 NpcString 1800013, and any other
// nothing.
func TestGludioSwordGuardUseSkillShoutsAtWhatItSees(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		draw int
		want []string
	}{{0, []string{siegeShoutLordsOfDawn}}, {1, []string{siegeShoutLord}}, {2, nil}} {
		t.Run(fmt.Sprintf("draw %d", c.draw), func(t *testing.T) {
			t.Parallel()
			w := bootSiegeGuards(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptRand(func(int) int { return c.draw })}},
				siegeGuardTemplate(swordGuardUseSkillID))
			guard := w.spawn(t, swordGuardUseSkillID, 150, 0)

			frames := w.ticks(t, 1)
			if got := shouts(t, frames, guard.ObjectID()); !slices.Equal(got, c.want) {
				t.Fatalf("sword guard shouts = %q, want %q", got, c.want)
			}
			if d, ok := desire(guard, actorai.IntentionAttack); !ok || d.Weight != 200 || !d.MoveToTarget || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
				t.Fatalf("sword guard attack desire = %+v, %v, want one on the character, weight 200", d, ok)
			}
		})
	}
}
