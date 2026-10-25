package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// ind2fDebuffTemplate is a fixture WarriorPhysicalSpecialDebuffAggressive
// of id listing its debuff by type.
func ind2fDebuffTemplate(id int32) *npc.Template {
	return ind2fTemplate(id, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeDebuff: ind2fDebuffSkill})
}

// TestWarriorPhysicalSpecialDebuffAggressiveDebuffsWhatItFights ports
// WarriorPhysicalSpecialDebuffAggressive's created, attacked and timer
// hooks, every draw 0. It spawns with i_ai1 and i_ai2 clear. The first hit
// arms the 5 s timer "2001" and wants no debuff: the character is not yet
// the most hated. Once it fights, the tick wants the debuff on the most
// hated, weight 1000000. A hit by the most hated wants the debuff on it
// and marks the next tick to skip it, which keeps i_ai1 and clears i_ai2;
// the tick after wants the debuff again. Out of a fight the tick clears
// both slots and the timer stops.
func TestWarriorPhysicalSpecialDebuffAggressiveDebuffsWhatItFights(t *testing.T) {
	t.Parallel()
	// Rooted, it never reaches the character: no swing of its own takes
	// up a debuff it queued.
	tmpl := ind2fDebuffTemplate(ind2fDebuffID)
	tmpl.CanMove = false
	w := bootIndividual2f(t, raidBoot{}, tmpl)
	n := w.spawn(t, ind2fDebuffID, 300, 0)
	me := w.player.ObjectID()
	char := w.charOf(t)
	debuff := map[modelskill.ID]ind1Cast{ind2fDebuffSkill: {me, 1000000}}
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 0 || a2 != 0 {
		t.Fatalf("created i_ai1, i_ai2 = %d, %d, want 0, 0", a1, a2)
	}

	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after the first hit = %d, want 1", got)
	}
	w.ticks(t, 1)
	if got := n.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("intention after the first hit = %v, want attack", got)
	}
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the first tick", n, debuff)

	w.clear(t, n)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by the most hated", n, debuff)
	if got := ind1Slot(n, script.SlotIntAI2); got != 1 {
		t.Fatalf("i_ai2 after a hit by the most hated = %d, want 1", got)
	}
	w.onQueue(t, n, func() {
		n.AI().Desires().Clear()
		script.NPCOf(n).AddAttackDesire(char, 100)
	})
	w.ticks(t, 1)
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the skipped tick", n, nil)
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 1 || a2 != 0 {
		t.Fatalf("i_ai1, i_ai2 after the skipped tick = %d, %d, want 1, 0", a1, a2)
	}
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the tick after", n, debuff)

	w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
	w.ticks(t, 1)
	if got := n.AI().CurrentIntention(); got == actorai.IntentionAttack || got == actorai.IntentionCast {
		t.Fatalf("intention after the desires were dropped = %v, want neither attack nor cast", got)
	}
	w.srv.Advance(t, 5*time.Second)
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 0 || a2 != 0 {
		t.Fatalf("i_ai1, i_ai2 after the tick out of a fight = %d, %d, want 0, 0", a1, a2)
	}
	w.srv.Advance(t, 10*time.Second)
	w.wantCasts(t, "once the timer stopped", n, nil)
}

// TestWarriorPhysicalSpecialDebuffAggressiveAnswersItsClan ports
// WarriorPhysicalSpecialDebuffAggressive's clan-attacked and see-creature
// hooks, every draw 0: a settled clan member wants its debuff, weight
// 1000000, on the attacker of another member; a young one does not.
// Seeing the character it attacks it (hate 200).
func TestWarriorPhysicalSpecialDebuffAggressiveAnswersItsClan(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		settle int
		cast   bool
	}{{"settled", 8, true}, {"young", 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2f(t, raidBoot{}, ind2fDebuffTemplate(ind2fDebuffID), ind2fDebuffTemplate(ind2fDebuffTwoID))
			victim := w.spawn(t, ind2fDebuffID, 1500, 0)
			member := w.spawn(t, ind2fDebuffTwoID, 1500, 200)
			w.ticks(t, row.settle)
			w.clear(t, member)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind2fDebuffSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the clan call", member, want)

			w.clear(t, member)
			w.moveCharacter(t, member, -50, 0)
			w.ticks(t, 1)
			if got := member.AggroHate(w.player); got < 200 {
				t.Fatalf("hate of the character it sees = %v, want 200 at least", got)
			}
		})
	}
}

// ind2fHoldTemplate is a fixture WarriorPhysicalSpecialHoldAggressive of
// id calling for help within 400, listing its physical special skill and
// DD magic by type.
func ind2fHoldTemplate(id int32) *npc.Template {
	return ind2fTemplate(id, npc.AIParams{"HelpCastRange": "400"}, map[npc.SkillType]modelskill.ID{
		npc.SkillTypePhysicalSpecial: ind2fSpecialSkill, npc.SkillTypeDDMagic: ind2fDDMagicSkill,
	})
}

// TestWarriorPhysicalSpecialHoldAggressiveWalksBackToItsSpawn ports
// WarriorPhysicalSpecialHoldAggressive's no-desire and move-finished
// hooks. Off its spawn point with nothing to do it wants to walk back
// there, weight 30, and never wanders; arriving there it wants to do
// nothing for 40, weight 30. Arriving anywhere else it wants to walk back,
// weight 30.
func TestWarriorPhysicalSpecialHoldAggressiveWalksBackToItsSpawn(t *testing.T) {
	t.Parallel()
	w := bootIndividual2f(t, raidBoot{}, ind2fHoldTemplate(ind2fHoldID))
	n := w.spawn(t, ind2fHoldID, 1500, 0)
	home := positionOf(n)
	w.teleport(t, n, 100, 0)
	w.clear(t, n)

	w.ticks(t, 2)
	if d, ok := desire(n, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location != home {
		t.Fatalf("off its spawn point move desire = %+v (%v), want weight 30 to %+v", d, ok, home)
	}
	if d, ok := desire(n, actorai.IntentionWander); ok {
		t.Fatalf("it wants to wander: %+v", d)
	}
	w.walk(t, n)
	if got := positionOf(n); got != home {
		t.Fatalf("it stands at %+v, want its spawn point %+v", got, home)
	}
	if d, ok := desire(n, actorai.IntentionNothing); !ok || d.Weight != 30 || d.Timer != 40 {
		t.Fatalf("on its spawn point do-nothing desire = %+v (%v), want timer 40, weight 30", d, ok)
	}

	elsewhere := script.Loc{X: home.X, Y: home.Y + 100, Z: home.Z}
	w.onQueue(t, n, func() {
		n.AI().Desires().Clear()
		script.NPCOf(n).AddMoveToDesire(elsewhere, 100)
	})
	w.ticks(t, 1)
	w.walk(t, n)
	if got := positionOf(n); got.X != elsewhere.X || got.Y != elsewhere.Y {
		t.Fatalf("it stands at %+v, want %+v", got, elsewhere)
	}
	if d, ok := desire(n, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location != home {
		t.Fatalf("arrived elsewhere, move desire = %+v (%v), want weight 30 to %+v", d, ok, home)
	}
}

// walk runs h's AI once and then moves it until it stops.
func (w *ind2eWorld) walk(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.ticks(t, 1)
	for i := 0; h.IsMoving(); i++ {
		if i >= 200 {
			t.Fatal("the NPC never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// TestWarriorPhysicalSpecialHoldAggressiveAnswersHitsWhereItStands ports
// WarriorPhysicalSpecialHoldAggressive's attacked, clan-attacked and
// script event hooks. Hit by the character, it calls for help (event
// 10001 naming the character) within its HelpCastRange of 400, not to an
// NPC 600 away; from closer than 80 on a draw under 33 it wants its
// physical special skill, from 80 or farther its DD magic, weight 1000000
// without moving, and hates the hit as a Warrior does (125 for a hit of
// 10). The hit makes the same answer twice, from its clan call to itself
// and from its attacked hook: weight 2000000, hate 250. A clan member
// farther than 80 from the character wants its DD magic on it and hates
// it once alike. The help call wants nothing of the member: it ignores
// every script event.
func TestWarriorPhysicalSpecialHoldAggressiveAnswersHitsWhereItStands(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dist int
		draw int
		cast modelskill.ID
	}{{"close", 50, 32, ind2fSpecialSkill}, {"close draw 33", 50, 33, 0}, {"80 away", 80, 0, ind2fDDMagicSkill}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2f(t, raidBoot{}, ind2fHoldTemplate(ind2fHoldID), ind2fHoldTemplate(ind2fHoldTwoID), warriorTemplate(ind1ListenerID, nil))
			n := w.spawn(t, ind2fHoldID, 1500, 0)
			member := w.spawn(t, ind2fHoldTwoID, 1500, 300)
			near := w.spawn(t, ind1ListenerID, 1500, -300)
			far := w.spawn(t, ind1ListenerID, 1500, -600)
			w.clear(t, n)
			w.clear(t, member)
			w.moveCharacter(t, n, -row.dist, 0)
			w.srv.Settle(t)
			me := w.player.ObjectID()

			w.draws.set(row.draw)
			w.hit(t, n, 10)
			if got, want := w.heard2e.take(near.ObjectID()), []string{fmt.Sprintf("event=10001 arg1=%d arg2=0", me)}; !slices.Equal(got, want) {
				t.Fatalf("the NPC 300 away heard %q, want %q", got, want)
			}
			if got := w.heard2e.take(far.ObjectID()); len(got) != 0 {
				t.Fatalf("the NPC 600 away heard %q", got)
			}
			want := map[modelskill.ID]ind1Cast{}
			if row.cast != 0 {
				want[row.cast] = ind1Cast{me, 2000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			for _, d := range ind1Casts(n) {
				if d.MoveToTarget {
					t.Errorf("cast desire %+v moves, want it held", d)
				}
			}
			if got := n.AggroHate(w.player); got != 250 {
				t.Fatalf("hate after the hit = %v, want 250", got)
			}
			w.wantCasts(t, "the member after the call", member, map[modelskill.ID]ind1Cast{ind2fDDMagicSkill: {me, 1000000}})
			if got := member.AggroHate(w.player); got != 125 {
				t.Fatalf("the member's hate after the call = %v, want 125", got)
			}
			if d, ok := desire(member, actorai.IntentionAttack); !ok || d.MoveToTarget {
				t.Fatalf("the member's attack desire = %+v (%v), want one held", d, ok)
			}
		})
	}
}

// TestWarriorPhysicalSpecialHoldAggressiveAttacksWhatItSeesWhereItStands
// ports WarriorPhysicalSpecialHoldAggressive's see-creature and see-spell
// hooks: seeing the character it wants to attack it, weight 50, without
// moving; seeing it cast a buff with aggro points it hates it by
// 1 / (level + 7) * 150.
func TestWarriorPhysicalSpecialHoldAggressiveAttacksWhatItSeesWhereItStands(t *testing.T) {
	t.Parallel()
	w := bootIndividual2f(t, ind1AggroSpellBoot(t), ind2fHoldTemplate(ind2fHoldID), ind2fHoldTemplate(ind2fHoldTwoID))
	seer := w.spawn(t, ind2fHoldID, 1500, 0)
	w.clear(t, seer)
	w.moveCharacter(t, seer, -150, 0)
	w.ticks(t, 1)
	if d, ok := desire(seer, actorai.IntentionAttack); !ok || d.MoveToTarget || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("attack desire of the character it sees = %+v (%v), want one held on the character", d, ok)
	}
	if got := seer.AggroHate(w.player); got != 50 {
		t.Fatalf("hate of the character it sees = %v, want 50", got)
	}

	caster := w.spawn(t, ind2fHoldTwoID, 1350, 200)
	w.castAggroBuff()
	w.srv.AdvanceUntil(t, "the see-spell hate", func() bool { return caster.AggroHate(w.player) > 0 })
	if got, want := caster.AggroHate(w.player), 1.0/8*150; got != want {
		t.Fatalf("hate of the caster = %v, want %v", got, want)
	}
}

// ind2fStone is a fixture WarriorPhysicalSpecialStoneAggressive of id in
// the faction clan.
func ind2fStone(id int32, faction string) *npc.Template {
	tmpl := ind2fTemplate(id, nil, nil)
	tmpl.Clans, tmpl.ClanRange = []string{faction}, 500
	return tmpl
}

// TestWarriorPhysicalSpecialStonePetrifiesAllies ports
// WarriorPhysicalSpecialStoneAggressive's see-creature, attacked and
// clan-attacked hooks for a Ketra and a Varka NPC. A character allied with
// its faction is not attacked on sight; its hit makes the NPC want to
// petrify it (4515, weight 1000000 from its attacked hook and as much
// again from its own clan call, which the hit makes first to itself) and
// leaves it no hate. A character with no alliance is attacked on sight
// with weight 200.
func TestWarriorPhysicalSpecialStonePetrifiesAllies(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		id       int32
		faction  string
		standing int
	}{{"ketra", ind2fKetraStoneID, "ketra_orc_clan", 1}, {"varka", ind2fVarkaStoneID, "varka_silenos_clan", -1}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2f(t, raidBoot{}, ind2fStone(row.id, row.faction))
			ch := ind3Character(t, w.ind1World)
			ch.SetVarkaKetraAlliance(row.standing)

			allied := w.spawn(t, row.id, 150, 0)
			w.srv.InstallNPCCastSeam(t, allied, modelskill.NewTable([]modelskill.Definition{npcSkill(petrifyID, 1, true)}))
			w.ticks(t, 2)
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of an ally it sees = %v, want none", got)
			}
			w.hit(t, allied, 10)
			if d, ok := castDesireOf(allied, petrifyID); !ok || d.Weight != 2000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
				t.Fatalf("petrify desire = %+v (%v), want on the character, weight 2000000", d, ok)
			}
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of the ally that hit it = %v, want none", got)
			}

			ch.SetVarkaKetraAlliance(0)
			open := w.spawn(t, row.id, 0, 150)
			w.ticks(t, 2)
			if got := open.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of a character with no alliance it sees = %v, want 200", got)
			}
		})
	}
}

// TestWarriorPhysicalSpecialStonePetrifiesAlliedCaster ports
// WarriorPhysicalSpecialStoneAggressive's see-spell hook: a Ketra ally's
// buff with aggro points makes the Ketra NPC want to petrify the caster
// (4515, weight 1000000) and leaves it no hate.
func TestWarriorPhysicalSpecialStonePetrifiesAlliedCaster(t *testing.T) {
	t.Parallel()
	w := bootIndividual2f(t, ind1AggroSpellBoot(t), ind2fStone(ind2fKetraStoneID, "ketra_orc_clan"))
	ch := ind3Character(t, w.ind1World)
	ch.SetVarkaKetraAlliance(1)
	stone := w.spawn(t, ind2fKetraStoneID, 150, 0)
	w.srv.InstallNPCCastSeam(t, stone, modelskill.NewTable([]modelskill.Definition{npcSkill(petrifyID, 1, true)}))
	w.clear(t, stone)
	w.castAggroBuff()
	w.srv.AdvanceUntil(t, "the petrify desire", func() bool {
		_, ok := castDesireOf(stone, petrifyID)
		return ok
	})
	if d, _ := castDesireOf(stone, petrifyID); d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("petrify desire = %+v, want on the caster, weight 1000000", d)
	}
	if got := stone.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the allied caster = %v, want none", got)
	}
}

// TestWarriorPhysicalSpecialStrongAggressiveStrikesBelowHalf ports
// WarriorPhysicalSpecialStrongAggressive's attacked and see-creature
// hooks. A hit while it stands at half its HP wants nothing; below half,
// on a draw under 33, a hit by a character in no party wants its physical
// special skill on it, and one by a character in a party its self range
// physical special skill, each weight 1000000. Seeing the character it
// attacks it.
func TestWarriorPhysicalSpecialStrongAggressiveStrikesBelowHalf(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		share float64
		draw  int
		party bool
		cast  modelskill.ID
	}{
		{"alone", 0.49, 32, false, ind2fSpecialSkill},
		{"alone draw 33", 0.49, 33, false, 0},
		{"in a party", 0.49, 0, true, ind2fRangeSkill},
		{"in a party draw 33", 0.49, 33, true, 0},
		{"half", 0.5, 0, false, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2f(t, raidBoot{}, ind2fTemplate(ind2fStrongID, nil, map[npc.SkillType]modelskill.ID{
				npc.SkillTypePhysicalSpecial: ind2fSpecialSkill, npc.SkillTypeSelfRangePhysicalSpecial: ind2fRangeSkill,
			}))
			if row.party {
				w.joinMate(t)
			}
			n := w.spawn(t, ind2fStrongID, 1500, 0)
			w.setHPShare(t, n, row.share)
			w.draws.set(row.draw)
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast != 0 {
				want[row.cast] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
		})
	}
	t.Run("sight", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual2f(t, raidBoot{}, ind2fTemplate(ind2fStrongID, nil, nil))
		n := w.spawn(t, ind2fStrongID, 1500, 0)
		w.moveCharacter(t, n, -50, 0)
		w.ticks(t, 1)
		if got := n.AggroHate(w.player); got != 200 {
			t.Fatalf("hate of the character it sees = %v, want 200", got)
		}
	})
}

// ind2fServitorID is the fixture servitor the character summons, frail
// enough that one blow kills it.
const ind2fServitorID = int32(12564)

// summonFrail summons the character's frail servitor and returns it once
// it is in the world.
func (w *ind2eWorld) summonFrail(t *testing.T) *summon.Actor {
	t.Helper()
	w.onCharacter(t, func(c *player.Character) {
		c.SummonServitor(modelskill.Definition{ID: 1111, Level: 1, SkillType: "SUMMON", NpcID: int(ind2fServitorID), SummonTotalLifeTime: 1_200_000})
	})
	var servitor *summon.Actor
	w.srv.AdvanceUntil(t, "the servitor is in the world", func() bool {
		obj, ok := w.srv.State.Summon(w.player.ObjectID())
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	w.srv.ReadQueued(t, w.srv.Client)
	return servitor
}

// TestWarriorPhysicalSpecialHoldAggressiveTurnsOnTheOwnerOfAKilledSummon
// ports WarriorPhysicalSpecialHoldAggressive's attack-finished and
// skill-finished hooks, every draw 0. Killing the character's servitor
// with a blow, or with its DD magic, it wants its physical special skill
// on the character standing closer than 80, weight 1000000, and to attack
// it without moving.
func TestWarriorPhysicalSpecialHoldAggressiveTurnsOnTheOwnerOfAKilledSummon(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		magic bool
	}{{"blow", false}, {"DD magic", true}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			servitorTmpl := &npc.Template{
				ID: int(ind2fServitorID), TemplateID: int(ind2fServitorID), Type: "Servitor", Name: "Frail", Level: 20,
				HPMax: 20, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
			}
			// Strong enough that its first blow or bolt kills, and swinging
			// fast enough (an attack time of 400ms) that a killing swing's
			// finish comes ahead of the AI's re-run 300ms after its hit, from
			// a reach of 300.
			hold := ind2fHoldTemplate(ind2fHoldID)
			hold.PAtk, hold.MAtk, hold.AtkSpd, hold.BaseAttackRange = 1000, 1000, 1250, 300
			w := bootIndividual2f(t, raidBoot{}, hold, servitorTmpl)
			servitor := w.summonFrail(t)
			sx, sy, _ := servitor.Position()
			// Beside the servitor, on the far side from the character and
			// 70 from it.
			px, py, _ := w.srv.PlayerPosition(t, w.player.ObjectID())
			n := w.spawn(t, ind2fHoldID, sx-px+30, sy-py)
			w.srv.InstallNPCCastSeam(t, n, ind2fSkills())
			me := w.player.ObjectID()
			w.onQueue(t, n, func() {
				n.AI().Desires().Clear()
				if row.magic {
					n.AI().AddCastDesire(servitor, modelskill.Ref{ID: ind2fDDMagicSkill, Level: 1}, 1e9, true, false)
				} else {
					n.AddAttackDesireHold(servitor, 1e9)
				}
			})
			w.ticks(t, 1)
			// A whole swing or cast plays out within each second advanced
			// between AI runs; the answer is checked before the next run
			// decays its weight.
			for i := 0; !hasCast(n, ind2fSpecialSkill); i++ {
				if i == 30 {
					t.Fatalf("no answer to the kill within 30 AI runs: servitor dead %v", servitor.Dead())
				}
				w.srv.Advance(t, time.Second)
				w.srv.Settle(t)
				if !hasCast(n, ind2fSpecialSkill) {
					w.ticks(t, 1)
				}
			}
			if !servitor.Dead() {
				t.Fatal("it answered before the servitor died")
			}
			w.srv.Settle(t)
			w.wantCasts(t, "after the kill", n, map[modelskill.ID]ind1Cast{ind2fSpecialSkill: {me, 1000000}})
			if d, ok := desire(n, actorai.IntentionAttack); !ok || d.MoveToTarget || d.FinalTarget == nil || d.FinalTarget.ObjectID() != me {
				t.Fatalf("attack desire = %+v (%v), want one held on the character", d, ok)
			}
		})
	}
}
