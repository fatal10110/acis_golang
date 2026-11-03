package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// retarget selects target for the online player playerID the way a skill
// effect does, without a click, and drains c of the selection. A Christmas
// tree answers every click with ActionFailed alone, so this is how a game
// master's command reaches one.
func retarget(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, playerID int32, target world.Tracked) {
	t.Helper()
	obj, ok := srv.State.Player(playerID)
	if !ok {
		t.Fatalf("player %d not online", playerID)
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d = %T is not an online character", playerID, obj)
	}
	ch.SetTarget(target)
	srv.Settle(t)
	drain(t, c)
	if cur := ch.Target(); cur != target {
		t.Fatalf("player %d target = %v, want %v", playerID, cur, target)
	}
}
