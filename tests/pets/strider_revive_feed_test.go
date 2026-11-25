package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestStriderRiderReviveKeepsPetMeal: a rider killed on a strider and
// revived resumes the strider's gauge at the meal it had at the death, not
// a full one, so the dismount writes that meal back to the strider's row
// (#5090).
func TestStriderRiderReviveKeepsPetMeal(t *testing.T) {
	t.Parallel()
	h, _ := bootStriderOwner(t, striderRow5, 700)
	h.mountStrider(t, encodeUserCommand(cmdMount))
	scale := int32(10000 / striderRow5.rideMeal)
	maxMeal := int32(striderRow5.maxMeal) * scale
	left := 700 - 2*striderRow5.rideMeal
	wantGauges(t, "two periods", feedGauges(t, h.advanceTicks(t, 2)),
		feedGauge{int32(699) * scale, maxMeal}, feedGauge{int32(left) * scale, maxMeal})

	if !h.rider(t).Die(nil) {
		t.Fatal("Die() = false for the living rider")
	}
	drainFrames(t, h.client)
	if got := feedGauges(t, h.advanceTicks(t, 3)); len(got) != 0 {
		t.Fatalf("feed gauges while dead = %v, want none", got)
	}

	h.client.Send(encodeRequestRestartPoint(0))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "restart teleport")
	kept := feedGauge{int32(left) * scale, maxMeal}
	wantGauges(t, "revive", feedGauges(t, frames), kept, kept)
	if !h.rider(t).Mounted() {
		t.Fatal("revived rider lost its mount")
	}
	drainFrames(t, h.client)

	h.requireDismounted(t, "/dismount", h.mountCommand(t, cmdDismount))
	if got := h.savedPetState(t).Fed; got != left {
		t.Fatalf("pet row fed after the revived rider's dismount = %d, want the %d left at the death", got, left)
	}
}
