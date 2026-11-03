package ai

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The classes the heretic scenarios give the character.
const (
	ind3eFighterClass = 0  // a class of no group the heretics read
	ind3ePaladinClass = 5  // a class the heretics name by id
	ind3eClericClass  = 15 // a class of the @cleric_group
)

// heretic2Template is the fixture PartyLeaderHeretic2Aggressive of id, free
// to walk, with privates of the fixture private.
func heretic2Template(id int32, privates int) *npc.Template {
	tmpl := ind3eTemplate(id, nil, privates, false)
	tmpl.CanMove = true
	return tmpl
}

// heardFrom keeps the events of heard whose second argument is the object
// id of n: the ones n sent.
func heardFrom(heard []string, n *npc.Hostile) []string {
	var out []string
	suffix := fmt.Sprintf(" arg2=%d", n.ObjectID())
	for _, e := range heard {
		if len(e) >= len(suffix) && e[len(e)-len(suffix):] == suffix {
			out = append(out, e)
		}
	}
	return out
}

// TestPartyLeaderHeretic2AggressiveCallsHelpAndHeals ports
// PartyLeaderHeretic2Aggressive's created, see-creature and
// skill-finished hooks. It spawns with i_ai3 clear, wanting to walk back to
// its spawn point with weight 30. Seeing the character while it neither
// attacks nor casts, it broadcasts event 10033 with the character and
// itself to those within 500, attacks it with hate 200, and wants its
// special attack on it and its self heal, each weight 1000000. Once the
// self heal is cast, i_ai3 is set.
func TestPartyLeaderHeretic2AggressiveCallsHelpAndHeals(t *testing.T) {
	t.Parallel()
	w := bootIndividual3e(t, raidBoot{}, heretic2Template(ind3eHeretic2ID, 0))
	n := w.ind3eSpawn(t, ind3eHeretic2ID, 1500, 0)
	w.spawn(t, ind1ListenerID, 1500, 300)
	x, y, _ := n.Position()
	if got := ind1Slot(n, script.SlotIntAI3); got != 0 {
		t.Fatalf("created i_ai3 = %d, want 0", got)
	}
	if d, ok := desire(n, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != x || d.Location.Y != y {
		t.Fatalf("created move desire = %+v (%v), want weight 30 to %d,%d", d, ok, x, y)
	}
	w.heard.take()

	me := w.player.ObjectID()
	w.clearDesires(t, n)
	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	want := fmt.Sprintf("event=10033 arg1=%d arg2=%d", me, n.ObjectID())
	if got := heardFrom(w.heard.take(), n); !slices.Contains(got, want) {
		t.Fatalf("the listener heard %q from the heretic, want %q", got, want)
	}
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character it sees = %v, want some", got)
	}
	w.wantCasts(t, "after it saw the character", n, map[modelskill.ID]ind1Cast{ind3eSpecialAttack: {me, 1000000}, ind3eSelfHeal: {n.ObjectID(), 1000000}})
	var frames [][]byte
	for i := 0; ind1Slot(n, script.SlotIntAI3) == 0; i++ {
		if i == 10 {
			t.Fatal("the self heal never landed within 10 cycles")
		}
		frames = append(frames, w.ticks(t, 1)...)
		w.srv.Advance(t, time.Second)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	}
	if got := castsOf(frames, n, ind3eSpecialAttack); !slices.Equal(got, []int32{me}) {
		t.Fatalf("special attack MagicSkillUse targets = %v, want one on %d", got, me)
	}
	if got := castsOf(frames, n, ind3eSelfHeal); !slices.Equal(got, []int32{n.ObjectID()}) {
		t.Fatalf("self heal MagicSkillUse targets = %v, want one on itself", got)
	}
}

// TestPartyLeaderHeretic2AggressiveTellsOfItsHealers ports
// PartyLeaderHeretic2Aggressive's attacked hook. Every hit while it
// neither attacks nor casts broadcasts event 10033 with the attacker and
// itself. Under 40% of its HP, on two draws under 33, a paladin or a
// cleric hitting it is also told with event 10002; a fighter, or a first
// draw of 33, is not.
func TestPartyLeaderHeretic2AggressiveTellsOfItsHealers(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		class int
		share float64
		draws []int
		tells bool
	}{
		{"a paladin at full HP", ind3ePaladinClass, 1, nil, false},
		{"a paladin at 30%", ind3ePaladinClass, 0.3, nil, true},
		{"a cleric at 30%", ind3eClericClass, 0.3, nil, true},
		{"a fighter at 30%", ind3eFighterClass, 0.3, nil, false},
		{"a paladin at 30%, first draw 33", ind3ePaladinClass, 0.3, []int{33}, false},
		{"a paladin at 30%, second draw 33", ind3ePaladinClass, 0.3, []int{0, 33}, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := heretic2Template(ind3eHeretic2ID, 0)
			tmpl.CanMove = false
			w := bootIndividual3e(t, raidBoot{}, tmpl)
			n := w.ind3eSpawn(t, ind3eHeretic2ID, 1500, 0)
			w.spawn(t, ind1ListenerID, 1500, 300)
			w.setClass(t, w.player, row.class)
			w.setHPShare(t, n, row.share)
			w.clearDesires(t, n)
			w.heard.take()
			me := w.player.ObjectID()
			w.draws.set(0, row.draws...)
			w.hit(t, n, 1)
			want := []string{fmt.Sprintf("event=10033 arg1=%d arg2=%d", me, n.ObjectID())}
			if row.tells {
				want = append(want, fmt.Sprintf("event=10002 arg1=%d arg2=%d", me, n.ObjectID()))
			}
			if got := heardFrom(w.heard.take(), n); !slices.Equal(got, want) {
				t.Fatalf("the listener heard %q, want %q", got, want)
			}
		})
	}
}

// TestPartyLeaderHeretic2AggressiveStopsOnAnNPCAttacker ports the branch
// of PartyLeaderHeretic2Aggressive's attacked hook that reads the class of
// the player behind the attacker. Hit at 30% of its HP by an NPC, which has
// none, on a first draw of 0, it still broadcasts event 10033, then stops:
// no event 10002, and its parents never run, so PartyLeaderWarrior, with
// SummonPrivateRate 100, calls no private. On a first draw of 33 it never
// reads the class, and the parent calls its private.
func TestPartyLeaderHeretic2AggressiveStopsOnAnNPCAttacker(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name    string
		draw    int
		parents bool
	}{{"first draw 0", 0, false}, {"first draw 33", 33, true}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := heretic2Template(ind3eHeretic2ID, 1)
			tmpl.CanMove = false
			tmpl.AIParams = npc.AIParams{"SummonPrivateRate": "100"}
			attackerTmpl := warriorTemplate(ind3eSilhouetteID, nil)
			attackerTmpl.CanMove = false
			w := bootIndividual3e(t, raidBoot{}, tmpl, attackerTmpl, warriorTemplate(partyPrivateID, nil))
			n := w.ind3eSpawn(t, ind3eHeretic2ID, 1500, 0)
			attacker := w.spawn(t, ind3eSilhouetteID, 1500, -100)
			w.spawn(t, ind1ListenerID, 1500, 300)
			privatesOf(t, n, 0)
			w.setHPShare(t, n, 0.3)
			w.clearDesires(t, n)
			w.heard.take()
			w.draws.set(0, row.draw)
			w.onQueue(t, attacker, func() { n.TakeDamage(1, attacker) })
			// A called private is seen as it spawns, and called for too.
			call := fmt.Sprintf("event=10033 arg1=%d arg2=%d", attacker.ObjectID(), n.ObjectID())
			got := heardFrom(w.heard.take(), n)
			told := slices.ContainsFunc(got, func(e string) bool { return strings.HasPrefix(e, "event=10002 ") })
			if !slices.Contains(got, call) || told {
				t.Fatalf("the listener heard %q, want %q and no event 10002", got, call)
			}
			want := 0
			if row.parents {
				want = 1
			}
			privatesOf(t, n, want)
		})
	}
}

// TestPartyLeaderHeretic2AggressiveLooksAroundForItsParty ports
// PartyLeaderHeretic2Aggressive's party-attacked hook. Under 40% of its
// HP, with its self heal not yet landed, a hit on its private makes it
// look around again: it sees the character beside it anew and wants its
// special attack on it and its self heal. With i_ai3 set it does not.
func TestPartyLeaderHeretic2AggressiveLooksAroundForItsParty(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		ai3   int32
		looks bool
	}{{"its heal not landed", 0, true}, {"its heal landed", 1, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := heretic2Template(ind3eHeretic2ID, 1)
			tmpl.CanMove = false
			w := bootIndividual3e(t, raidBoot{}, tmpl, warriorTemplate(partyPrivateID, nil))
			n := w.ind3eSpawn(t, ind3eHeretic2ID, 150, 0)
			private := privatesOf(t, n, 1)[0]
			w.ticks(t, 1)
			if _, ok := castDesireOf(n, ind3eSpecialAttack); !ok {
				t.Fatal("setup: the heretic did not see the character")
			}
			w.setHPShare(t, n, 0.3)
			w.onQueue(t, n, func() {
				n.AI().Desires().Clear()
				script.NPCOf(n).SetInt(script.SlotIntAI3, row.ai3)
			})
			w.hit(t, private, 1)
			_, ok := castDesireOf(n, ind3eSpecialAttack)
			if ok != row.looks {
				t.Fatalf("special attack wanted after the hit on its private = %v, want %v", ok, row.looks)
			}
		})
	}
}

// TestPartyLeaderHeretic2AggressiveTellsOfClericSpells ports
// PartyLeaderHeretic2Aggressive's see-spell hook: under 40% of its HP, on
// a draw under 33, a cleric casting beside it is told with event 10002 to
// those within 500; a fighter casting is not.
func TestPartyLeaderHeretic2AggressiveTellsOfClericSpells(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		class int
		tells bool
	}{{"a cleric", ind3eClericClass, true}, {"a fighter", ind3eFighterClass, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := heretic2Template(ind3eHeretic2ID, 0)
			tmpl.CanMove = false
			w := bootIndividual3e(t, ind1AggroSpellBoot(t), tmpl)
			n := w.ind3eSpawn(t, ind3eHeretic2ID, 150, 0)
			w.spawn(t, ind1ListenerID, 150, 300)
			w.setClass(t, w.player, row.class)
			w.ticks(t, 1)
			w.setHPShare(t, n, 0.3)
			w.heard.take()
			w.castAggroBuff()
			w.srv.Advance(t, 2*time.Second)
			w.srv.ReadQueued(t, w.srv.Client)
			want := fmt.Sprintf("event=10002 arg1=%d arg2=%d", w.player.ObjectID(), n.ObjectID())
			if got := heardFrom(w.heard.take(), n); slices.Contains(got, want) != row.tells {
				t.Fatalf("the listener heard %q, want %q heard %v", got, want, row.tells)
			}
		})
	}
}

// TestPartyLeaderHeretic2AggressiveWalksHomeWhenIdle ports
// PartyLeaderHeretic2Aggressive's no-desire hook: left with nothing to do
// away from its spawn point, it wants to walk back there with weight 30.
func TestPartyLeaderHeretic2AggressiveWalksHomeWhenIdle(t *testing.T) {
	t.Parallel()
	w := bootIndividual3e(t, raidBoot{}, heretic2Template(ind3eHeretic2ID, 0))
	n := w.ind3eSpawn(t, ind3eHeretic2ID, 3000, 0)
	x, y, _ := n.Position()
	w.teleport(t, n, 200, 0)
	w.clearDesires(t, n)
	w.ticks(t, 2)
	if d, ok := desire(n, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != x || d.Location.Y != y {
		t.Fatalf("move desire when idle = %+v (%v), want weight 30 to %d,%d", d, ok, x, y)
	}
}

// hereticDens are two npcmakers, each spawning one heretic leader in a 400
// by 400 territory.
const hereticDens = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="heretic2_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="heretic2_den" territory="heretic2_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22159" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
	<territory name="heretic_den" minZ="-1000" maxZ="1000"><node x="1000" y="0"/><node x="1400" y="0"/><node x="1400" y="400"/><node x="1000" y="400"/></territory>
	<npcmaker name="heretic_den" territory="heretic_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22188" total="1" pos="1040;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// TestPartyLeaderHereticsHeadHomeOutOfTerritory ports the out-of-territory
// hook PartyLeaderHeretic2Aggressive and PartyLeaderHereticAggressive
// share: arriving outside its territory, a heretic leader holds no attack
// desire, wants to walk back to its spawn point with weight 100, and
// broadcasts event 10035 with itself to those within 500. The character
// stands out of the heretics' sight, so neither chases it.
func TestPartyLeaderHereticsHeadHomeOutOfTerritory(t *testing.T) {
	t.Parallel()
	two, one := heretic2Template(ind3eHeretic2Two, 0), heretic2Template(ind3eHereticID, 0)
	w := bootIndividual3e(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(ind3bSpawnTable(t, hereticDens))}}, two, one)
	ow := &ootRoamWorld{srv: w.srv}
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	if err := w.srv.State.Move(obj, 700, -1300, 30); err != nil {
		t.Fatalf("move the character: %v", err)
	}
	for _, row := range []struct {
		id   int32
		home location.Location
	}{{ind3eHeretic2Two, location.Location{X: 40, Y: 20, Z: 30}}, {ind3eHereticID, location.Location{X: 1040, Y: 20, Z: 30}}} {
		live := w.liveNPCs(row.id)
		if len(live) != 1 {
			t.Fatalf("setup: %d heretics of %d in the world, want 1", len(live), row.id)
		}
		n := live[0]
		out := location.Location{X: row.home.X, Y: 700, Z: 30}
		if _, err := w.srv.NpcSpawns.AddSpawn(ind1ListenerID, out.X, out.Y+200, out.Z, 0, false, 0); err != nil {
			t.Fatalf("spawn the listener: %v", err)
		}
		w.srv.Settle(t)
		w.clearDesires(t, n)
		w.heard.take()
		ow.walkTo(t, n, out)
		if n.InTerritory() {
			t.Fatalf("%d: the walk ended inside the territory, want outside", row.id)
		}
		if d, ok := desire(n, actorai.IntentionAttack); ok {
			t.Fatalf("%d: attack desire outside its territory = %+v, want none", row.id, d)
		}
		home := slices.ContainsFunc(n.AI().Desires().Snapshot(), func(d actorai.Desire) bool {
			return d.Kind == actorai.IntentionMoveTo && d.Weight == 100 && d.Location.X == row.home.X && d.Location.Y == row.home.Y
		})
		if !home {
			t.Fatalf("%d: desires outside its territory = %+v, want a move back to %v with weight 100", row.id, n.AI().Desires().Snapshot(), row.home)
		}
		want := []string{fmt.Sprintf("event=10035 arg1=%d arg2=0", n.ObjectID())}
		if got := w.heard.take(); !slices.Equal(got, want) {
			t.Fatalf("%d: the listener heard %q, want %q", row.id, got, want)
		}
	}
}

// TestPartyLeaderHereticAggressiveCallsHelpAndTellsItsDeath ports
// PartyLeaderHereticAggressive's created, see-creature, attacked and
// dying hooks. It spawns with i_ai3 clear. Seeing the character while it
// neither attacks nor casts, it broadcasts event 10033 with the character
// and itself, and attacks it, wanting no skill: its see-creature hook calls
// no parent. A hit while it does not fight broadcasts the same. As it dies
// it broadcasts event 2 with itself to those within 4000.
func TestPartyLeaderHereticAggressiveCallsHelpAndTellsItsDeath(t *testing.T) {
	t.Parallel()
	tmpl := ind3eTemplate(ind3eHereticID, nil, 0, false)
	w := bootIndividual3e(t, raidBoot{}, tmpl)
	n := w.ind3eSpawn(t, ind3eHereticID, 1500, 0)
	w.spawn(t, ind1ListenerID, 1500, 300)
	if got := ind1Slot(n, script.SlotIntAI3); got != 0 {
		t.Fatalf("created i_ai3 = %d, want 0", got)
	}
	me := w.player.ObjectID()
	call := fmt.Sprintf("event=10033 arg1=%d arg2=%d", me, n.ObjectID())

	w.clearDesires(t, n)
	w.heard.take()
	w.hit(t, n, 1)
	if got := heardFrom(w.heard.take(), n); !slices.Equal(got, []string{call}) {
		t.Fatalf("the listener heard %q after a hit, want %q", got, call)
	}

	w.die(t, n)
	n = w.ind3eSpawn(t, ind3eHereticID, 1500, 0)
	w.clearDesires(t, n)
	w.heard.take()
	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	call = fmt.Sprintf("event=10033 arg1=%d arg2=%d", me, n.ObjectID())
	if got := heardFrom(w.heard.take(), n); !slices.Contains(got, call) {
		t.Fatalf("the listener heard %q after it saw the character, want %q", got, call)
	}
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character it sees = %v, want some", got)
	}
	w.wantCasts(t, "after it saw the character", n, nil)

	w.heard.take()
	w.die(t, n)
	if got := w.heard.take(); !slices.Contains(got, fmt.Sprintf("event=2 arg1=0 arg2=%d", n.ObjectID())) {
		t.Fatalf("the listener heard %q as the heretic died, want event 2", got)
	}
}
