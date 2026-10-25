package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// hpPriestID is a WarriorHereticPriestAggressive NPC, Triol's Layperson.
const hpPriestID = int32(22142)

// The skills the fixture heretic priest lists by type.
const (
	hpSpecialID  = modelskill.ID(9481) // PHYSICAL_SPECIAL
	hpSpecial2ID = modelskill.ID(9482) // PHYSICAL_SPECIAL2
	hpHoldID     = modelskill.ID(9483) // HOLD_MAGIC
)

// bootHereticPriest boots WarriorHereticPriestAggressive after the Warrior
// chain beside templates, with extra, the event listener on
// plwListenerID, the director script charOf runs, every NPC knowing the priest's skills, every draw 0 until
// a scenario queues others.
func bootHereticPriest(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	const priest, listener = "script.ai.individual.Monster.WarriorBase.Warrior.WarriorHereticPriestAggressive", "ai.EventListener"
	const director = "test.Ind1Director"
	catalog[priest], catalog[listener] = warrior.HereticPriestAggressive, eventListener
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: priest}, script.Listing{Path: listener}, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable([]modelskill.Definition{
			ind1Skill(hpSpecialID, 900, false), ind1Skill(hpSpecial2ID, 900, false), ind1Skill(hpHoldID, 900, false),
		})),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// hpTemplate is a fixture heretic priest with ai as its AI parameters,
// listing its three skills by type, in the clan the scenario's priests
// share, standing still unless walk is set.
func hpTemplate(ai npc.AIParams, walk bool) *npc.Template {
	tmpl := warriorTemplate(hpPriestID, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypePhysicalSpecial:  {ID: hpSpecialID, Level: 1},
		npc.SkillTypePhysicalSpecial2: {ID: hpSpecial2ID, Level: 1},
		npc.SkillTypeHoldMagic:        {ID: hpHoldID, Level: 1},
	}
	tmpl.Clans, tmpl.ClanRange = []string{"heretic_priest"}, 600
	tmpl.CanMove = walk
	return tmpl
}

// hpLookAt moves the character 100 in front of h and runs one AI cycle,
// whose see-creature scan notices it, and returns the frames the
// character was sent until the server settled.
func (w *ind1World) hpLookAt(t *testing.T, h *npc.Hostile) [][]byte {
	t.Helper()
	w.moveNear(t, w.player, h, -100)
	frames := w.ticks(t, 1)
	w.srv.Settle(t)
	return append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
}

// hpWantHeld fails unless every queued cast desire of h is held in place
// when held is set, and closes in on its target otherwise.
func hpWantHeld(t *testing.T, when string, h *npc.Hostile, held bool) {
	t.Helper()
	for id, d := range ind1Casts(h) {
		if d.MoveToTarget == held {
			t.Errorf("%s: cast desire of skill %d closes in = %v, want %v", when, id, d.MoveToTarget, !held)
		}
	}
}

// TestHereticPriestKeepsHomeAndCallsForHelp ports the priest's created,
// attacked and no-desire hooks, every draw 0. As it spawns it wants to
// walk to its spawn point, weight 30, and its parent's created hook does
// not run. A hit while it neither attacks nor casts broadcasts event 10033
// naming the attacker. Moved 200 away with nothing left to do, its
// no-desire hook wants it back at its spawn point, weight 30, where its
// parent would wander.
func TestHereticPriestKeepsHomeAndCallsForHelp(t *testing.T) {
	t.Parallel()
	w := bootHereticPriest(t, raidBoot{}, hpTemplate(nil, true), warriorTemplate(plwListenerID, nil))
	h := w.spawn(t, hpPriestID, 1500, 0)
	listener := w.spawn(t, plwListenerID, 1500, 100)
	home := script.NPCOf(h).SpawnLocation()
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire after spawning = %+v (%v), want weight 30 to %d,%d", d, ok, home.X, home.Y)
	}

	w.hit(t, h, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), listener.ObjectID()); !slices.Contains(got, fmt.Sprintf("event 10033 %d", w.player.ObjectID())) {
		t.Fatalf("events heard after an idle hit = %q, want event 10033 naming the character", got)
	}

	w.teleport(t, h, 200, 0)
	w.onQueue(t, h, func() {
		script.NPCOf(h).RemoveAllAttackDesire()
		h.AI().Desires().Clear()
	})
	w.ticks(t, 2)
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire with nothing to do = %+v (%v), want weight 30 to %d,%d", d, ok, home.X, home.Y)
	}
	if d, ok := desire(h, actorai.IntentionWander); ok {
		t.Fatalf("wander desire with nothing to do = %+v, want none", d)
	}
}

// TestHereticPriestStrikesItsTopAttacker ports the priest's attacked hook,
// every draw 0. Hit by the creature it most wants to attack, it wants its
// first physical special on it within 40 and its second one under 500,
// weight 1000000; from 500 on it wants neither. Free to move (Hold 0) its
// casts close in and its parent's hook adds the hit's hate; holding its
// ground (Hold 1) its casts are held, its parent's hook does not run, and
// it wants to attack the attacker from where it stands, weight 100.
func TestHereticPriestStrikesItsTopAttacker(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		hold  bool
		dx    int
		skill modelskill.ID
	}{
		{"free within 40", false, 30, hpSpecialID},
		{"free under 500", false, 300, hpSpecial2ID},
		{"free from 500", false, 600, 0},
		{"held within 40", true, 30, hpSpecialID},
		{"held under 500", true, 300, hpSpecial2ID},
		{"held from 500", true, 600, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			var ai npc.AIParams
			if row.hold {
				ai = npc.AIParams{"Hold": "1"}
			}
			// In no clan, the hit raises no clan call on the priest itself.
			tmpl := hpTemplate(ai, false)
			tmpl.Clans = nil
			w := bootHereticPriest(t, raidBoot{}, tmpl)
			h := w.spawn(t, hpPriestID, 1500, 0)
			me := w.player.ObjectID()
			w.moveNear(t, w.player, h, row.dx)
			w.setTop(t, h, w.charOf(t))

			w.hit(t, h, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.skill != 0 {
				want[row.skill] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hit", h, want)
			hpWantHeld(t, "after the hit", h, row.hold)
			hate := h.AggroHate(w.player)
			if row.hold {
				if hate != 100 {
					t.Errorf("hate after a held hit = %v, want 100", hate)
				}
				if d, ok := desire(h, actorai.IntentionAttack); !ok || d.MoveToTarget || d.FinalTarget.ObjectID() != me {
					t.Errorf("attack desire after a held hit = %+v (%v), want one on the character held in place", d, ok)
				}
			} else if hate <= 0 {
				t.Errorf("hate after a free hit = %v, want the parent's damage hate", hate)
			}
		})
	}
}

// TestHereticPriestAnswersItsClan ports the priest's clan-attacked hook,
// every draw 0, on a priest 300 from the character whose clan member the
// character hits. Free to move with no creature it most wants to attack,
// it wants its second physical special on the character, closing in, and
// both its parent's hate and its own held attack desire. Holding its
// ground once settled (8 AI cycles) and idle, it wants the same skill held
// in place and to attack the character from where it stands.
func TestHereticPriestAnswersItsClan(t *testing.T) {
	t.Parallel()
	for _, hold := range []bool{false, true} {
		t.Run(fmt.Sprintf("hold %v", hold), func(t *testing.T) {
			t.Parallel()
			var ai npc.AIParams
			if hold {
				ai = npc.AIParams{"Hold": "1"}
			}
			w := bootHereticPriest(t, raidBoot{}, hpTemplate(ai, false))
			caller := w.spawn(t, hpPriestID, 1500, 0)
			called := w.spawn(t, hpPriestID, 1500, 300)
			me := w.player.ObjectID()
			w.ticks(t, 9)
			if fightingIntention(called) {
				t.Fatalf("setup: called priest intention = %v, want neither attack nor cast", called.AI().CurrentIntention())
			}
			// The character steps 300 from the called priest and hits its
			// clan member before either looks around.
			w.moveNear(t, w.player, called, 300)

			w.hit(t, caller, 10)
			w.wantCasts(t, "after the hit on its clan member", called, map[modelskill.ID]ind1Cast{hpSpecial2ID: {me, 1000000}})
			hpWantHeld(t, "after the hit on its clan member", called, hold)
			d, ok := desire(called, actorai.IntentionAttack)
			if !ok || d.FinalTarget.ObjectID() != me {
				t.Fatalf("attack desire after the hit on its clan member = %+v (%v), want one on the character", d, ok)
			}
			if hold && d.MoveToTarget {
				t.Errorf("held priest's attack desire closes in, want it held in place")
			}
		})
	}
}

// TestHereticPriestSeesAndBindsHealers ports the priest's see-creature
// hook for a character in no party. Seeing the character 100 away while
// idle it broadcasts event 10033 naming it, wants its second physical
// special on it held in place, weight 1000000, and to attack it, weight
// 200. On a draw under 33 a paladin (5), a phoenix knight (90) or a cleric
// also draws its hold magic, held, and its second physical special again,
// closing in; a warrior (1), or a cleric on a draw of 33, does not.
func TestHereticPriestSeesAndBindsHealers(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		class int
		draw  int
		bind  bool
	}{{1, 0, false}, {5, 0, true}, {90, 0, true}, {15, 0, true}, {15, 33, false}} {
		t.Run(fmt.Sprintf("class %d draw %d", row.class, row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootHereticPriest(t, raidBoot{}, hpTemplate(nil, false), warriorTemplate(plwListenerID, nil))
			h := w.spawn(t, hpPriestID, 1500, 0)
			listener := w.spawn(t, plwListenerID, 1500, 100)
			me := w.player.ObjectID()
			w.setClass(t, w.player, row.class)
			w.srv.ReadQueued(t, w.srv.Client)
			w.draws.set(row.draw)

			if got := says(w.hpLookAt(t, h), listener.ObjectID()); !slices.Contains(got, fmt.Sprintf("event 10033 %d", me)) {
				t.Fatalf("events heard after it saw the character = %q, want event 10033 naming it", got)
			}
			want := map[modelskill.ID]ind1Cast{hpSpecial2ID: {me, 1000000}}
			if row.bind {
				want[hpSpecial2ID] = ind1Cast{me, 2000000}
				want[hpHoldID] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after it saw the character", h, want)
			hpWantHeld(t, "after it saw the character", h, true)
			if got := h.AggroHate(w.player); got != 200 {
				t.Fatalf("hate after it saw the character = %v, want 200", got)
			}
		})
	}
}

// hpTelPos are the priest's three teleport points, relative to the
// character's start.
var hpTelPos = [3][2]int{{3000, 0}, {0, 3000}, {-3000, 0}}

// hpTeleportParams are AI parameters naming hpTelPos around at, with
// partyMembers and noPCTeleport.
func hpTeleportParams(at location.Location, partyMembers, noPCTeleport int) npc.AIParams {
	ai := npc.AIParams{"party_members": fmt.Sprint(partyMembers), "NoPCTeleport": fmt.Sprint(noPCTeleport)}
	for i, d := range hpTelPos {
		ai[fmt.Sprintf("TelPosX%d", i+1)] = fmt.Sprint(at.X + d[0])
		ai[fmt.Sprintf("TelPosY%d", i+1)] = fmt.Sprint(at.Y + d[1])
		ai[fmt.Sprintf("TelPosZ%d", i+1)] = fmt.Sprint(at.Z)
	}
	return ai
}

// TestHereticPriestSendsAPartyPlayerAway ports the see-creature hook's
// teleport. Seeing a player of a party larger than its party_members
// parameter in its spawn's territory, on a draw under 33 it sends the
// player to its first, second or third teleport point as a draw of 3
// picks: the character sees one ordinary TeleportToLocation of itself
// there and stands there. A player alone, a party no larger than
// party_members, or NoPCTeleport set keeps the player where it is.
func TestHereticPriestSendsAPartyPlayerAway(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name         string
		party        bool
		partyMembers int
		noPCTeleport int
		draw         int
		point        int
	}{
		{"first point", true, 1, 0, 0, 0},
		{"second point", true, 1, 0, 1, 1},
		{"third point", true, 1, 0, 2, 2},
		{"alone", false, 0, 0, 0, -1},
		{"party not larger", true, 2, 0, 0, -1},
		{"no PC teleport", true, 1, 1, 0, -1},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := hpTemplate(nil, false)
			w := bootHereticPriest(t, raidBoot{}, tmpl)
			// The points lie around the character, whose place the boot
			// decides; the priest spawns after they are set.
			tmpl.AIParams = hpTeleportParams(w.at, row.partyMembers, row.noPCTeleport)
			if row.party {
				w.joinMate(t)
			}
			h := w.spawn(t, hpPriestID, 1500, 0)
			me := w.player.ObjectID()
			w.draws.set(row.draw)

			frames := w.hpLookAt(t, h)
			jumps := framesOf(frames, serverpackets.OpcodeTeleportToLocation, me)
			x, y, _ := w.srv.PlayerPosition(t, me)
			if row.point < 0 {
				if len(jumps) != 0 {
					t.Fatalf("TeleportToLocation frames of the character = %d, want none", len(jumps))
				}
				return
			}
			want := location.Location{X: w.at.X + hpTelPos[row.point][0], Y: w.at.Y + hpTelPos[row.point][1]}
			if len(jumps) != 1 {
				t.Fatalf("TeleportToLocation frames of the character = %d, want 1", len(jumps))
			}
			if at, fast := teleportFrame(jumps[0]); at.X != want.X || at.Y != want.Y || fast {
				t.Fatalf("TeleportToLocation = %v fast=%v, want an ordinary one to %d,%d", at, fast, want.X, want.Y)
			}
			if x != want.X || y != want.Y {
				t.Fatalf("character at %d,%d, want %d,%d", x, y, want.X, want.Y)
			}
		})
	}
}

// TestHereticPriestTurnsOnWhatItCastAt ports the priest's skill-finished
// hook: once its cast on the character lands, it wants to attack the
// character 100 more, closing in when free to move (Hold 0) and held in
// place when holding its ground (Hold 1).
func TestHereticPriestTurnsOnWhatItCastAt(t *testing.T) {
	t.Parallel()
	for _, hold := range []bool{false, true} {
		t.Run(fmt.Sprintf("hold %v", hold), func(t *testing.T) {
			t.Parallel()
			var ai npc.AIParams
			if hold {
				ai = npc.AIParams{"Hold": "1"}
			}
			w := bootHereticPriest(t, raidBoot{}, hpTemplate(ai, false))
			h := w.spawn(t, hpPriestID, 1500, 0)
			me := w.player.ObjectID()
			// Seeing the character it wants its second physical special
			// on it and to attack it, weight 200.
			frames := w.hpLookAt(t, h)
			before, ok := desire(h, actorai.IntentionAttack)
			if !ok {
				t.Fatal("setup: no attack desire after it saw the character")
			}
			for i := 0; len(castsOf(frames, h, hpSpecial2ID)) == 0; i++ {
				if i == 40 {
					t.Fatal("no cast of the special within 40 AI cycles")
				}
				frames = append(frames, w.ticks(t, 1)...)
				w.srv.Advance(t, 500*time.Millisecond)
				frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
			}
			w.srv.Advance(t, 2*time.Second)
			if got := castsOf(frames, h, hpSpecial2ID); got[0] != me {
				t.Fatalf("casts of the special = %v, want one on the character", got)
			}
			d, ok := desire(h, actorai.IntentionAttack)
			if !ok || d.FinalTarget.ObjectID() != me || d.Weight != before.Weight+100 {
				t.Fatalf("attack desire after the cast = %+v (%v), want one on the character at %v", d, ok, before.Weight+100)
			}
			if d.MoveToTarget == hold {
				t.Fatalf("attack desire after the cast closes in = %v, want %v", d.MoveToTarget, !hold)
			}
		})
	}
}

// hpDen is an npcmaker spawning one heretic priest in a 400 by 400
// territory.
const hpDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="heretic_priest_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="heretic_priest_den" territory="heretic_priest_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22142" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// TestHereticPriestWalksHomeFromOutside ports the priest's
// out-of-territory hook on a priest its npcmaker spawned. Free to move
// (Hold 0), arriving outside its territory it drops its attack desires, wants
// to walk to its spawn point with weight 100 more (on top of the 30 its
// created hook queued) and broadcasts event 10035 with argument 0.
// Holding its ground (Hold 1) it does none of that.
func TestHereticPriestWalksHomeFromOutside(t *testing.T) {
	t.Parallel()
	for _, hold := range []bool{false, true} {
		t.Run(fmt.Sprintf("hold %v", hold), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(hpDen), 0o644); err != nil {
				t.Fatalf("write spawnlist: %v", err)
			}
			table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				t.Fatalf("load spawnlist: %v", err)
			}
			var ai npc.AIParams
			if hold {
				ai = npc.AIParams{"Hold": "1"}
			}
			w := bootHereticPriest(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}}, hpTemplate(ai, true), warriorTemplate(plwListenerID, nil))
			var h *npc.Hostile
			for _, obj := range w.srv.State.Objects() {
				if o, ok := obj.(*npc.Hostile); ok && o.NpcID() == int(hpPriestID) {
					h = o
				}
			}
			if h == nil {
				t.Fatal("the den spawned no priest")
			}
			n, err := w.srv.NpcSpawns.AddSpawn(plwListenerID, 40, 800, 30, 0, false, 0)
			if err != nil {
				t.Fatalf("spawn the listener: %v", err)
			}
			w.srv.Settle(t)
			w.srv.ReadQueued(t, w.srv.Client)
			home := script.NPCOf(h).SpawnLocation()
			w.onQueue(t, h, func() { h.AI().Hates().Add(w.player, 100) })

			to := location.Location{X: 40, Y: 700, Z: 30}
			if !h.AI().AddMoveToDesire(to, 1e6) {
				t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", to)
			}
			for i := 0; !h.IsMoving(); i++ {
				if i == 5 {
					t.Fatalf("no walk to %+v after five cycles", to)
				}
				cycle(t, w.srv)
			}
			for i := 0; h.IsMoving(); i++ {
				if i >= int(20*time.Second/move.PositionUpdateInterval) {
					t.Fatal("walk never arrived")
				}
				w.srv.TickPositions()
			}
			w.srv.Settle(t)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			if h.InTerritory() {
				t.Fatal("setup: the walk ended inside the territory")
			}
			heard := slices.Contains(says(frames, n.ObjectID()), "event 10035 0")
			d, ok := desire(h, actorai.IntentionMoveTo)
			homeward := ok && d.Location.X == home.X && d.Location.Y == home.Y
			if hold {
				if heard || h.AI().Hates().Hate(w.player) != 100 || (homeward && d.Weight != 30) {
					t.Fatalf("held priest outside: heard 10035 %v, hate %v, move %+v (%v); want nothing changed", heard, h.AI().Hates().Hate(w.player), d, ok)
				}
				return
			}
			if !homeward || d.Weight != 30+100 {
				t.Fatalf("move desire after arriving outside = %+v (%v), want its walk home (30) raised by 100 to %d,%d", d, ok, home.X, home.Y)
			}
			if d, ok := desire(h, actorai.IntentionAttack); ok {
				t.Fatalf("attack desire after leaving its territory = %+v, want none", d)
			}
			if !heard {
				t.Fatalf("events heard = %q, want event 10035 0", says(frames, n.ObjectID()))
			}
		})
	}
}

// TestHereticPriestHeldDropsAFarTopForItsClanAttacker ports the third
// branch of the priest's clan-attacked hook, every draw 0: a freshly
// spawned priest holding its ground (Hold 1, life time 7 or less) whose
// top desire target is another creature farther than 500 drops its attack
// desire on that creature and wants its second physical special, held in
// place, on its clan member's attacker 300 away. With that creature at
// exactly 500 it keeps its desire and casts nothing. Either way it wants
// to attack the attacker from where it stands, weight 100.
func TestHereticPriestHeldDropsAFarTopForItsClanAttacker(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		dx   int
		drop bool
	}{{700, true}, {500, false}} {
		t.Run(fmt.Sprintf("top %d away", row.dx), func(t *testing.T) {
			t.Parallel()
			w := bootHereticPriest(t, raidBoot{}, hpTemplate(npc.AIParams{"Hold": "1"}, false), warriorTemplate(plwListenerID, nil))
			caller := w.spawn(t, hpPriestID, 1500, 0)
			called := w.spawn(t, hpPriestID, 1500, 300)
			far := w.spawn(t, plwListenerID, 1500+row.dx, 300)
			me := w.player.ObjectID()
			if got := script.NPCOf(called).LifeTime(); got > 7 {
				t.Fatalf("setup: called priest life time = %d, want 7 or less", got)
			}
			w.onQueue(t, called, func() {
				n := script.NPCOf(called)
				called.AI().Desires().Clear()
				n.AddAttackDesireHold(script.NPCOf(far), 50)
				n.SetTopDesireTarget(script.NPCOf(far))
			})
			w.moveNear(t, w.player, called, 300)

			w.hit(t, caller, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.drop {
				want[hpSpecial2ID] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hit on its clan member", called, want)
			hpWantHeld(t, "after the hit on its clan member", called, true)
			var onFar, onMe *actorai.Desire
			for _, d := range called.AI().Desires().Snapshot() {
				if d.Kind != actorai.IntentionAttack || d.FinalTarget == nil {
					continue
				}
				switch d.FinalTarget.ObjectID() {
				case far.ObjectID():
					onFar = &d
				case me:
					onMe = &d
				}
			}
			if row.drop && onFar != nil {
				t.Errorf("attack desire on the far top = %+v, want it dropped", *onFar)
			}
			if !row.drop && (onFar == nil || onFar.Weight != 50) {
				t.Errorf("attack desire on the top at 500 = %+v, want it kept at 50", onFar)
			}
			if onMe == nil || onMe.Weight != 100 || onMe.MoveToTarget {
				t.Errorf("attack desire on the attacker = %+v, want weight 100 held in place", onMe)
			}
		})
	}
}
