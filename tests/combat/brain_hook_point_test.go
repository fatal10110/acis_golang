package combat

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// runAICycle runs one production AI cycle, posted to every registered
// actor's queue, and waits for it to finish.
func runAICycle(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	if err := srv.AI.Tick(); err != nil {
		t.Fatalf("AI.Tick() error: %v", err)
	}
	srv.Settle(t)
}

// framesOf returns the opcodes of the frames whose leading object id is id.
func framesOf(frames [][]byte, id int32) []byte {
	var ops []byte
	for _, f := range frames {
		if len(f) >= 5 && int32(binary.LittleEndian.Uint32(f[1:5])) == id {
			ops = append(ops, f[0])
		}
	}
	return ops
}

// readFor collects every frame that arrives within d of quiet.
func readFor(c *scriptedClient, d time.Duration) [][]byte {
	var frames [][]byte
	for f := c.ReadWithTimeout(d); f != nil; f = c.ReadWithTimeout(d) {
		frames = append(frames, f)
	}
	return frames
}

// monsterFrame is one frame as a scenario pins it: its opcode and leading
// object id.
type monsterFrame struct {
	op   byte
	lead int32
}

func leadOf(frames [][]byte) []monsterFrame {
	out := make([]monsterFrame, 0, len(frames))
	for _, f := range frames {
		lead := int32(-1)
		if len(f) >= 5 {
			lead = int32(binary.LittleEndian.Uint32(f[1:5]))
		}
		out = append(out, monsterFrame{op: f[0], lead: lead})
	}
	return out
}

// readUntilFrame collects frames up to and including the first one with
// opcode op and leading object id id.
func readUntilFrame(t *testing.T, c *scriptedClient, op byte, id int32, what string) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		f := c.ReadWithTimeout(2 * time.Second)
		if f == nil {
			t.Fatalf("%s never arrived; got %v", what, leadOf(frames))
		}
		frames = append(frames, f)
		if f[0] == op && len(f) >= 5 && int32(binary.LittleEndian.Uint32(f[1:5])) == id {
			return frames
		}
	}
	t.Fatalf("%s not within 100 frames", what)
	return nil
}

// TestBrainArrivalOutsideTerritoryWalksHome pins the order a monster's
// queue runs an arrival and the AI cycles after it in, with every hook
// point on the way open: a walk out of its territory arrives (move-finished
// and out-of-territory points) with no packet, the next cycle idles it
// (no-desire point) into its walk stance, and the cycle after walks it
// home.
func TestBrainArrivalOutsideTerritoryWalksHome(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAITask(),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, objID)
	home := location.Location{X: x + 400, Y: y, Z: z}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, gameservertest.MovingHostileTemplate("Monster"), home, home)
	srv.AI.Add(hostile)
	id := hostile.ObjectID()
	drainUntilQuiet(t, c)

	step := func(what string, run func(), want ...byte) [][]byte {
		t.Helper()
		run()
		var mine [][]byte
		for _, f := range readFor(c, 300*time.Millisecond) {
			if len(f) >= 5 && int32(binary.LittleEndian.Uint32(f[1:5])) == id {
				mine = append(mine, f)
			}
		}
		if got := framesOf(mine, id); !slices.Equal(got, want) {
			t.Fatalf("monster frames on %s = % x, want % x", what, got, want)
		}
		return mine
	}
	cycle := func() { runAICycle(t, srv) }

	step("the first cycle", cycle)
	dest := location.Location{X: home.X, Y: home.Y + 300, Z: home.Z}
	if !hostile.AI().AddMoveToDesire(dest, 1_000) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}
	walk := step("the walk cycle", cycle, serverpackets.OpcodeMoveToLocation)
	assertMoveDestination(t, walk[0], dest)
	step("the arrival", func() {
		for i := 0; hostile.Move().Moving(); i++ {
			if i >= int(10*time.Second/move.PositionUpdateInterval) {
				t.Fatal("walk never arrived")
			}
			srv.TickPositions()
		}
	})
	if hostile.InTerritory() {
		t.Fatal("InTerritory() after the walk = true, want the monster outside")
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the arrival = %v, want the finished walk kept", got)
	}

	step("the idle cycle", cycle, serverpackets.OpcodeChangeMoveType)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() after the idle cycle = %v, want %v", got, ai.IntentionIdle)
	}
	back := step("the cycle after the idle", cycle, serverpackets.OpcodeMoveToLocation)
	assertMoveDestination(t, back[0], home)
}

// assertMoveDestination asserts a MoveToLocation frame walks to want.
func assertMoveDestination(t *testing.T, frame []byte, want location.Location) {
	t.Helper()
	r := wireReader(frame[5:])
	if got := (location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}); got != want {
		t.Fatalf("MoveToLocation destination = %+v, want %+v", got, want)
	}
}

// TestBrainCyclesDuringAFight pins a monster's first reaction to a hit,
// which runs desire selection on the attacker's queue, and the AI cycles
// that run on the monster's own queue while the fight goes on. The
// reaction swings back after the attacker's stance and before the
// monster's HP update; cycles posted while hits land, each opening its
// see-creature point, never idle or walk the fighting monster.
func TestBrainCyclesDuringAFight(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAITask(),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, objID)
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.BaseAttackRange = 40
	tmpl.PAtk = 0.25
	at := location.Location{X: x + 20, Y: y, Z: z}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, tmpl, at, at)
	srv.AI.Add(hostile)
	id := hostile.ObjectID()
	drainUntilQuiet(t, c)
	runAICycle(t, srv)
	if got := framesOf(readFor(c, 300*time.Millisecond), id); len(got) != 0 {
		t.Fatalf("monster frames on the first cycle = % x, want none", got)
	}

	c.Send(encodeAttackRequest(id, int32(x), int32(y), int32(z), false))
	readUntilFrame(t, c, serverpackets.OpcodeStatusUpdate, id, "selection StatusUpdate")
	c.Send(encodeAttackRequest(id, int32(x), int32(y), int32(z), false))
	frames := readUntilFrame(t, c, serverpackets.OpcodeStatusUpdate, id, "monster HP StatusUpdate")
	// The hit's damage messages depend on its rolls (a critical adds one),
	// so only the object frames are pinned. The player keeps swinging on the
	// wall clock, and on a loaded run its next swing can start before the
	// monster reacts, so only its first Attack is pinned.
	playerSwings := 0
	frames = slices.DeleteFunc(frames, func(f []byte) bool {
		if f[0] == serverpackets.OpcodeAttack && int32(binary.LittleEndian.Uint32(f[1:5])) == objID {
			playerSwings++
			return playerSwings > 1
		}
		return f[0] == serverpackets.OpcodeSystemMessage
	})
	want := []monsterFrame{
		{serverpackets.OpcodeAttack, objID},
		{serverpackets.OpcodeAutoAttackStart, objID},
		{serverpackets.OpcodeAttack, id},
		{serverpackets.OpcodeStatusUpdate, id},
	}
	if got := leadOf(frames); !slices.Equal(got, want) {
		t.Fatalf("first hit and reaction frames = %+v, want %+v", got, want)
	}
	if target := wireReader(frames[2][5:]).ReadInt32(); target != objID {
		t.Fatalf("monster Attack target = %d, want the player %d", target, objID)
	}

	cycles := make(chan error, 1)
	go func() {
		for range 3 {
			if err := srv.AI.Tick(); err != nil {
				cycles <- err
				return
			}
		}
		cycles <- nil
	}()
	during := readFor(c, 300*time.Millisecond)
	if err := <-cycles; err != nil {
		t.Fatalf("AI.Tick() error: %v", err)
	}
	srv.Settle(t)
	during = append(during, readFor(c, 100*time.Millisecond)...)
	for _, f := range during {
		if len(f) < 5 || int32(binary.LittleEndian.Uint32(f[1:5])) != id {
			continue
		}
		switch f[0] {
		case serverpackets.OpcodeMoveToLocation, serverpackets.OpcodeChangeMoveType, serverpackets.OpcodeStopMove:
			t.Fatalf("fighting monster sent opcode %#x during the cycles, want it kept on the fight", f[0])
		case serverpackets.OpcodeAttack:
			if target := wireReader(f[5:]).ReadInt32(); target != objID {
				t.Fatalf("monster Attack target during the cycles = %d, want the player %d", target, objID)
			}
		}
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the cycles = %v, want %v", got, ai.IntentionAttack)
	}
	if got := hostile.AI().TopDesireTarget(); got == nil || got.ObjectID() != objID {
		t.Fatalf("TopDesireTarget() after the cycles = %v, want the player %d", got, objID)
	}
}
