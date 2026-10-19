package ai

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// The Devastated Castle scenarios play amid Gustav's privates: the
// character stands at castleSpot and Gustav appears at gustavSpot.
var (
	castleSpot = location.Location{X: 178200, Y: -17700, Z: -2210}
	gustavSpot = location.Location{X: 178400, Y: -17700, Z: -2210}
	// innerHall is where an escaped defender lands.
	innerHall = location.Location{X: 177134, Y: -18807, Z: -2263}
)

// classBishop is the Bishop's class id, one Dietrich follows the spells of.
const classBishop = 16

// bootDevastatedCastle boots the character, of class, at castleSpot beside
// the Devastated Castle templates, every script draw draw, with extra, and
// raises Gustav at gustavSpot. It returns the world,
// Gustav, Dietrich, Mikhail and the frames Gustav's appearance sent.
func bootDevastatedCastle(t *testing.T, class, draw int, extra raidBoot) (w *warriorWorld, gustav, dietrich, mikhail *npc.Hostile, frames [][]byte) {
	t.Helper()
	w = bootHalls(t, class, draw, castleSpot, extra,
		agitTemplate(gustavID), agitTemplate(dietrichID), agitTemplate(mikhailID), agitTemplate(doomKnightID), agitTemplate(doomArcherID))
	gustav, frames = w.spawnSeen(t, gustavID, gustavSpot)
	return w, gustav, privateOf(t, gustav, dietrichID), privateOf(t, gustav, mikhailID), frames
}

// privateOf returns master's one private of template id.
func privateOf(t *testing.T, master *npc.Hostile, id int32) *npc.Hostile {
	t.Helper()
	var found []*npc.Hostile
	for _, m := range master.Minions() {
		if h := m.(*npc.Hostile); h.NpcID() == int(id) {
			found = append(found, h)
		}
	}
	if len(found) != 1 {
		t.Fatalf("master %d privates of %d = %d, want 1", master.NpcID(), id, len(found))
	}
	return found[0]
}

// wantHate fails unless h hates the character by about want.
func (w *warriorWorld) wantHate(t *testing.T, name string, h *npc.Hostile, want float64) {
	t.Helper()
	if got := h.AggroHate(w.player); math.Abs(got-want) > 1e-12 {
		t.Fatalf("%s hate of the character = %v, want %v", name, got, want)
	}
}

// TestGustavRaisesHisGuardAsHeAppears ports the created hooks of
// Agit02Gustav, Agit02Dietrich and Agit02Mikhail: Gustav shouts NpcString
// 1000275, takes weight point 100 and raises his 32 privates where the
// reference places them, facing 32768: Mikhail, Dietrich, 15 doom knights
// and 15 doom archers. As they appear Mikhail shouts 1000276 and Dietrich
// 1000277, and Dietrich remembers himself as the one to walk to.
func TestGustavRaisesHisGuardAsHeAppears(t *testing.T) {
	t.Parallel()
	w, gustav, dietrich, mikhail, frames := bootDevastatedCastle(t, 0, 0, raidBoot{})

	for _, c := range []struct {
		name string
		h    *npc.Hostile
		line int32
	}{{"Gustav", gustav, 1000275}, {"Mikhail", mikhail, 1000276}, {"Dietrich", dietrich, 1000277}} {
		if got := says(frames, c.h.ObjectID()); !slices.Equal(got, []string{script.NpcString(c.line)}) {
			t.Errorf("%s lines = %q, want NpcString %d", c.name, got, c.line)
		}
	}
	if got := gustav.Scratch().Int(npc.IntWeightPoint); got != 100 {
		t.Errorf("Gustav weight point = %d, want 100", got)
	}
	if c0 := script.NPCOf(dietrich).Slot(script.SlotCreature0); c0 == nil || c0.ObjectID() != dietrich.ObjectID() {
		t.Errorf("Dietrich creature slot 0 = %v, want himself", c0)
	}

	want := []string{"35409 178304,-17712,-2194", "35408 178306,-17535,-2195"}
	for _, at := range [][2]int{
		{178178, -17346},
		{178110, -17768},
		{178132, -17683},
		{178180, -17953},
		{178147, -17172},
		{178056, -17424},
		{178120, -17331},
		{178124, -17577},
		{178159, -18138},
		{178396, -18050},
		{178078, -17917},
		{178261, -17269},
		{178152, -18063},
		{178025, -17541},
		{178285, -18013},
	} {
		want = append(want, fmt.Sprintf("35416 %d,%d,-2226", at[0], at[1]))
	}
	for _, at := range [][2]int{
		{178112, -17968},
		{178079, -18092},
		{178039, -17732},
		{178224, -18025},
		{178111, -17510},
		{178088, -17620},
		{178059, -17223},
		{178043, -17325},
		{178311, -17946},
		{178170, -17885},
		{178317, -18097},
		{178099, -17823},
		{178290, -17335},
		{178116, -17386},
		{178169, -17266},
	} {
		want = append(want, fmt.Sprintf("35413 %d,%d,-2221", at[0], at[1]))
	}
	var got []string
	for _, m := range gustav.Minions() {
		h := m.(*npc.Hostile)
		at := positionOf(h)
		got = append(got, fmt.Sprintf("%d %d,%d,%d", h.NpcID(), at.X, at.Y, at.Z))
		if heading := script.NPCOf(h).SpawnLocation().Heading; heading != 32768 {
			t.Errorf("private %d at %v faces %d, want 32768", h.NpcID(), at, heading)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("Gustav's privates:\n got %q\nwant %q", got, want)
	}
	_ = w
}

// TestGustavsPartyAnswersAHit ports the party-attacked hooks of the three:
// a hit taking a twentieth of a member's hit points makes each want to
// attack the character by the damage's share of its hit points times the
// damage and the hit member's weight point, over 1000000. On draws of 0
// Gustav also wants to strike the character with 4236 and Mikhail with
// 4237, each weight 1000000; Dietrich, who remembers no healer, walks
// nowhere. The hit on Gustav (weight point 100) weighs 100 times the hit
// on Dietrich (weight point 1).
func TestGustavsPartyAnswersAHit(t *testing.T) {
	t.Parallel()
	w, gustav, dietrich, mikhail, _ := bootDevastatedCastle(t, 0, 0, raidBoot{})
	share := float64(siegeTwentieth) / float64(siegeGuardMaxHP) / 0.05 * float64(siegeTwentieth)

	w.hit(t, dietrich, siegeTwentieth)
	for name, h := range map[string]*npc.Hostile{"Gustav": gustav, "Dietrich": dietrich, "Mikhail": mikhail} {
		w.wantHate(t, name+" after the hit on Dietrich", h, share*1/1000000)
	}
	wantCast(t, "Gustav", gustav, hallGustavHit, w.player, 1000000, true)
	wantCast(t, "Mikhail", mikhail, hallMikhailHit, w.player, 1000000, true)
	if d, ok := desire(dietrich, actorai.IntentionMoveTo); ok {
		t.Fatalf("Dietrich, who remembers only himself, wants to walk: %+v", d)
	}

	w.hit(t, gustav, siegeTwentieth)
	for name, h := range map[string]*npc.Hostile{"Gustav": gustav, "Dietrich": dietrich, "Mikhail": mikhail} {
		w.wantHate(t, name+" after the hit on Gustav", h, share*1/1000000+share*100/1000000)
	}
}

// TestLieutenantsEscapeWhenGustavFalls ports the escape of Dietrich and
// Mikhail: a hit's party calls run before it lowers Gustav's hit points,
// so the hit that leaves him at 10 of 440 calls nobody away. The next hit
// on the party finds him below 5%: each lieutenant, once, wants to cast
// its escape (4235) on itself, weight 1000000000, and says its line
// (NpcString 1000280 and 1000279). Done casting it, each lands in the
// castle's inner hall with its attacks dropped.
func TestLieutenantsEscapeWhenGustavFalls(t *testing.T) {
	t.Parallel()
	w, gustav, dietrich, mikhail, _ := bootDevastatedCastle(t, 0, 2, raidBoot{})
	lieutenants := map[string]*npc.Hostile{"Dietrich": dietrich, "Mikhail": mikhail}

	w.hit(t, gustav, siegeGuardMaxHP-10)
	for name, h := range lieutenants {
		if _, ok := castDesireOf(h, hallEscape); ok || h.Scratch().Int(npc.IntAI0) != 0 {
			t.Fatalf("%s escapes on the hit that brought Gustav down", name)
		}
	}
	w.srv.ReadQueued(t, w.srv.Client)

	w.hit(t, dietrich, 1)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	for name, c := range map[string]struct {
		h    *npc.Hostile
		line int32
	}{"Dietrich": {dietrich, 1000280}, "Mikhail": {mikhail, 1000279}} {
		wantCast(t, name, c.h, hallEscape, c.h, 1000000000, true)
		if got := c.h.Scratch().Int(npc.IntAI0); got != 1 {
			t.Fatalf("%s int slot 0 = %d, want 1", name, got)
		}
		if got := says(frames, c.h.ObjectID()); !slices.Equal(got, []string{script.NpcString(c.line)}) {
			t.Fatalf("%s lines = %q, want NpcString %d", name, got, c.line)
		}
	}

	w.hit(t, dietrich, 1)
	frames = w.srv.ReadQueued(t, w.srv.Client)
	for name, h := range lieutenants {
		if d, _ := castDesireOf(h, hallEscape); d.Weight != 1000000000 {
			t.Fatalf("%s escape desire weight after a second call = %v, want it asked once", name, d.Weight)
		}
		if got := says(frames, h.ObjectID()); len(got) != 0 {
			t.Fatalf("%s says %q again", name, got)
		}
	}

	w.srv.AdvanceUntil(t, "the lieutenants' escape", func() bool {
		cycle(t, w.srv)
		w.srv.TickPositions()
		return positionOf(dietrich) == innerHall && positionOf(mikhail) == innerHall
	})
	for name, h := range lieutenants {
		noAttackDesire(t, name, h)
	}
}

// TestGustavDiesAfterHisEscape ports Agit02Gustav's skill-finished hook and
// its 1002 timer: done casting the escape, Gustav lands in the inner hall
// with his attacks dropped, and 3 s later dies, his int slot 1 set.
func TestGustavDiesAfterHisEscape(t *testing.T) {
	t.Parallel()
	w, gustav, _, _, _ := bootDevastatedCastle(t, 0, 2, raidBoot{})
	w.hit(t, gustav, 1)
	w.onQueue(t, gustav, func() {
		n := script.NPCOf(gustav)
		n.AddCastDesire(n, modelskill.Ref{ID: hallEscape, Level: 1}, 1000000000)
	})

	w.srv.AdvanceUntil(t, "Gustav's escape", func() bool {
		cycle(t, w.srv)
		w.srv.TickPositions()
		return positionOf(gustav) == innerHall
	})
	noAttackDesire(t, "Gustav", gustav)
	if gustav.Dead() {
		t.Fatal("Gustav died as he landed")
	}
	w.srv.Advance(t, 3000*time.Millisecond)
	w.srv.Settle(t)
	if !gustav.Dead() || gustav.Scratch().Int(npc.IntAI1) != 1 {
		t.Fatalf("Gustav 3 s after his escape: dead %v, int slot 1 %d, want dead and 1", gustav.Dead(), gustav.Scratch().Int(npc.IntAI1))
	}
}

// TestGustavBringsBackAFallenPrivate ports Agit02Gustav's party-died hook:
// a doom knight's death arms its respawn while Gustav's int slot 1 is
// unset, and arms none once it is 1.
func TestGustavBringsBackAFallenPrivate(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		slot1 int32
		armed bool
	}{{"unset", 0, true}, {"escaped", 1, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, gustav, _, _, _ := bootDevastatedCastle(t, 0, 2, raidBoot{})
			w.onQueue(t, gustav, func() { gustav.Scratch().SetInt(npc.IntAI1, c.slot1) })
			var knight *npc.Hostile
			for _, m := range gustav.Minions() {
				if h := m.(*npc.Hostile); h.NpcID() == int(doomKnightID) {
					knight = h
					break
				}
			}
			w.kill(t, knight)
			var armed []string
			for i := 1; i <= 64; i++ {
				if key := fmt.Sprintf("private#%d", i); w.srv.NpcRespawns.Tracked(key) {
					armed = append(armed, key)
				}
			}
			if (len(armed) == 1) != c.armed || len(armed) > 1 {
				t.Fatalf("private respawns pending = %q, want armed %v", armed, c.armed)
			}
		})
	}
}

// TestDevastatedCastleDefendersGoBack ports the 1001 checks of the three:
// at the first, 1 s after they appear, with Gustav displaced out of his
// territory, the lieutenants, whose territory is his, jump onto him and
// drop their attacks, then Gustav jumps back to where he appeared,
// dropping his. A minute later, displaced again on his own, he goes back
// again.
func TestDevastatedCastleDefendersGoBack(t *testing.T) {
	t.Parallel()
	w, gustav, dietrich, mikhail, _ := bootDevastatedCastle(t, 0, 0, raidBoot{})
	home := positionOf(gustav)
	w.hit(t, gustav, 1)
	w.teleport(t, gustav, 1000, 0)
	displaced := positionOf(gustav)

	w.srv.Advance(t, hallTimerStep)
	w.srv.Settle(t)
	for name, h := range map[string]*npc.Hostile{"Dietrich": dietrich, "Mikhail": mikhail} {
		if got := positionOf(h); got != displaced {
			t.Errorf("%s at %v, want on Gustav where he was displaced, %v", name, got, displaced)
		}
		noAttackDesire(t, name, h)
	}
	if got := positionOf(gustav); got != home {
		t.Fatalf("Gustav at %v, want back at %v", got, home)
	}
	noAttackDesire(t, "Gustav", gustav)

	w.teleport(t, gustav, 0, 1000)
	w.srv.Advance(t, 60000*time.Millisecond)
	w.srv.Settle(t)
	if got := positionOf(gustav); got != home {
		t.Fatalf("Gustav a minute later at %v, want back at %v", got, home)
	}
}

// TestDietrichWalksToAHealerAndCurses ports Agit02Dietrich's see-spell,
// party-attacked and move-finished hooks: a Bishop's good spell he sees, on
// Mikhail, makes him remember the Bishop; the next hit on his party, on a draw of 0, makes him
// want to walk to where the Bishop stands, weight 100000000. Arrived, he
// wants to cast 4238 on himself, weight 1000000, and casts it. With 4238
// in its reuse a further hit sends him nowhere.
func TestDietrichWalksToAHealerAndCurses(t *testing.T) {
	t.Parallel()
	w, _, dietrich, mikhail, _ := bootDevastatedCastle(t, classBishop, 0, siegeSkillBoot(t))
	w.selectTarget(t, mikhail)
	w.cast(t, siegeBlessID)
	if c0 := script.NPCOf(dietrich).Slot(script.SlotCreature0); c0 == nil || c0.ObjectID() != w.player.ObjectID() {
		t.Fatalf("Dietrich creature slot 0 after a Bishop's spell = %v, want the character", c0)
	}

	w.hit(t, dietrich, 1)
	x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
	bishop := location.Location{X: x, Y: y, Z: z}
	d, ok := desire(dietrich, actorai.IntentionMoveTo)
	if !ok || d.Weight != 100000000 || d.Location != bishop {
		t.Fatalf("Dietrich move desire = %+v, %v, want one to the Bishop at %v, weight 100000000", d, ok, bishop)
	}

	var frames [][]byte
	w.srv.AdvanceUntil(t, "Dietrich's walk and curse", func() bool {
		frames = append(frames, cycle(t, w.srv)...)
		w.srv.TickPositions()
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		return len(castsOf(frames, dietrich, hallReuseCurse)) > 0
	})
	if got := castsOf(frames, dietrich, hallReuseCurse); !slices.Equal(got, []int32{dietrich.ObjectID()}) {
		t.Fatalf("Dietrich casts of 4238 = %v, want one on himself", got)
	}
	w.srv.AdvanceUntil(t, "Dietrich's curse to end", func() bool {
		cycle(t, w.srv)
		return !dietrich.CastingNow()
	})

	w.hit(t, dietrich, 1)
	if d, ok := desire(dietrich, actorai.IntentionMoveTo); ok {
		t.Fatalf("Dietrich with 4238 in reuse wants to walk: %+v", d)
	}
}

// TestDefendersAttackWhoSpellsWhatTheyFight ports the see-spell hooks of
// Mikhail and Gustav: one attacking the character it wants most that sees
// the character's spell with 22 aggro points wants to attack it further
// by 22 / 440 / 0.05 * 150 = 150.
func TestDefendersAttackWhoSpellsWhatTheyFight(t *testing.T) {
	t.Parallel()
	w, gustav, _, mikhail, _ := bootDevastatedCastle(t, 0, 2, siegeSkillBoot(t))
	w.hit(t, mikhail, 1)
	w.srv.AdvanceUntil(t, "the defenders' attack", func() bool {
		cycle(t, w.srv)
		return mikhail.AI().CurrentIntention() == actorai.IntentionAttack && gustav.AI().CurrentIntention() == actorai.IntentionAttack
	})
	before := map[*npc.Hostile]float64{mikhail: mikhail.AggroHate(w.player), gustav: gustav.AggroHate(w.player)}

	w.selectTarget(t, mikhail)
	w.cast(t, siegeBoltID)
	// The bolt's own party calls add a share of its 120 under one.
	for name, h := range map[string]*npc.Hostile{"Mikhail": mikhail, "Gustav": gustav} {
		if got := h.AggroHate(w.player) - before[h]; got < 150 || got > 151 {
			t.Fatalf("%s hate gained from the spell = %v, want 150 and the bolt's party share", name, got)
		}
	}
}

// TestDietrichStaysPutWhileLockedOutOfHisSkills ports the skill lock
// Dietrich's reuse check of 4238 reads: with 4238's reuse expired but still
// tracked, a feared Dietrich that remembers a Bishop walks nowhere on his
// party's hit, as crowd control locks every skill while any reuse is
// tracked (fear, unlike a stun, leaves him free to move); not feared, the
// expired wait lets him walk to the Bishop.
func TestDietrichStaysPutWhileLockedOutOfHisSkills(t *testing.T) {
	t.Parallel()
	for _, feared := range []bool{true, false} {
		t.Run(fmt.Sprintf("feared %v", feared), func(t *testing.T) {
			t.Parallel()
			w, _, dietrich, mikhail, _ := bootDevastatedCastle(t, classBishop, 0, siegeSkillBoot(t))
			w.selectTarget(t, mikhail)
			w.cast(t, siegeBlessID)
			key := actorcast.ReuseKey(modelskill.Definition{ID: hallReuseCurse, Level: 1})
			w.onQueue(t, dietrich, func() { dietrich.DisableSkill(key, time.Millisecond) })
			w.srv.Advance(t, hallTimerStep)
			w.srv.Settle(t)
			if feared {
				e, err := effect.New(effect.Skill{ID: 9615, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Fear", Count: 1, Time: 300})
				if err != nil {
					t.Fatalf("effect.New(Fear): %v", err)
				}
				e.Effector, e.Effected = w.player.(effect.Actor), dietrich
				w.onQueue(t, dietrich, func() { dietrich.EffectList().Add(e) })
				if !dietrich.Afraid() || dietrich.MovementDisabled() {
					t.Fatalf("setup: Dietrich afraid %v, movement disabled %v, want afraid and free to move", dietrich.Afraid(), dietrich.MovementDisabled())
				}
			}

			w.hit(t, dietrich, 1)
			if d, ok := desire(dietrich, actorai.IntentionMoveTo); ok == feared {
				t.Fatalf("feared %v: Dietrich move desire = %+v, %v", feared, d, ok)
			}
		})
	}
}
