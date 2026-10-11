package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/guard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The guard ids the scenarios spawn: town guards of GuardStand and
// GuardMoveAround, and the civilian ones the datapack types as folk.
const (
	standID          = int32(30042) // GuardStand
	standTwoID       = int32(30043) // GuardStand
	standFolkID      = int32(31677) // GuardStand, folk-typed
	standFolkTwoID   = int32(31678) // GuardStand, folk-typed
	moveAroundID     = int32(31844) // GuardMoveAround
	moveAroundFolkID = int32(31853) // GuardMoveAround, folk-typed
)

// guardScripts are the guard behaviors and the class they derive from, in
// scripts.xml order.
func guardScripts() ([]script.Listing, script.Catalog) {
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Guard.Guard",
		"script.ai.individual.Guard.GuardMoveAround",
		"script.ai.individual.Guard.GuardStand",
	}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New,
		paths[1]: guard.Guard,
		paths[2]: guard.MoveAround,
		paths[3]: guard.Stand,
	}
	list := make([]script.Listing, len(paths))
	for i, path := range paths {
		list[i] = script.Listing{Path: path}
	}
	return list, catalog
}

// guardTemplate is a level 1 fixture town guard of id with ai as its AI
// parameters.
func guardTemplate(id int32, ai npc.AIParams) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.Type, tmpl.Name = "Guard", "Guard"
	return tmpl
}

// folkGuardTemplate is the fixture civilian guard of id that fights, with
// ai as its AI parameters.
func folkGuardTemplate(id int32, ai npc.AIParams) *npc.Template {
	tmpl := folkFighterTemplate(id)
	tmpl.AIParams = ai
	return tmpl
}

// bootGuards boots one character in the world beside the guard templates,
// every script draw 0.
func bootGuards(t *testing.T, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	list, catalog := guardScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
		if tmpl.Type == "Folk" {
			kinds[int32(tmpl.ID)] = script.KindFolk
		}
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	)
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	return &warriorWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, player: obj.(attackable.Combatant)}
}

// spawnFolk places a civilian NPC of id dx, dy from the character, through
// the live population, so its created hooks run.
func (w *warriorWorld) spawnFolk(t *testing.T, id int32, dx, dy int) *npc.Folk {
	t.Helper()
	n, err := w.srv.NpcSpawns.AddSpawn(id, w.at.X+dx, w.at.Y+dy, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", id, err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	return n.(*npc.Folk)
}

// onFolkQueue runs fn on f's queue and waits for it.
func (w *warriorWorld) onFolkQueue(t *testing.T, f *npc.Folk, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !f.Queue().Post(func() {
		defer close(done)
		fn()
	}) {
		t.Fatal("the npc's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// setKarma gives the character karma.
func (w *warriorWorld) setKarma(t *testing.T, karma int) {
	t.Helper()
	w.player.(player.CharacterHolder).PlayerCharacter().SetKarma(karma)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// position returns where c stands.
func position(c attackable.Combatant) location.Location {
	x, y, z := c.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// TestGuardAttacksKarmaPlayersOnSight ports Guard's see-creature hook on a
// town guard and a civilian one: a character with no karma walks past them
// untouched; once it has karma, the guards that see it next want to attack
// it with weight 1500, and attack.
func TestGuardAttacksKarmaPlayersOnSight(t *testing.T) {
	t.Parallel()
	w := bootGuards(t, guardTemplate(standID, nil), guardTemplate(standTwoID, nil),
		folkGuardTemplate(standFolkID, nil), folkGuardTemplate(standFolkTwoID, nil))
	calm := w.spawn(t, standID, 150, 0)
	calmFolk := w.spawnFolk(t, standFolkID, -150, 0)
	w.ticks(t, 2)
	if got := calm.AggroHate(w.player); got != 0 {
		t.Fatalf("town guard hate of a character with no karma = %v, want none", got)
	}
	if got := calm.AI().CurrentIntention(); got == actorai.IntentionAttack {
		t.Fatal("the town guard attacks a character with no karma")
	}
	if got := calmFolk.CurrentIntention(); got == actorai.IntentionAttack {
		t.Fatal("the civilian guard attacks a character with no karma")
	}

	w.setKarma(t, 100)
	angry := w.spawn(t, standTwoID, 0, 150)
	angryFolk := w.spawnFolk(t, standFolkTwoID, 0, -150)
	w.ticks(t, 2)
	if got := angry.AggroHate(w.player); got != 1500 {
		t.Fatalf("town guard hate of a karma character it sees = %v, want 1500", got)
	}
	if got := angry.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("town guard intention = %v, want attack", got)
	}
	if got := angryFolk.CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("civilian guard intention = %v, want attack", got)
	}
	if c, ok := angryFolk.MostHated(); !ok || c.ObjectID() != w.player.ObjectID() {
		t.Fatalf("civilian guard most hated = %v, %v, want the character", c, ok)
	}
}

// TestGuardStrikesBackWhenHit ports Guard's attacked hook: a hit of 10 on a
// town guard makes it want to attack the attacker with weight 2000, and
// nothing more (the built-in attacked hate stays off for a bound id).
func TestGuardStrikesBackWhenHit(t *testing.T) {
	t.Parallel()
	w := bootGuards(t, guardTemplate(standID, nil), folkGuardTemplate(standFolkID, nil))
	h := w.spawn(t, standID, 100, 0)
	f := w.spawnFolk(t, standFolkID, -60, 0)

	w.hit(t, h, 10)
	if got := h.AggroHate(w.player); got != 2000 {
		t.Fatalf("town guard hate after a hit of 10 = %v, want 2000", got)
	}
	if got := h.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("town guard intention = %v, want attack", got)
	}
	hitFolk(t, w.srv, f, w.player, 10)
	if got := f.CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("civilian guard intention = %v, want attack", got)
	}
	if c, ok := f.MostHated(); !ok || c.ObjectID() != w.player.ObjectID() {
		t.Fatalf("civilian guard most hated = %v, %v, want the character", c, ok)
	}
}

// TestGuardStandWalksBackToItsPost ports GuardStand's no-desire and
// move-finished hooks on a town guard and a civilian one standing off
// their spawn point: with nothing to do each wants to walk back to it with
// weight 30, and never to wander. Arriving there, each wants to do nothing
// for 40 with weight 30.
func TestGuardStandWalksBackToItsPost(t *testing.T) {
	t.Parallel()
	w := bootGuards(t, guardTemplate(standID, nil), folkGuardTemplate(standFolkID, nil))
	h := w.spawn(t, standID, 700, 0)
	f := w.spawnFolk(t, standFolkID, 700, 300)
	hHome, fHome := position(h), position(f)
	w.teleport(t, h, 100, 0)
	w.onFolkQueue(t, f, func() {
		f.TeleportTo(location.Location{X: fHome.X + 100, Y: fHome.Y, Z: fHome.Z})
	})

	// An NPC first idles on its second cycle; the cycle after acts on what
	// its hook queued.
	w.ticks(t, 2)
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location != hHome {
		t.Fatalf("town guard off its post move desire = %+v (%v), want weight 30 to %+v", d, ok, hHome)
	}
	if d, ok := desire(h, actorai.IntentionWander); ok {
		t.Fatalf("town guard wants to wander: %+v", d)
	}
	frames := w.ticks(t, 1)
	for _, c := range []struct {
		id   int32
		home location.Location
	}{{h.ObjectID(), hHome}, {f.ObjectID(), fHome}} {
		moves := framesOf(frames, serverpackets.OpcodeMoveToLocation, c.id)
		if len(moves) != 1 || moveDestination(moves[0]) != c.home {
			t.Fatalf("npc %d walk frames = %d (%v), want one MoveToLocation to %+v", c.id, len(moves), moves, c.home)
		}
	}

	for i := 0; h.IsMoving() || f.IsMoving(); i++ {
		if i >= 200 {
			t.Fatal("the guards never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	if d, ok := desire(h, actorai.IntentionNothing); !ok || d.Weight != 30 || d.Timer != 40 {
		t.Fatalf("town guard on its post do-nothing desire = %+v (%v), want timer 40, weight 30", d, ok)
	}
	w.ticks(t, 1)
	if got := h.AI().CurrentIntention(); got != actorai.IntentionNothing {
		t.Fatalf("town guard intention on its post = %v, want do nothing", got)
	}
	if got := f.CurrentIntention(); got != actorai.IntentionNothing {
		t.Fatalf("civilian guard intention on its post = %v, want do nothing", got)
	}
	if got := position(f); got != fHome {
		t.Fatalf("civilian guard stands at %+v, want its post %+v", got, fHome)
	}
}

// TestGuardMoveAroundWanders ports GuardMoveAround's no-desire hook: with
// nothing to do, a town guard and a civilian one want to wander, trying a
// walk every 5 s, with weight 5.
func TestGuardMoveAroundWanders(t *testing.T) {
	t.Parallel()
	w := bootGuards(t, guardTemplate(moveAroundID, nil), folkGuardTemplate(moveAroundFolkID, nil))
	h := w.spawn(t, moveAroundID, 700, 0)
	f := w.spawnFolk(t, moveAroundFolkID, 700, 300)

	w.ticks(t, 2)
	if d, ok := desire(h, actorai.IntentionWander); !ok || d.Weight != 5 || d.Timer != 5 {
		t.Fatalf("town guard wander desire = %+v (%v), want timer 5, weight 5", d, ok)
	}
	w.ticks(t, 1)
	if got := h.AI().CurrentIntention(); got != actorai.IntentionWander {
		t.Fatalf("town guard intention = %v, want wander", got)
	}
	if got := f.CurrentIntention(); got != actorai.IntentionWander {
		t.Fatalf("civilian guard intention = %v, want wander", got)
	}
}

// TestGuardTimerPlaysSocial ports Guard's 1671 timer: a guard whose
// MoveAroundSocial1 parameter is 92 arms it at creation, and every 10 s,
// while healthy and not attacking, it plays social animation 3 (the draw
// under 40 wins with every draw 0). A guard with neither parameter arms
// nothing.
func TestGuardTimerPlaysSocial(t *testing.T) {
	t.Parallel()
	social := npc.AIParams{"MoveAroundSocial": "0", "MoveAroundSocial1": "92"}
	w := bootGuards(t, guardTemplate(standID, social), guardTemplate(standTwoID, nil), folkGuardTemplate(standFolkID, social))
	h := w.spawn(t, standID, 700, 0)
	still := w.spawn(t, standTwoID, 700, 150)
	f := w.spawnFolk(t, standFolkID, 700, 300)

	w.srv.Advance(t, 10*time.Second)
	if d, ok := desire(h, actorai.IntentionSocial); !ok || d.Weight != 50 || d.Timer != 92*1000/30 {
		t.Fatalf("town guard social desire = %+v (%v), want timer %d, weight 50", d, ok, 92*1000/30)
	}
	frames := append(w.srv.ReadQueued(t, w.srv.Client), w.ticks(t, 2)...)
	for _, id := range []int32{h.ObjectID(), f.ObjectID()} {
		if got := framesOf(frames, serverpackets.OpcodeSocialAction, id); len(got) != 1 || got[0][5] != 3 {
			t.Fatalf("npc %d social animations = %v, want one of id 3", id, got)
		}
	}
	if got := framesOf(frames, serverpackets.OpcodeSocialAction, still.ObjectID()); len(got) != 0 {
		t.Fatalf("social animations of the guard without the parameters = %d, want none", len(got))
	}
}

// TestGuardTimerSendsItHomeFromOutsideItsTerritory ports Guard's 9903
// timer: every 60 s a guard standing out of its territory, the strict
// 200 sphere around its spawn point, teleports back to its spawn point,
// town and civilian guard alike; one still inside it stays where it is.
func TestGuardTimerSendsItHomeFromOutsideItsTerritory(t *testing.T) {
	t.Parallel()
	w := bootGuards(t, guardTemplate(standID, nil), guardTemplate(standTwoID, nil), folkGuardTemplate(standFolkID, nil))
	far := w.spawn(t, standID, 700, 0)
	near := w.spawn(t, standTwoID, 700, -400)
	f := w.spawnFolk(t, standFolkID, 700, 400)
	farHome, fHome := position(far), position(f)
	w.teleport(t, far, 300, 0)
	w.teleport(t, near, 150, 0)
	w.onFolkQueue(t, f, func() {
		f.TeleportTo(location.Location{X: fHome.X, Y: fHome.Y + 300, Z: fHome.Z})
	})
	nearAway := position(near)

	w.srv.Advance(t, 59*time.Second)
	if got := position(far); got == farHome {
		t.Fatal("the town guard went home before its timer fired")
	}
	w.srv.Advance(t, time.Second)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	for _, c := range []struct {
		npc  attackable.Combatant
		home location.Location
	}{{far, farHome}, {f, fHome}} {
		if got := position(c.npc); got != c.home {
			t.Fatalf("npc %d stands at %+v after its timer, want its spawn point %+v", c.npc.ObjectID(), got, c.home)
		}
		if got := framesOf(frames, serverpackets.OpcodeTeleportToLocation, c.npc.ObjectID()); len(got) != 1 {
			t.Fatalf("npc %d TeleportToLocation frames = %d, want 1", c.npc.ObjectID(), len(got))
		}
	}
	if got := position(near); got != nearAway {
		t.Fatalf("the guard inside its territory moved to %+v, want it to stay at %+v", got, nearAway)
	}
}
