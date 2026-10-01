package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestMountedRiderCannotSwitchToggle pins RequestMagicSkillUse's mount gate
// ("players mounted on pets cannot use any toggle skills"): a toggle request
// from a mounted player is answered with ActionFailed alone, before it
// becomes a cast intention, so no MagicSkillUse is broadcast and the toggle
// does not start.
func TestMountedRiderCannotSwitchToggle(t *testing.T) {
	t.Parallel()
	h := bootWyvernOwner(t)
	observer := h.joinSecondPlayer(t, "Watcher")
	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeSetupGauge, "feed gauge")
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, observer.client)
	rider := h.character(t)
	if !rider.Mounted() {
		t.Fatal("owner not mounted after using the wyvern collar")
	}

	h.client.Send(encodeRequestMagicSkillUse(riderToggleSkill))
	frames := drainFrames(t, h.client)
	if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeActionFailed}; string(got) != string(want) {
		t.Fatalf("mounted toggle request frames = % x, want % x", got, want)
	}
	if _, ok := rider.EffectList().ActiveBySkillID(riderToggleSkill); ok {
		t.Fatal("toggle started while mounted")
	}
	for _, f := range drainFrames(t, observer.client) {
		if f[0] == serverpackets.OpcodeMagicSkillUse {
			t.Fatal("observer saw a MagicSkillUse for a mounted rider's toggle")
		}
	}
}

// TestDismountedRiderSwitchesToggleAgain pins the other side of the mount
// gate: once the starving mount throws its rider, the same toggle request
// starts the toggle again.
func TestDismountedRiderSwitchesToggleAgain(t *testing.T) {
	t.Parallel()
	h := bootWyvernOwner(t)
	if !h.srv.DrivesClock() {
		t.Skip("starving the mount needs the driven clock")
	}
	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeSetupGauge, "feed gauge")
	drainFrames(t, h.client)

	// 508 - 10*50 = 8 is no more than one meal: tick 51 starves.
	h.advanceTicks(t, 51)
	rider := h.character(t)
	if rider.Mounted() {
		t.Fatal("rider still mounted after the starving tick")
	}

	h.client.Send(encodeRequestMagicSkillUse(riderToggleSkill))
	readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillUse, "toggle MagicSkillUse")
	readUntilOpcode(t, h.client, serverpackets.OpcodeAbnormalStatusUpdate, "toggle icon")
	if _, ok := rider.EffectList().ActiveBySkillID(riderToggleSkill); !ok {
		t.Fatal("toggle not active after the dismount")
	}
}
