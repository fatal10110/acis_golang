package ai

import (
	"encoding/binary"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The civilian ids of the folk desire scenarios, both folk-typed in the
// datapack and bound to a behavior in the reference: a border patrol that
// walks back to its post (GuardStand), and a ritual offering that plays
// social animations where it stands (SacrificialVictim).
const (
	folkPatrolID   = int32(31677)
	folkOfferingID = int32(32038)
)

// folkLog records, one line per call, the hooks a test behavior bound to a
// civilian NPC receives. Hooks run on the NPC's queue, so lines and calls
// are guarded by mu.
type folkLog struct {
	mu    sync.Mutex
	lines []string
	// calls counts the lines ever added.
	calls int
}

// add records a line and returns how many lines were ever added.
func (l *folkLog) add(format string, args ...any) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
	l.calls++
	return l.calls
}

// take returns the lines since the last call.
func (l *folkLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	lines := l.lines
	l.lines = nil
	return lines
}

// bootFolkDesires boots one character in the world, with the AI task, and
// the behaviors of catalog listed.
func bootFolkDesires(t *testing.T, catalog script.Catalog) (*gameservertest.Server, location.Location) {
	t.Helper()
	var list []script.Listing
	for path := range catalog {
		list = append(list, script.Listing{Path: path})
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Watcher", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAITask(),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{folkPatrolID: script.KindFolk, folkOfferingID: script.KindFolk}, list, catalog),
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
	x, y, z := srv.PlayerPosition(t, srv.SoleObjectID(t))
	return srv, location.Location{X: x, Y: y, Z: z}
}

func encodeGameStart() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeFolkAction(objectID int32, at location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteUint8(0)
	return w.Bytes()
}

// cycle runs one AI task cycle and returns the frames it sent the player.
func cycle(t *testing.T, srv *gameservertest.Server) [][]byte {
	t.Helper()
	if err := srv.AI.Tick(); err != nil {
		t.Fatalf("AI.Tick() error: %v", err)
	}
	return srv.ReadQueued(t, srv.Client)
}

// framesOf returns the frames with opcode whose leading object id is id.
func framesOf(frames [][]byte, opcode byte, id int32) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if len(f) >= 5 && f[0] == opcode && int32(binary.LittleEndian.Uint32(f[1:5])) == id {
			out = append(out, f)
		}
	}
	return out
}

// moveDestination reads a MoveToLocation frame's destination.
func moveDestination(f []byte) location.Location {
	v := func(i int) int { return int(int32(binary.LittleEndian.Uint32(f[5+4*i:]))) }
	return location.Location{X: v(0), Y: v(1), Z: v(2)}
}

// walkUntilArrived ticks f's movement until its walk ends.
func walkUntilArrived(t *testing.T, srv *gameservertest.Server, f *npc.Folk) {
	t.Helper()
	for i := 0; f.IsMoving(); i++ {
		if i >= int(20*time.Second/move.PositionUpdateInterval) {
			t.Fatal("walk never arrived")
		}
		srv.TickPositions()
	}
	srv.ReadQueued(t, srv.Client)
}

// TestFolkPatrolWalksDoesNothingThenWanders runs a civilian border patrol
// under a behavior shaped like its reference one: its idle sends it away
// and lines up a light wander, each arrival opens the move-finished hook
// with the point reached, the return home queues a do-nothing just
// heavier than the wander. The civilian AI takes the walks in turn, then
// keeps still while the do-nothing outweighs the wander and takes up the
// wander once the do-nothing decays below it. A civilian NPC has no wander
// step, so it then stands, with the wander current and queued: no further
// idle, no walk.
func TestFolkPatrolWalksDoesNothingThenWanders(t *testing.T) {
	t.Parallel()
	log := &folkLog{}
	var home, away location.Location
	patrol := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkPatrolID}, Hooks: script.Hooks{
			OnNoDesire: func(_ *script.Script, e script.NoDesire) {
				if log.add("NO_DESIRE") == 1 {
					e.NPC.AddMoveToDesire(script.Loc{X: away.X, Y: away.Y, Z: away.Z}, 30)
					e.NPC.AddWanderDesire(5, 5)
				}
			},
			OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
				log.add("MOVE_TO_FINISHED at=%d,%d,%d", e.X, e.Y, e.Z)
				if (location.Location{X: int(e.X), Y: int(e.Y), Z: int(e.Z)}) != home {
					e.NPC.AddMoveToDesire(script.Loc{X: home.X, Y: home.Y, Z: home.Z}, 30)
					return
				}
				e.NPC.AddDoNothingDesire(40, 5.6)
			},
		}}
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkPatrol": patrol})
	home = location.Location{X: at.X + 100, Y: at.Y, Z: at.Z}
	away = location.Location{X: home.X, Y: home.Y + 200, Z: home.Z}
	tmpl := gameservertest.FolkTemplate("Folk", int(folkPatrolID))
	tmpl.CanMove = true
	f := srv.SpawnFolkNPCAt(t, tmpl, home)
	id := f.ObjectID()
	srv.ReadQueued(t, srv.Client)

	if frames := cycle(t, srv); len(log.take()) != 0 || len(framesOf(frames, serverpackets.OpcodeMoveToLocation, id)) != 0 {
		t.Fatal("the patrol acted on its first cycle; it acts on nothing before its first one is over")
	}
	cycle(t, srv)
	if got := log.take(); !slices.Equal(got, []string{"NO_DESIRE"}) {
		t.Fatalf("hooks on the second cycle = %q, want the idle's no-desire", got)
	}

	walk := func(to location.Location) {
		t.Helper()
		moves := framesOf(cycle(t, srv), serverpackets.OpcodeMoveToLocation, id)
		if len(moves) != 1 || moveDestination(moves[0]) != to {
			t.Fatalf("walk frames = %d (%v), want one MoveToLocation to %+v", len(moves), moves, to)
		}
		if got := f.CurrentIntention(); got != actorai.IntentionMoveTo {
			t.Fatalf("intention = %v while walking, want move_to", got)
		}
		walkUntilArrived(t, srv, f)
		want := []string{fmt.Sprintf("MOVE_TO_FINISHED at=%d,%d,%d", to.X, to.Y, to.Z)}
		if got := log.take(); !slices.Equal(got, want) {
			t.Fatalf("hooks on arriving at %+v = %q, want %q", to, got, want)
		}
	}
	walk(away)
	walk(home)

	// The do-nothing (5.6) outweighs the wander (5) until its weight has
	// decayed twice by 0.5, on every third cycle.
	nothing := 0
	for f.CurrentIntention() != actorai.IntentionWander {
		frames := cycle(t, srv)
		if got := f.CurrentIntention(); got != actorai.IntentionNothing && got != actorai.IntentionWander {
			t.Fatalf("intention = %v after the walk home, want nothing then wander", got)
		}
		if f.CurrentIntention() == actorai.IntentionNothing {
			nothing++
		}
		if moves := framesOf(frames, serverpackets.OpcodeMoveToLocation, id); len(moves) != 0 {
			t.Fatalf("the patrol walked while doing nothing: %v", moves)
		}
		if nothing > 6 {
			t.Fatal("the do-nothing never decayed below the wander")
		}
	}
	if nothing < 3 {
		t.Fatalf("did nothing for %d cycles, want at least 3: one decay leaves it above the wander", nothing)
	}
	for range 5 {
		frames := cycle(t, srv)
		if got := f.CurrentIntention(); got != actorai.IntentionWander {
			t.Fatalf("intention = %v, want the wander kept current", got)
		}
		if moves := framesOf(frames, serverpackets.OpcodeMoveToLocation, id); len(moves) != 0 {
			t.Fatalf("a civilian wander walked: %v", moves)
		}
	}
	if got := log.take(); len(got) != 0 {
		t.Fatalf("hooks while wandering = %q, want none: the wander stays queued", got)
	}
}

// TestFolkOfferingSocialHoldsSelectionAndTalk runs a civilian ritual
// offering under a behavior shaped like its reference one: its idle queues
// a social animation with a 7 s hold, and the idle right after it, a second
// social with no hold. The first plays on the next cycle, which then idles
// on its empty queue; the second waits out the hold however many cycles
// run, then plays. The talk animation shares the social clock: a talk
// inside the hold, or within 12 s of the last animation, plays none.
func TestFolkOfferingSocialHoldsSelectionAndTalk(t *testing.T) {
	t.Parallel()
	log := &folkLog{}
	offering := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkOfferingID}, Hooks: script.Hooks{
			OnNoDesire: func(_ *script.Script, e script.NoDesire) {
				switch log.add("NO_DESIRE") {
				case 1:
					e.NPC.AddSocialDesire(1, 7000, 1000)
				case 2:
					e.NPC.AddSocialDesire(2, 0, 1000)
				}
			},
		}}
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkOffering": offering})
	f := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", int(folkOfferingID)), location.Location{X: at.X + 40, Y: at.Y, Z: at.Z})
	id := f.ObjectID()
	srv.ReadQueued(t, srv.Client)
	socials := func(frames [][]byte) []int32 {
		var ids []int32
		for _, fr := range framesOf(frames, serverpackets.OpcodeSocialAction, id) {
			ids = append(ids, int32(binary.LittleEndian.Uint32(fr[5:9])))
		}
		return ids
	}
	talk := func() []int32 {
		t.Helper()
		srv.Client.Send(encodeFolkAction(id, at))
		return socials(srv.ReadQueued(t, srv.Client))
	}
	// Select the offering, so each later action is a talk.
	srv.Client.Send(encodeFolkAction(id, at))
	srv.ReadQueued(t, srv.Client)

	cycle(t, srv)
	cycle(t, srv)
	if got := log.take(); !slices.Equal(got, []string{"NO_DESIRE"}) {
		t.Fatalf("hooks on the second cycle = %q, want the idle's no-desire", got)
	}
	if got := socials(cycle(t, srv)); !slices.Equal(got, []int32{1}) {
		t.Fatalf("socials on the third cycle = %v, want [1]", got)
	}
	if got := log.take(); !slices.Equal(got, []string{"NO_DESIRE"}) {
		t.Fatalf("hooks on the social's cycle = %q, want the idle on the emptied queue", got)
	}
	if got := f.CurrentIntention(); got != actorai.IntentionSocial {
		t.Fatalf("intention = %v after the social, want social", got)
	}
	if got := talk(); len(got) != 0 {
		t.Fatalf("talk inside the hold animated %v, want none", got)
	}
	for range 3 {
		if got := socials(cycle(t, srv)); len(got) != 0 {
			t.Fatalf("socials inside the hold = %v, want none", got)
		}
	}
	if got := log.take(); len(got) != 0 {
		t.Fatalf("hooks inside the hold = %q, want none: the second social stays queued", got)
	}

	srv.Advance(t, 7*time.Second)
	if got := socials(cycle(t, srv)); !slices.Equal(got, []int32{2}) {
		t.Fatalf("socials once the hold ran out = %v, want [2]", got)
	}
	srv.Advance(t, 12*time.Second)
	if got := talk(); len(got) != 0 {
		t.Fatalf("talk 12 s after the last social animated %v, want none", got)
	}
	srv.Advance(t, time.Second)
	if got := talk(); len(got) != 1 {
		t.Fatalf("talk past 12 s after the last social animated %v, want one", got)
	}
}
