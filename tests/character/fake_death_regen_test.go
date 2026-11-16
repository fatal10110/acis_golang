package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// TestFakeDeadPlayerRegenerates pins CreatureStatus.doRegeneration
// (CreatureStatus.java:402-414) and its CP override
// (PlayerStatus.java:333-340): the only gate is death itself, so a player
// feigning death (Player.startFakeDeath, Player.java:7017-7033) restores
// HP, MP and CP on every regeneration tick and reads its status each time.
func TestFakeDeadPlayerRegenerates(t *testing.T) {
	t.Parallel()
	srv, c := bootRegenTerms(t, nil)
	if !srv.DrivesClock() {
		t.Skip("pinning regeneration ticks needs the driven clock")
	}
	client := srv.Client
	c.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 100, CurrentMP: 100, MaxCP: 100, CurrentCP: 100})
	c.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 50, MaxMP: 100, CurrentMP: 50, MaxCP: 100, CurrentCP: 50})
	srv.ReadQueued(t, client)
	if !c.StartFakeDeath() {
		t.Fatal("StartFakeDeath reported no change")
	}
	mustReadOpcode(t, client, serverpackets.OpcodeChangeWaitType, "fake death")
	srv.ReadQueued(t, client)

	regen := task.NewNPCRegen(srv.State)
	last := c.ResourceValues()
	for tick := 1; tick <= 3; tick++ {
		srv.Advance(t, task.NPCRegenTick)
		regen.Tick()
		srv.Settle(t)
		got := c.ResourceValues()
		if !c.FakeDead() {
			t.Fatalf("tick %d: fake death ended", tick)
		}
		if got.CurrentHP <= last.CurrentHP || got.CurrentMP <= last.CurrentMP || got.CurrentCP <= last.CurrentCP {
			t.Fatalf("tick %d: resources %+v after %+v, want HP, MP and CP restored", tick, got, last)
		}
		if i := firstOpcode(srv.ReadQueued(t, client), serverpackets.OpcodeStatusUpdate); i < 0 {
			t.Fatalf("tick %d sent no StatusUpdate", tick)
		}
		last = got
	}
}
