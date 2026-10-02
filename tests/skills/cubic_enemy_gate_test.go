package skills

import (
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// gateCubicOwner is a live cubic owner with target selected, whose every
// roll is 0.
type gateCubicOwner struct {
	*player.Character
	target world.Tracked
}

func (o gateCubicOwner) Target() world.Tracked       { return o.target }
func (o gateCubicOwner) Roll(int) int                { return 0 }
func (o gateCubicOwner) Attacker() skilltarget.Actor { return o.Character }

// TestCubicFiresOnlyAtPlayersAttackableWithoutForce pins Cubic.pickEnemyTarget's
// isAttackableWithoutForceBy gate (Cubic.java:189-202, Playable.java:494-533)
// for a player selection: an unflagged player standing by is never fired
// at, the same player once PvP-flagged is.
func TestCubicFiresOnlyAtPlayersAttackableWithoutForce(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Owner", 20, 0),
		gameservertest.WithWantChars(1),
	)
	ownerID := srv.SoleObjectID(t)
	bystanderID := srv.SeedCharacterFor(t, "player2", "Bystander", 20, 0).ID
	bystander := srv.DialClient(t, "player2", 1)
	startInWorld(t, srv.Client)
	startInWorldAmongPlayers(t, bystander)

	ownerObj, _ := srv.State.Player(ownerID)
	ownerChar, ok := network.OnlineCharacter(ownerObj)
	if !ok {
		t.Fatalf("owner is %T", ownerObj)
	}
	target, _ := srv.State.Player(bystanderID)
	owner := gateCubicOwner{Character: ownerChar, target: target}

	if _, _, ok := actorcast.DecideCubicFire(owner, []int{4049}, 100); ok {
		t.Fatal("a cubic fires at an unflagged player")
	}
	target.(interface{ UpdatePvPFlag(task.PvPFlagState) }).UpdatePvPFlag(task.PvPFlagOn)
	if _, got, ok := actorcast.DecideCubicFire(owner, []int{4049}, 100); !ok || got.ObjectID() != bystanderID {
		t.Fatalf("cubic target = %v, %v, want the flagged player", got, ok)
	}
}
