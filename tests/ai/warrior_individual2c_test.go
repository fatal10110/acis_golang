package ai

import (
	"fmt"
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
// third PR spawn (#187 PR 3), each a fixture monster of level 1 unless a
// scenario says otherwise.
const (
	ind2cEnchantID     = int32(20681) // WarriorCastingEnchant
	ind2cClanID        = int32(21616) // WarriorCastingEnchantClan
	ind2cClanAggroID   = int32(21237) // WarriorCastingEnchantClanAggressive
	ind2cArcherID      = int32(35413) // Agit02DoomArcherAgit
	ind2cArcherPartyID = int32(35646) // Agit02DoomArcherAgitParty
	ind2cRaikelID      = int32(29015) // OrfenPrivateRaikel
	ind2cWarriorID     = int32(35415) // Agit02DoomWarriorAgit
	ind2cWarriorTwoID  = int32(35636) // Agit02DoomWarriorAgit
	ind2cQueenID       = int32(21084) // SpecialBloodyQueen
	ind2cBuffSkill     = modelskill.ID(9505)
)

// ind2cBatch are the classes the PR ports, in scripts.xml order.
var ind2cBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.SpecialBloodyQueen", warrior.SpecialBloodyQueen},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgit", warrior.Agit02DoomArcherAgit},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgitParty", warrior.Agit02DoomArcherAgitParty},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomWarriorAgit", warrior.Agit02DoomWarriorAgit},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.OrfenPrivateRaikel", warrior.OrfenPrivateRaikel},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchant", warrior.CastingEnchant},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantClan", warrior.CastingEnchantClan},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantClanAggressive", warrior.CastingEnchantClanAggressive},
}

// bootIndividual2c boots the PR's classes after the party Warriors beside
// templates, with extra's options added last and its step run before the
// character enters the world, every draw 0 until a scenario queues others,
// with the director script the scenarios land effects through.
func bootIndividual2c(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := partyScripts()
	for _, c := range ind2cBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind2cDirector"
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
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind2cTemplate is a fixture batch NPC of id listing only its buff by
// type, in the clan the scenario's NPCs share.
func ind2cTemplate(id int32) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypeBuff: {ID: ind2cBuffSkill, Level: 1}}
	tmpl.Clans, tmpl.ClanRange = []string{"doom"}, 500
	return tmpl
}

// ind2cQueenTemplate is a fixture SpecialBloodyQueen listing only its DD
// magic by type, so its parent's debuff never queues, unable to move.
func ind2cQueenTemplate() *npc.Template {
	tmpl := warriorTemplate(ind2cQueenID, nil)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypeDDMagic: {ID: ind2DDMagicSkill, Level: 1}}
	tmpl.Clans, tmpl.ClanRange = []string{"doom"}, 500
	tmpl.CanMove = false
	return tmpl
}

// ind2cBuff is the fixture buff, reaching 2000 and holding an effect of
// its own stack for a minute.
func ind2cBuff() modelskill.Definition {
	d := ind1Skill(ind2cBuffSkill, 2000, false)
	d.Offensive, d.Magic, d.SkillType, d.Power = false, false, "BUFF", 0
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "ind2c_buff", StackOrder: 1}}
	return d
}

// ind2cSkills are the batch NPCs' skills: the buff and the DD magic.
func ind2cSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{ind2cBuff(), ind1Skill(ind2DDMagicSkill, 2000, false)})
}

// ind2cLandBuff lands the fixture buff's effect, cast by caster, on on,
// and waits for it.
func ind2cLandBuff(t *testing.T, w *ind1World, caster, on *npc.Hostile) {
	t.Helper()
	ref := modelskill.Ref{ID: ind2cBuffSkill, Level: 1}
	me := w.player.ObjectID()
	if !w.srv.RunScript(t, me, "Ind2cDirector", func(s *script.Script, _ *script.Player) {
		s.ApplyEffects(ref, script.NPCOf(caster), script.NPCOf(on))
	}) {
		t.Fatal("the buff landing panicked")
	}
	w.srv.AdvanceUntil(t, "the buff landed", func() bool {
		var level int32
		w.srv.RunScript(t, me, "Ind2cDirector", func(s *script.Script, _ *script.Player) {
			level = s.AbnormalLevel(script.NPCOf(on), ref)
		})
		return level > 0
	})
}

// ind2cHate is the hate a clan hall siege NPC of maxHP adds for amount,
// a hit's damage or a skill's aggro points: ratio for each twentieth of
// its maximum hit points.
func ind2cHate(amount, maxHP int, ratio float64) float64 {
	return float64(amount) / float64(maxHP) / 0.05 * ratio
}

// TestWarriorCastingEnchantBuffsItselfWhenFirstHit ports
// WarriorCastingEnchant's created and attacked hooks. It spawns with i_ai0
// and i_ai1 clear. A hit by the character, on a draw under 33 of 100 and
// with more than half its hit points left after it, wants its buff on
// itself (weight 1000000) and spends it (i_ai1): a later hit wants
// nothing. A draw of 33, or half its hit points, wants nothing and keeps
// the buff.
func TestWarriorCastingEnchantBuffsItselfWhenFirstHit(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		draw  int
		share float64
		buff  bool
	}{
		{"draw 32", 32, 1, true},
		{"draw 33", 33, 1, false},
		{"half its hit points", 0, 0.5, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind2cTemplate(ind2cEnchantID)
			tmpl.CanMove = false
			w := bootIndividual2c(t, raidBoot{}, tmpl)
			n := w.spawn(t, ind2cEnchantID, 300, 0)
			w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
			if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 0 || a1 != 0 {
				t.Fatalf("created i_ai0, i_ai1 = %d, %d, want 0, 0", a0, a1)
			}
			if row.share < 1 {
				w.setHPShare(t, n, row.share)
			}
			w.draws.set(0, row.draw)
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			spent := int32(0)
			if row.buff {
				want[ind2cBuffSkill] = ind1Cast{n.ObjectID(), 1000000}
				spent = 1
			}
			w.wantCasts(t, "after the first hit", n, want)
			if got := ind1Slot(n, script.SlotIntAI1); got != spent {
				t.Fatalf("i_ai1 after the first hit = %d, want %d", got, spent)
			}
			if !row.buff {
				return
			}
			w.onQueue(t, n, func() { n.AI().Desires().RemoveKind(actorai.IntentionCast) })
			w.hit(t, n, 10)
			w.wantCasts(t, "after a second hit", n, map[modelskill.ID]ind1Cast{})
		})
	}
}

// TestWarriorCastingEnchantClanBuffsAHitMember ports
// WarriorCastingEnchantClan's clan-attacked hook. A hit by the character
// on a member of its clan makes the settled NPC, not attacking and above
// half its hit points, on a draw under 50 of 100, want its buff on the hit
// member (weight 1000000), unless the buff already holds on the member.
// Every call spends its buff (i_ai1), cast or not, and a young NPC never
// buffs.
func TestWarriorCastingEnchantClanBuffsAHitMember(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		draw   int
		settle int
		held   bool
		buff   bool
	}{
		{"draw 49", 49, 8, false, true},
		{"draw 50", 50, 8, false, false},
		{"buff held on the member", 0, 8, true, false},
		{"young", 0, 0, false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			member := warriorTemplate(unboundID, nil)
			member.Clans, member.ClanRange = []string{"doom"}, 500
			w := bootIndividual2c(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(ind2cBuff())}},
				ind2cTemplate(ind2cClanID), member)
			n := w.spawn(t, ind2cClanID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
			victim := w.spawn(t, unboundID, 1500, 100)
			if row.held {
				ind2cLandBuff(t, w, n, victim)
			}
			w.ticks(t, row.settle)
			w.draws.set(0, row.draw)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.buff {
				want[ind2cBuffSkill] = ind1Cast{victim.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit on its clan", n, want)
			if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
				t.Fatalf("i_ai1 after the clan call = %d, want 1", got)
			}
		})
	}
}

// TestWarriorCastingEnchantClanAggressiveBuffsOnSight ports
// WarriorCastingEnchantClanAggressive's see-creature hook. Settled in its
// territory, seeing the character on a draw under 33 of 100, it wants its
// buff on itself (weight 1000000); either way the sight spends the buff
// (i_ai1). A young NPC neither buffs nor spends it. Each one attacks the
// character it sees (hate 200).
func TestWarriorCastingEnchantClanAggressiveBuffsOnSight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		draw   int
		settle int
		buff   bool
		spent  int32
	}{
		{"draw 32", 32, 9, true, 1},
		{"draw 33", 33, 9, false, 1},
		{"young", 0, 0, false, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2c(t, raidBoot{}, ind2cTemplate(ind2cClanAggroID))
			n := w.spawn(t, ind2cClanAggroID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
			w.ticks(t, row.settle)
			if row.settle > 0 && (n.LifeTime() <= 7 || !script.NPCOf(n).InMyTerritory()) {
				t.Fatalf("setup: life time %d, in territory %v, want past 7 and in it", n.LifeTime(), script.NPCOf(n).InMyTerritory())
			}
			w.draws.set(0, row.draw)
			w.moveCharacter(t, n, -50, 0)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			if row.buff {
				want[ind2cBuffSkill] = ind1Cast{n.ObjectID(), 1000000}
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

// TestAgit02DoomArcherAgitHoldsItsGround ports Agit02DoomArcherAgit's
// created, no-desire, see-creature and attacked hooks, every draw 0. It
// spawns with flag 0 and, with no desire left, does nothing. It wants to
// attack the character it sees from where it stands (weight 200). The
// character's first hit wants its buff on itself (weight 1000000), spends
// it, and adds hate by the damage, held in place: 100 for each twentieth
// of its maximum hit points, twice, as the hit's clan call reaches the
// archer itself, which shares its own flag.
func TestAgit02DoomArcherAgitHoldsItsGround(t *testing.T) {
	t.Parallel()
	w := bootIndividual2c(t, raidBoot{}, ind2cTemplate(ind2cArcherID))
	n := w.spawn(t, ind2cArcherID, 1500, 0)
	w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
	me := w.player.ObjectID()
	if got := ind1Slot(n, script.SlotFlag); got != 0 {
		t.Fatalf("created flag = %d, want 0", got)
	}
	w.ticks(t, 3)
	if d, ok := desire(n, actorai.IntentionWander); ok || n.AI().CurrentIntention() == actorai.IntentionWander {
		t.Fatalf("the archer wanders: desire %+v (%v), intention %v", d, ok, n.AI().CurrentIntention())
	}

	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}
	ind2bWantHold(t, "attack on sight", n, actorai.IntentionAttack, me, 200)

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.hit(t, n, 10)
	w.wantCasts(t, "after the hit", n, map[modelskill.ID]ind1Cast{ind2cBuffSkill: {n.ObjectID(), 1000000}})
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after the hit = %d, want 1", got)
	}
	hate := 2 * ind2cHate(10, n.MaxHP(), 100)
	if want := 200 + hate; n.AggroHate(w.player) != want {
		t.Fatalf("hate after the hit = %v, want %v", n.AggroHate(w.player), want)
	}
	ind2bWantHold(t, "attack after the hit", n, actorai.IntentionAttack, me, hate)
}

// TestAgit02DoomArcherAgitAnswersItsFlag ports Agit02DoomArcherAgit's
// clan-attacked hook, every draw 0, which never calls its parent. A hit by
// the character on a clan member that shares its flag makes the archer,
// not attacking, want its buff on the member (weight 1000000) while the
// member is above half its hit points, and want to attack the character
// by the damage. A member of another flag draws neither.
// Every call spends the buff (i_ai1).
func TestAgit02DoomArcherAgitAnswersItsFlag(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		flag  int32
		share float64
		buff  bool
		hate  bool
	}{
		{"same flag", 0, 1, true, true},
		{"member at half its hit points", 0, 0.5, false, true},
		{"other flag", 1, 1, false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			member := warriorTemplate(unboundID, nil)
			member.Clans, member.ClanRange = []string{"doom"}, 500
			w := bootIndividual2c(t, raidBoot{}, ind2cTemplate(ind2cArcherID), member)
			n := w.spawn(t, ind2cArcherID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
			victim := w.spawn(t, unboundID, 1500, 0)
			w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotFlag, row.flag) })
			if row.share < 1 {
				w.setHPShare(t, victim, row.share)
			}
			w.ticks(t, 8)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.buff {
				want[ind2cBuffSkill] = ind1Cast{victim.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit on its clan", n, want)
			if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
				t.Fatalf("i_ai1 after the clan call = %d, want 1", got)
			}
			if !row.hate {
				if got := n.AggroHate(w.player); got != 0 {
					t.Fatalf("other flag archer hate = %v, want none", got)
				}
				return
			}
			if want := ind2cHate(10, n.MaxHP(), 100); n.AggroHate(w.player) != want {
				t.Fatalf("archer hate after the hit on its clan = %v, want %v", n.AggroHate(w.player), want)
			}
		})
	}
}

// TestAgit02DoomArcherAgitHatesAnAggroSpell ports Agit02DoomArcherAgit's
// see-spell hook: the character's self buff of 50 aggro points makes the
// archer want to attack it, held in place, by 150 for each twentieth of
// its maximum hit points the aggro makes.
func TestAgit02DoomArcherAgitHatesAnAggroSpell(t *testing.T) {
	t.Parallel()
	tmpl := ind2cTemplate(ind2cArcherID)
	tmpl.CanMove = false
	w := bootIndividual2c(t, ind2bSpellBoot(t), tmpl)
	n := w.spawn(t, ind2cArcherID, 400, 0)
	want := ind2cHate(ind2bAggro, n.MaxHP(), 150)
	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the archer hates the caster", func() bool { return n.AggroHate(w.player) > 0 })
	if got := n.AggroHate(w.player); got != want {
		t.Fatalf("hate of the caster = %v, want %v", got, want)
	}
	ind2bWantAttack(t, "after the spell", n, false)
}

// TestAgit02DoomArcherAgitPartyJumpsToItsMaster ports
// Agit02DoomArcherAgitParty's created and timer hooks: its 60 s timer
// "4001", first fired 1 s after it spawns. Its master out of the master's
// territory, on a draw under 1 of 3 the private jumps to its master (the
// character sees TeleportToLocation) and drops its attack desires; on a
// draw of 1 it stays.
func TestAgit02DoomArcherAgitPartyJumpsToItsMaster(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw int
		jump bool
	}{{0, true}, {1, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2c(t, raidBoot{}, ind4Leader(ind2cArcherPartyID, 1), ind2cTemplate(ind2cArcherPartyID))
			w.draws.set(4)
			leader := w.spawn(t, leaderID, 1500, 0)
			n := privatesOf(t, leader, 1)[0]
			w.teleport(t, leader, -1350, 0)
			if script.NPCOf(n).InMyTerritory() {
				t.Fatal("setup: the private is still in its master's territory")
			}
			w.onQueue(t, n, func() { script.NPCOf(n).AddAttackDesire(script.PlayerOf(w.player), 100) })
			w.draws.set(4, row.draw, 4)
			w.srv.Advance(t, time.Second)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			x, y, _ := n.Position()
			mx, my, _ := leader.Position()
			if atMaster := x == mx && y == my; atMaster != row.jump {
				t.Fatalf("private at %d,%d, master at %d,%d: at the master = %v, want %v", x, y, mx, my, atMaster, row.jump)
			}
			if hates := n.AggroHate(w.player) > 0; hates == row.jump {
				t.Fatalf("hate of the character = %v, want it dropped only for a jump", n.AggroHate(w.player))
			}
			if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, n.ObjectID())); (got > 0) != row.jump {
				t.Fatalf("TeleportToLocation frames = %d, want them only for a jump", got)
			}
		})
	}
}

// TestAgit02DoomArcherAgitPartyAnswersItsParty ports
// Agit02DoomArcherAgitParty's attacked, party-attacked and clan-attacked
// hooks, every draw 0. A hit on its master makes the private want its buff
// on the master (weight 1000000) and, sharing the master's flag, want to
// attack the attacker by the damage, held in place; with another flag it
// does neither. Its own hit wants the buff on itself and adds no hate of
// its own: the hate it gets comes from its own party call. A hit on a
// member of its clan adds nothing.
func TestAgit02DoomArcherAgitPartyAnswersItsParty(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		flag int32
		buff bool
	}{{"same flag", 0, true}, {"other flag", 1, false}} {
		t.Run("master hit, "+row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2c(t, raidBoot{}, ind4Leader(ind2cArcherPartyID, 1), ind2cTemplate(ind2cArcherPartyID))
			leader := w.spawn(t, leaderID, 400, 0)
			n := privatesOf(t, leader, 1)[0]
			w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
			w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotFlag, row.flag) })
			w.ticks(t, 9)
			w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
			w.hit(t, leader, 10)
			want := map[modelskill.ID]ind1Cast{}
			wantHate := 0.0
			if row.buff {
				want[ind2cBuffSkill] = ind1Cast{leader.ObjectID(), 1000000}
				wantHate = ind2cHate(10, n.MaxHP(), 100)
			}
			w.wantCasts(t, "after the hit on its master", n, want)
			if got := n.AggroHate(w.player); got != wantHate {
				t.Fatalf("private hate = %v, want %v", got, wantHate)
			}
			if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
				t.Fatalf("i_ai1 after the party call = %d, want 1", got)
			}
		})
	}
	t.Run("own hit", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual2c(t, raidBoot{}, ind4Leader(ind2cArcherPartyID, 1), ind2cTemplate(ind2cArcherPartyID))
		leader := w.spawn(t, leaderID, 400, 0)
		n := privatesOf(t, leader, 1)[0]
		w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
		w.ticks(t, 9)
		w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
		w.hit(t, n, 10)
		w.wantCasts(t, "after its own hit", n, map[modelskill.ID]ind1Cast{ind2cBuffSkill: {n.ObjectID(), 1000000}})
		if want := ind2cHate(10, n.MaxHP(), 100); n.AggroHate(w.player) != want {
			t.Fatalf("private hate after its own hit = %v, want %v, its party call's", n.AggroHate(w.player), want)
		}
	})
	t.Run("clan hit", func(t *testing.T) {
		t.Parallel()
		member := warriorTemplate(unboundID, nil)
		member.Clans, member.ClanRange = []string{"doom"}, 500
		w := bootIndividual2c(t, raidBoot{}, ind2cTemplate(ind2cArcherPartyID), member)
		n := w.spawn(t, ind2cArcherPartyID, 1500, 0)
		w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
		clan := w.spawn(t, unboundID, 1500, 100)
		w.ticks(t, 9)
		w.hit(t, clan, 10)
		if hate, casts := n.AggroHate(w.player), ind1Casts(n); hate != 0 || len(casts) != 0 {
			t.Fatalf("after a hit on its clan: hate %v, casts %+v, want neither", hate, casts)
		}
		if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
			t.Fatalf("i_ai1 after a hit on its clan = %d, want 0", got)
		}
	})
}

// TestAgit02DoomArcherAgitPartyHatesAnAggroSpell ports
// Agit02DoomArcherAgitParty's see-spell hook: the character's self buff of
// 50 aggro points makes it want to attack the character, held in place,
// by 150 for each twentieth of its maximum hit points the aggro makes.
func TestAgit02DoomArcherAgitPartyHatesAnAggroSpell(t *testing.T) {
	t.Parallel()
	tmpl := ind2cTemplate(ind2cArcherPartyID)
	tmpl.CanMove = false
	w := bootIndividual2c(t, ind2bSpellBoot(t), tmpl)
	n := w.spawn(t, ind2cArcherPartyID, 400, 0)
	want := ind2cHate(ind2bAggro, n.MaxHP(), 150)
	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the archer hates the caster", func() bool { return n.AggroHate(w.player) > 0 })
	if got := n.AggroHate(w.player); got != want {
		t.Fatalf("hate of the caster = %v, want %v", got, want)
	}
	ind2bWantAttack(t, "after the spell", n, false)
}

// TestAgit02DoomWarriorAgitReturnsHome ports Agit02DoomWarriorAgit's
// created, no-desire and timer hooks. It spawns with flag 0 and does
// nothing with no desire left. Its 60 s timer "4001", first fired 1 s
// after it spawns: out of its territory, on a draw under 1 of 3, it jumps
// back to its spawn point (TeleportToLocation) and drops its attack
// desires; on a draw of 1 it stays.
func TestAgit02DoomWarriorAgitReturnsHome(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw int
		home bool
	}{{0, true}, {1, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2c(t, raidBoot{}, ind2cTemplate(ind2cWarriorID))
			w.draws.set(4)
			n := w.spawn(t, ind2cWarriorID, 1500, 0)
			if got := ind1Slot(n, script.SlotFlag); got != 0 {
				t.Fatalf("created flag = %d, want 0", got)
			}
			if d, ok := desire(n, actorai.IntentionWander); ok {
				t.Fatalf("the warrior queued a wander desire %+v", d)
			}
			home := script.NPCOf(n).SpawnLocation()
			w.teleport(t, n, -1350, 0)
			if script.NPCOf(n).InMyTerritory() {
				t.Fatal("setup: the warrior is still in its territory")
			}
			w.onQueue(t, n, func() { script.NPCOf(n).AddAttackDesire(script.PlayerOf(w.player), 100) })
			w.draws.set(4, row.draw, 4)
			w.srv.Advance(t, time.Second)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			x, y, _ := n.Position()
			if atHome := x == home.X && y == home.Y; atHome != row.home {
				t.Fatalf("warrior at %d,%d, spawn point %d,%d: at home = %v, want %v", x, y, home.X, home.Y, atHome, row.home)
			}
			if _, attacking := desire(n, actorai.IntentionAttack); attacking == row.home {
				t.Fatalf("attack desire queued = %v, want %v", attacking, !row.home)
			}
			if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, n.ObjectID())); (got > 0) != row.home {
				t.Fatalf("TeleportToLocation frames = %d, want them only for a jump home", got)
			}
		})
	}
}

// TestAgit02DoomWarriorAgitAnswersItsFlag ports Agit02DoomWarriorAgit's
// clan-attacked hook, every draw 0, which never calls its parent: a clan
// member that shares the hit warrior's flag wants to attack the attacker
// by the damage, and never buffs the hit warrior. A member of
// another flag does nothing.
func TestAgit02DoomWarriorAgitAnswersItsFlag(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		flag int32
	}{{"same flag", 0}, {"other flag", 1}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2c(t, raidBoot{}, ind2cTemplate(ind2cWarriorID), ind2cTemplate(ind2cWarriorTwoID))
			victim := w.spawn(t, ind2cWarriorID, 1500, 0)
			member := w.spawn(t, ind2cWarriorTwoID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind2cSkills())
			w.onQueue(t, member, func() { script.NPCOf(member).SetInt(script.SlotFlag, row.flag) })
			w.ticks(t, 8)
			w.onQueue(t, member, func() { member.AI().Desires().Clear() })
			w.hit(t, victim, 10)
			if d, ok := ind1Casts(member)[ind2cBuffSkill]; ok && d.FinalTarget != nil && d.FinalTarget.ObjectID() == victim.ObjectID() {
				t.Fatalf("member buffs the hit warrior: %+v", d)
			}
			if row.flag != 0 {
				if hate := member.AggroHate(w.player); hate != 0 {
					t.Fatalf("other flag member hate = %v, want none", hate)
				}
				return
			}
			if want := ind2cHate(10, member.MaxHP(), 100); member.AggroHate(w.player) != want {
				t.Fatalf("member hate = %v, want %v", member.AggroHate(w.player), want)
			}
		})
	}
}

// TestOrfenPrivateRaikelCursesOverLevelled ports OrfenPrivateRaikel's
// attacked and see-spell hooks. Against the level 20 character, a level 11
// Raikel wants its raid curse on a hit and its raid mute on the
// character's spell, weight 1000000; a level 12 one wants neither. A hit
// adds no hate and never buffs.
func TestOrfenPrivateRaikelCursesOverLevelled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		curse bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2cTemplate(ind2cRaikelID)
			tmpl.Level, tmpl.CanMove = row.level, false
			w := bootIndividual2c(t, raidSpellBoot(t), tmpl)
			n := w.spawn(t, ind2cRaikelID, 400, 0)
			w.srv.InstallNPCCastSeam(t, n, modelskill.NewTable([]modelskill.Definition{raidCurseDefinition(), ind1Skill(premoMuteID, 600, false), ind2cBuff()}))
			me := w.player.ObjectID()

			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.curse {
				want[raidCurseID] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			if got := n.AggroHate(w.player); got != 0 {
				t.Errorf("hate after the hit = %v, want none", got)
			}

			w.castSelfBuff()
			if row.curse {
				w.srv.AdvanceUntil(t, "the raid mute desire", func() bool {
					_, ok := ind1Casts(n)[premoMuteID]
					return ok
				})
				want[premoMuteID] = ind1Cast{me, 1000000}
			} else {
				w.srv.Advance(t, 2*time.Second)
			}
			w.wantCasts(t, "after the spell", n, want)
		})
	}
}

// TestOrfenPrivateRaikelShufflesItsHate ports OrfenPrivateRaikel's created
// and timer hooks. It arms timer "5001" 90 s plus a draw of up to 240 s
// after it spawns (draw 0: 90 s). On a draw under 66 the tick shuffles its
// hate, raising the character it hates less than another NPC to that
// NPC's hate plus 200; on a draw of 66 it does not. Either way the tick
// re-arms with a fresh draw: 1000 brings the next tick 91 s later.
func TestOrfenPrivateRaikelShufflesItsHate(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw    int
		shuffle bool
	}{{65, true}, {66, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2cTemplate(ind2cRaikelID)
			tmpl.CanMove = false
			w := bootIndividual2c(t, raidBoot{}, tmpl, warriorTemplate(unboundID, nil))
			n := w.spawn(t, ind2cRaikelID, 400, 0)
			other := w.spawn(t, unboundID, 400, 200)
			ind2Hate(t, w, n, other, 1000, 100)

			w.srv.Advance(t, 89*time.Second)
			if got := n.AggroHate(w.player); got != 100 {
				t.Fatalf("hate of the character before 90 s = %v, want 100", got)
			}
			w.draws.set(0, row.draw, 1000)
			w.srv.Advance(t, time.Second)
			want := 100.0
			if row.shuffle {
				want = 1200
			}
			if got := n.AggroHate(w.player); got != want {
				t.Fatalf("hate of the character after the first tick = %v, want %v", got, want)
			}
			if row.shuffle {
				return
			}
			w.srv.Advance(t, 90*time.Second)
			if got := n.AggroHate(w.player); got != 100 {
				t.Fatalf("hate of the character 90 s after the first tick = %v, want 100", got)
			}
			w.srv.Advance(t, time.Second)
			if got := n.AggroHate(w.player); got != 1200 {
				t.Fatalf("hate of the character after the re-armed tick = %v, want 1200", got)
			}
		})
	}
}

// TestSpecialBloodyQueenCastsOnceAtAnAttacker ports SpecialBloodyQueen's
// created and attacked hooks. It spawns with i_ai0 clear. A hit by the
// character draws once: from beyond 300 a draw under 80 wants its DD magic
// on the character (weight 1000000); from beyond 100 a draw under 70 does
// when the character is the creature it desires most, and under 30 when
// another is; from 100 or nearer only a draw under 10 does, and only when
// the character is the one it desires most. A cast spends the DD magic
// for good (i_ai0): a later hit wants nothing.
func TestSpecialBloodyQueenCastsOnceAtAnAttacker(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		dist  int
		top   string
		draw  int
		magic bool
	}{
		{"far draw 79", 400, "", 79, true},
		{"far draw 80", 400, "", 80, false},
		{"mid on top draw 69", 200, "character", 69, true},
		{"mid on top draw 70", 200, "character", 70, false},
		{"mid under another draw 29", 200, "other", 29, true},
		{"mid under another draw 30", 200, "other", 30, false},
		{"mid with no desire", 200, "", 0, false},
		{"near on top draw 9", 80, "character", 9, true},
		{"near on top draw 10", 80, "character", 10, false},
		{"near under another draw 0", 80, "other", 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2c(t, raidBoot{}, ind2cQueenTemplate(), warriorTemplate(unboundID, nil))
			n := w.spawn(t, ind2cQueenID, row.dist, 0)
			w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
			if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
				t.Fatalf("created i_ai0 = %d, want 0", got)
			}
			switch row.top {
			case "character":
				w.onQueue(t, n, func() { script.NPCOf(n).AddAttackDesire(script.PlayerOf(w.player), 1000) })
			case "other":
				other := w.spawn(t, unboundID, row.dist, 300)
				w.onQueue(t, n, func() { script.NPCOf(n).AddAttackDesire(script.NPCOf(other), 1000) })
			}
			w.draws.set(50, row.draw)
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			spent := int32(0)
			if row.magic {
				want[ind2DDMagicSkill] = ind1Cast{w.player.ObjectID(), 1000000}
				spent = 1
			}
			w.wantCasts(t, "after the hit", n, want)
			if got := ind1Slot(n, script.SlotIntAI0); got != spent {
				t.Fatalf("i_ai0 after the hit = %d, want %d", got, spent)
			}
			if !row.magic {
				return
			}
			w.onQueue(t, n, func() { n.AI().Desires().RemoveKind(actorai.IntentionCast) })
			w.draws.set(0)
			w.hit(t, n, 10)
			w.wantCasts(t, "after a second hit", n, map[modelskill.ID]ind1Cast{})
		})
	}
}

// TestSpecialBloodyQueenAnswersItsClanFromAfar ports SpecialBloodyQueen's
// clan-attacked hook. A hit by the character, more than 100 away, on a
// member of its clan makes the settled queen, on a draw under 40 of 100,
// want its DD magic on the character (weight 1000000) and spend it
// (i_ai0); a draw of 40 does not.
func TestSpecialBloodyQueenAnswersItsClanFromAfar(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw  int
		magic bool
	}{{39, true}, {40, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			member := warriorTemplate(unboundID, nil)
			member.Clans, member.ClanRange = []string{"doom"}, 500
			w := bootIndividual2c(t, raidBoot{}, ind2cQueenTemplate(), member)
			n := w.spawn(t, ind2cQueenID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
			victim := w.spawn(t, unboundID, 1500, 50)
			w.ticks(t, 8)
			w.draws.set(50, row.draw)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			spent := int32(0)
			if row.magic {
				want[ind2DDMagicSkill] = ind1Cast{w.player.ObjectID(), 1000000}
				spent = 1
			}
			w.wantCasts(t, "after the hit on its clan", n, want)
			if got := ind1Slot(n, script.SlotIntAI0); got != spent {
				t.Fatalf("i_ai0 after the hit on its clan = %d, want %d", got, spent)
			}
		})
	}
}

// TestSpecialBloodyQueenCastsOnSight ports SpecialBloodyQueen's
// see-creature hook. Settled in its territory, seeing the character more
// than 100 away, on a draw under 20 of 100 it wants its DD magic on the
// character (weight 1000000) and spends it (i_ai0); a draw of 20 does
// not. Either way its parent attacks the character it sees (hate 200).
func TestSpecialBloodyQueenCastsOnSight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw  int
		magic bool
	}{{19, true}, {20, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2c(t, raidBoot{}, ind2cQueenTemplate())
			n := w.spawn(t, ind2cQueenID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind2cSkills())
			w.ticks(t, 9)
			if n.LifeTime() <= 7 || !script.NPCOf(n).InMyTerritory() {
				t.Fatalf("setup: life time %d, in territory %v, want past 7 and in it", n.LifeTime(), script.NPCOf(n).InMyTerritory())
			}
			w.draws.set(50, row.draw)
			w.moveCharacter(t, n, -150, 0)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			spent := int32(0)
			if row.magic {
				want[ind2DDMagicSkill] = ind1Cast{w.player.ObjectID(), 1000000}
				spent = 1
			}
			w.wantCasts(t, "after it saw the character", n, want)
			if got := ind1Slot(n, script.SlotIntAI0); got != spent {
				t.Fatalf("i_ai0 after the sight = %d, want %d", got, spent)
			}
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
		})
	}
}
