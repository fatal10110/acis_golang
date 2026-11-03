package ai

import (
	"fmt"
	"math"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the scenarios of the second individual behavior batch's
// fourth PR spawn (#187 PR 4), each a fixture monster of level 1 unless a
// scenario says otherwise.
const (
	ind2dSelfAggroID   = int32(20260) // WarriorCastingEnchantSelfAggressive
	ind2dDollID        = int32(29023) // ZakenPrivateDollBlader
	ind2dRoyalID       = int32(18133) // RoyalRushWarriorPhysicalSpecial
	ind2dRoyalTwoID    = int32(18136) // RoyalRushWarriorPhysicalSpecial
	ind2dStoneID       = int32(18243) // RoyalRushWarriorStone
	ind2dNinjaID       = int32(21539) // SaintNinja, a main form
	ind2dNinjaFormID   = int32(21540) // SaintNinja, the main form's other self
	ind2dNinjaTwoID    = int32(21524) // SaintNinja, another form
	ind2dVampireID     = int32(21424) // WarriorCorpseVampire
	ind2dVampAggroID   = int32(21425) // WarriorCorpseVampireAggressive
	ind2dMobHateID     = int32(21582) // WarriorCorpseVampireMobHate
	ind2dMobHateTwoID  = int32(21594) // WarriorCorpseVampireMobHate
	ind2dPlainWarrior  = warriorID    // a Warrior, which shouts when created
	ind2dCreatedShout  = "1000286"
	ind2dSelfBuffSkill = modelskill.ID(9601)
	ind2dBuffSkill     = modelskill.ID(9602)
	ind2dSpecialSkill  = modelskill.ID(9603)
	ind2dDebuffSkill   = modelskill.ID(9604)
	ind2dRangeSkill    = modelskill.ID(9605)
	ind2dBuff1Skill    = modelskill.ID(9606)
	ind2dBuff2Skill    = modelskill.ID(9607)
	ind2dTeleportSkill = modelskill.ID(9608)
	ind2dDD1Skill      = modelskill.ID(9609)
	ind2dDD2Skill      = modelskill.ID(9610)
)

// ind2dBatch are the classes the PR ports, in scripts.xml order.
var ind2dBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantSelfAggressive", warrior.CastingEnchantSelfAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.ZakenPrivateDollBlader", warrior.ZakenPrivateDollBlader},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.RoyalRushWarriorPhysicalSpecial", warrior.RoyalRushWarriorPhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.RoyalRushWarriorStone", warrior.RoyalRushWarriorStone},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.SaintNinja", warrior.SaintNinja},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampire", warrior.CorpseVampire},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampireAggressive", warrior.CorpseVampireAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampireMobHate", warrior.CorpseVampireMobHate},
}

// bootIndividual2d boots the PR's classes after the party Warriors and the
// WarriorCastingEnchant subtree beside templates, with extra's options
// added last and its step run before the character enters the world,
// every draw 0 until a scenario queues others, every NPC knowing the
// batch's skills, with the director script the shared helpers land
// effects and send script events through.
func bootIndividual2d(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := partyScripts()
	for _, c := range append(append(ind2cBatch[:0:0], ind2cBatch...), ind2dBatch...) {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(ind2dSkills()),
		gameservertest.WithScriptSkills(ind2dHeld(ind2dSelfBuffSkill, true), ind2dHeld(ind2dDebuffSkill, false)),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind2dTemplate is a fixture batch NPC of id with ai, listing skills by
// type, in the clan the scenario's NPCs share.
func ind2dTemplate(id int32, ai npc.AIParams, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for k, v := range skills {
		tmpl.SkillsByType[k] = modelskill.Ref{ID: v, Level: 1}
	}
	tmpl.Clans, tmpl.ClanRange = []string{"ind2d"}, 600
	return tmpl
}

// ind2dHeld is a fixture skill of id reaching 2000 whose effect, of a
// stack of its own, holds for a minute: a self buff when self is set, a
// one-target debuff otherwise.
func ind2dHeld(id modelskill.ID, self bool) modelskill.Definition {
	d := ind1Skill(id, 2000, self)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: fmt.Sprintf("ind2d_%d", id), StackOrder: 1}}
	return d
}

// ind2dSkills are the batch NPCs' skills.
func ind2dSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		ind2dHeld(ind2dSelfBuffSkill, true), ind1Skill(ind2dBuffSkill, 0, true), ind1Skill(ind2dSpecialSkill, 2000, false),
		ind2dHeld(ind2dDebuffSkill, false), ind1Skill(ind2dRangeSkill, 0, true), ind1Skill(ind2dBuff1Skill, 0, true),
		ind1Skill(ind2dBuff2Skill, 0, true), ind1Skill(ind2dTeleportSkill, 2000, false), ind1Skill(ind2dDD1Skill, 2000, false),
		ind1Skill(ind2dDD2Skill, 2000, false),
	})
}

// hasCast reports whether h holds a cast desire of skill id.
func hasCast(h *npc.Hostile, id modelskill.ID) bool {
	_, ok := ind1Casts(h)[id]
	return ok
}

// TestWarriorCastingEnchantSelfAggressiveBuffsOnSight ports
// WarriorCastingEnchantSelfAggressive's see-creature hook. Settled in its
// territory and above half its hit points, seeing the character on a draw
// under 33 of 100, it wants its buff on itself (weight 1000000); any
// settled sight spends the buff (i_ai1). A young NPC neither buffs nor
// spends it. Each one attacks the character it sees (hate 200).
func TestWarriorCastingEnchantSelfAggressiveBuffsOnSight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		draw   int
		share  float64
		settle int
		buff   bool
		spent  int32
	}{
		{"draw 32", 32, 1, 9, true, 1},
		{"draw 33", 33, 1, 9, false, 1},
		{"half its hit points", 0, 0.5, 9, false, 1},
		{"young", 0, 1, 0, false, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2d(t, raidBoot{}, ind2dTemplate(ind2dSelfAggroID, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeBuff: ind2dBuffSkill}))
			n := w.spawn(t, ind2dSelfAggroID, 1500, 0)
			w.ticks(t, row.settle)
			if row.settle > 0 && (n.LifeTime() <= 7 || !script.NPCOf(n).InMyTerritory()) {
				t.Fatalf("setup: life time %d, in territory %v, want past 7 and in it", n.LifeTime(), script.NPCOf(n).InMyTerritory())
			}
			if row.share < 1 {
				w.setHPShare(t, n, row.share)
			}
			w.draws.set(0, row.draw)
			w.moveCharacter(t, n, -50, 0)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			if row.buff {
				want[ind2dBuffSkill] = ind1Cast{n.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after it saw the character", n, want)
			if got := ind1Slot(n, script.SlotIntAI1); got != row.spent {
				t.Fatalf("i_ai1 after the sight = %d, want %d", got, row.spent)
			}
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
		})
	}
}

// TestZakenPrivateDollBladerKeepsWhereItWasCreated ports
// ZakenPrivateDollBlader's created hook: it keeps the x, y and z it was
// created at in its weight point, respawn time and flag, and, calling no
// parent, never shouts as a created Warrior does.
func TestZakenPrivateDollBladerKeepsWhereItWasCreated(t *testing.T) {
	t.Parallel()
	shout := npc.AIParams{"ShoutMsg1": ind2dCreatedShout}
	w := bootIndividual2d(t, raidBoot{}, ind2dTemplate(ind2dDollID, shout, nil), warriorTemplate(ind2dPlainWarrior, shout))
	plain := w.spawnFrames(t, ind2dPlainWarrior, 1500, 300)
	if got := says(plain.frames, plain.h.ObjectID()); len(got) != 1 {
		t.Fatalf("setup: a plain Warrior's created shouts = %q, want one", got)
	}
	d := w.spawnFrames(t, ind2dDollID, 700, 40)
	if got := says(d.frames, d.h.ObjectID()); len(got) != 0 {
		t.Fatalf("doll blader created shouts = %q, want none", got)
	}
	x, y, z := d.h.Position()
	scratch := script.NPCOf(d.h)
	for _, slot := range []struct {
		name string
		slot script.IntSlot
		want int
	}{{"weight point", script.SlotWeightPoint, x}, {"respawn time", npc.IntRespawnTime, y}, {"flag", script.SlotFlag, z}} {
		if got := scratch.Int(slot.slot); int(got) != slot.want {
			t.Errorf("created %s = %d, want %d", slot.name, got, slot.want)
		}
	}
}

// TestZakenPrivateDollBladerSeesOnlyItsHeight ports
// ZakenPrivateDollBlader's see-creature hook. It notices the character
// only less than 100 above or below it: in its territory it wants to
// attack it (hate 200), then runs its own attacked hook as if the
// character had hit it for 9, which hates by that damage. At 100 above or
// below it notices nothing.
func TestZakenPrivateDollBladerSeesOnlyItsHeight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		dz     int
		notice bool
	}{{0, true}, {99, true}, {-99, true}, {100, false}, {-100, false}} {
		t.Run(fmt.Sprintf("dz %d", row.dz), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2dTemplate(ind2dDollID, nil, nil)
			tmpl.Level, tmpl.CanMove = 15, false
			w := bootIndividual2d(t, raidBoot{}, tmpl)
			n := w.spawn(t, ind2dDollID, 1500, 0)
			w.ticks(t, 9)
			obj, ok := w.srv.State.Player(w.player.ObjectID())
			if !ok {
				t.Fatal("the character is not in the world")
			}
			x, y, z := n.Position()
			if err := w.srv.State.Move(obj, x-50, y, z+row.dz); err != nil {
				t.Fatalf("move the character: %v", err)
			}
			w.ticks(t, 1)
			want := 0.0
			if row.notice {
				// The attacked hook the sight runs hates by the damage, 100
				// for each level-and-seven of it.
				want = 200 + 9.0/float64(15+7)*100
			}
			if got := n.AggroHate(w.player); math.Abs(got-want) > 1e-9 {
				t.Fatalf("hate of the character %d above = %v, want %v", row.dz, got, want)
			}
		})
	}
}

// TestZakenPrivateDollBladerCursesAndMutesOverLevelled ports
// ZakenPrivateDollBlader's attacked and see-spell hooks. Against the level
// 20 character, a level 11 doll blader (9 levels below, more than 8) wants
// its raid curse on the character when hit and its raid mute, instead of
// its parent's answer, when it sees the character cast, each weight
// 1000000; a level 12 one (8 levels below) wants neither. Standing 150
// below the character it never notices it by sight.
func TestZakenPrivateDollBladerCursesAndMutesOverLevelled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		curse bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2dTemplate(ind2dDollID, nil, nil)
			tmpl.Level, tmpl.CanMove = row.level, false
			w := bootIndividual2d(t, raidSpellBoot(t), tmpl)
			n := w.spawnBelow(t, ind2dDollID, 400, 0, 150)
			w.srv.InstallNPCCastSeam(t, n, modelskill.NewTable([]modelskill.Definition{raidCurseDefinition(), ind1Skill(premoMuteID, 600, false)}))
			me := w.player.ObjectID()

			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.curse {
				want[raidCurseID] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			if got := n.AggroHate(w.player); got <= 0 {
				t.Errorf("hate after the hit = %v, want the parent's", got)
			}

			w.castSelfBuff()
			if row.curse {
				w.srv.AdvanceUntil(t, "the raid mute desire", func() bool { return hasCast(n, premoMuteID) })
				want[premoMuteID] = ind1Cast{me, 1000000}
			} else {
				w.srv.Advance(t, 2*time.Second)
			}
			w.wantCasts(t, "after the spell", n, want)
		})
	}
}

// ind2dRoyalTemplate is a fixture RoyalRushWarriorPhysicalSpecial NPC of
// id with ai, ten soulshots (AI parameter SoulShot), its self buff and its
// physical special by type.
func ind2dRoyalTemplate(id int32, ai npc.AIParams) *npc.Template {
	params := npc.AIParams{"SoulShot": "10"}
	for k, v := range ai {
		params[k] = v
	}
	return ind2dTemplate(id, params, map[npc.SkillType]modelskill.ID{npc.SkillTypeSelfBuff: ind2dSelfBuffSkill, npc.SkillTypePhysicalSpecial: ind2dSpecialSkill})
}

// TestRoyalRushWarriorPhysicalSpecialKeepsItsBuffUp ports
// RoyalRushWarriorPhysicalSpecial's created and timer hooks. Created, it
// wants its self buff on itself (weight 1000000), clears i_ai0 and, calling
// no parent, never shouts; every 120 s after it wants the buff again
// unless it still holds it. With no self buff listed it wants nothing and
// starts no timer.
func TestRoyalRushWarriorPhysicalSpecialKeepsItsBuffUp(t *testing.T) {
	t.Parallel()
	shout := npc.AIParams{"ShoutMsg1": ind2dCreatedShout}
	bare := ind2dTemplate(ind2dRoyalTwoID, shout, nil)
	w := bootIndividual2d(t, raidBoot{}, ind2dRoyalTemplate(ind2dRoyalID, shout), bare)
	g := w.spawnFrames(t, ind2dRoyalID, 400, 0)
	n := g.h
	if got := says(g.frames, n.ObjectID()); len(got) != 0 {
		t.Fatalf("created shouts = %q, want none", got)
	}
	w.wantCasts(t, "created", n, map[modelskill.ID]ind1Cast{ind2dSelfBuffSkill: {n.ObjectID(), 1000000}})
	if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("created i_ai0 = %d, want 0", got)
	}
	other := w.spawn(t, ind2dRoyalTwoID, 400, 200)
	w.wantCasts(t, "created with no self buff", other, nil)

	w.clear(t, n)
	w.srv.Advance(t, 119*time.Second)
	if hasCast(n, ind2dSelfBuffSkill) {
		t.Fatal("wants its self buff before 120 s")
	}
	w.srv.AdvanceUntil(t, "the self buff desire at 120 s", func() bool { return hasCast(n, ind2dSelfBuffSkill) })

	w.land(t, ind2dSelfBuffSkill, n)
	w.clear(t, n)
	w.srv.Advance(t, 120*time.Second)
	if hasCast(n, ind2dSelfBuffSkill) {
		t.Fatal("wants its self buff while it holds it")
	}
	w.wantCasts(t, "with no self buff, after 240 s", other, nil)
}

// TestRoyalRushWarriorPhysicalSpecialStrikesUntilItDrawsItsWeapon ports
// RoyalRushWarriorPhysicalSpecial's attacked hook, every draw 0. Hit by
// its top desire target closer than 100, it wants its physical special on
// it (weight 1000000, on top of the 1000000 its parent wants for the
// creature it hates most); from farther it leaves the strike to its
// parent. Once a hit finds it below half of its hit points it draws its
// WeaponID weapon glowing at +15 (an NpcInfo to the character) and strikes
// no more on its own, nor charges a soulshot.
func TestRoyalRushWarriorPhysicalSpecialStrikesUntilItDrawsItsWeapon(t *testing.T) {
	t.Parallel()
	ai := npc.AIParams{"WeaponID": fmt.Sprint(royalWeaponID)}
	w := bootIndividual2d(t, raidBoot{}, ind2dRoyalTemplate(ind2dRoyalID, ai))
	near := w.spawn(t, ind2dRoyalID, 50, 0)
	far := w.spawn(t, ind2dRoyalID, 300, 0)
	me := w.player.ObjectID()
	for _, h := range []*npc.Hostile{near, far} {
		w.clear(t, h)
		w.hit(t, h, 10)
		w.wantCasts(t, "after the first hit", h, nil)
		w.hit(t, h, 10)
	}
	w.wantCasts(t, "near, after a hit by its top desire target", near, map[modelskill.ID]ind1Cast{ind2dSpecialSkill: {me, 2000000}})
	w.wantCasts(t, "far, after a hit by its top desire target", far, map[modelskill.ID]ind1Cast{ind2dSpecialSkill: {me, 1000000}})

	n := near
	w.hit(t, n, int(n.HP())-n.MaxHP()*45/100)
	if s := n.NPCInfoSnapshot(); s.RightHand == int(royalWeaponID) {
		t.Fatal("drew its weapon above half of its hit points")
	}
	w.clear(t, n)
	w.srv.ReadQueued(t, w.srv.Client)
	w.hit(t, n, 10)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if s := n.NPCInfoSnapshot(); s.RightHand != int(royalWeaponID) || s.EnchantEffect != 15 {
		t.Fatalf("hands after a hit below half = %d glow %d, want %d glowing at 15", s.RightHand, s.EnchantEffect, royalWeaponID)
	}
	if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after it drew its weapon = %d, want 1", got)
	}
	if infos := framesOf(frames, serverpackets.OpcodeNPCInfo, n.ObjectID()); len(infos) == 0 {
		t.Fatal("the character was sent no NpcInfo of the drawn weapon")
	}
	w.wantCasts(t, "after it drew its weapon", n, map[modelskill.ID]ind1Cast{ind2dSpecialSkill: {me, 1000000}})
	w.clear(t, n)
	w.hit(t, n, 10)
	frames = w.srv.ReadQueued(t, w.srv.Client)
	w.wantCasts(t, "hit with its weapon drawn", n, map[modelskill.ID]ind1Cast{ind2dSpecialSkill: {me, 1000000}})
	if got := castsOf(frames, n, 2154); len(got) != 0 {
		t.Fatalf("soulshot MagicSkillUse frames after its weapon was drawn = %d, want none", len(got))
	}
}

// TestRoyalRushWarriorPhysicalSpecialAssistsItsClan ports
// RoyalRushWarriorPhysicalSpecial's clan-attacked hook, every draw 0. A
// settled clan member that is not attacking wants its physical special on
// an attacker farther than 100 from it (weight 1000000, on top of the
// 1000000 its parent wants); a member that drew its weapon charges a
// soulshot (MagicSkillUse 2154) instead; a member closer than 100 leaves
// it to its parent.
func TestRoyalRushWarriorPhysicalSpecialAssistsItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual2d(t, raidBoot{}, ind2dRoyalTemplate(ind2dRoyalID, nil), ind2dRoyalTemplate(ind2dRoyalTwoID, nil))
	victim := w.spawn(t, ind2dRoyalID, 0, 400)
	far := w.spawn(t, ind2dRoyalTwoID, 300, 0)
	drawn := w.spawn(t, ind2dRoyalTwoID, -300, 0)
	near := w.spawn(t, ind2dRoyalTwoID, 50, 0)
	members := []*npc.Hostile{far, drawn, near}
	w.onQueue(t, drawn, func() { script.NPCOf(drawn).SetInt(script.SlotIntAI0, 1) })
	w.ticks(t, 8)
	for _, h := range append(members, victim) {
		if h.LifeTime() <= 7 {
			t.Fatalf("member life time = %d, want past 7", h.LifeTime())
		}
		w.clear(t, h)
	}
	w.srv.ReadQueued(t, w.srv.Client)
	w.hit(t, victim, 10)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	me := w.player.ObjectID()
	w.wantCasts(t, "far member", far, map[modelskill.ID]ind1Cast{ind2dSpecialSkill: {me, 2000000}})
	w.wantCasts(t, "member with its weapon drawn", drawn, map[modelskill.ID]ind1Cast{ind2dSpecialSkill: {me, 1000000}})
	w.wantCasts(t, "near member", near, map[modelskill.ID]ind1Cast{ind2dSpecialSkill: {me, 1000000}})
	if got := castsOf(frames, drawn, 2154); len(got) != 1 {
		t.Fatalf("soulshot MagicSkillUse frames of the member with its weapon drawn = %d, want 1", len(got))
	}
	for _, h := range []*npc.Hostile{far, near} {
		if got := castsOf(frames, h, 2154); len(got) != 0 {
			t.Fatalf("member %d charged a soulshot", h.ObjectID())
		}
	}
}

// TestRoyalRushWarriorPhysicalSpecialAttacksTheNamedCreature ports
// RoyalRushWarriorPhysicalSpecial's script event 1234: on a draw under 80
// it wants the creature the event names with weight 300; otherwise it
// drops its attack desires first and wants it with weight 1000. Another
// event does nothing.
func TestRoyalRushWarriorPhysicalSpecialAttacksTheNamedCreature(t *testing.T) {
	t.Parallel()
	w := bootIndividual2d(t, raidBoot{}, ind2dTemplate(ind2dRoyalID, nil, nil))
	n := w.spawn(t, ind2dRoyalID, 1500, 0)
	me := w.player.ObjectID()
	w.sendEvent(t, n, 1235, me, 0)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate after event 1235 = %v, want none", got)
	}
	w.sendEvent(t, n, 1234, me, 0)
	if got := n.AggroHate(w.player); got != 300 {
		t.Fatalf("hate after event 1234, draw 0 = %v, want 300", got)
	}
	w.draws.set(80)
	w.sendEvent(t, n, 1234, me, 0)
	if got := n.AggroHate(w.player); got != 1000 {
		t.Fatalf("hate after event 1234, draw 80 = %v, want 1000", got)
	}
}

// ind2dStoneTemplate is a fixture RoyalRushWarriorStone listing its two
// self buffs, its debuff and its self range DD magic by type.
func ind2dStoneTemplate(ai npc.AIParams) *npc.Template {
	return ind2dTemplate(ind2dStoneID, ai, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSelfBuff1: ind2dBuff1Skill, npc.SkillTypeSelfBuff2: ind2dBuff2Skill,
		npc.SkillTypeDebuff: ind2dDebuffSkill, npc.SkillTypeSelfRangeDDMagic: ind2dRangeSkill,
	})
}

// TestRoyalRushWarriorStoneBuffsAndAnswersItsTopTarget ports
// RoyalRushWarriorStone's created and attacked hooks, every draw 0.
// Created, it wants its first self buff on itself (weight 1000000) and
// shouts as a created Warrior. A hit by its top desire target wants its
// debuff on it, its self range DD magic and its second self buff, each
// weight 1000000; a hit while it desires nothing first wants none of
// them.
func TestRoyalRushWarriorStoneBuffsAndAnswersItsTopTarget(t *testing.T) {
	t.Parallel()
	w := bootIndividual2d(t, raidBoot{}, ind2dStoneTemplate(npc.AIParams{"ShoutMsg1": ind2dCreatedShout}))
	s := w.spawnFrames(t, ind2dStoneID, 400, 0)
	n := s.h
	if got := says(s.frames, n.ObjectID()); len(got) != 1 {
		t.Fatalf("created shouts = %q, want one", got)
	}
	w.wantCasts(t, "created", n, map[modelskill.ID]ind1Cast{ind2dBuff1Skill: {n.ObjectID(), 1000000}})
	w.clear(t, n)
	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by its top desire target", n, map[modelskill.ID]ind1Cast{
		ind2dDebuffSkill: {w.player.ObjectID(), 1000000},
		ind2dRangeSkill:  {n.ObjectID(), 1000000},
		ind2dBuff2Skill:  {n.ObjectID(), 1000000},
	})
}

// TestRoyalRushWarriorStoneSeesAndAssists ports RoyalRushWarriorStone's
// see-creature and clan-attacked hooks, every draw 0. Settled and not
// attacking, it attacks the character it sees (hate 200); young, it does
// not. A settled member not attacking wants its second self buff on
// itself (weight 1000000) when the character hits its clan; one already
// attacking does not.
func TestRoyalRushWarriorStoneSeesAndAssists(t *testing.T) {
	t.Parallel()
	t.Run("sight", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual2d(t, raidBoot{}, ind2dStoneTemplate(nil))
		young := w.spawn(t, ind2dStoneID, 1500, 0)
		w.moveCharacter(t, young, -50, 0)
		w.ticks(t, 1)
		if got := young.AggroHate(w.player); got != 0 {
			t.Fatalf("hate of a young stone = %v, want none", got)
		}
		w.moveCharacter(t, young, -1500, 0)
		w.ticks(t, 9)
		w.moveCharacter(t, young, -50, 0)
		w.ticks(t, 1)
		if got := young.AggroHate(w.player); got != 200 {
			t.Fatalf("hate of a settled stone = %v, want 200", got)
		}
	})
	t.Run("clan", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual2d(t, raidBoot{}, ind2dStoneTemplate(nil))
		victim := w.spawn(t, ind2dStoneID, 1500, 0)
		idle := w.spawn(t, ind2dStoneID, 1500, 300)
		attacking := w.spawn(t, ind2dStoneID, 1500, -300)
		// Beside the member it hits, the character stands out of the idle
		// member's sight; the hit lands before any member has settled, so
		// none answers it.
		w.moveCharacter(t, attacking, 0, -60)
		w.clear(t, attacking)
		w.hit(t, attacking, 10)
		w.ticks(t, 8)
		if got := attacking.AI().CurrentIntention(); got != actorai.IntentionAttack {
			t.Fatalf("setup: the hit stone's intention = %v, want attack", got)
		}
		for _, h := range []*npc.Hostile{victim, idle, attacking} {
			if h.LifeTime() <= 7 {
				t.Fatalf("member life time = %d, want past 7", h.LifeTime())
			}
		}
		w.clear(t, idle)
		w.hit(t, victim, 10)
		w.wantCasts(t, "idle member", idle, map[modelskill.ID]ind1Cast{ind2dBuff2Skill: {idle.ObjectID(), 1000000}})
		if hasCast(attacking, ind2dBuff2Skill) {
			t.Fatal("the attacking member wants its second self buff")
		}
	})
}

// ind2dNinjaTemplate is a fixture SaintNinja of id, its main form when
// main is set, whose other self is ind2dNinjaFormID, listing its self buff
// and its teleport effect by type.
func ind2dNinjaTemplate(id int32, main bool) *npc.Template {
	form := "0"
	if main {
		form = "1"
	}
	return ind2dTemplate(id, npc.AIParams{"IsMainForm": form, "OtherSelf": fmt.Sprint(ind2dNinjaFormID)},
		map[npc.SkillType]modelskill.ID{npc.SkillTypeSelfBuff: ind2dBuffSkill, npc.SkillTypeTeleportEffect: ind2dTeleportSkill})
}

// TestSaintNinjaCallsItsOtherSelf ports SaintNinja's created and attacked
// hooks. Created, it clears i_ai0 and wants its self buff on itself
// (weight 1000000). Hit, a main form, on a draw under 80 of 100, calls its
// other self as its private, at its own point plus a draw of 20 on x and
// on y, with the spawn parameters 1000, the attacker and 1,
// and spends the call (i_ai0): a later hit calls none. A draw of 80, or a
// form that is not the main one, calls none.
func TestSaintNinjaCallsItsOtherSelf(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		main bool
		draw int
		call bool
	}{{"main form, draw 79", true, 79, true}, {"main form, draw 80", true, 80, false}, {"other form", false, 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind2dNinjaTemplate(ind2dNinjaID, row.main)
			tmpl.CanMove = false
			// The other self is in no clan, so the hit's clan call does not
			// bring it to the character.
			self := ind2dNinjaTemplate(ind2dNinjaFormID, false)
			self.Clans = nil
			w := bootIndividual2d(t, raidBoot{}, tmpl, self)
			n := w.spawn(t, ind2dNinjaID, 400, 0)
			w.wantCasts(t, "created", n, map[modelskill.ID]ind1Cast{ind2dBuffSkill: {n.ObjectID(), 1000000}})
			if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
				t.Fatalf("created i_ai0 = %d, want 0", got)
			}
			x, y, z := n.Position()
			w.draws.set(0, row.draw, 7, 9)
			w.hit(t, n, 10)
			selves := w.liveNPCs(ind2dNinjaFormID)
			if !row.call {
				if len(selves) != 0 || ind1Slot(n, script.SlotIntAI0) != 0 {
					t.Fatalf("other selves = %d, i_ai0 %d; want none and 0", len(selves), ind1Slot(n, script.SlotIntAI0))
				}
				return
			}
			if len(selves) != 1 {
				t.Fatalf("other selves after the hit = %d, want 1", len(selves))
			}
			p := selves[0]
			px, py, pz := p.Position()
			if px != x+7 || py != y+9 || pz != z {
				t.Errorf("other self at %d,%d,%d; want %d,%d,%d", px, py, pz, x+7, y+9, z)
			}
			if p.Master() != n {
				t.Errorf("other self's master = %v, want the main form", p.Master())
			}
			params := []int32{p.Scratch().Int(npc.IntParam1), p.Scratch().Int(npc.IntParam2), p.Scratch().Int(npc.IntParam3)}
			if params[0] != 1000 || params[1] != w.player.ObjectID() || params[2] != 1 {
				t.Errorf("other self's spawn parameters = %v, want 1000, %d, 1", params, w.player.ObjectID())
			}
			if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
				t.Fatalf("i_ai0 after the call = %d, want 1", got)
			}
			w.draws.set(0)
			w.hit(t, n, 10)
			if got := len(w.liveNPCs(ind2dNinjaFormID)); got != 1 {
				t.Fatalf("other selves after a second hit = %d, want still 1", got)
			}
		})
	}
}

// TestSaintNinjaFlashesAndVanishes ports SaintNinja's timers. Out of a
// fight, a minute after it was created and every five minutes after that,
// it wants its teleport effect on itself (weight 1000000). A form that is
// not the main one leaves the world, for good, five minutes after it was
// created when it is not attacking; the main form stays.
func TestSaintNinjaFlashesAndVanishes(t *testing.T) {
	t.Parallel()
	for _, main := range []bool{true, false} {
		t.Run(fmt.Sprintf("main form %v", main), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2dNinjaTemplate(ind2dNinjaID, main)
			tmpl.CanMove = false
			w := bootIndividual2d(t, raidBoot{}, tmpl)
			n := w.spawn(t, ind2dNinjaID, 1500, 0)
			w.clear(t, n)
			w.srv.Advance(t, 59*time.Second)
			if hasCast(n, ind2dTeleportSkill) {
				t.Fatal("wants its teleport effect before a minute")
			}
			w.srv.AdvanceUntil(t, "the teleport effect at a minute", func() bool { return hasCast(n, ind2dTeleportSkill) })
			w.wantCasts(t, "at a minute", n, map[modelskill.ID]ind1Cast{ind2dTeleportSkill: {n.ObjectID(), 1000000}})
			w.clear(t, n)
			w.srv.Advance(t, 3*time.Minute+58*time.Second)
			if !w.inWorld(n) {
				t.Fatal("left the world before five minutes")
			}
			w.srv.Advance(t, 2*time.Second)
			if w.inWorld(n) == !main {
				t.Fatalf("in the world after five minutes = %v, want %v", w.inWorld(n), main)
			}
			if !main {
				w.srv.Advance(t, 10*time.Minute)
				if w.inWorld(n) || len(w.liveNPCs(ind2dNinjaID)) != 0 {
					t.Fatal("a vanished form came back")
				}
				return
			}
			w.srv.Advance(t, 59*time.Second)
			if hasCast(n, ind2dTeleportSkill) {
				t.Fatal("wants its teleport effect again before six minutes")
			}
			w.srv.AdvanceUntil(t, "the teleport effect at six minutes", func() bool { return hasCast(n, ind2dTeleportSkill) })
		})
	}
}

// TestSaintNinjaJumpsToItsClansAttacker ports SaintNinja's clan-attacked
// hook. A member not attacking and farther than 300 from the character
// hitting its clan stops, jumps to the character, wants its teleport
// effect on it (weight 1000000) and to attack it with weight 30 times the
// hit's damage over its level and seven. A member 300 or nearer stays and
// adds nothing, and the hook calls no parent.
func TestSaintNinjaJumpsToItsClansAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual2d(t, raidBoot{}, ind2dNinjaTemplate(ind2dNinjaFormID, false), ind2dNinjaTemplate(ind2dNinjaTwoID, false))
	victim := w.spawn(t, ind2dNinjaFormID, 0, 300)
	far := w.spawn(t, ind2dNinjaTwoID, 400, 0)
	near := w.spawn(t, ind2dNinjaTwoID, 200, 0)
	for _, h := range []*npc.Hostile{far, near} {
		w.clear(t, h)
	}
	nearX, nearY, _ := near.Position()
	w.srv.ReadQueued(t, w.srv.Client)
	w.hit(t, victim, 80)
	me := w.player.ObjectID()
	fx, fy, _ := far.Position()
	if fx != w.at.X || fy != w.at.Y {
		t.Fatalf("far member at %d,%d after the call, want the character's %d,%d", fx, fy, w.at.X, w.at.Y)
	}
	w.wantCasts(t, "far member", far, map[modelskill.ID]ind1Cast{ind2dTeleportSkill: {me, 1000000}})
	if got := far.AggroHate(w.player); got != 300 {
		t.Fatalf("far member's hate = %v, want 30 x 80/8 = 300", got)
	}
	if x, y, _ := near.Position(); x != nearX || y != nearY {
		t.Errorf("near member moved to %d,%d", x, y)
	}
	w.wantCasts(t, "near member", near, nil)
	if got := near.AggroHate(w.player); got != 0 {
		t.Errorf("near member's hate = %v, want none", got)
	}
}

// ind2dVampireTemplate is a fixture WarriorCorpseVampire child of id
// listing its self buff, debuff, physical special and two DD magics by
// type.
func ind2dVampireTemplate(id int32) *npc.Template {
	return ind2dTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSelfBuff: ind2dBuffSkill, npc.SkillTypeDebuff: ind2dDebuffSkill,
		npc.SkillTypeDDMagic1: ind2dDD1Skill, npc.SkillTypeDDMagic2: ind2dDD2Skill,
	})
}

// TestWarriorCorpseVampireBuffsEveryTenSeconds ports WarriorCorpseVampire's
// created and timer hooks. Created, it clears i_ai0 and wants its self
// buff on itself (weight 1000000); every 10 s after that, while it is not
// attacking, it wants it again.
func TestWarriorCorpseVampireBuffsEveryTenSeconds(t *testing.T) {
	t.Parallel()
	w := bootIndividual2d(t, raidBoot{}, ind2dVampireTemplate(ind2dVampireID))
	n := w.spawn(t, ind2dVampireID, 1500, 0)
	w.wantCasts(t, "created", n, map[modelskill.ID]ind1Cast{ind2dBuffSkill: {n.ObjectID(), 1000000}})
	if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("created i_ai0 = %d, want 0", got)
	}
	for tick := range 2 {
		w.clear(t, n)
		w.srv.Advance(t, 9*time.Second)
		if hasCast(n, ind2dBuffSkill) {
			t.Fatalf("wants its self buff before tick %d", tick+1)
		}
		w.srv.AdvanceUntil(t, fmt.Sprintf("the self buff at tick %d", tick+1), func() bool { return hasCast(n, ind2dBuffSkill) })
	}
}

// TestWarriorCorpseVampireAnswersItsMostHated ports WarriorCorpseVampire's
// attacked hook. Hit by the player it hates most, on a draw under 33 of
// 100 it wants its debuff on the player (weight 1000000) unless the player
// holds it; below half its hit points, with i_ai0 clear, a second draw
// under 33 wants its first DD magic on the player too. A hit by a player
// it does not hate most wants neither.
func TestWarriorCorpseVampireAnswersItsMostHated(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		draws  []int
		share  float64
		held   bool
		debuff bool
		dd     bool
	}{
		{"draws 32, 32 below half", []int{32, 32}, 0.4, false, true, true},
		{"draws 33, 33 below half", []int{33, 33}, 0.4, false, false, false},
		{"above half", []int{0, 0}, 1, false, true, false},
		{"debuff held", []int{0, 0}, 1, true, false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2d(t, raidBoot{}, ind2dVampireTemplate(ind2dVampireID))
			n := w.spawn(t, ind2dVampireID, 400, 0)
			me := w.player.ObjectID()
			w.hit(t, n, 1)
			if row.held {
				w.landOnCharacter(t, ind2dDebuffSkill, n)
			}
			if row.share < 1 {
				w.setHPShare(t, n, row.share)
			}
			w.clear(t, n)
			w.draws.set(99, row.draws...)
			w.hit(t, n, 1)
			want := map[modelskill.ID]ind1Cast{}
			if row.debuff {
				want[ind2dDebuffSkill] = ind1Cast{me, 1000000}
			}
			if row.dd {
				want[ind2dDD1Skill] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after a hit by its most hated", n, want)
		})
	}
}

// TestWarriorCorpseVampireAggressiveStrikesWhatItSeesFromAfar ports
// WarriorCorpseVampireAggressive's see-creature hook. Settled in its
// territory and not attacking, seeing the character farther than 100 on a
// draw under 33 of 100, it wants its physical special on it (weight
// 1000000); from nearer, or young, it does not. Each attacks the
// character it sees (hate 200).
func TestWarriorCorpseVampireAggressiveStrikesWhatItSeesFromAfar(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		dx     int
		settle int
		strike bool
	}{{"150 away", 150, 9, true}, {"50 away", 50, 9, false}, {"young", 150, 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind2dTemplate(ind2dVampAggroID, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypePhysicalSpecial: ind2dSpecialSkill})
			tmpl.CanMove = false
			w := bootIndividual2d(t, raidBoot{}, tmpl)
			n := w.spawn(t, ind2dVampAggroID, 1500, 0)
			w.ticks(t, row.settle)
			w.clear(t, n)
			w.moveCharacter(t, n, -row.dx, 0)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			if row.strike {
				want[ind2dSpecialSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after it saw the character", n, want)
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
		})
	}
}

// TestWarriorCorpseVampireMobHateBlastsNearAttackers ports
// WarriorCorpseVampireMobHate's clan-attacked hook, every draw 0. A
// settled member already attacking wants its second DD magic on a player
// hitting its clan closer than 100 to it (weight 1000000); a member not
// attacking does not.
func TestWarriorCorpseVampireMobHateBlastsNearAttackers(t *testing.T) {
	t.Parallel()
	w := bootIndividual2d(t, raidBoot{}, ind2dVampireTemplate(ind2dMobHateID), ind2dVampireTemplate(ind2dMobHateTwoID))
	victim := w.spawn(t, ind2dMobHateTwoID, 0, 300)
	attacking := w.spawn(t, ind2dMobHateID, 50, 0)
	idle := w.spawn(t, ind2dMobHateID, -50, 0)
	// The hit lands before any member has settled, so none answers it; the
	// created self buff no longer outweighs the attack.
	w.clear(t, attacking)
	w.hit(t, attacking, 10)
	w.ticks(t, 8)
	if got := attacking.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: the hit member's intention = %v, want attack", got)
	}
	if got := idle.AI().CurrentIntention(); got == actorai.IntentionAttack {
		t.Fatal("setup: the idle member attacks")
	}
	for _, h := range []*npc.Hostile{attacking, idle} {
		if h.LifeTime() <= 7 {
			t.Fatalf("member life time = %d, want past 7", h.LifeTime())
		}
	}
	w.clear(t, idle)
	w.hit(t, victim, 1)
	if c, ok := ind1Casts(attacking)[ind2dDD2Skill]; !ok || c.Weight != 1000000 || c.FinalTarget == nil || c.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("attacking member's second DD magic desire = %+v (%v), want weight 1000000 on the character", c, ok)
	}
	if hasCast(idle, ind2dDD2Skill) {
		t.Fatal("the idle member wants its second DD magic")
	}
}
