package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// summonHop is how far the friend's own teleport moves it.
const summonHop = 200

// TestSummonFriendTeleportRacesTheTargetsOwnLanding pins the teleport lock
// order on the real pool, under -race: the friend starts a teleport of its
// own, and the caster's party summon asks the friend's teleport from the
// caster's queue while the friend's Appearing lands the first one. Both
// take the friend's teleport lock; neither waits on the other forever. A
// summon reaching the friend mid-landing is refused, so it ends at its own
// landing point or beside the caster, and every teleport it is told of
// lands. Each round moves the Appearing nearer the launch.
func TestSummonFriendTeleportRacesTheTargetsOwnLanding(t *testing.T) {
	t.Parallel()
	w := bootSummonWorld(t, gameservertest.WithRealPool())
	w.formParty(t)
	cx, cy, _ := w.srv.PlayerPosition(t, w.casterID)
	appearing := encodeSingleOpcode(clientpackets.OpcodeAppearing)

	for _, lead := range []time.Duration{0, 400 * time.Millisecond, 480 * time.Millisecond, 500 * time.Millisecond} {
		tx, ty, tz := w.srv.PlayerPosition(t, w.targetID)
		w.on(t, w.targetID, func(pc *player.Character) { pc.TeleportOnQueue(tx+summonHop, ty, tz, 0) })
		readUntil(t, w.target, serverpackets.OpcodeTeleportToLocation)

		w.caster.Send(encodeRequestMagicSkillUse(summonPartySkill, false, false))
		w.srv.Advance(t, lead)
		w.target.Send(appearing)
		w.srv.Advance(t, summonFriendCastSettle)
		w.srv.Settle(t)
		if teleported(queueFrames(t, w.target)) {
			w.target.Send(appearing)
			w.srv.Settle(t)
		}
		w.drain(t)

		x, y, _ := w.srv.PlayerPosition(t, w.targetID)
		ownLanding := x == tx+summonHop && y == ty
		besideCaster := abs(x-cx) <= 100 && abs(y-cy) <= 100
		t.Logf("lead %v: own landing %v, beside the caster %v", lead, ownLanding, besideCaster)
		if !ownLanding && !besideCaster {
			t.Fatalf("lead %v: friend at (%d,%d), want its own landing (%d,%d) or beside the caster (%d,%d)", lead, x, y, tx+summonHop, ty, cx, cy)
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
