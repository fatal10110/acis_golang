package admin

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminTargetSelectsThenActs pins //target name as a click on that
// online player: without a name the GM is asked for a correct one (205);
// a name nobody online carries is offline (1356); the first click on the
// player selects it, matched ignoring case, and the second follows it,
// selecting nothing anew.
func TestAdminTargetSelectsThenActs(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	_, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("target"))
	if len(frames) != 1 {
		t.Fatalf("//target frames = %x, want one message", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], 205)

	for _, cmd := range []string{"admin_target Nobody", "admin_target ", "admin_target Player "} {
		frames = exchange(t, gm, encodeBypass(cmd))
		if len(frames) != 1 {
			t.Fatalf("%q frames = %x, want one message", cmd, testsupport.FrameOpcodes(frames))
		}
		assertStatic(t, frames[0], 1356)
	}

	frames = exchange(t, gm, encodeBuildCmd("target player"))
	selected := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeMyTargetSelected {
			selected = i
		}
	}
	if selected < 0 || int32(binary.LittleEndian.Uint32(frames[selected][1:5])) != userID {
		t.Fatalf("//target player frames = %x, want MyTargetSelected of %d", testsupport.FrameOpcodes(frames), userID)
	}

	frames = exchange(t, gm, encodeBuildCmd("target Player"))
	ops := opcodesOf(frames)
	if slices.Contains(ops, serverpackets.OpcodeMyTargetSelected) || !slices.Contains(ops, serverpackets.OpcodeActionFailed) {
		t.Fatalf("second //target Player frames = %x, want the follow's ActionFailed and no new selection", testsupport.FrameOpcodes(frames))
	}
}
