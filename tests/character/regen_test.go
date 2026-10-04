package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

type regenPlayer interface {
	ResourceValues() player.Resources
	SetResourceValues(player.Resources)
}

// TestPlayerRegenTickRestoresResourcesAndSendsStatus drops a player's HP,
// MP and CP and runs the regeneration sweep: nothing regenerates until one
// period after the drop (CreatureStatus.startHpMpRegeneration's
// scheduleAtFixedRate(3000, 3000)), then every short resource gains its rate
// and the player reads its status.
func TestPlayerRegenTickRestoresResourcesAndSendsStatus(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	if !srv.DrivesClock() {
		t.Skip("pinning the first regeneration tick needs the driven clock")
	}
	c := srv.Client
	objID := srv.SoleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	drainQuiet(t, c)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	p, ok := obj.(regenPlayer)
	if !ok {
		t.Fatalf("world player %T does not expose resources", obj)
	}
	dropped := player.Resources{MaxHP: 100, CurrentHP: 10, MaxMP: 100, CurrentMP: 10, MaxCP: 100, CurrentCP: 10}
	p.SetResourceValues(dropped)

	regen := task.NewNPCRegen(srv.State)
	sweepAfter := func(d time.Duration) player.Resources {
		srv.Advance(t, d)
		regen.Tick()
		srv.Settle(t) // the sweep runs a due tick on the player's queue
		return p.ResourceValues()
	}
	if got := sweepAfter(task.NPCRegenTick - time.Millisecond); got.CurrentHP != 10 || got.CurrentMP != 10 || got.CurrentCP != 10 {
		t.Fatalf("resources %v before the first period = %+v, want the drop kept", task.NPCRegenTick-time.Millisecond, got)
	}

	got := sweepAfter(time.Millisecond)
	if got.CurrentHP <= 10 || got.CurrentMP <= 10 || got.CurrentCP <= 10 {
		t.Fatalf("resources after regen = %+v, want every short resource restored", got)
	}
	frame := c.Read()
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("opcode = %#x, want StatusUpdate (%#x)", frame[0], serverpackets.OpcodeStatusUpdate)
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != objID {
		t.Fatalf("StatusUpdate object id = %d, want %d", id, objID)
	}
	attrs := make(map[serverpackets.StatusType]int32, r.ReadInt32())
	for range 4 {
		attrs[serverpackets.StatusType(r.ReadInt32())] = r.ReadInt32()
	}
	if gotCP := attrs[serverpackets.StatusCurrentCP]; gotCP != int32(got.CurrentCP) {
		t.Fatalf("StatusUpdate current CP = %d, want %d", gotCP, int32(got.CurrentCP))
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate: %v", err)
	}
}
