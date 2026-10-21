package ai

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the scenarios of the second individual behavior batch's
// fifth PR spawn (#187 PR 5), each a fixture monster of level 1.
const (
	ind2eFleeID       = int32(21443) // WarriorFleeFromClanAggressive
	ind2eFleeTwoID    = int32(21446) // WarriorFleeFromClanAggressive
	ind2ePowershotID  = int32(20268) // WarriorPassiveUsePowershot
	ind2ePatrolID     = int32(22124) // WarriorPatrolWeaponEquipped
	ind2eAggroID      = int32(21427) // WarriorPhysicalSpecialAggressive
	ind2eAreaID       = int32(22011) // WarriorPhysicalSpecialAreaAggressive
	ind2eGatherID     = int32(21676) // WarriorPhysicalSpecialGatheringAggressive
	ind2ePotionID     = int32(22022) // WarriorPhysicalSpecialPotionAggressive
	ind2eSpoilID      = int32(22017) // WarriorPhysicalSpecialSpoilAggressive
	ind2eSpecialSkill = modelskill.ID(9621)
	ind2eDebuffSkill  = modelskill.ID(9622)
	ind2eDebuff2Skill = modelskill.ID(9623)
	ind2eRangeSkill   = modelskill.ID(9624)
	ind2eHealSkill    = modelskill.ID(9625)
	ind2eEffectSkill  = modelskill.ID(9626)
	ind2eSwordID      = int32(30) // the fixture item table's one-handed sword
)

// ind2eBatch are the classes the PR ports, in scripts.xml order.
var ind2eBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorFleeFromClanAggressive", warrior.FleeFromClanAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPassiveUsePowershot", warrior.PassiveUsePowershot},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPatrolWeaponEquipped", warrior.PatrolWeaponEquipped},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialAggressive", warrior.PhysicalSpecialAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialAreaAggressive", warrior.PhysicalSpecialAreaAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialGatheringAggressive", warrior.PhysicalSpecialGatheringAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialPotionAggressive", warrior.PhysicalSpecialPotionAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialSpoilAggressive", warrior.PhysicalSpecialSpoilAggressive},
}

// ind2eHeard records, per receiving NPC, the script events the listener
// behavior on ind1ListenerID receives.
type ind2eHeard struct {
	mu     sync.Mutex
	events map[int32][]string
}

func (h *ind2eHeard) take(objectID int32) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.events[objectID]
	delete(h.events, objectID)
	return out
}

// ind2eWorld is ind1World with the listener's per-NPC record.
type ind2eWorld struct {
	*ind1World
	heard2e *ind2eHeard
}

// bootIndividual2e boots the PR's classes after the Warrior chain beside
// templates, with extra's options added last and its step run before the
// character enters the world, every draw 0 until a scenario queues others,
// every NPC knowing the batch's skills, a listener behavior on
// ind1ListenerID and the director script the shared helpers send through.
func bootIndividual2e(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind2eWorld {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind2eBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	heard := &ind2eHeard{events: map[int32][]string{}}
	const listener, director = "test.Ind2eListener", "test.Ind1Director"
	catalog[listener] = func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ind1ListenerID}, Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				heard.mu.Lock()
				defer heard.mu.Unlock()
				id := e.NPC.ObjectID()
				heard.events[id] = append(heard.events[id], fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
			},
		}}
	}
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: listener}, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(ind2eSkills()),
		gameservertest.WithScriptSkills(ind2eSkillDefs()...),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind2eWorld{ind1World: &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}, heard2e: heard}
}

// ind2eTemplate is a fixture batch NPC of id with ai, listing skills by
// type, in the clan the scenario's NPCs share.
func ind2eTemplate(id int32, ai npc.AIParams, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for k, v := range skills {
		tmpl.SkillsByType[k] = modelskill.Ref{ID: v, Level: 1}
	}
	tmpl.Clans, tmpl.ClanRange = []string{"ind2e"}, 600
	return tmpl
}

// ind2eSkillDefs are the batch NPCs' skills: one-target strikes and
// debuffs reaching 2000, a self heal.
func ind2eSkillDefs() []modelskill.Definition {
	return []modelskill.Definition{
		ind1Skill(ind2eSpecialSkill, 2000, false), ind1Skill(ind2eDebuffSkill, 2000, false),
		ind1Skill(ind2eDebuff2Skill, 2000, false), ind1Skill(ind2eRangeSkill, 2000, false),
		ind1Skill(ind2eHealSkill, 0, true), ind1Skill(ind2eEffectSkill, 2000, false),
	}
}

// ind2eSkills is ind2eSkillDefs as the table the NPCs cast from.
func ind2eSkills() *modelskill.Table { return modelskill.NewTable(ind2eSkillDefs()) }

// slotOf returns the object id of the creature slot of h's script memory
// holds, 0 for none.
func slotOf(h *npc.Hostile, slot script.CreatureSlot) int32 {
	c := script.NPCOf(h).Slot(slot)
	if c == nil {
		return 0
	}
	return c.ObjectID()
}

// TestWarriorFleeFromClanAggressiveRunsFromAFallenMember ports
// WarriorFleeFromClanAggressive's clan died and see-creature hooks. A
// clan member's death makes the other want to run from the dead one as far
// as its Distance AI parameter (300), weight 10000. Seeing the character it
// attacks it (hate 200).
func TestWarriorFleeFromClanAggressiveRunsFromAFallenMember(t *testing.T) {
	t.Parallel()
	ai := npc.AIParams{"Distance": "300"}
	w := bootIndividual2e(t, raidBoot{}, ind2eTemplate(ind2eFleeID, ai, nil), ind2eTemplate(ind2eFleeTwoID, ai, nil))
	fallen := w.spawn(t, ind2eFleeID, 1500, 0)
	other := w.spawn(t, ind2eFleeTwoID, 1500, 200)
	w.clear(t, other)
	w.kill(t, fallen)
	w.srv.Settle(t)
	d, ok := desire(other, actorai.IntentionFlee)
	if !ok || d.Distance != 300 || d.Weight != 10000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != fallen.ObjectID() {
		t.Fatalf("flee desire = %+v (%v), want a 300 run from the fallen member, weight 10000", d, ok)
	}

	w.clear(t, other)
	w.moveCharacter(t, other, -50, 0)
	w.ticks(t, 1)
	if got := other.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}
}

// TestWarriorPassiveUsePowershotRunsFromACloseAttacker ports
// WarriorPassiveUsePowershot's created, attacked and timer hooks with a
// drift range of 250. A first hit from closer than 100 sets i_ai2 and, 2 s
// later (timer "100002"), makes it want to run 250 from that attacker,
// weight 10000, clearing i_ai2. A hit from 150 sets nothing. Its parent's
// attacked hook runs either way: the hit adds its Warrior hate.
func TestWarriorPassiveUsePowershotRunsFromACloseAttacker(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dist int
		runs bool
	}{{"close", 50, true}, {"150 away", 150, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2e(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithMaxDriftRange(250)}}, ind2eTemplate(ind2ePowershotID, nil, nil))
			n := w.spawn(t, ind2ePowershotID, 1500, 0)
			if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
				t.Fatalf("i_ai2 after it appeared = %d, want 0", got)
			}
			w.moveCharacter(t, n, row.dist, 0)
			w.srv.Settle(t)
			w.hit(t, n, 10)
			if got := n.AggroHate(w.player); got != 125 {
				t.Fatalf("hate after the hit = %v, want 125", got)
			}
			want := int32(0)
			if row.runs {
				want = 1
			}
			if got := ind1Slot(n, script.SlotIntAI2); got != want {
				t.Fatalf("i_ai2 after the hit = %d, want %d", got, want)
			}
			w.srv.Advance(t, 2*time.Second)
			w.srv.Settle(t)
			d, ok := desire(n, actorai.IntentionFlee)
			if !row.runs {
				if ok {
					t.Fatalf("flee desire after a hit from %d = %+v, want none", row.dist, d)
				}
				return
			}
			if !ok || d.Distance != 250 || d.Weight != 10000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
				t.Fatalf("flee desire = %+v (%v), want a 250 run from the character, weight 10000", d, ok)
			}
			if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
				t.Fatalf("i_ai2 after the run = %d, want 0", got)
			}
		})
	}
}

// ind2ePatrolTemplate is a fixture WarriorPatrolWeaponEquipped listing its
// two debuffs and its self range physical special skill by type.
func ind2ePatrolTemplate() *npc.Template {
	return ind2eTemplate(ind2ePatrolID, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeDebuff: ind2eDebuffSkill, npc.SkillTypeDebuff2: ind2eDebuff2Skill,
		npc.SkillTypeSelfRangePhysicalSpecial: ind2eRangeSkill,
	})
}

// bootArmed boots the batch beside templates with the character holding
// the fixture sword in its right hand when armed is set.
func bootArmed(t *testing.T, armed bool, templates ...*npc.Template) *ind2eWorld {
	t.Helper()
	var sword int32
	extra := raidBoot{}
	if armed {
		extra.ready = func(srv *gameservertest.Server, objID int32) { sword = srv.GiveItem(t, objID, ind2eSwordID, 1) }
	}
	w := bootIndividual2e(t, extra, templates...)
	if !armed {
		return w
	}
	use := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	use.WriteInt32(sword)
	use.WriteInt32(0)
	w.srv.Client.Send(use.Bytes())
	w.srv.AdvanceUntil(t, "the sword in the right hand", func() bool {
		var hand int32
		w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(_ *script.Script, p *script.Player) { hand = p.RightHandItemID() })
		return hand == ind2eSwordID
	})
	w.srv.ReadQueued(t, w.srv.Client)
	return w
}

// TestWarriorPatrolWeaponEquippedDebuffsTheArmed ports
// WarriorPatrolWeaponEquipped's created and see-creature hooks. Created it
// wants to wander (every 5 s, weight 5), keeps itself in c_ai0 and clears
// i_ai3. Seeing the character with a weapon in its right hand it wants its
// debuff on it (weight 1000000) and keeps it in c_ai0; an unarmed
// character is left alone.
func TestWarriorPatrolWeaponEquippedDebuffsTheArmed(t *testing.T) {
	t.Parallel()
	for _, armed := range []bool{true, false} {
		t.Run(fmt.Sprintf("armed %v", armed), func(t *testing.T) {
			t.Parallel()
			w := bootArmed(t, armed, ind2ePatrolTemplate())
			n := w.spawn(t, ind2ePatrolID, 1500, 0)
			if d, ok := desire(n, actorai.IntentionWander); !ok || d.Timer != 5 || d.Weight != 5 {
				t.Fatalf("created wander desire = %+v (%v), want every 5 s, weight 5", d, ok)
			}
			if got := slotOf(n, script.SlotCreature0); got != n.ObjectID() {
				t.Fatalf("created c_ai0 = %d, want itself %d", got, n.ObjectID())
			}
			if got := ind1Slot(n, script.SlotIntAI3); got != 0 {
				t.Fatalf("created i_ai3 = %d, want 0", got)
			}
			w.clear(t, n)
			w.moveCharacter(t, n, -50, 0)
			w.ticks(t, 1)
			me := w.player.ObjectID()
			want, keep := map[modelskill.ID]ind1Cast{}, n.ObjectID()
			if armed {
				want[ind2eDebuffSkill], keep = ind1Cast{me, 1000000}, me
			}
			w.wantCasts(t, "after it saw the character", n, want)
			if got := slotOf(n, script.SlotCreature0); got != keep {
				t.Fatalf("c_ai0 after the sight = %d, want %d", got, keep)
			}
		})
	}
}

// TestWarriorPatrolWeaponEquippedAnswersItsTopTarget ports
// WarriorPatrolWeaponEquipped's attacked hook. Hit by the character while
// it is the one it most wants to act on, each of two draws under 33 wants
// its self range physical special skill and its second debuff on it
// (weight 1000000), and the hit adds 10000 times the damage over its level
// and seven to its hate, on top of its Warrior hate (125 for a hit of 10).
// With no one to act on the hit adds only the Warrior hate.
func TestWarriorPatrolWeaponEquippedAnswersItsTopTarget(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		top    bool
		draws  []int
		casts  []modelskill.ID
		hateUp float64
	}{
		{"top, draws 0 0", true, []int{0, 0}, []modelskill.ID{ind2eRangeSkill, ind2eDebuff2Skill}, 12500},
		{"top, draws 33 32", true, []int{33, 32}, []modelskill.ID{ind2eDebuff2Skill}, 12500},
		{"top, draws 33 33", true, []int{33, 33}, nil, 12500},
		{"no top", false, []int{0, 0}, nil, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2e(t, raidBoot{}, ind2ePatrolTemplate())
			n := w.spawn(t, ind2ePatrolID, 1500, 0)
			me := w.player.ObjectID()
			if row.top {
				w.setTop(t, n, w.charOf(t))
			} else {
				w.clear(t, n)
			}
			w.draws.set(99, row.draws...)
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			for _, id := range row.casts {
				want[id] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			if got := n.AggroHate(w.player); got != 125+row.hateUp {
				t.Fatalf("hate after the hit = %v, want %v", got, 125+row.hateUp)
			}
		})
	}
}

// TestWarriorPatrolWeaponEquippedShoutsOnceAndCharges ports
// WarriorPatrolWeaponEquipped's skill finished and no-desire hooks. Its
// debuff cast on the armed character ends in wanting to attack the
// character it keeps in c_ai0 (hate 10000) and, with i_ai3 clear, in
// NpcString 1121006, setting i_ai3; with i_ai3 already set it says
// nothing. With nothing to do it clears i_ai3 again.
func TestWarriorPatrolWeaponEquippedShoutsOnceAndCharges(t *testing.T) {
	t.Parallel()
	for _, shouted := range []bool{false, true} {
		t.Run(fmt.Sprintf("shouted %v", shouted), func(t *testing.T) {
			t.Parallel()
			w := bootArmed(t, true, ind2ePatrolTemplate())
			n := w.spawn(t, ind2ePatrolID, 1500, 0)
			w.clear(t, n)
			w.moveCharacter(t, n, -50, 0)
			frames := w.ticks(t, 1)
			if !hasCast(n, ind2eDebuffSkill) {
				t.Fatal("setup: no debuff desire after the sight")
			}
			if shouted {
				w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI3, 1) })
			}
			frames = append(frames, w.ticks(t, 1)...)
			w.srv.AdvanceUntil(t, "the debuff cast ended", func() bool {
				frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
				return n.AggroHate(w.player) > 0
			})
			frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
			me := w.player.ObjectID()
			if got := castsOf(frames, n, ind2eDebuffSkill); len(got) != 1 || got[0] != me {
				t.Fatalf("debuff casts = %v, want one on the character", got)
			}
			if got := n.AggroHate(w.player); got != 10000 {
				t.Fatalf("hate after the debuff = %v, want 10000", got)
			}
			var want []string
			if !shouted {
				want = []string{script.NpcString(1121006)}
			}
			if got := says(frames, n.ObjectID()); !slices.Equal(got, want) {
				t.Fatalf("lines after the debuff = %q, want %q", got, want)
			}
			if got := ind1Slot(n, script.SlotIntAI3); got != 1 {
				t.Fatalf("i_ai3 after the debuff = %d, want 1", got)
			}
		})
	}
}

// TestWarriorPatrolWeaponEquippedForgetsItsShoutIdle ports
// WarriorPatrolWeaponEquipped's no-desire hook: with nothing to do it
// clears i_ai3 and, through its Warrior parent, wants to wander (every 5
// s, weight 5).
func TestWarriorPatrolWeaponEquippedForgetsItsShoutIdle(t *testing.T) {
	t.Parallel()
	w := bootIndividual2e(t, raidBoot{}, ind2ePatrolTemplate())
	n := w.spawn(t, ind2ePatrolID, 1500, 0)
	w.onQueue(t, n, func() {
		script.NPCOf(n).SetInt(script.SlotIntAI3, 1)
		n.AI().Desires().Clear()
	})
	// An NPC first idles on its second cycle.
	w.ticks(t, 2)
	if got := ind1Slot(n, script.SlotIntAI3); got != 0 {
		t.Fatalf("i_ai3 with nothing to do = %d, want 0", got)
	}
	if d, ok := desire(n, actorai.IntentionWander); !ok || d.Timer != 5 || d.Weight != 5 {
		t.Fatalf("wander desire with nothing to do = %+v (%v), want every 5 s, weight 5", d, ok)
	}
}

// TestWarriorPhysicalSpecialAggressiveStrikesFromAfar ports
// WarriorPhysicalSpecialAggressive's see-creature hook. Settled in its
// territory and not attacking, seeing the character it most wants to act
// on farther than 100 on a draw under 33, it wants its physical special
// skill on it (weight 1000000). A draw of 33, a character 100 or nearer,
// another to act on, or a young NPC wants nothing. Each one attacks the
// character it sees (hate 200).
func TestWarriorPhysicalSpecialAggressiveStrikesFromAfar(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		draw   int
		dist   int
		top    bool
		settle int
		strike bool
	}{
		{"draw 32", 32, 150, true, 9, true},
		{"draw 33", 33, 150, true, 9, false},
		{"100 away", 0, 100, true, 9, false},
		{"no top", 0, 150, false, 9, false},
		{"young", 0, 150, true, 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2e(t, raidBoot{}, ind2eTemplate(ind2eAggroID, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypePhysicalSpecial: ind2eSpecialSkill}))
			n := w.spawn(t, ind2eAggroID, 1500, 0)
			w.ticks(t, row.settle)
			if row.settle > 0 && (n.LifeTime() <= 7 || !script.NPCOf(n).InMyTerritory()) {
				t.Fatalf("setup: life time %d, in territory %v, want past 7 and in it", n.LifeTime(), script.NPCOf(n).InMyTerritory())
			}
			if row.top {
				w.setTop(t, n, w.charOf(t))
			} else {
				w.clear(t, n)
			}
			w.draws.set(0, row.draw)
			w.moveCharacter(t, n, -row.dist, 0)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			if row.strike {
				want[ind2eSpecialSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after it saw the character", n, want)
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
		})
	}
}

// TestWarriorPhysicalSpecialAreaAggressiveKeepsItsParentsLines ports
// WarriorPhysicalSpecialAreaAggressive's created and dying hooks, which
// switch no area and reach the Warrior's: it shouts ShoutMsg1 as it
// appears and, on a draw under 30, ShoutMsg4 as it dies.
func TestWarriorPhysicalSpecialAreaAggressiveKeepsItsParentsLines(t *testing.T) {
	t.Parallel()
	ai := npc.AIParams{"ShoutMsg1": "1000286", "ShoutMsg4": "1000287"}
	w := bootIndividual2e(t, raidBoot{}, ind2eTemplate(ind2eAreaID, ai, nil))
	s := w.spawnFrames(t, ind2eAreaID, 1500, 0)
	if got, want := says(s.frames, s.h.ObjectID()), []string{script.NpcString(1000286)}; !slices.Equal(got, want) {
		t.Fatalf("lines as it appears = %q, want %q", got, want)
	}
	w.onQueue(t, s.h, func() { script.NPCOf(s.h).DoDie(nil) })
	w.srv.Advance(t, 3500*time.Millisecond)
	if got, want := says(w.srv.ReadQueued(t, w.srv.Client), s.h.ObjectID()), []string{script.NpcString(1000287)}; !slices.Equal(got, want) {
		t.Fatalf("lines as it dies = %q, want %q", got, want)
	}
}

// TestWarriorPhysicalSpecialGatheringAggressiveCallsEveryFiveSeconds ports
// WarriorPhysicalSpecialGatheringAggressive's created and timer hooks:
// every 5 s from its creation (timer "3100") it sends script event 10019
// with its object id to the NPCs within 500, not to one 800 away.
func TestWarriorPhysicalSpecialGatheringAggressiveCallsEveryFiveSeconds(t *testing.T) {
	t.Parallel()
	w := bootIndividual2e(t, raidBoot{}, ind2eTemplate(ind2eGatherID, nil, nil), warriorTemplate(ind1ListenerID, nil))
	n := w.spawn(t, ind2eGatherID, 1500, 0)
	near := w.spawn(t, ind1ListenerID, 1500, 300)
	far := w.spawn(t, ind1ListenerID, 1500, 800)
	call := fmt.Sprintf("event=10019 arg1=%d arg2=0", n.ObjectID())
	w.srv.Advance(t, 4900*time.Millisecond)
	if got := w.heard2e.take(near.ObjectID()); len(got) != 0 {
		t.Fatalf("the near NPC heard %q before 5 s", got)
	}
	for tick := 1; tick <= 2; tick++ {
		w.srv.AdvanceUntil(t, fmt.Sprintf("call %d", tick), func() bool {
			w.heard2e.mu.Lock()
			defer w.heard2e.mu.Unlock()
			return len(w.heard2e.events[near.ObjectID()]) > 0
		})
		if got := w.heard2e.take(near.ObjectID()); !slices.Equal(got, []string{call}) {
			t.Fatalf("call %d: the near NPC heard %q, want %q", tick, got, call)
		}
		w.srv.Advance(t, 4900*time.Millisecond)
		if got := w.heard2e.take(near.ObjectID()); len(got) != 0 {
			t.Fatalf("after call %d: the near NPC heard %q within 5 s", tick, got)
		}
	}
	if got := w.heard2e.take(far.ObjectID()); len(got) != 0 {
		t.Fatalf("the NPC 800 away heard %q", got)
	}
}

// ind2ePotionTemplate is a fixture WarriorPhysicalSpecialPotionAggressive
// listing its heal and physical special skill by type, and a created shout
// no parent reaches.
func ind2ePotionTemplate() *npc.Template {
	return ind2eTemplate(ind2ePotionID, npc.AIParams{"ShoutMsg1": "1000286"}, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeMagicHeal: ind2eHealSkill, npc.SkillTypePhysicalSpecial: ind2eSpecialSkill,
	})
}

// TestWarriorPhysicalSpecialPotionAggressiveHealsByBand ports
// WarriorPhysicalSpecialPotionAggressive's created and attacked hooks.
// Created it clears i_ai1 and, reaching no parent, never shouts. Hit below
// 0.8, 0.5 or 0.33 of its hit points with i_ai1 not 3, the first band
// whose draw is under 33 wants its heal on itself (weight 1000000) and
// sets i_ai1 to 1, 2 or 3; a further draw under 33 wants its physical
// special skill on the attacker.
func TestWarriorPhysicalSpecialPotionAggressiveHealsByBand(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name    string
		share   float64
		before  int32
		draws   []int
		heal    bool
		special bool
		after   int32
	}{
		{"0.7, first band", 0.7, 0, []int{0, 99}, true, false, 1},
		{"0.7, no heal draw", 0.7, 0, []int{33, 0}, false, true, 0},
		{"0.4, second band", 0.4, 0, []int{33, 0, 99}, true, false, 2},
		{"0.3, third band", 0.3, 0, []int{33, 33, 0, 0}, true, true, 3},
		{"0.3, healed below a third", 0.3, 3, []int{0}, false, true, 3},
		{"full", 1, 0, []int{99}, false, false, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2e(t, raidBoot{}, ind2ePotionTemplate())
			s := w.spawnFrames(t, ind2ePotionID, 1500, 0)
			if got := says(s.frames, s.h.ObjectID()); len(got) != 0 {
				t.Fatalf("lines as it appears = %q, want none", got)
			}
			n := s.h
			if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
				t.Fatalf("created i_ai1 = %d, want 0", got)
			}
			w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI1, row.before) })
			w.clear(t, n)
			if row.share < 1 {
				w.setHPShare(t, n, row.share)
			}
			w.draws.set(99, row.draws...)
			w.hit(t, n, 1)
			want := map[modelskill.ID]ind1Cast{}
			if row.heal {
				want[ind2eHealSkill] = ind1Cast{n.ObjectID(), 1000000}
			}
			if row.special {
				want[ind2eSpecialSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			if got := ind1Slot(n, script.SlotIntAI1); got != row.after {
				t.Fatalf("i_ai1 after the hit = %d, want %d", got, row.after)
			}
		})
	}
}

// TestWarriorPhysicalSpecialPotionAggressiveSpeaksAfterItsHeal ports
// WarriorPhysicalSpecialPotionAggressive's skill finished hook: its heal
// cast ends in NpcString 10071 on a draw under 30, 10072 under 60, and
// 10073 otherwise.
func TestWarriorPhysicalSpecialPotionAggressiveSpeaksAfterItsHeal(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw int
		line int32
	}{{29, 10071}, {30, 10072}, {59, 10072}, {60, 10073}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2e(t, raidBoot{}, ind2ePotionTemplate())
			n := w.spawn(t, ind2ePotionID, 1500, 0)
			w.clear(t, n)
			w.setHPShare(t, n, 0.7)
			w.draws.set(row.draw, 0, 99)
			w.hit(t, n, 1)
			var frames [][]byte
			w.srv.AdvanceUntil(t, "a line after the heal", func() bool {
				frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
				return len(says(frames, n.ObjectID())) > 0
			})
			if got := castsOf(frames, n, ind2eHealSkill); len(got) != 1 || got[0] != n.ObjectID() {
				t.Fatalf("heal casts = %v, want one on itself", got)
			}
			if got, want := says(frames, n.ObjectID()), []string{script.NpcString(row.line)}; !slices.Equal(got, want) {
				t.Fatalf("lines after the heal = %q, want %q", got, want)
			}
		})
	}
}

// ind2eSpoilTemplate is a fixture WarriorPhysicalSpecialSpoilAggressive of
// base listing its effect skill and physical special skill by type, and a
// created shout no parent reaches.
func ind2eSpoilTemplate(base *npc.Template) *npc.Template {
	tmpl := ind2eTemplate(ind2eSpoilID, npc.AIParams{"ShoutMsg1": "1000286"}, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeEffectSkill: ind2eEffectSkill, npc.SkillTypePhysicalSpecial: ind2eSpecialSkill,
	})
	if base == nil {
		return tmpl
	}
	base.ID, base.TemplateID = tmpl.ID, tmpl.TemplateID
	base.AIParams, base.SkillsByType, base.Clans, base.ClanRange = tmpl.AIParams, tmpl.SkillsByType, tmpl.Clans, tmpl.ClanRange
	return base
}

// TestWarriorPhysicalSpecialSpoilAggressiveMarksItsFirstAttacker ports
// WarriorPhysicalSpecialSpoilAggressive's created and attacked hooks.
// Created it clears i_ai1 and, reaching no parent, never shouts. Its first
// hit wants its effect skill on the attacker (weight 1000000) and sets
// i_ai1; later hits do not. On any hit a draw under 33 wants its physical
// special skill on the attacker.
func TestWarriorPhysicalSpecialSpoilAggressiveMarksItsFirstAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual2e(t, raidBoot{}, ind2eSpoilTemplate(nil))
	s := w.spawnFrames(t, ind2eSpoilID, 1500, 0)
	n := s.h
	if got := says(s.frames, n.ObjectID()); len(got) != 0 {
		t.Fatalf("lines as it appears = %q, want none", got)
	}
	if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
		t.Fatalf("created i_ai1 = %d, want 0", got)
	}
	me := w.player.ObjectID()
	w.clear(t, n)
	w.draws.set(99, 33)
	w.hit(t, n, 1)
	w.wantCasts(t, "after the first hit", n, map[modelskill.ID]ind1Cast{ind2eEffectSkill: {me, 1000000}})
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after the first hit = %d, want 1", got)
	}
	w.clear(t, n)
	w.draws.set(99, 32)
	w.hit(t, n, 1)
	w.wantCasts(t, "after the second hit", n, map[modelskill.ID]ind1Cast{ind2eSpecialSkill: {me, 1000000}})
}

// TestWarriorPhysicalSpecialSpoilAggressiveSpeaksAfterItsEffect ports
// WarriorPhysicalSpecialSpoilAggressive's skill finished hook: its effect
// skill cast on the character ends in NpcString 10068 on a draw under 30,
// 10069 under 60, and 10070 otherwise.
func TestWarriorPhysicalSpecialSpoilAggressiveSpeaksAfterItsEffect(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw int
		line int32
	}{{29, 10068}, {30, 10069}, {60, 10070}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2e(t, raidBoot{}, ind2eSpoilTemplate(nil))
			n := w.spawn(t, ind2eSpoilID, 1500, 0)
			w.clear(t, n)
			w.draws.set(row.draw, 99)
			w.hit(t, n, 1)
			var frames [][]byte
			w.srv.AdvanceUntil(t, "a line after the effect skill", func() bool {
				frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
				return len(says(frames, n.ObjectID())) > 0
			})
			if got := castsOf(frames, n, ind2eEffectSkill); len(got) != 1 || got[0] != w.player.ObjectID() {
				t.Fatalf("effect skill casts = %v, want one on the character", got)
			}
			if got, want := says(frames, n.ObjectID()), []string{script.NpcString(row.line)}; !slices.Equal(got, want) {
				t.Fatalf("lines after the effect skill = %q, want %q", got, want)
			}
		})
	}
}

// TestWarriorPhysicalSpecialSpoilAggressiveGloatsOverItsKill ports
// WarriorPhysicalSpecialSpoilAggressive's attack finished hook: a swing
// that leaves the character alive ends in no line, the swing that kills it
// in NpcString 1010584.
func TestWarriorPhysicalSpecialSpoilAggressiveGloatsOverItsKill(t *testing.T) {
	t.Parallel()
	tmpl := ind2eSpoilTemplate(gameservertest.AttackingHostileTemplate())
	w := bootIndividual2e(t, raidBoot{}, tmpl)
	at := w.at
	at.X += 40
	n := w.srv.SpawnAttackingHostileNPCTemplate(t, tmpl, at)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	me := w.player.ObjectID()
	n.DoAttack(t, w.player)
	w.srv.Settle(t)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), n.ObjectID()); len(got) != 0 {
		t.Fatalf("lines after a swing the character lived through = %q, want none", got)
	}
	w.srv.DamagePlayerHP(t, me, w.srv.PlayerCurrentHP(t, me)-1)
	n.DoAttack(t, w.player)
	w.srv.Settle(t)
	if got, want := says(w.srv.ReadQueued(t, w.srv.Client), n.ObjectID()), []string{script.NpcString(1010584)}; !slices.Equal(got, want) {
		t.Fatalf("lines after the killing swing = %q, want %q", got, want)
	}
}
