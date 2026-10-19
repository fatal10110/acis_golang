package ai

import (
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

// The NPC ids of the fifth individual behavior batch's second PR the
// tests spawn.
const (
	ind5bBombID          = int32(21666) // WarriorBomb
	ind5bBombAggrID      = int32(18333) // WarriorBombAggressive
	ind5bTeleportBombID  = int32(22133) // WarriorTeleportBombAggressive
	ind5bEnchantID       = int32(22009) // WarriorCastEnchant1Of4
	ind5bEnchantAggrID   = int32(21426) // WarriorCastEnchant1Of4Aggressive
	ind5bEnchantAggrTwo  = int32(20554) // WarriorCastEnchant1Of4Aggressive
	ind5bGhostID         = int32(21555) // WarriorCorpseGhostPhysicalSpecial
	ind5bGhostAggrID     = int32(21571) // WarriorCorpseGhostPhysicalSpecialAggressive
	ind5bRoyalRushBombID = int32(18149) // RoyalRushBomb
	ind5bRoyalRushTwoID  = int32(18195) // RoyalRushBomb
)

// The skills the batch's fixture NPCs list by type.
const (
	ind5bBlast      = modelskill.ID(9350) // SELF_RANGE_DD_MAGIC
	ind5bBlast2     = modelskill.ID(9351) // SELF_RANGE_DD_MAGIC2
	ind5bBlast3     = modelskill.ID(9352) // SELF_RANGE_DD_MAGIC3
	ind5bTeleportFx = modelskill.ID(9353) // TELEPORT_EFFECT
	ind5bBuff1      = modelskill.ID(9354) // SELF_BUFF1
	ind5bBuff2      = modelskill.ID(9355) // SELF_BUFF2
	ind5bBuff3      = modelskill.ID(9356) // SELF_BUFF3
	ind5bBuff4      = modelskill.ID(9357) // SELF_BUFF4
	ind5bGhostBuff  = modelskill.ID(9358) // SELF_BUFF
	ind5bStrike     = modelskill.ID(9359) // PHYSICAL_SPECIAL
)

// ind5bGhostBuffStack is the stack type of the corpse ghost's self buff.
const ind5bGhostBuffStack = "ind5b_ghost_buff"

// ind5bBatch are the classes of the PR, in scripts.xml order.
var ind5bBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBomb", warrior.Bomb},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBombAggressive", warrior.BombAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorTeleportBombAggressive", warrior.TeleportBombAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchant1Of4", warrior.CastEnchant1Of4},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchant1Of4Aggressive", warrior.CastEnchant1Of4Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseGhostPhysicalSpecial", warrior.CorpseGhostPhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseGhostPhysicalSpecialAggressive", warrior.CorpseGhostPhysicalSpecialAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushBomb", warrior.RoyalRushBomb},
}

// ind5bGhostBuffDef is the corpse ghost's self buff: an effect of its own
// stack type lasting a minute.
func ind5bGhostBuffDef() modelskill.Definition {
	d := ind1Skill(ind5bGhostBuff, 0, true)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: ind5bGhostBuffStack, StackOrder: 1}}
	return d
}

// ind5bSkills are the skills the batch's fixture NPCs cast: every self
// skill a self buff, the physical special a one-target strike of reach
// 600.
func ind5bSkills() *modelskill.Table {
	defs := []modelskill.Definition{ind5bGhostBuffDef(), ind1Skill(ind5bStrike, 600, false)}
	for _, id := range []modelskill.ID{ind5bBlast, ind5bBlast2, ind5bBlast3, ind5bTeleportFx, ind5bBuff1, ind5bBuff2, ind5bBuff3, ind5bBuff4} {
		defs = append(defs, ind1Skill(id, 0, true))
	}
	return modelskill.NewTable(defs)
}

// ind5bTemplate is a fixture monster of id listing every skill of the
// batch by its type, with ai as its AI parameters.
func ind5bTemplate(id int32, ai npc.AIParams) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeSelfRangeDDMagic:  {ID: ind5bBlast, Level: 1},
		npc.SkillTypeSelfRangeDDMagic2: {ID: ind5bBlast2, Level: 1},
		npc.SkillTypeSelfRangeDDMagic3: {ID: ind5bBlast3, Level: 1},
		npc.SkillTypeTeleportEffect:    {ID: ind5bTeleportFx, Level: 1},
		npc.SkillTypeSelfBuff1:         {ID: ind5bBuff1, Level: 1},
		npc.SkillTypeSelfBuff2:         {ID: ind5bBuff2, Level: 1},
		npc.SkillTypeSelfBuff3:         {ID: ind5bBuff3, Level: 1},
		npc.SkillTypeSelfBuff4:         {ID: ind5bBuff4, Level: 1},
		npc.SkillTypeSelfBuff:          {ID: ind5bGhostBuff, Level: 1},
		npc.SkillTypePhysicalSpecial:   {ID: ind5bStrike, Level: 1},
	}
	return tmpl
}

// ind5bClan puts tmpl in the scenario clan, of range 500.
func ind5bClan(tmpl *npc.Template) *npc.Template {
	tmpl.Clans, tmpl.ClanRange = []string{"ind5b_clan"}, 500
	return tmpl
}

// bootIndividual5b boots the PR's classes after the Warrior chain beside
// templates, every NPC casting the batch's skills, every draw 0 until a
// scenario queues others, with a director script to land effects through.
func bootIndividual5b(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind5bBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind5bDirector"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(ind5bSkills()),
		gameservertest.WithScriptSkills(ind5bGhostBuffDef()),
	}}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind5bRunUntil runs AI cycles half a second apart until cond holds,
// failing after 40 of them, and returns the frames they sent.
func ind5bRunUntil(t *testing.T, w *ind1World, what string, cond func() bool) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 40 {
		if cond() {
			return frames
		}
		frames = append(frames, w.ticks(t, 1)...)
		w.srv.Advance(t, 500*time.Millisecond)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	}
	if !cond() {
		t.Fatalf("%s not observed within 40 AI cycles", what)
	}
	return frames
}

// ind5bWantCast fails unless h wants to cast skill id on the creature of
// object id target with weight 1000000, or 934000 for a desire raised
// during the AI cycle just run, which decays it once.
func ind5bWantCast(t *testing.T, when string, h *npc.Hostile, id modelskill.ID, target int32) {
	t.Helper()
	d, ok := ind1Casts(h)[id]
	if !ok || (d.Weight != 1000000 && d.Weight != 934000) || d.FinalTarget == nil || d.FinalTarget.ObjectID() != target {
		t.Fatalf("%s: cast desire of skill %d = %+v (%v), want weight 1000000 on %d", when, id, d, ok, target)
	}
}

// ind5bNoCast fails when h wants to cast skill id.
func ind5bNoCast(t *testing.T, when string, h *npc.Hostile, id modelskill.ID) {
	t.Helper()
	if d, ok := ind1Casts(h)[id]; ok {
		t.Fatalf("%s: unexpected cast desire %+v", when, d)
	}
}

// TestWarriorBombBlastsOnACloseHitAndDies ports WarriorBomb's attacked and
// skill-finished hooks. A hit from 300 away wants no blast; a hit from 100
// away wants its self range magic on itself, weight 1000000, and the
// blast's landing kills it (MagicSkillUse, then Die).
func TestWarriorBombBlastsOnACloseHitAndDies(t *testing.T) {
	t.Parallel()
	w := bootIndividual5b(t, ind5bTemplate(ind5bBombID, nil))
	far := w.spawn(t, ind5bBombID, 300, 0)
	near := w.spawn(t, ind5bBombID, 0, 100)

	w.hit(t, far, 10)
	ind5bNoCast(t, "a hit from 300 away", far, ind5bBlast)
	w.hit(t, near, 10)
	ind5bWantCast(t, "a hit from 100 away", near, ind5bBlast, near.ObjectID())

	frames := ind5bRunUntil(t, w, "the bomb's death", near.Dead)
	if got := castsOf(frames, near, ind5bBlast); len(got) != 1 || got[0] != near.ObjectID() {
		t.Fatalf("blasts on itself = %v, want one", got)
	}
	if len(framesOf(frames, serverpackets.OpcodeDie, near.ObjectID())) != 1 {
		t.Fatal("the bomb did not die after its blast")
	}
	if far.Dead() {
		t.Fatal("the bomb hit from afar died")
	}
}

// TestBatch5bAggressivesAttackOnSight ports the see-creature hook of
// WarriorBombAggressive, WarriorCastEnchant1Of4Aggressive and
// WarriorCorpseGhostPhysicalSpecialAggressive: each wants to attack the
// character it sees with weight 200.
func TestBatch5bAggressivesAttackOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual5b(t, warriorTemplate(ind5bBombAggrID, nil), warriorTemplate(ind5bEnchantAggrID, nil), warriorTemplate(ind5bGhostAggrID, nil))
	hs := []*npc.Hostile{w.spawn(t, ind5bBombAggrID, 150, 0), w.spawn(t, ind5bEnchantAggrID, 0, 150), w.spawn(t, ind5bGhostAggrID, -150, 0)}
	w.ticks(t, 2)
	for _, h := range hs {
		if got := h.AggroHate(w.player); got != 200 {
			t.Errorf("npc %d hate of the character it sees = %v, want 200", h.NpcID(), got)
		}
	}
}

// TestTeleportBombJumpsOntoTheCharacterThenBlows ports
// WarriorTeleportBombAggressive's see-creature, skill-finished and timer
// hooks. Seeing the character it jumps onto it and wants its teleport
// effect on itself; it attacks no one. The effect's landing arms the 5 s
// timer "1001", whose draw picks its blast: 32 the first, 33 and 65 the
// second, 66 the third. The blast's landing kills it.
func TestTeleportBombJumpsOntoTheCharacterThenBlows(t *testing.T) {
	t.Parallel()
	w := bootIndividual5b(t, ind5bTemplate(ind5bTeleportBombID, nil))
	for i, row := range []struct {
		draw  int
		blast modelskill.ID
	}{{32, ind5bBlast}, {33, ind5bBlast2}, {65, ind5bBlast2}, {66, ind5bBlast3}} {
		w.draws.set(row.draw)
		h := w.spawn(t, ind5bTeleportBombID, 300-200*i, 300)

		frames := w.ticks(t, 2)
		x, y, _ := h.Position()
		if dx, dy := x-w.at.X, y-w.at.Y; dx*dx+dy*dy > 20*20 {
			t.Fatalf("draw %d: bomb at %d,%d after seeing the character, want on it at %d,%d", row.draw, x, y, w.at.X, w.at.Y)
		}
		if got := ind1Slot(h, script.SlotIntAI1); got != 1 {
			t.Fatalf("draw %d: IntAI1 after the jump = %d, want 1", row.draw, got)
		}
		if got := h.AggroHate(w.player); got != 0 {
			t.Fatalf("draw %d: bomb hate of the character it jumped onto = %v, want none", row.draw, got)
		}
		frames = append(frames, ind5bRunUntil(t, w, "the teleport effect's end", func() bool {
			return len(castsOf(frames, h, ind5bTeleportFx)) == 1 && len(ind1Casts(h)) == 0 && h.AI().CurrentIntention() != actorai.IntentionCast
		})...)
		w.srv.Advance(t, 4500*time.Millisecond)
		if h.Dead() || len(ind1Casts(h)) != 0 {
			t.Fatalf("draw %d: the bomb wants %v, dead %v, before its 5 s timer", row.draw, ind1Casts(h), h.Dead())
		}
		frames = append(frames, ind5bRunUntil(t, w, "the bomb's death", h.Dead)...)
		for _, id := range []modelskill.ID{ind5bBlast, ind5bBlast2, ind5bBlast3} {
			want := 0
			if id == row.blast {
				want = 1
			}
			if got := len(castsOf(frames, h, id)); got != want {
				t.Fatalf("draw %d: casts of blast %d = %d, want %d", row.draw, id, got, want)
			}
		}
	}
}

// TestTeleportBombJumpsOntoItsClansAttacker ports
// WarriorTeleportBombAggressive's clan-attacked hook: a hit on a clan
// member by the character makes the bomb jump onto the character and want
// its teleport effect on itself, once.
func TestTeleportBombJumpsOntoItsClansAttacker(t *testing.T) {
	t.Parallel()
	tmpl := ind5bClan(ind5bTemplate(ind5bTeleportBombID, nil))
	w := bootIndividual5b(t, tmpl, ind5bClan(warriorTemplate(warriorID, nil)))
	member := w.spawn(t, warriorID, 2500, 0)
	h := w.spawn(t, ind5bTeleportBombID, 2600, 0)

	w.hit(t, member, 10)
	x, y, _ := h.Position()
	if dx, dy := x-w.at.X, y-w.at.Y; dx*dx+dy*dy > 20*20 {
		t.Fatalf("bomb at %d,%d after its clan was hit, want on the character at %d,%d", x, y, w.at.X, w.at.Y)
	}
	ind5bWantCast(t, "after its clan was hit", h, ind5bTeleportFx, h.ObjectID())
	if got := ind1Slot(h, script.SlotIntAI1); got != 1 {
		t.Fatalf("IntAI1 after the jump = %d, want 1", got)
	}
}

// TestTeleportBombJumpsBackWhenIdle ports WarriorTeleportBombAggressive's
// created and no-desire hooks on a bomb too far to see the character. Moved
// off its spawn point, it stays there with nothing to do while fewer than
// 10 s passed since its creation; once 10 s passed (its creation tick
// moved back by 10, the game tick being the wall-clock uptime), it jumps
// back to the point its spawn gives, wants its teleport effect on itself
// and counts the 10 s again from the jump.
func TestTeleportBombJumpsBackWhenIdle(t *testing.T) {
	t.Parallel()
	w := bootIndividual5b(t, ind5bTemplate(ind5bTeleportBombID, nil))
	h := w.spawn(t, ind5bTeleportBombID, 4000, 0)
	hx, hy, _ := h.Position()
	w.teleport(t, h, 300, 0)

	w.ticks(t, 2)
	if x, _, _ := h.Position(); x != hx+300 {
		t.Fatalf("bomb at x %d right after its creation, want still at %d", x, hx+300)
	}
	ind5bNoCast(t, "right after its creation", h, ind5bTeleportFx)
	created := ind1Slot(h, script.SlotIntAI0)
	w.onQueue(t, h, func() { script.NPCOf(h).SetInt(script.SlotIntAI0, created-10) })
	w.ticks(t, 1)
	if x, y, _ := h.Position(); x != hx || y != hy {
		t.Fatalf("bomb at %d,%d 10 s after its creation, want back on its spawn point %d,%d", x, y, hx, hy)
	}
	ind5bWantCast(t, "after the jump back", h, ind5bTeleportFx, h.ObjectID())
	if got := ind1Slot(h, script.SlotIntAI0); got < created {
		t.Fatalf("IntAI0 after the jump back = %d, want the tick of the jump, at least %d", got, created)
	}
}

// TestCastEnchant1Of4BuffsOnTheFirstHit ports WarriorCastEnchant1Of4's
// created and attacked hooks. Its creation draws which of its four self
// buffs it keeps: 2, its third. The first hit by the character, on a draw
// under 33 and above half of its hit points, wants that buff on itself; a
// later hit does not. A first hit on a draw of 33, or one landing at half
// of its hit points or less (the hook reads them before the hit), spends
// its chance; a hit by an NPC does not.
func TestCastEnchant1Of4BuffsOnTheFirstHit(t *testing.T) {
	t.Parallel()
	tmpl := ind5bTemplate(ind5bEnchantID, nil)
	tmpl.CanMove = false
	w := bootIndividual5b(t, tmpl)

	w.draws.set(2)
	n := w.spawn(t, ind5bEnchantID, 300, 0)
	w.draws.set(0)
	w.hit(t, n, 10)
	ind5bWantCast(t, "first hit", n, ind5bBuff3, n.ObjectID())
	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.hit(t, n, 10)
	ind5bNoCast(t, "second hit", n, ind5bBuff3)

	w.draws.set(33)
	missed := w.spawn(t, ind5bEnchantID, 0, 300)
	w.hit(t, missed, 10)
	w.draws.set(0)
	w.hit(t, missed, 10)
	for _, id := range []modelskill.ID{ind5bBuff1, ind5bBuff2, ind5bBuff3, ind5bBuff4} {
		ind5bNoCast(t, "after a missed first draw", missed, id)
	}

	wounded := w.spawn(t, ind5bEnchantID, -300, 0)
	other := w.spawn(t, ind5bEnchantID, -300, 100)
	w.onQueue(t, wounded, func() { wounded.TakeDamage(230, other) })
	ind5bNoCast(t, "after a hit by an NPC", wounded, ind5bBuff1)
	if got := ind1Slot(wounded, script.SlotIntAI1); got != 0 {
		t.Fatalf("IntAI1 after a hit by an NPC = %d, want 0", got)
	}
	w.hit(t, wounded, 10)
	w.hit(t, wounded, 10)
	ind5bNoCast(t, "after a first hit by the character at half of its hit points or less", wounded, ind5bBuff1)
}

// TestCastEnchant1Of4AggressiveBuffsOnceSettled ports
// WarriorCastEnchant1Of4Aggressive's see-creature hook, every draw 0. One
// that sees the character before its eighth AI cycle attacks it and keeps
// its chance at its buff. One that first sees it once settled in its
// territory also wants its first self buff on itself, weight 1000000, and
// seeing the character again after it left does not want it again.
func TestCastEnchant1Of4AggressiveBuffsOnceSettled(t *testing.T) {
	t.Parallel()
	tmpl := ind5bTemplate(ind5bEnchantAggrID, nil)
	tmpl.CanMove = false
	two := ind5bTemplate(ind5bEnchantAggrTwo, nil)
	two.CanMove = false
	w := bootIndividual5b(t, tmpl, two)
	early := w.spawn(t, ind5bEnchantAggrTwo, 150, 0)
	w.ticks(t, 1)
	if got := early.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character seen before settling = %v, want 200", got)
	}
	ind5bNoCast(t, "seeing the character before settling", early, ind5bBuff1)
	if got := ind1Slot(early, script.SlotIntAI1); got != 0 {
		t.Fatalf("IntAI1 after seeing the character before settling = %d, want 0", got)
	}

	n := w.spawn(t, ind5bEnchantAggrID, 1500, 0)
	w.ticks(t, 8)
	if n.LifeTime() <= 7 {
		t.Fatalf("life time = %d, want past 7", n.LifeTime())
	}
	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	ind5bWantCast(t, "seeing the character once settled", n, ind5bBuff1, n.ObjectID())
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character seen once settled = %v, want it attacked", got)
	}

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.moveCharacter(t, n, 800, 0)
	w.ticks(t, 1)
	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	ind5bNoCast(t, "seeing the character again", n, ind5bBuff1)
}

// ind5bApplyGhostBuff lands the ghost's self buff's effect on h, and
// waits for it.
func ind5bApplyGhostBuff(t *testing.T, w *ind1World, h *npc.Hostile) {
	t.Helper()
	me := w.player.ObjectID()
	ref := modelskill.Ref{ID: ind5bGhostBuff, Level: 1}
	if !w.srv.RunScript(t, me, "Ind5bDirector", func(s *script.Script, _ *script.Player) {
		s.ApplyEffects(ref, script.NPCOf(h), script.NPCOf(h))
	}) {
		t.Fatal("the self buff landing panicked")
	}
	w.srv.AdvanceUntil(t, "the self buff landed", func() bool {
		var level int32
		w.srv.RunScript(t, me, "Ind5bDirector", func(s *script.Script, _ *script.Player) {
			level = s.AbnormalLevel(script.NPCOf(h), ref)
		})
		return level > 0
	})
}

// TestCorpseGhostBuffsItselfOnItsTimer ports
// WarriorCorpseGhostPhysicalSpecial's created and timer hooks. It wants
// its self buff on itself as it spawns, weight 1000000. Every 10 s, while
// it does not attack, its timer "2001" wants the buff again when it holds
// no effect of the buff's stack, and not while it holds one.
func TestCorpseGhostBuffsItselfOnItsTimer(t *testing.T) {
	t.Parallel()
	tmpl := ind5bTemplate(ind5bGhostID, nil)
	tmpl.CanMove = false
	w := bootIndividual5b(t, tmpl)
	n := w.spawn(t, ind5bGhostID, 300, 0)
	ind5bWantCast(t, "after it spawned", n, ind5bGhostBuff, n.ObjectID())

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.srv.Advance(t, 10*time.Second)
	ind5bWantCast(t, "at its first timer", n, ind5bGhostBuff, n.ObjectID())

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	ind5bApplyGhostBuff(t, w, n)
	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.srv.Advance(t, 10*time.Second)
	ind5bNoCast(t, "at a timer while it holds the buff", n, ind5bGhostBuff)
}

// TestCorpseGhostStrikesItsMostHatedAndItsClansAttacker ports
// WarriorCorpseGhostPhysicalSpecial's attacked and clan-attacked hooks,
// every draw 0. The first hit makes the character its most hated, so only
// the second wants its physical special on the character. A settled clan
// member that is not attacking wants it on the character at the first hit
// on its clan.
func TestCorpseGhostStrikesItsMostHatedAndItsClansAttacker(t *testing.T) {
	t.Parallel()
	ghost := func(id int32) *npc.Template {
		tmpl := ind5bClan(ind5bTemplate(id, nil))
		tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypePhysicalSpecial: {ID: ind5bStrike, Level: 1}}
		tmpl.CanMove = false
		return tmpl
	}
	w := bootIndividual5b(t, ghost(ind5bGhostID), ghost(ind5bGhostAggrID))
	victim := w.spawn(t, ind5bGhostID, 300, 0)
	member := w.spawn(t, ind5bGhostID, 300, 100)
	w.ticks(t, 8)
	if member.LifeTime() <= 7 {
		t.Fatalf("clan member life time = %d, want past 7", member.LifeTime())
	}

	w.hit(t, victim, 10)
	ind5bNoCast(t, "the hit that made the character its most hated", victim, ind5bStrike)
	ind5bWantCast(t, "a settled clan member", member, ind5bStrike, w.player.ObjectID())
	w.hit(t, victim, 10)
	ind5bWantCast(t, "a hit by its most hated", victim, ind5bStrike, w.player.ObjectID())
}

// TestCorpseGhostAggressiveStrikesWhatItFightsOnSight ports
// WarriorCorpseGhostPhysicalSpecialAggressive's see-creature hook, every
// draw 0. Seeing the character before it attacks anyone, it only attacks
// it. Once settled, attacking the character it hates most, it sees that
// one again after the character left its sight and came back, and wants
// its physical special on it, weight 1000000.
func TestCorpseGhostAggressiveStrikesWhatItFightsOnSight(t *testing.T) {
	t.Parallel()
	tmpl := ind5bTemplate(ind5bGhostAggrID, nil)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypePhysicalSpecial: {ID: ind5bStrike, Level: 1}}
	w := bootIndividual5b(t, tmpl)
	n := w.spawn(t, ind5bGhostAggrID, 150, 0)
	w.ticks(t, 1)
	ind5bNoCast(t, "seeing the character while not attacking", n, ind5bStrike)
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character seen = %v, want 200", got)
	}

	w.ticks(t, 8)
	if got := n.AI().CurrentIntention(); got != actorai.IntentionAttack || n.LifeTime() <= 7 {
		t.Fatalf("setup: intention %v at life time %d, want attack past 7", got, n.LifeTime())
	}
	w.moveCharacter(t, n, 800, 0)
	w.ticks(t, 1)
	ind5bNoCast(t, "the character out of its sight", n, ind5bStrike)
	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	ind5bWantCast(t, "seeing the character it fights", n, ind5bStrike, w.player.ObjectID())
}

// TestRoyalRushBombBlastsBelowItsHPRateAndLeaves ports RoyalRushBomb's
// attacked and skill-finished hooks, every draw 0, on bombs of 440 hit
// points; the hook reads them before the hit. With the default
// DDMagicUseHpRate of 30, hits landing at 100 and 77 percent want no blast
// and one landing at 29.5 percent wants its self range magic on itself;
// with DDMagicUseHpRate 95, the hit landing at 77 percent does. Its
// blast's end takes it out of the world with no death.
func TestRoyalRushBombBlastsBelowItsHPRateAndLeaves(t *testing.T) {
	t.Parallel()
	w := bootIndividual5b(t, ind5bTemplate(ind5bRoyalRushBombID, nil), ind5bTemplate(ind5bRoyalRushTwoID, npc.AIParams{"DDMagicUseHpRate": "95"}))
	n := w.spawn(t, ind5bRoyalRushBombID, 300, 0)
	w.hit(t, n, 100)
	w.hit(t, n, 210)
	ind5bNoCast(t, "hits at 100 and 77 percent", n, ind5bBlast)
	w.hit(t, n, 10)
	ind5bWantCast(t, "a hit at 29.5 percent", n, ind5bBlast, n.ObjectID())

	tuned := w.spawn(t, ind5bRoyalRushTwoID, 0, 300)
	w.hit(t, tuned, 100)
	ind5bNoCast(t, "a hit at 100 percent under a rate of 95", tuned, ind5bBlast)
	w.hit(t, tuned, 10)
	ind5bWantCast(t, "a hit at 77 percent under a rate of 95", tuned, ind5bBlast, tuned.ObjectID())

	frames := ind5bRunUntil(t, w, "the bomb leaving the world", func() bool { return !w.inWorld(n) })
	if got := castsOf(frames, n, ind5bBlast); len(got) != 1 {
		t.Fatalf("blasts = %v, want one", got)
	}
	if len(framesOf(frames, serverpackets.OpcodeDie, n.ObjectID())) != 0 || len(framesOf(frames, serverpackets.OpcodeDeleteObject, n.ObjectID())) != 1 {
		t.Fatal("the bomb did not leave the world without a death")
	}
}
