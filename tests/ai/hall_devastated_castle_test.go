package ai

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/devastatedcastle"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// castleScript is the Devastated Castle's hall script. These scenarios
// bind it alone to the castle's defenders, so each NPC event runs its
// hooks and none of the defenders' behaviors.
const castleScript = "script.siegablehall.DevastatedCastle"

// bootCastleScript boots the character, of class, at castleSpot beside the
// Devastated Castle templates, standing still, with the hall script bound
// to them, every script draw draw, with extra, and raises Gustav at
// gustavSpot. It returns the world, Gustav, Dietrich, Mikhail and the
// frames Gustav's appearance sent.
func bootCastleScript(t *testing.T, class, draw int, extra raidBoot) (w *warriorWorld, gustav, dietrich, mikhail *npc.Hostile, frames [][]byte) {
	t.Helper()
	var templates []*npc.Template
	kinds := map[int32]script.NPCKind{}
	for _, id := range []int32{gustavID, dietrichID, mikhailID, doomKnightID, doomArcherID} {
		tmpl := agitTemplate(id)
		tmpl.CanMove = false
		templates = append(templates, tmpl)
		kinds[id] = script.KindHostile
	}
	extra.opts = append(extra.opts, gameservertest.WithNPCScripts(kinds,
		[]script.Listing{{Path: castleScript}}, script.Catalog{castleScript: devastatedcastle.New}))
	w = bootHalls(t, class, draw, castleSpot, extra, templates...)
	gustav, frames = w.spawnSeen(t, gustavID, gustavSpot)
	return w, gustav, privateOf(t, gustav, dietrichID), privateOf(t, gustav, mikhailID), frames
}

// TestCastleScriptRaisesGustavsGuard ports DevastatedCastle's created
// hook: Gustav shouts NpcString 1000275 and raises Mikhail, Dietrich, 15
// doom knights and 15 doom archers, each facing 32768, Mikhail and Dietrich
// where the reference places them; as they appear Mikhail shouts 1000276
// and Dietrich 1000277.
func TestCastleScriptRaisesGustavsGuard(t *testing.T) {
	t.Parallel()
	_, gustav, dietrich, mikhail, frames := bootCastleScript(t, 0, 4, raidBoot{})
	for _, c := range []struct {
		name string
		h    *npc.Hostile
		line int32
	}{{"Gustav", gustav, 1000275}, {"Mikhail", mikhail, 1000276}, {"Dietrich", dietrich, 1000277}} {
		if got := says(frames, c.h.ObjectID()); !slices.Equal(got, []string{script.NpcString(c.line)}) {
			t.Errorf("%s lines = %q, want NpcString %d", c.name, got, c.line)
		}
	}
	counts := map[int]int{}
	for _, m := range gustav.Minions() {
		h := m.(*npc.Hostile)
		counts[h.NpcID()]++
		if heading := script.NPCOf(h).SpawnLocation().Heading; heading != 32768 {
			t.Errorf("private %d faces %d, want 32768", h.NpcID(), heading)
		}
	}
	if want := map[int]int{int(mikhailID): 1, int(dietrichID): 1, int(doomKnightID): 15, int(doomArcherID): 15}; !maps.Equal(counts, want) {
		t.Fatalf("Gustav's privates = %v, want %v", counts, want)
	}
	for name, c := range map[string]struct {
		h    *npc.Hostile
		x, y int
	}{"Mikhail": {mikhail, 178304, -17712}, "Dietrich": {dietrich, 178306, -17535}} {
		if at := positionOf(c.h); at.X != c.x || at.Y != c.y {
			t.Errorf("%s stands at %v, want %d,%d", name, at, c.x, c.y)
		}
	}
}

// TestCastleScriptSendsTheDefendersHomeOnce ports the 1001 timer the
// created hook arms once, a minute after a defender appears: with Gustav
// put out of his territory, which is his lieutenants' too, each defender
// goes back, on draws of 0, to where it appeared, its hate dropped: Gustav
// returns home and the lieutenants stay on their own spawn points, not on
// him. The check runs once: displaced again, Gustav stays where he is a
// minute later.
func TestCastleScriptSendsTheDefendersHomeOnce(t *testing.T) {
	t.Parallel()
	w, gustav, dietrich, mikhail, _ := bootCastleScript(t, 0, 0, raidBoot{})
	home := map[*npc.Hostile]location.Location{gustav: positionOf(gustav), dietrich: positionOf(dietrich), mikhail: positionOf(mikhail)}
	w.hit(t, gustav, 1)
	w.teleport(t, gustav, 1000, 0)
	if positionOf(gustav) == home[gustav] {
		t.Fatal("setup: Gustav was not displaced")
	}

	w.srv.Advance(t, 60000*time.Millisecond)
	w.srv.Settle(t)
	for name, h := range map[string]*npc.Hostile{"Gustav": gustav, "Dietrich": dietrich, "Mikhail": mikhail} {
		if got := positionOf(h); got != home[h] {
			t.Errorf("%s a minute after appearing at %v, want at %v", name, got, home[h])
		}
	}
	if hate := gustav.AggroHate(w.player); hate != 0 {
		t.Fatalf("Gustav hate of the character after going home = %v, want 0", hate)
	}
	noAttackDesire(t, "Gustav", gustav)

	w.teleport(t, gustav, 1000, 0)
	displaced := positionOf(gustav)
	w.srv.Advance(t, 60000*time.Millisecond)
	w.srv.Settle(t)
	if got := positionOf(gustav); got != displaced {
		t.Fatalf("Gustav another minute later at %v, want left at %v", got, displaced)
	}
}

// TestCastleScriptPartyStrikesBack ports the party-attacked hook: on draws
// of 0 a hit on Dietrich makes Gustav want to strike the character with
// 4236 and Mikhail with 4237, each weight 1000000; Dietrich, who has seen
// no healer's spell, wants to cast nothing.
func TestCastleScriptPartyStrikesBack(t *testing.T) {
	t.Parallel()
	w, gustav, dietrich, mikhail, _ := bootCastleScript(t, 0, 0, raidBoot{})
	w.hit(t, dietrich, 1)
	wantCast(t, "Gustav", gustav, hallGustavHit, w.player, 1000000, true)
	wantCast(t, "Mikhail", mikhail, hallMikhailHit, w.player, 1000000, true)
	if d, ok := castDesireOf(dietrich, hallReuseCurse); ok {
		t.Fatalf("Dietrich with no healer seen wants to curse: %+v", d)
	}
}

// TestCastleScriptDefendersEscapeOnce ports the escape of the party
// calls: a hit's calls run before it lowers Gustav's hit points, so the
// hit leaving him at 10 of 440 sends nobody away. The next hit on the
// party finds him below 5%: Dietrich and Mikhail, watching their master,
// and Gustav, watching himself, each set their script value, want to cast
// the escape (4235) on themselves, weight 1000000, and say their line
// (NpcString 1000280, 1000279, 1000278). A further hit asks nothing again.
func TestCastleScriptDefendersEscapeOnce(t *testing.T) {
	t.Parallel()
	w, gustav, dietrich, mikhail, _ := bootCastleScript(t, 0, 2, raidBoot{})
	defenders := map[string]struct {
		h    *npc.Hostile
		line int32
	}{"Dietrich": {dietrich, 1000280}, "Mikhail": {mikhail, 1000279}, "Gustav": {gustav, 1000278}}

	w.hit(t, gustav, siegeGuardMaxHP-10)
	for name, c := range defenders {
		if _, ok := castDesireOf(c.h, hallEscape); ok || script.NPCOf(c.h).ScriptValue() != 0 {
			t.Fatalf("%s escapes on the hit that brought Gustav down", name)
		}
	}
	w.srv.ReadQueued(t, w.srv.Client)

	w.hit(t, dietrich, 1)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	for name, c := range defenders {
		wantCast(t, name, c.h, hallEscape, c.h, 1000000, true)
		if got := script.NPCOf(c.h).ScriptValue(); got != 1 {
			t.Fatalf("%s script value = %d, want 1", name, got)
		}
		if got := says(frames, c.h.ObjectID()); !slices.Equal(got, []string{script.NpcString(c.line)}) {
			t.Fatalf("%s lines = %q, want NpcString %d", name, got, c.line)
		}
	}

	w.hit(t, dietrich, 1)
	frames = w.srv.ReadQueued(t, w.srv.Client)
	for name, c := range defenders {
		if got := says(frames, c.h.ObjectID()); len(got) != 0 {
			t.Fatalf("%s says %q again", name, got)
		}
	}
}

// TestCastleScriptDietrichCursesAHealer ports the see-spell hook and
// Dietrich's party call: a Bishop's spell he sees, cast on Mikhail, makes
// the script remember the Bishop; the next hit on the party, on draws of
// 0, with the Bishop in the territory Dietrich was spawned in, makes him
// want to cast 4238 at the Bishop, weight 1000000.
func TestCastleScriptDietrichCursesAHealer(t *testing.T) {
	t.Parallel()
	w, _, dietrich, mikhail, _ := bootCastleScript(t, classBishop, 0, siegeSkillBoot(t))
	w.selectTarget(t, mikhail)
	w.cast(t, siegeBlessID)
	if d, ok := castDesireOf(dietrich, hallReuseCurse); ok {
		t.Fatalf("Dietrich wants to curse before his party is hit: %+v", d)
	}

	w.hit(t, mikhail, 1)
	if d, ok := castDesireOf(dietrich, hallReuseCurse); !ok || d.Weight != 1000000 {
		t.Fatalf("Dietrich curse desire = %+v, %v, want one of weight 1000000", d, ok)
	}
}

// TestCastleScriptEscapedLieutenantLandsInTheInnerHall ports the
// skill-finished hook, bound to the lieutenants: done casting the escape,
// Dietrich lands in the castle's inner hall with his hate dropped. Gustav,
// whose finished casts the script does not hear, stays where he cast it
// and lives on.
func TestCastleScriptEscapedLieutenantLandsInTheInnerHall(t *testing.T) {
	t.Parallel()
	w, gustav, dietrich, _, _ := bootCastleScript(t, 0, 2, raidBoot{})
	w.hit(t, dietrich, 1)
	gustavAt := positionOf(gustav)
	escape := modelskill.Ref{ID: hallEscape, Level: 1}
	for _, h := range []*npc.Hostile{dietrich, gustav} {
		w.onQueue(t, h, func() {
			n := script.NPCOf(h)
			n.AddCastDesire(n, escape, 1000000000)
		})
	}

	var frames [][]byte
	w.srv.AdvanceUntil(t, "the escapes", func() bool {
		frames = append(frames, cycle(t, w.srv)...)
		w.srv.TickPositions()
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		return positionOf(dietrich) == innerHall && len(castsOf(frames, gustav, hallEscape)) > 0 && !gustav.CastingNow()
	})
	if hate := dietrich.AggroHate(w.player); hate != 0 {
		t.Fatalf("Dietrich hate of the character after his escape = %v, want 0", hate)
	}
	w.srv.Advance(t, 3000*time.Millisecond)
	w.srv.Settle(t)
	if got := positionOf(gustav); got != gustavAt || gustav.Dead() {
		t.Fatalf("Gustav after his escape: at %v, dead %v; want at %v, alive", got, gustav.Dead(), gustavAt)
	}
}
