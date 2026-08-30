package character

import (
	"testing"

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

func TestPlayerRegenTickRestoresResourcesAndSendsStatus(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
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
	p.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 10, MaxMP: 100, CurrentMP: 10, MaxCP: 100, CurrentCP: 10})

	task.NewNPCRegen(srv.State).Tick()

	got := p.ResourceValues()
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
