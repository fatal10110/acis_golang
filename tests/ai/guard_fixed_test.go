package ai

import (
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/guard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The ids of the fixed guards: two town guards of GuardFixed and two of
// GuardMoveAroundFixed. standID (GuardStand) fights beside them.
const (
	fixedID              = int32(30040)
	fixedTwoID           = int32(30041)
	moveAroundFixedID    = int32(31032)
	moveAroundFixedTwoID = int32(31033)
)

// bootFixedGuards boots one character beside the guard templates, the
// guard chain registered in scripts.xml order down to the fixed guards,
// every script draw 0.
func bootFixedGuards(t *testing.T, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Guard.Guard",
		"script.ai.individual.Guard.GuardFixed",
		"script.ai.individual.Guard.GuardMoveAround",
		"script.ai.individual.Guard.GuardMoveAroundFixed",
		"script.ai.individual.Guard.GuardStand",
	}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New,
		paths[1]: guard.Guard,
		paths[2]: guard.Fixed,
		paths[3]: guard.MoveAround,
		paths[4]: guard.MoveAroundFixed,
		paths[5]: guard.Stand,
	}
	list := make([]script.Listing, len(paths))
	for i, path := range paths {
		list[i] = script.Listing{Path: path}
	}
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	return bootRaid(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNPCScripts(kinds, list, catalog)}}, templates...)
}

// TestGuardFixedStrikesBackFromItsPost ports GuardFixed's attacked hook: a
// hit of 10 on a fixed guard 300 from the character makes it hate the
// attacker by 2000 and want to attack it from where it stands, so it never
// walks toward it. A GuardStand guard hit the same way walks up to the
// character.
func TestGuardFixedStrikesBackFromItsPost(t *testing.T) {
	t.Parallel()
	w := bootFixedGuards(t, guardTemplate(fixedID, nil), guardTemplate(standID, nil))
	fixed := w.spawn(t, fixedID, 300, 0)
	stand := w.spawn(t, standID, -300, 0)
	post := position(fixed)

	w.hit(t, fixed, 10)
	w.hit(t, stand, 10)
	if got := fixed.AggroHate(w.player); got != 2000 {
		t.Fatalf("fixed guard hate after a hit of 10 = %v, want 2000", got)
	}
	if d, ok := desire(fixed, actorai.IntentionAttack); !ok || d.MoveToTarget || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("fixed guard attack desire = %+v (%v), want one on the character that holds its post", d, ok)
	}
	frames := append(w.srv.ReadQueued(t, w.srv.Client), w.ticks(t, 3)...)
	if got := len(framesOf(frames, serverpackets.OpcodeMoveToPawn, fixed.ObjectID())) +
		len(framesOf(frames, serverpackets.OpcodeMoveToLocation, fixed.ObjectID())); got != 0 {
		t.Fatalf("fixed guard move frames = %d, want none", got)
	}
	if got := position(fixed); got != post {
		t.Fatalf("fixed guard stands at %+v, want its post %+v", got, post)
	}
	if got := len(framesOf(frames, serverpackets.OpcodeMoveToPawn, stand.ObjectID())) +
		len(framesOf(frames, serverpackets.OpcodeMoveToLocation, stand.ObjectID())); got == 0 {
		t.Fatal("the GuardStand guard did not walk up to its attacker")
	}
}

// TestGuardFixedHoldsAgainstKarmaPlayers ports GuardFixed's see-creature
// hook: a character with no karma walks past a fixed guard untouched; once
// it has karma, the fixed guard that sees it next hates it by 1500 and
// wants to attack it without leaving its post.
func TestGuardFixedHoldsAgainstKarmaPlayers(t *testing.T) {
	t.Parallel()
	w := bootFixedGuards(t, guardTemplate(fixedID, nil), guardTemplate(fixedTwoID, nil))
	calm := w.spawn(t, fixedID, 150, 0)
	w.ticks(t, 2)
	if got := calm.AggroHate(w.player); got != 0 {
		t.Fatalf("fixed guard hate of a character with no karma = %v, want none", got)
	}
	if _, ok := desire(calm, actorai.IntentionAttack); ok {
		t.Fatal("the fixed guard wants to attack a character with no karma")
	}

	w.setKarma(t, 100)
	angry := w.spawn(t, fixedTwoID, 0, 150)
	w.ticks(t, 2)
	if got := angry.AggroHate(w.player); got != 1500 {
		t.Fatalf("fixed guard hate of a karma character it sees = %v, want 1500", got)
	}
	if d, ok := desire(angry, actorai.IntentionAttack); !ok || d.MoveToTarget || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("fixed guard attack desire = %+v (%v), want one on the character that holds its post", d, ok)
	}
}

// TestGuardMoveAroundFixedReturnsEveryFiveMinutes ports GuardMoveAroundFixed's
// 5001 timer: 300 s after it spawns and every 300 s after that, a guard that
// is not attacking teleports back to its spawn point, wherever it wandered;
// one attacking stays where it is.
func TestGuardMoveAroundFixedReturnsEveryFiveMinutes(t *testing.T) {
	t.Parallel()
	w := bootFixedGuards(t, guardTemplate(moveAroundFixedID, nil), guardTemplate(moveAroundFixedTwoID, nil))
	idle := w.spawn(t, moveAroundFixedID, 700, 0)
	busy := w.spawn(t, moveAroundFixedTwoID, 100, 0)
	home := position(idle)

	w.srv.Advance(t, 299*time.Second)
	w.teleport(t, idle, 150, 0)
	w.hit(t, busy, 10)
	if got := busy.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: the hit guard's intention = %v, want attack", got)
	}
	w.srv.ReadQueued(t, w.srv.Client)

	w.srv.Advance(t, time.Second)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := framesOf(frames, serverpackets.OpcodeTeleportToLocation, idle.ObjectID()); len(got) != 1 {
		t.Fatalf("idle guard TeleportToLocation frames at 300 s = %d, want 1", len(got))
	}
	if got := position(idle); got != home {
		t.Fatalf("idle guard stands at %+v after its timer, want its spawn point %+v", got, home)
	}
	if got := framesOf(frames, serverpackets.OpcodeTeleportToLocation, busy.ObjectID()); len(got) != 0 {
		t.Fatalf("attacking guard TeleportToLocation frames = %d, want none", len(got))
	}

	w.teleport(t, idle, 150, 0)
	w.srv.Advance(t, 299*time.Second)
	if got := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeTeleportToLocation, idle.ObjectID()); len(got) != 0 {
		t.Fatalf("idle guard teleported %d times before its second firing, want none", len(got))
	}
	w.srv.Advance(t, time.Second)
	if got := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeTeleportToLocation, idle.ObjectID()); len(got) != 1 {
		t.Fatalf("idle guard TeleportToLocation frames at 600 s = %d, want 1", len(got))
	}
}
