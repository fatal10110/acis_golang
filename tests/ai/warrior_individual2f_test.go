package ai

import (
	"fmt"
	"math"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the scenarios of the second individual behavior batch's
// sixth PR spawn (#187 PR 6), each a fixture monster of level 1.
const (
	ind2fRaptorID     = int32(22218) // WarriorPhysicalSpecialVelociraptor
	ind2fPowershotID  = int32(20812) // WarriorUsePowershotAggressive
	ind2fZombieID     = int32(29026) // ZakenPrivatePirateZombie
	ind2fDebuffID     = int32(21383) // WarriorPhysicalSpecialDebuffAggressive
	ind2fDebuffTwoID  = int32(21407) // WarriorPhysicalSpecialDebuffAggressive
	ind2fFollowID     = int32(21674) // WarriorPhysicalSpecialFollowing
	ind2fHoldID       = int32(22137) // WarriorPhysicalSpecialHoldAggressive
	ind2fHoldTwoID    = int32(22138) // WarriorPhysicalSpecialHoldAggressive
	ind2fKetraStoneID = int32(21328) // WarriorPhysicalSpecialStoneAggressive
	ind2fVarkaStoneID = int32(21351) // WarriorPhysicalSpecialStoneAggressive
	ind2fStrongID     = int32(22052) // WarriorPhysicalSpecialStrongAggressive
	ind2fSpecialSkill = modelskill.ID(9631)
	ind2fDebuffSkill  = modelskill.ID(9632)
	ind2fDDMagicSkill = modelskill.ID(9633)
	ind2fRangeSkill   = modelskill.ID(9634)
)

// ind2fBatch are the classes the PR ports, in scripts.xml order.
var ind2fBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialDebuffAggressive", warrior.PhysicalSpecialDebuffAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialFollowing", warrior.PhysicalSpecialFollowing},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialHoldAggressive", warrior.PhysicalSpecialHoldAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialStoneAggressive", warrior.PhysicalSpecialStoneAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialStrongAggressive", warrior.PhysicalSpecialStrongAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialVelociraptor", warrior.PhysicalSpecialVelociraptor},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorUsePowershotAggressive", warrior.UsePowershotAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.ZakenPrivatePirateZombie", warrior.ZakenPrivatePirateZombie},
}

// bootIndividual2f boots the PR's classes after the Warrior chain beside
// templates, with extra's options added last and its step run before the
// character enters the world, every draw 0 until a scenario queues others,
// every NPC knowing the batch's skills, a listener behavior on
// ind1ListenerID recording per receiving NPC, and the director script the
// shared helpers send through.
func bootIndividual2f(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind2eWorld {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind2fBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	heard := &ind2eHeard{events: map[int32][]string{}}
	const listener, director = "test.Ind2fListener", "test.Ind1Director"
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
		gameservertest.WithNpcSpawnSkills(ind2fSkills()),
		gameservertest.WithScriptSkills(ind2fSkillDefs()...),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind2eWorld{ind1World: &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}, heard2e: heard}
}

// ind2fTemplate is a fixture batch NPC of id with ai, listing skills by
// type, in the clan the scenario's NPCs share.
func ind2fTemplate(id int32, ai npc.AIParams, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for k, v := range skills {
		tmpl.SkillsByType[k] = modelskill.Ref{ID: v, Level: 1}
	}
	tmpl.Clans, tmpl.ClanRange = []string{"ind2f"}, 600
	return tmpl
}

// ind2fSkillDefs are the batch NPCs' skills: one-target strikes and a
// debuff reaching 2000.
func ind2fSkillDefs() []modelskill.Definition {
	return []modelskill.Definition{
		ind1Skill(ind2fSpecialSkill, 2000, false), ind1Skill(ind2fDebuffSkill, 2000, false),
		ind1Skill(ind2fDDMagicSkill, 2000, false), ind1Skill(ind2fRangeSkill, 2000, false),
	}
}

// ind2fSkills is ind2fSkillDefs as the table the NPCs cast from.
func ind2fSkills() *modelskill.Table { return modelskill.NewTable(ind2fSkillDefs()) }

// TestWarriorPhysicalSpecialVelociraptorCallsAfterAQuietSpell ports
// WarriorPhysicalSpecialVelociraptor's created, script event, no-desire
// and attacked hooks. Created, it notes the tick in i_ai4 and clears
// i_quest0; out of the character's sight it never wanders. Untold, it
// calls no one however long it idles. Script event 11049 sets i_quest0;
// then, with nothing to do, it sends script event 11051 (arguments 0) to
// the NPCs within 8000 once more than 600 ticks have passed since i_ai4,
// not at 600. A hit notes the tick again.
func TestWarriorPhysicalSpecialVelociraptorCallsAfterAQuietSpell(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		told  bool
		wait  time.Duration
		calls bool
	}{
		{"untold", false, 601 * time.Second, false},
		{"600 ticks", true, 600 * time.Second, false},
		{"601 ticks", true, 601 * time.Second, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			// The game ticks count the in-game clock's uptime, moved here by
			// hand.
			start := time.Now()
			var elapsed atomic.Int64
			clock := gameservertest.WithGameClockNow(func() time.Time { return start.Add(time.Duration(elapsed.Load())) })
			w := bootIndividual2f(t, raidBoot{opts: []gameservertest.Option{clock}}, ind2fTemplate(ind2fRaptorID, nil, nil), warriorTemplate(ind1ListenerID, nil))
			n := w.spawn(t, ind2fRaptorID, 1500, 0)
			listener := w.spawn(t, ind1ListenerID, 1500, 2000)
			created := ind1Slot(n, script.SlotIntAI4)
			if got := ind1Slot(n, npc.IntQuest0); got != 0 {
				t.Fatalf("created i_quest0 = %d, want 0", got)
			}
			w.ticks(t, 3)
			if d, ok := desire(n, actorai.IntentionWander); ok {
				t.Fatalf("the raptor wants to wander: %+v", d)
			}
			if row.told {
				w.sendEvent(t, n, 11049, 0, 0)
				if got := ind1Slot(n, npc.IntQuest0); got != 1 {
					t.Fatalf("i_quest0 after event 11049 = %d, want 1", got)
				}
			}
			elapsed.Add(int64(row.wait))
			w.ticks(t, 2)
			got := w.heard2e.take(listener.ObjectID())
			if !row.calls {
				if len(got) != 0 {
					t.Fatalf("the listener heard %q, want nothing", got)
				}
				return
			}
			call := "event=11051 arg1=0 arg2=0"
			if len(got) == 0 || slices.ContainsFunc(got, func(e string) bool { return e != call }) {
				t.Fatalf("the listener heard %q, want only %q", got, call)
			}
			w.hit(t, n, 10)
			if got := ind1Slot(n, script.SlotIntAI4); got != created+601 {
				t.Fatalf("i_ai4 after the hit = %d, want %d", got, created+601)
			}
		})
	}
}

// TestWarriorUsePowershotAggressiveRunsFromACloseAttacker ports
// WarriorUsePowershotAggressive's created, attacked and timer hooks with a
// drift range of 250. A first hit from closer than 100 sets i_ai2 and, 2 s
// later (timer "100002"), makes it want to run 250 from that attacker,
// weight 10000, clearing i_ai2. A hit from 150 sets nothing. Its parent's
// attacked hook runs either way: the hit adds its Warrior hate.
func TestWarriorUsePowershotAggressiveRunsFromACloseAttacker(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dist int
		runs bool
	}{{"close", 50, true}, {"150 away", 150, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual2f(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithMaxDriftRange(250)}}, ind2fTemplate(ind2fPowershotID, nil, nil))
			n := w.spawn(t, ind2fPowershotID, 1500, 0)
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

// TestZakenPrivatePirateZombieKeepsWhereItWasCreated ports
// ZakenPrivatePirateZombie's created hook: it keeps the x, y and z it was
// created at in its weight point, respawn time and flag and, calling no
// parent, never shouts as a created Warrior does.
func TestZakenPrivatePirateZombieKeepsWhereItWasCreated(t *testing.T) {
	t.Parallel()
	shout := npc.AIParams{"ShoutMsg1": "1000286"}
	w := bootIndividual2f(t, raidBoot{}, ind2fTemplate(ind2fZombieID, shout, nil))
	d := w.spawnFrames(t, ind2fZombieID, 700, 40)
	if got := says(d.frames, d.h.ObjectID()); len(got) != 0 {
		t.Fatalf("pirate zombie created shouts = %q, want none", got)
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

// TestZakenPrivatePirateZombieSeesOnlyItsHeight ports
// ZakenPrivatePirateZombie's see-creature hook. It notices the character
// only less than 100 above or below it: in its territory it wants to
// attack it (hate 200), then runs its own attacked hook as if the
// character had hit it for 9, which hates by that damage. At 100 above or
// below it notices nothing.
func TestZakenPrivatePirateZombieSeesOnlyItsHeight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		dz     int
		notice bool
	}{{0, true}, {-99, true}, {100, false}} {
		t.Run(fmt.Sprintf("dz %d", row.dz), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2fTemplate(ind2fZombieID, nil, nil)
			tmpl.Level, tmpl.CanMove = 15, false
			w := bootIndividual2f(t, raidBoot{}, tmpl)
			n := w.spawn(t, ind2fZombieID, 1500, 0)
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
				want = 200 + 9.0/float64(15+7)*100
			}
			if got := n.AggroHate(w.player); math.Abs(got-want) > 1e-9 {
				t.Fatalf("hate of the character %d above = %v, want %v", row.dz, got, want)
			}
		})
	}
}

// TestZakenPrivatePirateZombieCursesAndMutesOverLevelled ports
// ZakenPrivatePirateZombie's attacked and see-spell hooks. Against the
// level 20 character, a level 11 zombie (9 levels below, more than 8)
// wants its raid curse on the character when hit and its raid mute,
// instead of its parent's answer, when it sees the character cast, each
// weight 1000000; a level 12 one (8 levels below) wants neither. Standing
// 150 below the character it never notices it by sight.
func TestZakenPrivatePirateZombieCursesAndMutesOverLevelled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		curse bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind2fTemplate(ind2fZombieID, nil, nil)
			tmpl.Level, tmpl.CanMove = row.level, false
			w := bootIndividual2f(t, raidSpellBoot(t), tmpl)
			n := w.spawnBelow(t, ind2fZombieID, 400, 0, 150)
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

// TestWarriorPhysicalSpecialFollowingWalksToTheCaller ports
// WarriorPhysicalSpecialFollowing's script event hook. Event 10019 naming
// the character's object id, with draws 10 and 90, makes it want to walk
// to the character's x - 50 + 10, y - 50 + 90 and z, weight 15. The same
// event naming no object wants nothing, and neither does another event.
func TestWarriorPhysicalSpecialFollowingWalksToTheCaller(t *testing.T) {
	t.Parallel()
	w := bootIndividual2f(t, raidBoot{}, ind2fTemplate(ind2fFollowID, nil, nil))
	n := w.spawn(t, ind2fFollowID, 1500, 0)
	px, py, pz := w.srv.PlayerPosition(t, w.player.ObjectID())

	w.clear(t, n)
	w.sendEvent(t, n, 10019, 999999999, 0)
	w.sendEvent(t, n, 10020, w.player.ObjectID(), 0)
	if d, ok := desire(n, actorai.IntentionMoveTo); ok {
		t.Fatalf("move desire after an unknown caller and another event = %+v, want none", d)
	}

	w.draws.set(0, 10, 90)
	w.sendEvent(t, n, 10019, w.player.ObjectID(), 0)
	d, ok := desire(n, actorai.IntentionMoveTo)
	if !ok || d.Weight != 15 || d.Location.X != px-40 || d.Location.Y != py+40 || d.Location.Z != pz {
		t.Fatalf("move desire = %+v (%v), want weight 15 to %d,%d,%d", d, ok, px-40, py+40, pz)
	}
}
