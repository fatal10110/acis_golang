package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFolkCorpseShowsNotAttackable pins Creature.isAttackableBy's !isDead
// for a civilian NPC: a still civilian NPC standing in water shows the
// player ServerObjectInfo attackable 1 with its HP while it lives. Once it
// is dead, selecting the corpse colors it neutral (MyTargetSelected color
// 0, Player.setTarget), a forced attack on it lands no swing, and when it
// decays out of the water its last ServerObjectInfo shows it not
// attackable, with no HP sent (ServerObjectInfo.writeImpl).
func TestFolkCorpseShowsNotAttackable(t *testing.T) {
	t.Parallel()
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, zoneBox(t, 1_200, 2_000)))
	w := bootFolkWorld(t, nil, gameservertest.WithZones(zones))

	tmpl := folkTemplate("Folk", 13031)
	tmpl.RunSpeed, tmpl.WalkSpeed = 0, 0
	tmpl.Undying = false
	statue := w.srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: 1_500, Y: w.at.Y, Z: w.at.Z})
	alive := lastServerObjectInfo(t, drainFrames(t, w.c), statue.ObjectID())
	if alive.attackable != 1 || alive.maxHP == 0 {
		t.Fatalf("living still civilian NPC ServerObjectInfo = %+v, want attackable with its HP", alive)
	}

	if !statue.Kill(nil) {
		t.Fatal("civilian NPC did not die")
	}
	drainFrames(t, w.c)

	sel := w.selectFolk(t, statue)
	frame, ok := firstOpcode(sel, serverpackets.OpcodeMyTargetSelected)
	if !ok {
		t.Fatalf("corpse selection frames = %x, want MyTargetSelected", opcodes(sel))
	}
	r := wire.NewReader(frame[1:])
	if id, color := r.ReadInt32(), int16(r.ReadUint16()); id != statue.ObjectID() || color != 0 {
		t.Fatalf("corpse MyTargetSelected = %d color %d, want %d color 0", id, color, statue.ObjectID())
	}

	w.c.Send(encodeAttackRequest(statue.ObjectID(), w.at))
	w.srv.Settle(t)
	for _, f := range drainFrames(t, w.c) {
		if objectFrame(f, serverpackets.OpcodeAttack, w.player) {
			t.Fatal("a forced attack swung at the corpse")
		}
	}

	if !statue.Decay(w.srv.State, nil) {
		t.Fatal("corpse did not decay")
	}
	got := lastServerObjectInfo(t, drainFrames(t, w.c), statue.ObjectID())
	if want := (serverObjectInfoView{}); got != want {
		t.Fatalf("corpse ServerObjectInfo leaving the water = %+v, want %+v", got, want)
	}
}
